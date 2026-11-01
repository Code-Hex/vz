#!/usr/bin/env python3
"""Temporary, token-authenticated observation of one fixed diagnostic child."""
import argparse
import ctypes
import hmac
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
import re
import selectors
import socket
import signal
import stat
import subprocess
from socketserver import TCPServer
import threading
import time
from urllib.parse import parse_qs, urlsplit

OUTPUT_LIMIT = 65536
SAMPLE_LIMIT = 262144
NONCE = re.compile(r'[A-Za-z0-9_-]{1,128}\Z')
ENV_KEYS = ('HOME', 'TMPDIR', 'LANG', 'LC_ALL', 'NAT', 'DGRAM', 'COUNT', 'CONSOLE',
            'CONNECT', 'SOCKET_DEVICE', 'NONBLOCK', 'EARLY_RELEASE')


class APIError(Exception):
    def __init__(self, status, message):
        self.status = status
        self.message = message


class Clocks:
    def __init__(self):
        self.lib = ctypes.CDLL('/usr/lib/libSystem.B.dylib')
        self.lib.mach_absolute_time.argtypes = []
        self.lib.mach_absolute_time.restype = ctypes.c_uint64
        self.lib.mach_continuous_time.argtypes = []
        self.lib.mach_continuous_time.restype = ctypes.c_uint64

        class Timebase(ctypes.Structure):
            _fields_ = [('numer', ctypes.c_uint32), ('denom', ctypes.c_uint32)]

        self.lib.mach_timebase_info.argtypes = [ctypes.POINTER(Timebase)]
        self.lib.mach_timebase_info.restype = ctypes.c_int
        info = Timebase()
        if self.lib.mach_timebase_info(ctypes.byref(info)) != 0 or not info.denom:
            raise RuntimeError('mach timebase unavailable')
        self.timebase = {'numer': info.numer, 'denom': info.denom}

    def read(self):
        return {
            'realtime_ns': time.clock_gettime_ns(time.CLOCK_REALTIME),
            'monotonic_ns': time.clock_gettime_ns(time.CLOCK_MONOTONIC),
            'monotonic_raw_ns': time.clock_gettime_ns(time.CLOCK_MONOTONIC_RAW),
            'uptime_raw_ns': time.clock_gettime_ns(time.CLOCK_UPTIME_RAW),
            'mach_absolute_ticks': self.lib.mach_absolute_time(),
            'mach_continuous_ticks': self.lib.mach_continuous_time(),
            'mach_timebase': self.timebase,
        }


class Tail:
    def __init__(self, limit):
        self.lock = threading.Lock()
        self.limit = limit
        self.data = bytearray()
        self.total = 0
        self.eof = False

    def drain(self, stream):
        try:
            while True:
                block = os.read(stream.fileno(), 16384)
                if not block:
                    break
                with self.lock:
                    self.total += len(block)
                    self.data.extend(block)
                    if len(self.data) > self.limit:
                        del self.data[:-self.limit]
        finally:
            stream.close()
            with self.lock:
                self.eof = True

    def snapshot(self):
        with self.lock:
            return {
                'output_tail': self.data.decode('utf-8', errors='replace'),
                'output_total_bytes': self.total,
                'output_dropped_bytes': self.total - len(self.data),
                'output_eof': self.eof,
            }


class Controller:
    def __init__(self, argv):
        self.argv = tuple(argv)
        self.env = {key: os.environ[key] for key in ENV_KEYS if key in os.environ}
        self.env['PATH'] = '/usr/bin:/bin:/usr/sbin:/sbin'
        self.lock = threading.Lock()
        self.clocks = Clocks()
        self.sequence = 0
        self.started = False
        self.child = None
        self.child_status = 'not_started'
        self.returncode = None
        self.reaped = False
        self.output = Tail(OUTPUT_LIMIT)
        self.sample_active = False
        self.sample_started = False
        self.sample_returncode = None
        self.sample_error = None
        self.sample_output = Tail(SAMPLE_LIMIT)

    def observe_locked(self):
        if self.child is None or self.reaped:
            return
        exited = os.waitid(os.P_PID, self.child.pid, os.WEXITED | os.WNOHANG | os.WNOWAIT)
        if exited is not None and exited.si_code in (os.CLD_EXITED, os.CLD_KILLED, os.CLD_DUMPED):
            self.returncode = exited.si_status if exited.si_code == os.CLD_EXITED else -exited.si_status
            self.child_status = 'exited'
            # Keep the PID reserved until sample has finished attaching and reading.
            if not self.sample_active:
                actual = self.child.poll()
                if actual is not None:
                    self.returncode = actual
                    self.reaped = True
            return
        changed = os.waitid(os.P_PID, self.child.pid, os.WSTOPPED | os.WCONTINUED | os.WNOHANG)
        if changed is not None:
            self.child_status = 'stopped' if changed.si_code == os.CLD_STOPPED else 'running'

    def start_locked(self):
        if self.started:
            raise APIError(409, 'start already attempted')
        self.started = True
        self.child_status = 'starting'
        try:
            self.child = subprocess.Popen(self.argv, env=self.env, stdin=subprocess.DEVNULL,
                                          stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                                          bufsize=0, start_new_session=True, close_fds=True)
        except OSError:
            self.child_status = 'start_failed'
            raise APIError(500, 'fixed child spawn failed')
        self.child_status = 'running'
        threading.Thread(target=self.output.drain, args=(self.child.stdout,), daemon=True).start()

    def signal_locked(self, value):
        self.observe_locked()
        if self.child is None or self.child_status == 'exited':
            raise APIError(409, 'no live owned child')
        try:
            os.killpg(self.child.pid, value)
            return {'sent': True, 'number': value, 'errno': 0}
        except OSError as error:
            return {'sent': False, 'number': value, 'errno': error.errno}

    def sample_locked(self):
        self.observe_locked()
        if self.child is None or self.child_status == 'exited':
            raise APIError(409, 'no live owned child')
        if self.sample_started:
            raise APIError(409, 'sample already attempted')
        self.sample_started = True
        self.sample_active = True
        try:
            sample = subprocess.Popen(['/usr/bin/sample', str(self.child.pid), '1', '10',
                                       '-mayDie', '-file', '/dev/stdout'],
                                      env=self.env, stdin=subprocess.DEVNULL,
                                      stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                                      bufsize=0, start_new_session=True, close_fds=True)
        except OSError:
            self.sample_active = False
            self.sample_error = 'sample spawn failed'
            raise APIError(500, 'sample spawn failed')
        threading.Thread(target=self.finish_sample, args=(sample,), daemon=True).start()

    def finish_sample(self, sample):
        try:
            self.sample_output.drain(sample.stdout)
            result = sample.wait()
            with self.lock:
                self.sample_returncode = result
        finally:
            with self.lock:
                self.sample_active = False
                self.observe_locked()

    def dispatch(self, action, nonce):
        with self.lock:
            result = None
            if action == 'start':
                self.start_locked()
            elif action in ('term', 'kill'):
                result = self.signal_locked(signal.SIGTERM if action == 'term' else signal.SIGKILL)
            elif action == 'sample':
                self.sample_locked()
            elif action == 'finish':
                self.observe_locked()
                if self.child is None or self.child_status != 'exited':
                    raise APIError(409, 'owned child has not exited')
            self.observe_locked()
            self.sequence += 1
            child = {'status': self.child_status, 'pid': self.child.pid if self.child else None,
                     'pgid': self.child.pid if self.child else None, 'returncode': self.returncode,
                     'reaped': self.reaped}
            sample = {'started': self.sample_started, 'active': self.sample_active,
                      'returncode': self.sample_returncode, 'error': self.sample_error}
            sequence = self.sequence
        # Tail copies and clock reads never hold the lifecycle lock.
        child.update(self.output.snapshot())
        sample.update(self.sample_output.snapshot())
        response = {'nonce': nonce, 'sequence': sequence, 'clocks': self.clocks.read(),
                    'child': child, 'sample': sample}
        if result is not None:
            response['signal'] = result
        return response


class Handler(BaseHTTPRequestHandler):
    protocol_version = 'HTTP/1.0'

    def log_message(self, *args):
        pass

    def setup(self):
        super().setup()
        self.connection.settimeout(5)

    def reply(self, status, data):
        body = json.dumps(data, ensure_ascii=True, separators=(',', ':')).encode()
        self.send_response(status)
        self.send_header('Content-Type', 'application/json')
        self.send_header('Content-Length', str(len(body)))
        self.send_header('Cache-Control', 'no-store')
        self.send_header('Connection', 'close')
        self.end_headers()
        self.wfile.write(body)

    def route(self, method):
        try:
            auth = self.headers.get_all('Authorization') or []
            if len(auth) != 1 or not hmac.compare_digest(auth[0].encode(), self.server.authorization):
                raise APIError(401, 'unauthorized')
            path = urlsplit(self.path)
            if method == 'GET':
                if path.path != '/state':
                    raise APIError(404, 'unknown operation')
                values = parse_qs(path.query, strict_parsing=True)
                if set(values) != {'nonce'} or len(values['nonce']) != 1:
                    raise APIError(400, 'nonce required')
                nonce = values['nonce'][0]
                action = 'state'
            else:
                if path.query or path.path not in ('/state', '/start', '/term', '/kill', '/sample', '/finish'):
                    raise APIError(404, 'unknown operation')
                lengths = self.headers.get_all('Content-Length') or []
                if self.headers.get('Transfer-Encoding') or len(lengths) != 1:
                    raise APIError(400, 'fixed content length required')
                length = int(lengths[0])
                if not 1 <= length <= 1024:
                    raise APIError(400, 'invalid body length')
                data = json.loads(self.rfile.read(length))
                if not isinstance(data, dict) or set(data) != {'nonce'}:
                    raise APIError(400, 'only nonce is accepted')
                nonce = data['nonce']
                action = path.path[1:]
            if not isinstance(nonce, str) or not NONCE.fullmatch(nonce):
                raise APIError(400, 'invalid nonce')
            self.reply(200, self.server.controller.dispatch(action, nonce))
            if action == 'finish':
                threading.Thread(target=self.server.shutdown, daemon=True).start()
        except APIError as error:
            self.reply(error.status, {'error': error.message})
        except (ValueError, UnicodeError):
            self.reply(400, {'error': 'invalid request'})
        except (OSError, RuntimeError):
            self.reply(500, {'error': 'control operation failed'})

    def do_GET(self):
        self.route('GET')

    def do_POST(self):
        self.route('POST')


class Server(ThreadingHTTPServer):
    daemon_threads = True
    block_on_close = False

    def __init__(self, port, token, controller):
        self.slots = threading.BoundedSemaphore(8)
        self.authorization = ('Bearer ' + token).encode()
        self.controller = controller
        self.wake_read, self.wake_write = socket.socketpair()
        self.stopped = threading.Event()
        super().__init__(('127.0.0.1', port), Handler)

    def serve_forever(self):
        try:
            with selectors.DefaultSelector() as selector:
                selector.register(self.socket, selectors.EVENT_READ, 'request')
                selector.register(self.wake_read, selectors.EVENT_READ, 'shutdown')
                while True:
                    events = selector.select()
                    if any(key.data == 'shutdown' for key, _ in events):
                        return
                    for key, _ in events:
                        self._handle_request_noblock()
        finally:
            self.stopped.set()

    def shutdown(self):
        self.wake_write.sendall(b'x')
        self.stopped.wait()

    def server_close(self):
        super().server_close()
        self.wake_read.close()
        self.wake_write.close()

    def server_bind(self):
        TCPServer.server_bind(self)
        self.server_name = "127.0.0.1"
        self.server_port = self.server_address[1]

    def process_request(self, request, client_address):
        if not self.slots.acquire(blocking=False):
            self.close_request(request)
            return
        try:
            super().process_request(request, client_address)
        except BaseException:
            self.slots.release()
            raise

    def process_request_thread(self, request, client_address):
        try:
            super().process_request_thread(request, client_address)
        finally:
            self.slots.release()

    def handle_error(self, request, client_address):
        pass


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--port', type=int, required=True)
    parser.add_argument('--token-file', required=True)
    parser.add_argument('command', nargs=argparse.REMAINDER)
    args = parser.parse_args()
    command = args.command[1:] if args.command[:1] == ['--'] else args.command
    if not command or not os.path.isabs(command[0]) or not 1 <= args.port <= 65535:
        parser.error('absolute fixed command and valid port required')
    fd = os.open(args.token_file, os.O_RDONLY | os.O_NOFOLLOW)
    with os.fdopen(fd, 'r', encoding='ascii') as token_file:
        metadata = os.fstat(token_file.fileno())
        if not stat.S_ISREG(metadata.st_mode) or stat.S_IMODE(metadata.st_mode) != 0o600 or metadata.st_uid != os.getuid():
            raise ValueError('token must be an owned mode-0600 regular file')
        token = token_file.read(66).strip()
    if not re.fullmatch(r'[a-f0-9]{64}', token):
        raise ValueError('token must contain 32 random bytes encoded as lowercase hex')
    controller = Controller(command)
    server = Server(args.port, token, controller)
    # Incoming requests and explicit shutdown wake the selector without a timer.
    try:
        server.serve_forever()
    finally:
        server.server_close()


if __name__ == '__main__':
    main()

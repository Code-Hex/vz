#!/usr/bin/env python3
"""Temporary bootstrap for an externally observed native diagnostic."""
import base64
import collections
import http.client
import json
import os
from pathlib import Path
import queue
import re
import secrets
import signal
import subprocess
import sys
import tempfile
import threading
import time

PORT = 8765
CHECK_NAME = 'ci306-external-control'
TUNNEL_URL = re.compile(r'(?<![A-Za-z0-9.-])(https://[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.trycloudflare\.com)(?![A-Za-z0-9.-])')
SAFE_TOKEN = re.compile(r'[A-Za-z0-9._~+/-]+={0,2}\Z')
REPOSITORY = re.compile(r'[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+\Z')
SHA = re.compile(r'[a-fA-F0-9]{40}\Z')
RUN_ID = re.compile(r'[0-9]+\Z')


def child_environment():
    env = {name: os.environ[name] for name in ('HOME', 'TMPDIR', 'LANG', 'LC_ALL') if name in os.environ}
    env.update({'PATH': '/usr/bin:/bin:/usr/sbin:/sbin', 'NAT': '1', 'DGRAM': '0',
                'CONNECT': '0', 'SOCKET_DEVICE': '0', 'COUNT': '10', 'CONSOLE': '0'})
    return env


def check_request(method, url, body, token, tempdir, env, total_timeout=6):
    # The Authorization header travels on curl's stdin, never in argv or the child environment.
    config = ('silent\nshow-error\nfail\nheader = "Accept: application/vnd.github+json"\n'
              'header = "X-GitHub-Api-Version: 2022-11-28"\n'
              'header = "Authorization: Bearer ' + token + '"\n').encode('ascii')
    fd, body_file = tempfile.mkstemp(prefix='ci306-check-body-', dir=tempdir)
    try:
        with os.fdopen(fd, 'wb') as out:
            out.write(json.dumps(body, separators=(',', ':')).encode())
        result = subprocess.run(['/usr/bin/curl', '--config', '-', '--request', method,
                                 '--header', 'Content-Type: application/json',
                                 '--data-binary', '@' + body_file, '--connect-timeout', '3',
                                 '--max-time', str(total_timeout), url],
                                input=config, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                env=env, timeout=total_timeout + 2, check=True)
        return json.loads(result.stdout)
    finally:
        os.unlink(body_file)


def encrypt_token(token, public_key, env):
    result = subprocess.run(['/usr/bin/openssl', 'pkeyutl', '-encrypt', '-pubin',
                             '-inkey', str(public_key), '-pkeyopt', 'rsa_padding_mode:oaep'],
                            input=token.encode('ascii'), stdout=subprocess.PIPE,
                            stderr=subprocess.PIPE, env=env, timeout=15, check=True)
    return base64.b64encode(result.stdout).decode('ascii')


def await_server(server, token, timeout=20):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if server.poll() is not None:
            raise RuntimeError('server exited during startup')
        conn = http.client.HTTPConnection('127.0.0.1', PORT, timeout=1)
        try:
            conn.request('POST', '/state', body=b'{"nonce":"bootstrap"}',
                         headers={'Authorization': 'Bearer ' + token, 'Content-Type': 'application/json'})
            response = conn.getresponse()
            data = response.read(65537)
            if response.status == 200 and len(data) <= 65536 and json.loads(data).get('nonce') == 'bootstrap':
                return
        except (OSError, ValueError, http.client.HTTPException):
            pass
        finally:
            conn.close()
        time.sleep(0.1)
    raise TimeoutError('server startup timed out')


def drain_tunnel(stream, log, results):
    try:
        for line in iter(stream.readline, b''):
            if log.tell() < 65536:
                log.write(line)
            decoded = line.decode('utf-8', 'replace')
            for match in TUNNEL_URL.finditer(decoded):
                try:
                    results.put_nowait(match.group(1))
                except queue.Full:
                    pass
    finally:
        stream.close()


def await_tunnel(tunnel, results, timeout=45):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        try:
            return results.get(timeout=min(0.25, max(0.001, deadline - time.monotonic())))
        except queue.Empty:
            if tunnel.poll() is not None:
                raise RuntimeError('tunnel exited during startup')
    raise TimeoutError('tunnel startup timed out')


def stop_group(process):
    if process is None or process.poll() is not None:
        return
    try:
        os.killpg(process.pid, signal.SIGTERM)
    except ProcessLookupError:
        pass
    try:
        process.wait(timeout=5)
    except subprocess.TimeoutExpired:
        try:
            os.killpg(process.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
        process.wait(timeout=5)


class LocalObservation:
    SYMBOLS = ('runtime.stopTheWorldWithSema', 'runtime.gcMarkDone', 'runtime.forEachPInternal',
               'runtime.reentersyscall', 'runtime.sysmon', 'runtime.cgocall',
               'runtime.cgocallback', 'runtime.notetsleep_internal', 'runtime.entersyscall')

    def __init__(self, token, tunnel_pid, tempdir, env, stop, *, port=PORT, interval=5,
                 samples=(30, 60, 90), startup_limit=180, runtime_limit=360):
        self.token, self.tunnel_pid, self.tempdir, self.env, self.stop = token, tunnel_pid, tempdir, env, stop
        self.port, self.interval = port, interval
        self.startup_limit, self.runtime_limit = startup_limit, runtime_limit
        self.lock = threading.Lock()
        self.pending = queue.Queue(maxsize=1)
        self.history = collections.deque(maxlen=160)
        self.data = {'local_sequence': 0, 'last_local_state': None, 'last_local_error': None,
                     'publisher': None, 'samples': [{'offset_seconds': offset, 'status': 'not_requested'} for offset in samples]}
        self.finished = False
        self.native_tail = ''
        self.native_tail_sequence = None

    def snapshot(self):
        with self.lock:
            return json.loads(json.dumps(self.data))

    def artifact(self):
        with self.lock:
            return json.loads(json.dumps({'state': self.data, 'events': list(self.history),
                'native_output_tail': self.native_tail,
                'native_output_tail_server_sequence': self.native_tail_sequence}))

    def event(self, kind, **values):
        with self.lock:
            self.history.append({'kind': kind, 'wall_ns': time.time_ns(), **values})

    def call(self, action):
        nonce = secrets.token_hex(8)
        connection = http.client.HTTPConnection('127.0.0.1', self.port, timeout=2)
        try:
            connection.request('POST', '/' + action, json.dumps({'nonce': nonce}),
                               {'Authorization': 'Bearer ' + self.token, 'Content-Type': 'application/json'})
            response = connection.getresponse()
            raw = response.read(2000001)
            if response.status != 200 or len(raw) > 2000000:
                raise RuntimeError('local control response rejected')
            data = json.loads(raw)
            if data.get('nonce') != nonce:
                raise RuntimeError('local control nonce mismatch')
            return data
        finally:
            connection.close()

    def read_state(self):
        started = time.monotonic_ns()
        data = self.call('state')
        child = data['child']
        state = {'server_sequence': data['sequence'], 'received_wall_ns': time.time_ns(),
                 'local_rtt_ns': time.monotonic_ns() - started, 'clocks': data['clocks'],
                 'child_pid': child['pid'], 'child_status': child['status'],
                 'child_returncode': child['returncode'], 'child_reaped': child['reaped'],
                 'child_output_bytes': child['output_total_bytes']}
        with self.lock:
            self.data['local_sequence'] += 1
            self.data['last_local_state'] = state
            self.data['last_local_error'] = None
            self.native_tail = '\n'.join(line for line in child['output_tail'].splitlines()
                                         if line.startswith('VZTRACE native-only '))
            self.native_tail_sequence = data['sequence']
            active = any(s['status'] == 'sampling' for s in self.data['samples'])
        self.event('local_state', **state, sample_active=active)
        return data

    def finish_if_exited(self, data):
        if data['child']['status'] != 'exited':
            return False
        self.call('finish')
        self.finished = True
        self.event('finish', success=True)
        return True

    def terminate_child(self):
        for action in ('term', 'kill'):
            try:
                self.call(action)
                self.event(action, accepted=True)
            except Exception as error:
                self.event(action, accepted=False, error=type(error).__name__)
            if action == 'term':
                self.stop.wait(2)
        for _ in range(6):
            try:
                if self.finish_if_exited(self.read_state()):
                    return
            except Exception as error:
                self.event('termination_state', error=type(error).__name__)
            self.stop.wait(1)
        raise RuntimeError('local child exit not confirmed')

    def health_loop(self):
        startup = time.monotonic()
        native_started = None
        try:
            while not self.stop.is_set():
                now = time.monotonic()
                try:
                    data = self.read_state()
                    if self.finish_if_exited(data):
                        return
                    if data['child']['pid'] is not None and native_started is None:
                        native_started = now
                except Exception as error:
                    with self.lock:
                        self.data['last_local_error'] = {'type': type(error).__name__, 'wall_ns': time.time_ns()}
                    self.event('local_error', error=type(error).__name__)
                if native_started is None:
                    if now - startup >= self.startup_limit:
                        raise RuntimeError('native was not observed before startup deadline')
                else:
                    elapsed = now - native_started
                    with self.lock:
                        due = [i for i, sample in enumerate(self.data['samples'])
                               if sample['status'] == 'not_requested' and elapsed >= sample['offset_seconds']]
                        for index in due:
                            try:
                                self.pending.put_nowait(index)
                                self.data['samples'][index]['status'] = 'queued'
                            except queue.Full:
                                self.data['samples'][index]['status'] = 'skipped_busy'
                    if elapsed >= self.runtime_limit:
                        self.event('native_deadline')
                        self.terminate_child()
                        return
                self.stop.wait(self.interval)
        except Exception as error:
            self.event('health_failed', error=type(error).__name__)

    def sample_loop(self):
        while not self.stop.is_set():
            try:
                index = self.pending.get(timeout=0.2)
            except queue.Empty:
                continue
            output = self.tempdir / ('ci306-transport-sample-' + str(index) + '.txt')
            command_log = self.tempdir / ('ci306-transport-sample-' + str(index) + '.command.txt')
            record = {'status': 'sampling', 'started_wall_ns': time.time_ns(), 'pid': self.tunnel_pid}
            with self.lock:
                self.data['samples'][index].update(record)
            try:
                with command_log.open('wb') as log:
                    result = subprocess.run(['/usr/bin/sample', str(self.tunnel_pid), '1', '10',
                                             '-mayDie', '-file', str(output)],
                                            stdin=subprocess.DEVNULL, stdout=log, stderr=subprocess.STDOUT,
                                            env=self.env, timeout=8, check=False)
                record.update(status='captured' if result.returncode == 0 else 'failed', returncode=result.returncode)
                if output.is_file():
                    with output.open('rb') as raw:
                        text = raw.read(2000001)
                    record['symbol_scan_complete'] = len(text) <= 2000000
                    record['symbols_present'] = [name for name in self.SYMBOLS if name.encode() in text]
            except subprocess.TimeoutExpired:
                record.update(status='timed_out', returncode=None)
            except Exception as error:
                record.update(status='failed', error=type(error).__name__)
            record['finished_wall_ns'] = time.time_ns()
            with self.lock:
                self.data['samples'][index].update(record)
            self.event('sample_result', **record)
        with self.lock:
            for sample in self.data['samples']:
                if sample['status'] == 'queued':
                    sample['status'] = 'canceled'


def publish_observation(observation, api_url, metadata, token, tempdir, env, stop):
    while True:
        summary = dict(metadata, sidecar=observation.snapshot())
        try:
            check_request('PATCH', api_url, {'output': {'title': CHECK_NAME,
                          'summary': json.dumps(summary, separators=(',', ':'))}}, token, tempdir, env, total_timeout=6)
            result = {'status': 'published', 'wall_ns': time.time_ns()}
        except Exception as error:
            result = {'status': 'failed', 'error': type(error).__name__, 'wall_ns': time.time_ns()}
        with observation.lock:
            observation.data['publisher'] = result
        if stop.wait(10):
            return


def main():
    # Secrets remain in this process. None of the children inherit the parent environment.
    if os.environ.get('GITHUB_REPOSITORY') != 'Code-Hex/vz' or os.environ.get('GITHUB_REF') != 'refs/heads/codex/diagnose-ci306-intel':
        raise ValueError('diagnostic branch required')
    token_github = os.environ['GITHUB_TOKEN']
    repo = os.environ['GITHUB_REPOSITORY']
    sha = os.environ['GITHUB_SHA']
    run_id = os.environ['GITHUB_RUN_ID']
    tempdir = Path(os.environ['RUNNER_TEMP']).resolve(strict=True)
    if not (SAFE_TOKEN.fullmatch(token_github) and REPOSITORY.fullmatch(repo) and
            SHA.fullmatch(sha) and RUN_ID.fullmatch(run_id)):
        rejected = [name for name, valid in [('token_header', SAFE_TOKEN.fullmatch(token_github)), ('repo', REPOSITORY.fullmatch(repo)), ('sha', SHA.fullmatch(sha)), ('run_id', RUN_ID.fullmatch(run_id))] if not valid]
        print('bootstrap invalid fields: ' + ','.join(rejected), file=sys.stderr)
        raise ValueError('invalid GitHub configuration')
    here = Path(__file__).resolve().parent
    server_script = here / 'external-control.py'
    public_key = here / 'observer-public.pem'
    native = tempdir / 'native-pair-control'
    tunnel_bin = tempdir / 'cloudflared'
    kernel = (Path.cwd() / 'testdata' / 'Image').resolve(strict=True)
    initramfs = (Path.cwd() / 'testdata' / 'initramfs.cpio.gz').resolve(strict=True)
    for path in (server_script, public_key, native, tunnel_bin):
        if not path.is_file():
            raise FileNotFoundError('bootstrap input unavailable')
    env = child_environment()
    token = secrets.token_hex(32)
    check_id = None
    token_file = None
    server = None
    tunnel = None
    tunnel_reader = None
    server_log = None
    tunnel_log = None
    completed = False
    observation = None
    observer_stop = threading.Event()
    observer_threads = []
    api_url = 'https://api.github.com/repos/' + repo + '/check-runs'
    old_term = signal.getsignal(signal.SIGTERM)

    def terminate(_signum, _frame):
        raise SystemExit(1)

    signal.signal(signal.SIGTERM, terminate)
    try:
        fd, token_file = tempfile.mkstemp(prefix='ci306-control-token-', dir=tempdir)
        with os.fdopen(fd, 'w', encoding='ascii') as out:
            out.write(token)
        os.chmod(token_file, 0o600)
        server_log = open(tempdir / 'ci306-control-server.log', 'wb')
        server = subprocess.Popen([sys.executable, str(server_script), '--port', str(PORT),
                                   '--token-file', token_file, '--', str(native), str(kernel), str(initramfs)],
                                  stdin=subprocess.DEVNULL, stdout=server_log, stderr=subprocess.STDOUT,
                                  env=env, start_new_session=True, close_fds=True)
        print('bootstrap phase=server-health', file=sys.stderr)
        await_server(server, token)
        tunnel_log = open(tempdir / 'ci306-control-tunnel.log', 'wb')
        tunnel = subprocess.Popen([str(tunnel_bin), 'tunnel', '--no-autoupdate', '--url',
                                   'http://127.0.0.1:8765', '--protocol', 'http2'],
                                  stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.PIPE,
                                  env=env, start_new_session=True, close_fds=True)
        results = queue.Queue(maxsize=1)
        tunnel_reader = threading.Thread(target=drain_tunnel, args=(tunnel.stderr, tunnel_log, results), daemon=True)
        tunnel_reader.start()
        print('bootstrap phase=tunnel', file=sys.stderr)
        public_url = await_tunnel(tunnel, results)
        print('bootstrap phase=encrypt', file=sys.stderr)
        encrypted = encrypt_token(token, public_key, env)
        summary = json.dumps({'url': public_url, 'ciphertext': encrypted, 'run_id': run_id}, separators=(',', ':'))
        print('bootstrap phase=publish', file=sys.stderr)
        created = check_request('POST', api_url, {'name': CHECK_NAME, 'head_sha': sha, 'status': 'in_progress',
                                'output': {'title': CHECK_NAME, 'summary': summary}}, token_github, tempdir, env)
        check_id = int(created['id'])
        observation = LocalObservation(token, tunnel.pid, tempdir, env, observer_stop)
        health = threading.Thread(target=observation.health_loop, daemon=True)
        sampler = threading.Thread(target=observation.sample_loop, daemon=True)
        publisher = threading.Thread(target=publish_observation, args=(observation,
            api_url + '/' + str(check_id), json.loads(summary), token_github, tempdir, env, observer_stop), daemon=True)
        observer_threads = [health, sampler, publisher]
        for worker in observer_threads:
            worker.start()
        health.join()
        observer_stop.set()
        for worker in (sampler, publisher):
            worker.join()
        if not observation.finished:
            raise RuntimeError('local observation ended without confirmed child exit')
        completed = server.wait(timeout=5) == 0
        if not completed:
            raise RuntimeError('control server exited unsuccessfully')
    finally:
        signal.signal(signal.SIGTERM, signal.SIG_IGN)
        try:
            observer_stop.set()
            for worker in observer_threads:
                if worker.ident is not None:
                    worker.join(timeout=10)
            if observation is not None and not observation.finished:
                try:
                    state = observation.read_state()
                    if state['child']['pid'] is None:
                        observation.event('cleanup', status='native_not_started')
                    elif not observation.finish_if_exited(state):
                        observation.terminate_child()
                except Exception as error:
                    observation.event('cleanup', status='native_exit_unconfirmed', error=type(error).__name__)
            if observation is not None:
                (tempdir / 'ci306-sidecar-state.json').write_text(json.dumps(observation.artifact(), indent=2))
            for process in (tunnel, server):
                try:
                    stop_group(process)
                except Exception as error:
                    print('child cleanup failed: ' + type(error).__name__, file=sys.stderr)
            if tunnel_reader is not None:
                tunnel_reader.join(timeout=5)
            if token_file is not None:
                try:
                    os.unlink(token_file)
                except FileNotFoundError:
                    pass
                except OSError as error:
                    print('token cleanup failed: ' + type(error).__name__, file=sys.stderr)
            if check_id is not None:
                try:
                    check_request('PATCH', api_url + '/' + str(check_id),
                                  {'status': 'completed', 'conclusion': 'neutral',
                                   'output': {'title': CHECK_NAME,
                                              'summary': json.dumps(dict(json.loads(summary), sidecar=observation.snapshot(), completed=completed)) if observation is not None else 'External observation ended before completion.'}},
                                  token_github, tempdir, env)
                except Exception as error:
                    print('check completion failed: ' + type(error).__name__, file=sys.stderr)
        finally:
            if server_log is not None:
                server_log.close()
            if tunnel_log is not None:
                tunnel_log.close()
            signal.signal(signal.SIGTERM, old_term)


if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        print('bootstrap failed: ' + type(error).__name__, file=sys.stderr)
        sys.exit(1)

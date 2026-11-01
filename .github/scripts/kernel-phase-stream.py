#!/usr/bin/env python3
"""Bounded encrypted DTrace windows for an isolated CI diagnostic."""
import base64
import ctypes
import gzip
import hmac
import importlib.util
import json
import os
import platform
import re
import queue
from pathlib import Path
import secrets
import signal
import subprocess
import sys
import tempfile
import threading
import time

NAME = 'ci306-kernel-fbt-stream'
PHASE_NAME = 'ci306-native-phase-stream'
MAX_SUMMARY = 64000
OUTPUT_LIMIT = 65536
class SnapshotTooLarge(Exception):
    pass


class ParentClock:
    def __init__(self):
        self.lib = ctypes.CDLL('/usr/lib/libSystem.B.dylib')
        self.lib.mach_absolute_time.argtypes = []
        self.lib.mach_absolute_time.restype = ctypes.c_uint64
        class Timebase(ctypes.Structure):
            _fields_ = [('numer', ctypes.c_uint32), ('denom', ctypes.c_uint32)]
        self.lib.mach_timebase_info.argtypes = [ctypes.POINTER(Timebase)]
        self.lib.mach_timebase_info.restype = ctypes.c_int
        info = Timebase()
        if self.lib.mach_timebase_info(ctypes.byref(info)) != 0 or not info.numer or not info.denom:
            raise RuntimeError('mach timebase unavailable')
        self.timebase = {'numer': info.numer, 'denom': info.denom}
    def read(self):
        return {'mach_absolute_ticks': self.lib.mach_absolute_time(), 'wall_ns': time.time_ns(),
                'mach_timebase': self.timebase}

def helpers():
    p = Path(__file__).resolve().parent / 'external-bootstrap-observed.py'
    spec = importlib.util.spec_from_file_location('bootstrap', p)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def child_environment():
    env = {n: os.environ[n] for n in ('HOME', 'TMPDIR', 'LANG', 'LC_ALL') if n in os.environ}
    env.update(PATH='/usr/bin:/bin:/usr/sbin:/sbin', NAT='0', DGRAM='0', CONNECT='0', SOCKET_DEVICE='0', COUNT='20', CONSOLE='0')
    return env


def encrypt_record(record, public_key, env):
    password = secrets.token_hex(32).encode('ascii')
    packed = gzip.compress(json.dumps(record, separators=(',', ':'), ensure_ascii=True).encode(), mtime=0)
    rd, wr = os.pipe()
    try:
        os.write(wr, password + b'\n')
        os.close(wr)
        wr = -1
        encrypted = subprocess.run(['/usr/bin/openssl', 'enc', '-aes-256-cbc', '-pbkdf2', '-iter', '100000', '-pass', 'fd:' + str(rd)], input=packed, stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=env, pass_fds=(rd,), timeout=15, check=True).stdout
    finally:
        os.close(rd)
        if wr >= 0:
            os.close(wr)
    key = subprocess.run(['/usr/bin/openssl', 'pkeyutl', '-encrypt', '-pubin', '-inkey', str(public_key), '-pkeyopt', 'rsa_padding_mode:oaep'], input=password, stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=env, timeout=15, check=True).stdout
    mac_key = hmac.digest(password, b'vz-kernel-envelope-v1', 'sha256')
    tag = hmac.digest(mac_key, encrypted, 'sha256').hex()
    return {'format': 'gzip+aes-256-cbc-pbkdf2-sha256-100000+rsa-oaep+hmacsha256', 'key': base64.b64encode(key).decode(), 'data': base64.b64encode(encrypted).decode(), 'hmac_sha256': tag}


class Tail:
    def __init__(self):
        self.lock = threading.Lock()
        self.data = bytearray()
        self.total = 0
    def drain(self, stream):
        try:
            while True:
                block = stream.read(16384)
                if not block:
                    return
                with self.lock:
                    self.total += len(block)
                    self.data.extend(block)
                    del self.data[:-OUTPUT_LIMIT]
        finally:
            stream.close()
    def read(self):
        with self.lock:
            return {'tail': self.data.decode('utf-8', 'replace'), 'total_bytes': self.total, 'dropped_bytes': self.total - len(self.data)}


def watchdog(parent, deadline):
    while os.getppid() == parent and time.monotonic() < deadline:
        time.sleep(min(0.2, max(0, deadline - time.monotonic())))
    if os.getppid() == parent:
        try:
            os.kill(parent, signal.SIGTERM)
        except ProcessLookupError:
            pass


def fit_record(record):
    record = dict(record)
    record['window'] = dict(record['window'])
    record['window']['raw_lines'] = list(record['window']['raw_lines'])
    record['native_output'] = dict(record['native_output'])
    record['transport_dropped_lines'] = 0
    record['transport_dropped_bytes'] = 0
    record['transport_dropped_native_bytes'] = 0
    record['stderr'] = {name: dict(value) for name, value in record.get('stderr', {}).items()}
    record['transport_dropped_stderr_bytes'] = {name: 0 for name in record['stderr']}
    while len(gzip.compress(json.dumps(record, separators=(',', ':'), ensure_ascii=True).encode(), mtime=0)) > 44000:
        lines = record['window']['raw_lines']
        if lines:
            count = max(1, len(lines) // 4)
            dropped = lines[:count]
            del lines[:count]
            record['transport_dropped_lines'] += count
            record['transport_dropped_bytes'] += sum(len(x.encode('utf-8')) for x in dropped)
        elif record['native_output']['tail']:
            tail = record['native_output']['tail']
            cut = max(1, len(tail) // 2)
            record['transport_dropped_native_bytes'] += len(tail[:cut].encode('utf-8'))
            record['native_output']['tail'] = tail[cut:]
        elif any(value['tail'] for value in record['stderr'].values()):
            name = max(record['stderr'], key=lambda n: len(record['stderr'][n]['tail']))
            tail = record['stderr'][name]['tail']
            cut = max(1, len(tail) // 2)
            record['transport_dropped_stderr_bytes'][name] += len(tail[:cut].encode('utf-8'))
            record['stderr'][name]['tail'] = tail[cut:]
        else:
            raise SnapshotTooLarge('metadata exceeds compressed budget')
    return record


def main():
    if os.environ.get('GITHUB_REPOSITORY') != 'Code-Hex/vz' or os.environ.get('GITHUB_REF') != 'refs/heads/codex/diagnose-ci306-intel':
        raise ValueError('incorrect diagnostic repository or branch')
    bootstrap = helpers()
    env = child_environment()
    mode = os.environ.get('RETAIN_STOPPED')
    if mode not in ('0', '1'):
        raise ValueError('invalid retained-stop mode')
    env.update(COUNT='3', RETAIN_STOPPED=mode)
    phase_spec = importlib.util.spec_from_file_location('phase_protocol', Path(__file__).resolve().parent / 'phase-protocol-ci306.py')
    phase_module = importlib.util.module_from_spec(phase_spec)
    phase_spec.loader.exec_module(phase_module)
    protocol = phase_module.PhaseProtocol(mode)
    token = os.environ.pop('GITHUB_TOKEN')
    repo, sha, run = (os.environ[x] for x in ('GITHUB_REPOSITORY', 'GITHUB_SHA', 'GITHUB_RUN_ID'))
    if not (bootstrap.SAFE_TOKEN.fullmatch(token) and bootstrap.REPOSITORY.fullmatch(repo) and bootstrap.SHA.fullmatch(sha) and bootstrap.RUN_ID.fullmatch(run)):
        raise ValueError('invalid GitHub metadata')
    temp = Path(os.environ['RUNNER_TEMP']).resolve(strict=True)
    private = Path(tempfile.mkdtemp(prefix='ci306-trace-private-', dir=temp))
    artifacts = temp / 'ci306-kernel-fbt-encrypted'
    artifacts.mkdir(mode=0o700, exist_ok=False)
    public = Path(__file__).resolve().parent / 'observer-public.pem'
    collector_script = Path(__file__).resolve().parent / 'dtrace-window-ci306.py'
    dtrace_scripts = {name: Path(__file__).resolve().parent / path for name, path in [('fbt', 'dtrace-ept-mp-ci306.d'), ('profile', 'dtrace-profile-ci306.d')]}
    native = temp / 'native-phase-control'
    kernel = (Path.cwd() / 'testdata/Image').resolve(strict=True)
    initramfs = (Path.cwd() / 'testdata/initramfs.cpio.gz').resolve(strict=True)
    api = 'https://api.github.com/repos/' + repo + '/check-runs'
    groups = {}
    child = collector = None
    producers = {}
    tail = phase_module.NativeReader()
    stderr_tails = {'fbt': Tail(), 'profile': Tail(), 'collector': Tail()}
    stderr_readers = []
    native_reader = None
    clock = ParentClock()
    last = None
    next_window = 0
    missing_windows = []
    failure = None
    failure_detail = None
    check_id = None
    phase_check_id = None
    phase_done = False
    completed = False
    seen_sources = set()
    deadline = time.monotonic() + 220
    stop = threading.Event()

    def spawn(args, **kwargs):
        proc = subprocess.Popen(args, env=env, start_new_session=True, close_fds=True, **kwargs)
        groups[proc.pid] = proc
        return proc
    def state(proc):
        if proc is None:
            return {'status': 'not_started', 'returncode': None}
        code = proc.poll()
        return {'status': 'running' if code is None else 'exited', 'returncode': code}
    def public_state():
        return {'variant': int(mode), 'native': state(child), 'producers': {name: state(producers.get(name)) for name in ('fbt', 'profile')}, 'collector': state(collector), 'native_output_bytes': tail.read()['total_bytes'], 'pending_window_sequences': [int(p.name[7:13]) for p in sorted(private.glob('window-*.json.gz'))]}
    def publish(phase, conclusion=None, annotation=None):
        body = {'phase': phase, 'run_id': run, **public_state(), 'error': failure, 'snapshot': last}
        summary = json.dumps(body, separators=(',', ':'), ensure_ascii=True)
        if len(summary.encode()) > MAX_SUMMARY:
            raise SnapshotTooLarge('encrypted snapshot exceeds Check summary limit')
        req = {'output': {'title': NAME, 'summary': summary}}
        if annotation is not None:
            req['output']['annotations'] = [{'path': '.github/scripts/kernel-fbt-stream.py', 'start_line': 1, 'end_line': 1, 'annotation_level': 'notice', 'message': json.dumps(annotation, separators=(',', ':'))}]
            if len(req['output']['annotations'][0]['message'].encode()) > MAX_SUMMARY:
                raise SnapshotTooLarge('encrypted annotation exceeds limit')
        if conclusion:
            req.update(status='completed', conclusion=conclusion)
        bootstrap.check_request('PATCH', api + '/' + str(check_id), req, token, temp, env)
    def send_window(path, baseline=False):
        nonlocal last, next_window
        window = json.loads(gzip.decompress(path.read_bytes()))
        window_loaded = clock.read()
        if window['sequence'] != next_window:
            raise ValueError('unexpected window sequence')
        if baseline and window['stop_reason'] != 'window':
            raise RuntimeError('baseline window did not complete')
        record = {'type': 'dtrace-window-v1', 'clock_description': {'dtrace_timestamp': 'DTrace timestamp in ns; epoch not assumed', 'dtrace_walltimestamp': 'DTrace walltimestamp in ns', 'collector_window': 'collector receive monotonic and realtime', 'parent_mach': 'mach_absolute_time ticks with timebase'}, 'window': window, 'native_output': tail.read(),
                  'transport_missing_window_sequences': list(missing_windows),
                  'stderr': {name: value.read() for name, value in stderr_tails.items()}, **public_state()}
        record['method'] = {'fbt': 'DTrace FBT mp_cpus_call1 and UUID-gated pmap_flush_tlbs', 'profile': 'DTrace profile-10 with stack(8)', 'kernel_uuid': 'F6A1B3D4-1EBB-3628-A1C5-89506C51DDEC'}
        record['parent_window_loaded'] = window_loaded
        record['parent_send_preparation'] = clock.read()
        record = fit_record(record)
        envelope = encrypt_record(record, public, env)
        candidate = {'sequence': next_window, **envelope}
        previous = last
        last = candidate
        try:
            preview = json.dumps({'phase': 'sampled', 'run_id': run, **public_state(), 'error': failure, 'snapshot': candidate}, separators=(',', ':'), ensure_ascii=True)
            if len(preview.encode()) > MAX_SUMMARY:
                raise SnapshotTooLarge('encrypted snapshot exceeds Check summary limit')
            (artifacts / ('window-%06d.json' % next_window)).write_text(json.dumps(candidate, separators=(',', ':')))
            publish('baseline_window' if baseline else 'sampled', annotation=candidate)
        except BaseException:
            last = previous
            raise
        for name, marker in [('fbt', 'fbt\tBEGIN '), ('profile', 'profile\tPROFILE_BEGIN ')]:
            if any(line.startswith(marker) for line in window['raw_lines']):
                seen_sources.add(name)
        if any(line.startswith('profile\tCPU_SAMPLE ') for line in window['raw_lines']):
            seen_sources.add('cpu_sample')
        next_window += 1
        path.unlink()
        if baseline and any(line.startswith(('fbt\tERROR ', 'profile\tPROFILE_ERROR ')) for line in window['raw_lines']):
            raise RuntimeError('DTrace reported an error during baseline')
    def drain_windows(baseline=False):
        nonlocal next_window
        sent = 0
        while True:
            path = private / ('window-%06d.json.gz' % next_window)
            if not path.exists():
                available = sorted(private.glob('window-*.json.gz'))
                if available and available[0].name > path.name:
                    candidate_seq = int(available[0].name[7:13])
                    missing_windows.extend(range(next_window, candidate_seq))
                    next_window = candidate_seq
                    path = available[0]
                else:
                    break
            send_window(path, baseline=baseline and sent == 0)
            sent += 1
            break
        return sent
    def phase_publish(event, conclusion=None):
        nonlocal phase_done
        record = fit_record({'type': 'native-phase-v1', 'window': {'raw_lines': []}, 'native_output': tail.read(),
                             'run_id': run, 'variant': int(mode), 'event': event, 'observed_facts': list(protocol.history),
                             'native': state(child), 'parent_clock': clock.read()})
        envelope = {'sequence': event['sequence'], **encrypt_record(record, public, env)}
        summary = json.dumps({'phase': event['fact'], 'run_id': run, 'variant': int(mode), 'snapshot': envelope}, separators=(',', ':'), ensure_ascii=True)
        message = json.dumps(envelope, separators=(',', ':'))
        if len(summary.encode()) > MAX_SUMMARY or len(message.encode()) > MAX_SUMMARY:
            raise SnapshotTooLarge('encrypted phase exceeds Check limit')
        output = {'title': PHASE_NAME, 'summary': summary, 'annotations': [{'path': '.github/scripts/native-phase-ci306.m', 'start_line': 1, 'end_line': 1, 'annotation_level': 'notice', 'message': message}]}
        request = {'output': output}
        if conclusion is not None:
            request.update(status='completed', conclusion=conclusion)
        (artifacts / ('phase-%04d.json' % event['sequence'])).write_text(json.dumps(envelope, separators=(',', ':')))
        bootstrap.check_request('PATCH', api + '/' + str(phase_check_id), request, token, temp, env)
        if conclusion is not None:
            phase_done = True
    def wait_after_checkpoint(seconds):
        until = time.monotonic() + seconds
        while time.monotonic() < until:
            if child.poll() is not None or time.monotonic() >= deadline or stop.is_set() or tail.failed.is_set():
                raise RuntimeError('native unavailable while waiting after checkpoint')
            drain_windows()
            if any(proc.poll() is not None for proc in producers.values()) or collector.poll() is not None:
                raise RuntimeError('trace pipeline stopped while waiting after checkpoint')
            stop.wait(min(0.2, max(0, until - time.monotonic())))
    def pump_phases(allow_ready=True):
        if tail.failed.is_set():
            raise RuntimeError('native phase stream overflow or invalid output')
        while True:
            try:
                line = tail.events.get_nowait()
            except queue.Empty:
                return
            event = protocol.accept(line)
            if event['fact'] == 'ready':
                if not allow_ready or child.poll() is not None:
                    raise RuntimeError('native exited before permission')
                protocol.release(event, phase_publish, wait_after_checkpoint, child.stdin.fileno())
    def terminate(_signum, _frame):
        stop.set()
        raise SystemExit(1)
    def signal_group(proc, sig):
        if proc is not None and proc.poll() is None:
            subprocess.run(['/usr/bin/sudo', '-n', '/bin/kill', '-' + sig, '--', '-' + str(proc.pid)], env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=5, check=False)
    oldterm = signal.signal(signal.SIGTERM, terminate)
    guard = subprocess.Popen([sys.executable, str(Path(__file__).resolve()), '--watchdog', str(os.getpid()), str(deadline)], env=env, stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, close_fds=True, start_new_session=True)
    try:
        created = bootstrap.check_request('POST', api, {'name': NAME, 'head_sha': sha, 'status': 'in_progress', 'output': {'title': NAME, 'summary': json.dumps({'phase': 'initializing', 'run_id': run})}}, token, temp, env)
        check_id = int(created['id'])
        if platform.machine() != 'x86_64':
            raise RuntimeError('unexpected kernel architecture')
        result = subprocess.run(['/usr/bin/xcrun', 'dwarfdump', '--uuid', '/System/Library/Kernels/kernel'], env=env, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=15, check=True)
        if b'F6A1B3D4-1EBB-3628-A1C5-89506C51DDEC' not in result.stdout.upper():
            raise RuntimeError('unexpected kernel UUID')
        try:
            for name in ('fbt', 'profile'):
                proc = spawn(['/usr/bin/sudo', '-n', '/usr/sbin/dtrace', '-s', str(dtrace_scripts[name])], stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.PIPE, bufsize=0)
                producers[name] = proc
                reader = threading.Thread(target=stderr_tails[name].drain, args=(proc.stderr,), daemon=True)
                reader.start()
                stderr_readers.append(reader)
            fbt_fd = producers['fbt'].stdout.fileno()
            profile_fd = producers['profile'].stdout.fileno()
            collector = spawn([sys.executable, str(collector_script), '--output-dir', str(private), '--fbt-fd', str(fbt_fd), '--profile-fd', str(profile_fd), '--window-seconds', '3', '--duration-seconds', '205'], stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.PIPE, pass_fds=(fbt_fd, profile_fd), bufsize=0)
            collector_reader = threading.Thread(target=stderr_tails['collector'].drain, args=(collector.stderr,), daemon=True)
            collector_reader.start()
            stderr_readers.append(collector_reader)
        finally:
            for proc in producers.values():
                proc.stdout.close()
        publish('baseline_collecting')
        while seen_sources != {'fbt', 'profile', 'cpu_sample'} and time.monotonic() < deadline:
            if any(proc.poll() is not None for proc in producers.values()) or collector.poll() is not None:
                raise RuntimeError('trace pipeline stopped before baseline')
            drain_windows(baseline=True)
            stop.wait(0.2)
        if seen_sources != {'fbt', 'profile', 'cpu_sample'} or time.monotonic() >= deadline:
            raise TimeoutError('baseline deadline')
        publish('baseline_saved')
        phase_created = bootstrap.check_request('POST', api, {'name': PHASE_NAME, 'head_sha': sha, 'status': 'in_progress', 'output': {'title': PHASE_NAME, 'summary': json.dumps({'phase': 'initializing', 'run_id': run, 'variant': int(mode)})}}, token, temp, env)
        phase_check_id = int(phase_created['id'])
        child = spawn([str(native), str(kernel), str(initramfs)], stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, bufsize=0)
        native_reader = threading.Thread(target=tail.drain, args=(child.stdout,), daemon=True)
        native_reader.start()
        publish('native_started')
        while child.poll() is None and time.monotonic() < deadline and not stop.is_set():
            pump_phases()
            drain_windows()
            if any(proc.poll() is not None for proc in producers.values()) or collector.poll() is not None:
                raise RuntimeError('trace pipeline stopped during native execution')
            stop.wait(min(0.2, max(0, deadline - time.monotonic())))
        if child.poll() is None:
            raise TimeoutError('native deadline')
        code = child.wait()
        groups.pop(child.pid, None)
        native_reader.join(timeout=2)
        pump_phases(allow_ready=False)
        expected = 3 * 7
        actual = sum(item['fact'] == 'complete' for item in protocol.history)
        if code == 0 and (protocol.pending is not None or protocol.last_sequence != expected or actual != expected or 'ALL_COMPLETE count=3 result=0' not in tail.read()['tail']):
            raise RuntimeError('native exited without all phase completions')
        phase_publish({'fact': 'exited', 'sequence': protocol.last_sequence + 1, 'returncode': code}, 'success' if code == 0 else 'failure')
        for proc in producers.values():
            signal_group(proc, 'TERM')
        for proc in producers.values():
            try:
                proc.wait(timeout=5)
            except subprocess.TimeoutExpired:
                signal_group(proc, 'KILL')
                proc.wait(timeout=5)
            groups.pop(proc.pid, None)
        collector.wait(timeout=10)
        if collector.returncode != 0:
            raise RuntimeError('collector failed')
        groups.pop(collector.pid, None)
        for stderr_reader in stderr_readers:
            stderr_reader.join(timeout=2)
        while time.monotonic() < deadline and drain_windows():
            pass
        if list(private.glob('window-*.json.gz')):
            raise TimeoutError('deadline with unsent trace windows')
        publish('finished', 'success' if code == 0 else 'failure')
        completed = True
        return 0 if code == 0 else 1
    except Exception as exc:
        failure = type(exc).__name__
        failure_detail = str(exc)
        raise
    finally:
        signal.signal(signal.SIGTERM, signal.SIG_IGN)
        for proc in list(groups.values()):
            try:
                signal_group(proc, 'KILL')
                proc.wait(timeout=5)
            except (OSError, subprocess.TimeoutExpired):
                pass
        if native_reader is not None:
            native_reader.join(timeout=2)
        for stderr_reader in stderr_readers:
            stderr_reader.join(timeout=2)
        if phase_check_id is not None and not phase_done:
            try:
                phase_publish({'fact': 'failed', 'sequence': protocol.last_sequence + 1, 'error': failure or 'interrupted'}, 'failure')
            except Exception:
                pass
        if check_id is not None and not completed:
            try:
                record = fit_record({'type': 'dtrace-diagnostic-v1',
                    'window': {'sequence': next_window, 'stop_reason': 'diagnostic', 'input_bytes': 0, 'raw_lines': []},
                    'native_output': tail.read(), 'transport_missing_window_sequences': list(missing_windows),
                    'stderr': {name: value.read() for name, value in stderr_tails.items()},
                    'readers_pending': {'native': native_reader is not None and native_reader.is_alive(),
                                        'stderr': sum(reader.is_alive() for reader in stderr_readers)},
                    'error': failure or 'interrupted', 'failure_detail': failure_detail,
                    'parent_send_preparation': clock.read(), **public_state()})
                candidate = {'sequence': next_window, **encrypt_record(record, public, env)}
                summary = json.dumps({'phase': 'failed', 'run_id': run, **public_state(), 'error': failure, 'snapshot': candidate}, separators=(',', ':'), ensure_ascii=True)
                if len(summary.encode()) > MAX_SUMMARY:
                    raise SnapshotTooLarge('encrypted diagnostic exceeds Check summary limit')
                (artifacts / 'diagnostic-final.json').write_text(json.dumps(candidate, separators=(',', ':')))
                last = candidate
            except Exception as exc:
                print('kernel FBT diagnostic failed: ' + type(exc).__name__, file=sys.stderr)
            try: publish('failed', 'failure')
            except Exception: pass
        guard.terminate()
        try: guard.wait(timeout=5)
        except subprocess.TimeoutExpired: guard.kill(); guard.wait()
        for path in private.iterdir():
            if path.is_file(): path.unlink()
        private.rmdir()
        signal.signal(signal.SIGTERM, oldterm)

if __name__ == '__main__':
    if len(sys.argv) == 4 and sys.argv[1] == '--watchdog':
        watchdog(int(sys.argv[2]), float(sys.argv[3]))
    else:
        try: sys.exit(main())
        except Exception as exc:
            print('kernel-phase-stream failed: ' + type(exc).__name__, file=sys.stderr)
            sys.exit(1)

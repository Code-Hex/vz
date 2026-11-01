#!/usr/bin/env python3
"""Temporary encrypted, bounded kernel stack observation for CI diagnostics."""
import base64
import gzip
import hmac
import importlib.util
import json
import os
from pathlib import Path
import re
import secrets
import signal
import subprocess
import sys
import tempfile
import threading
import time

NAME = 'ci306-kernel-stream'
MAX_SUMMARY = 64000
OUTPUT_LIMIT = 65536
SELECTED = re.compile(r'^(?:kernel_task|native-pair-control|virtualizationd|com\.apple\.Virtualization(?:\.[^ ]*)?)$', re.I)
class SnapshotTooLarge(Exception):
    pass


PROCESS = re.compile(r'^Process:\s+(.+?)\s+\[(\d+)\]\s*$', re.M)


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


def filter_report(report):
    matches = list(PROCESS.finditer(report))
    if not matches:
        raise ValueError('no process sections')
    selected = [report[:matches[0].start()]]
    for index, match in enumerate(matches):
        if SELECTED.fullmatch(match.group(1).strip()):
            end = matches[index + 1].start() if index + 1 < len(matches) else len(report)
            selected.append(report[match.start():end])
    if not any(SELECTED.fullmatch(m.group(1).strip()) and m.group(1).strip().lower() == 'kernel_task' for m in matches):
        raise ValueError('kernel_task missing')
    return ''.join(selected)


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


def main():
    if os.environ.get('GITHUB_REPOSITORY') != 'Code-Hex/vz' or os.environ.get('GITHUB_REF') != 'refs/heads/codex/diagnose-ci306-intel':
        raise ValueError('incorrect diagnostic repository or branch')
    bootstrap = helpers()
    env = child_environment()
    token = os.environ.pop('GITHUB_TOKEN')
    repo, sha, run = (os.environ[x] for x in ('GITHUB_REPOSITORY', 'GITHUB_SHA', 'GITHUB_RUN_ID'))
    if not (bootstrap.SAFE_TOKEN.fullmatch(token) and bootstrap.REPOSITORY.fullmatch(repo) and bootstrap.SHA.fullmatch(sha) and bootstrap.RUN_ID.fullmatch(run)):
        raise ValueError('invalid GitHub metadata')
    temp = Path(os.environ['RUNNER_TEMP']).resolve(strict=True)
    root = Path(tempfile.mkdtemp(prefix='ci306-kernel-private-', dir=temp))
    artifacts = temp / 'ci306-kernel-encrypted'
    artifacts.mkdir(mode=0o700, exist_ok=False)
    raw = root / 'spindump.txt'
    public = Path(__file__).resolve().parent / 'observer-public.pem'
    native = temp / 'native-pair-control'
    kernel = (Path.cwd() / 'testdata/Image').resolve(strict=True)
    initramfs = (Path.cwd() / 'testdata/initramfs.cpio.gz').resolve(strict=True)
    api = 'https://api.github.com/repos/' + repo + '/check-runs'
    groups = {}
    child = None
    tail = Tail()
    sequence = 0
    last = None
    capture_rc = None
    failure = None
    check_id = None
    check_completed = False
    begin = time.monotonic()
    deadline = begin + 180
    stop = threading.Event()

    def spawn(args, **kwargs):
        proc = subprocess.Popen(args, env=env, stdin=subprocess.DEVNULL, start_new_session=True, close_fds=True, **kwargs)
        groups[proc.pid] = proc
        return proc
    def finished(proc):
        if proc.poll() is not None:
            groups.pop(proc.pid, None)
    def state():
        rc = child.poll() if child is not None else None
        out = tail.read()
        return {'status': 'not_started' if child is None else ('running' if rc is None else 'exited'), 'returncode': rc, 'output_bytes': out['total_bytes']}
    def publish(phase, conclusion=None):
        body = {'phase': phase, 'run_id': run, 'native': state(), 'capture_returncode': capture_rc, 'error': failure, 'snapshot': last}
        summary = json.dumps(body, separators=(',', ':'), ensure_ascii=True)
        if len(summary.encode()) > MAX_SUMMARY:
            raise SnapshotTooLarge('encrypted snapshot exceeds Check summary limit')
        req = {'output': {'title': NAME, 'summary': summary}}
        if conclusion:
            req.update(status='completed', conclusion=conclusion)
        bootstrap.check_request('PATCH', api + '/' + str(check_id), req, token, temp, env)
    def capture(baseline=False):
        nonlocal sequence, last, capture_rc
        publish('capturing_baseline' if baseline else 'capturing')
        try:
            raw.unlink(missing_ok=True)
            started = time.time_ns()
            proc = spawn(['/usr/bin/sudo', '-n', '/usr/sbin/spindump', '-noTarget', '1', '100', '-noSymbolicate', '-noBinary', '-o', str(raw)], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            try:
                try:
                    capture_rc = proc.wait(timeout=min(45 if baseline else 20, max(0.1, deadline - time.monotonic())))
                except subprocess.TimeoutExpired:
                    subprocess.run(['/usr/bin/sudo', '-n', '/bin/kill', '-KILL', '--', '-' + str(proc.pid)], env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=5, check=False)
                    proc.wait(timeout=5)
                    capture_rc = -9
            finally:
                finished(proc)
            ended = time.time_ns()
            if capture_rc != 0:
                raise RuntimeError('spindump failed or timed out')
            selected = filter_report(raw.read_text(encoding='utf-8', errors='replace'))
            sequence += 1
            record = {'sequence': sequence, 'started_ns': started, 'ended_ns': ended, 'capture_returncode': capture_rc, 'native': state(), 'native_output': tail.read(), 'report': selected}
            envelope = encrypt_record(record, public, env)
            candidate = {'sequence': sequence, **envelope}
            check = json.dumps({'phase': 'sampled', 'run_id': run, 'native': state(), 'capture_returncode': capture_rc, 'error': failure, 'snapshot': candidate}, separators=(',', ':'), ensure_ascii=True)
            if len(check.encode()) > MAX_SUMMARY:
                raise SnapshotTooLarge('encrypted snapshot exceeds Check summary limit')
            (artifacts / ('snapshot-%04d.json' % sequence)).write_text(json.dumps(candidate, separators=(',', ':')))
            last = candidate
            publish('sampled')
        finally:
            raw.unlink(missing_ok=True)
    def terminate(_signum, _frame):
        stop.set()
        raise SystemExit(1)
    oldterm = signal.signal(signal.SIGTERM, terminate)
    guard = subprocess.Popen([sys.executable, str(Path(__file__).resolve()), '--watchdog', str(os.getpid()), str(deadline)], env=env, stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, close_fds=True, start_new_session=True)
    try:
        created = bootstrap.check_request('POST', api, {'name': NAME, 'head_sha': sha, 'status': 'in_progress', 'output': {'title': NAME, 'summary': json.dumps({'phase': 'initializing', 'run_id': run})}}, token, temp, env)
        check_id = int(created['id'])
        capture(baseline=True)
        if time.monotonic() >= deadline:
            raise TimeoutError('deadline before native start')
        child = spawn([str(native), str(kernel), str(initramfs)], stdout=subprocess.PIPE, stderr=subprocess.STDOUT, bufsize=0)
        reader = threading.Thread(target=tail.drain, args=(child.stdout,), daemon=True)
        reader.start()
        publish('native_started')
        while time.monotonic() < deadline and child.poll() is None and not stop.is_set():
            capture()
            if child.poll() is None:
                stop.wait(min(2, max(0, deadline - time.monotonic())))
        if child.poll() is None:
            raise TimeoutError('native deadline')
        rc = child.wait()
        finished(child)
        reader.join(timeout=2)
        publish('finished', 'success' if rc == 0 else 'failure')
        check_completed = True
        return 0 if rc == 0 else 1
    except Exception as exc:
        failure = type(exc).__name__
        raise
    finally:
        signal.signal(signal.SIGTERM, signal.SIG_IGN)
        for pgid, proc in list(groups.items()):
            if proc.poll() is None:
                try:
                    subprocess.run(['/usr/bin/sudo', '-n', '/bin/kill', '-KILL', '--', '-' + str(pgid)], env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=5, check=False)
                    proc.wait(timeout=5)
                except (OSError, subprocess.TimeoutExpired):
                    pass
        if check_id is not None and not check_completed:
            try:
                publish('failed', 'failure')
            except Exception:
                pass
        guard.terminate()
        try: guard.wait(timeout=5)
        except subprocess.TimeoutExpired: guard.kill(); guard.wait()
        raw.unlink(missing_ok=True)
        root.rmdir()
        signal.signal(signal.SIGTERM, oldterm)

if __name__ == '__main__':
    if len(sys.argv) == 4 and sys.argv[1] == '--watchdog':
        watchdog(int(sys.argv[2]), float(sys.argv[3]))
    else:
        try:
            sys.exit(main())
        except Exception as exc:
            print('kernel-stream failed: ' + type(exc).__name__, file=sys.stderr)
            sys.exit(1)

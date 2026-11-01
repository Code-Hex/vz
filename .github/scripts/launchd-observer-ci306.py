#!/usr/bin/env python3
"""Temporary independent root launchd observer for the CI diagnostic."""
import importlib.util
import json
import os
from pathlib import Path
import plistlib
import re
import secrets
import shutil
import signal
import stat
import subprocess
import sys
import tempfile
import time

HERE = Path(__file__).resolve().parent

def load(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod

common = load('fbt_common', HERE / 'kernel-fbt-stream.py')
bootstrap = load('bootstrap_common', HERE / 'external-bootstrap-observed.py')
ParentClock = common.ParentClock
encrypt_record = common.encrypt_record
clean_environment = common.child_environment
CHECK_NAME = 'ci306-launchd-observer'
PREFIX = 'ci306-launchd-'


RECEIVER_URL_PATTERN = re.compile(r'https://[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.trycloudflare\.com/[0-9a-f]{64}\Z')


def valid_receiver_url(value):
    return isinstance(value, str) and RECEIVER_URL_PATTERN.fullmatch(value) is not None


class ReceiverError(Exception):
    def __init__(self, status):
        self.status = status
        super().__init__('receiver request failed')


def post_receiver(url, body, root, env):
    fd, path = tempfile.mkstemp(prefix='receiver-body-', dir=root)
    try:
        with os.fdopen(fd, 'wb') as stream:
            stream.write(json.dumps(body, separators=(',', ':')).encode())
        config = ('url = "' + url + '"\nsilent\nshow-error\nfail\n').encode('ascii')
        result = subprocess.run(['/usr/bin/curl', '--config', '-', '--request', 'POST', '--header', 'Content-Type: application/json', '--data-binary', '@' + path,
                                 '--connect-timeout', '2', '--max-time', '4', '--output', '/dev/null', '--write-out', '%{http_code}'],
                                input=config, stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=env, timeout=6, check=False)
        status = int(result.stdout) if re.fullmatch(rb'[0-9]{3}', result.stdout) else None
        if status is not None and not 100 <= status <= 599:
            status = None
        if result.returncode != 0 or status is None or not 200 <= status < 300:
            raise ReceiverError(status)
        return status
    finally:
        os.unlink(path)


def channel_failure(exc):
    stderr = getattr(exc, 'stderr', b'') or b''
    if isinstance(stderr, str):
        stderr = stderr.encode('utf-8', 'replace')
    match = re.search(rb'error: ([0-9]{3})', stderr)
    http = getattr(exc, 'status', None)
    if http is None and match:
        http = int(match.group(1))
    return {'outcome': 'error', 'error_type': type(exc).__name__, 'http_status': http}


def process_snapshot(pid, env):
    result = subprocess.run(['/bin/ps', '-p', str(pid), '-o', 'pid=', '-o', 'ppid=', '-o', 'pgid=', '-o', 'sess=', '-o', 'stat=', '-o', 'lstart=', '-o', 'comm='], env=env, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, timeout=3, check=False)
    if result.returncode != 0:
        return {'pid': pid, 'present': False, 'ps_returncode': result.returncode}
    columns = result.stdout.decode('utf-8', 'replace').strip().split(maxsplit=10)
    if len(columns) != 11:
        raise RuntimeError('unexpected ps output')
    return {'pid': int(columns[0]), 'ppid': int(columns[1]), 'pgid': int(columns[2]), 'sess': int(columns[3]),
            'stat': columns[4], 'lstart': ' '.join(columns[5:10]), 'comm': columns[10], 'present': True}


def make_snapshot(sequence, pids, helper_pid, clock):
    env = clean_environment()
    own = process_snapshot(os.getpid(), env)
    observations = [process_snapshot(pid, env) for pid in pids]
    helper = next(item for item in observations if item['pid'] == helper_pid)
    return {'type': 'launchd-observation-v1', 'sequence': sequence, 'clock': clock.read(),
            'observer': own, 'targets': observations, 'controlled_helper_present': helper['present']}


def validate_config(data):
    if data.get('repo') != 'Code-Hex/vz' or data.get('ref') != 'refs/heads/codex/diagnose-ci306-intel':
        raise ValueError('wrong repository or branch')
    if not (bootstrap.SAFE_TOKEN.fullmatch(data.get('token', '')) and bootstrap.SHA.fullmatch(data.get('sha', '')) and bootstrap.RUN_ID.fullmatch(data.get('run_id', ''))):
        raise ValueError('invalid GitHub metadata')
    if data.get('receiver_url') is not None and not valid_receiver_url(data['receiver_url']):
        raise ValueError('invalid receiver URL')
    variant = data.get('diagnostic_variant')
    if variant is not None and (type(variant) is not int or variant not in (0, 1)):
        raise ValueError('invalid diagnostic variant')
    pids = data.get('pids')
    if not isinstance(pids, list) or not 1 <= len(pids) <= 16 or any(type(pid) is not int or pid <= 1 for pid in pids):
        raise ValueError('invalid monitored PIDs')
    if type(data.get('helper_pid')) is not int or data['helper_pid'] not in pids:
        raise ValueError('invalid helper PID')
    if type(data.get('duration')) is not int or not 15 <= data['duration'] <= 300:
        raise ValueError('invalid duration')
    initial = data.get('initial_lstarts', {})
    if not isinstance(initial, dict) or any(key not in {str(pid) for pid in pids} or not isinstance(value, str) or not 1 <= len(value) <= 128 for key, value in initial.items()):
        raise ValueError('invalid initial process identities')


def root_directory(value):
    path = Path(value)
    if path.parent != Path('/private/tmp') or not re.fullmatch(PREFIX + r'[a-zA-Z0-9_]+', path.name):
        raise ValueError('invalid observer directory')
    st = path.lstat()
    if not path.is_dir() or path.is_symlink() or st.st_uid != 0 or st.st_mode & 0o077:
        raise ValueError('observer directory is not root private')
    return path


def install():
    if os.geteuid() != 0:
        raise PermissionError('root required')
    data = json.load(sys.stdin)
    validate_config(data)
    os.umask(0o077)
    root = Path(tempfile.mkdtemp(prefix=PREFIX, dir='/private/tmp'))
    label = 'org.codex.ci306.' + secrets.token_hex(12)
    attempted = False
    try:
        for name in ('launchd-observer-ci306.py', 'kernel-fbt-stream.py', 'external-bootstrap-observed.py', 'observer-public.pem'):
            shutil.copyfile(HERE / name, root / name)
        data['label'] = label
        (root / 'config.json').write_text(json.dumps(data, separators=(',', ':')))
        plist = {'Label': label, 'RunAtLoad': True, 'KeepAlive': False,
                 'ProgramArguments': ['/usr/bin/python3', str(root / 'launchd-observer-ci306.py'), '--daemon', str(root)],
                 'StandardOutPath': '/dev/null', 'StandardErrorPath': '/dev/null'}
        with (root / 'observer.plist').open('wb') as out: plistlib.dump(plist, out)
        attempted = True
        subprocess.run(['/bin/launchctl', 'bootstrap', 'system', str(root / 'observer.plist')], env=clean_environment(), stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=15, check=True)
        print(json.dumps({'directory': str(root), 'label': label}))
    except BaseException:
        if attempted:
            subprocess.run(['/bin/launchctl', 'bootout', 'system/' + label], env=clean_environment(), stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=10, check=False)
        shutil.rmtree(root, ignore_errors=True)
        raise


def daemon(directory):
    if os.geteuid() != 0: raise PermissionError('root required')
    root = root_directory(directory)
    env = clean_environment()
    check_id = None
    completed = False
    interrupted = False
    sequence = 0
    token = None
    data = {}
    receiver_url = None
    clock = None
    last_record = None
    channel_state = {'github': {'outcome': 'not_attempted'}, 'receiver': {'outcome': 'disabled'}}
    label = plistlib.loads((root / 'observer.plist').read_bytes())['Label']
    if not re.fullmatch(r'org\.codex\.ci306\.[a-f0-9]{24}', label):
        raise ValueError('invalid launchd label')
    def terminate(_signum, _frame):
        nonlocal interrupted
        interrupted = True
    signal.signal(signal.SIGTERM, terminate)
    try:
        config = root / 'config.json'
        fd = os.open(config, os.O_RDONLY | os.O_NOFOLLOW)
        with os.fdopen(fd, 'rb') as stream:
            meta = os.fstat(stream.fileno())
            if meta.st_uid != 0 or not stat.S_ISREG(meta.st_mode) or stat.S_IMODE(meta.st_mode) != 0o600:
                raise PermissionError('config is not root-owned mode 0600')
            raw = stream.read(16385)
        if len(raw) > 16384:
            raise ValueError('config too large')
        data = json.loads(raw)
        config.unlink()
        config_security = {'owner_uid': meta.st_uid, 'mode': stat.S_IMODE(meta.st_mode)}
        validate_config(data)
        if data['label'] != label: raise ValueError('launchd label mismatch')
        token = data.pop('token')
        receiver_url = data.pop('receiver_url', None)
        if receiver_url is not None:
            channel_state['receiver'] = {'outcome': 'not_attempted'}
        api = 'https://api.github.com/repos/' + data['repo'] + '/check-runs'
        clock = ParentClock()
        deadline = time.monotonic() + data['duration']
        initial_lstart = data.get('initial_lstarts', {}).get(str(data['helper_pid']))
        was_alive = False
        was_gone = False
        created = bootstrap.check_request('POST', api, {'name': CHECK_NAME, 'head_sha': data['sha'], 'status': 'in_progress', 'output': {'title': CHECK_NAME, 'summary': json.dumps({'phase':'initializing','run_id':data['run_id'],'diagnostic_variant':data.get('diagnostic_variant')})}}, token, root, env)
        check_id = int(created['id'])
        initial_status = {'parent_pid': os.getppid(), 'check_id': check_id, 'sequence': -1,
                          'observed_helper_alive': False, 'observed_helper_gone': False, 'check_created': True}
        temporary = root / 'status.tmp'
        temporary.write_text(json.dumps(initial_status, separators=(',', ':')))
        os.replace(temporary, root / 'status.json')
        next_time = time.monotonic()
        while time.monotonic() < deadline and not interrupted and not (root / 'finish').exists():
            record = make_snapshot(sequence, data['pids'], data['helper_pid'], clock)
            initial_lstarts = data.get('initial_lstarts', {})
            for item in record['targets']:
                expected = initial_lstarts.get(str(item['pid']))
                item['initial_lstart'] = expected
                item['initial_identity_matches'] = None if expected is None else item['present'] and item.get('lstart') == expected
            helper = next(item for item in record['targets'] if item['pid'] == data['helper_pid'])
            if helper['present'] and initial_lstart is None:
                initial_lstart = helper['lstart']
            identity_matches = helper['present'] and helper.get('lstart') == initial_lstart and initial_lstart is not None
            was_alive |= identity_matches
            was_gone |= was_alive and not identity_matches
            record.update(controlled_helper_original_lstart=initial_lstart, controlled_helper_identity_matches=identity_matches,
                          observed_helper_alive=was_alive, observed_helper_gone=was_gone,
                          run_id=data['run_id'], root_euid=os.geteuid(), config_security=config_security,
                          phase='observing', prior_channels=channel_state.copy(),
                          diagnostic_variant=data.get('diagnostic_variant'), observer_instance=label)
            last_record = record
            envelope = {'sequence': sequence, **encrypt_record(record, root / 'observer-public.pem', env)}
            summary = json.dumps({'phase':'observing','run_id':data['run_id'],'diagnostic_variant':data.get('diagnostic_variant'),'snapshot':envelope}, separators=(',', ':'))
            message = json.dumps(envelope, separators=(',', ':'))
            if len(summary.encode()) > 64000 or len(message.encode()) > 64000:
                raise ValueError('encrypted observation exceeds limit')
            output = {'title': CHECK_NAME, 'summary': summary, 'annotations': [{'path': '.github/scripts/launchd-observer-ci306.py', 'start_line':1, 'end_line':1, 'annotation_level':'notice', 'message':message}]}
            receiver_ok = receiver_url is None
            if receiver_url is not None:
                try:
                    code = post_receiver(receiver_url, {'run_id': data['run_id'], 'snapshot': envelope}, root, env)
                    channel_state['receiver'] = {'outcome': 'success', 'http_status': code}
                    receiver_ok = True
                except Exception as exc:
                    channel_state['receiver'] = channel_failure(exc)
            check_ok = False
            try:
                bootstrap.check_request('PATCH', api + '/' + str(check_id), {'output':output}, token, root, env)
                channel_state['github'] = {'outcome': 'success'}
                check_ok = True
            except Exception as exc:
                channel_state['github'] = channel_failure(exc)
            status = {'parent_pid': own_parent(record), 'observer_pid':record['observer']['pid'], 'observer_pgid':record['observer']['pgid'], 'observer_euid':os.geteuid(), 'check_id': check_id, 'sequence': sequence,
                      'observed_helper_alive': was_alive, 'observed_helper_gone': was_gone, 'check_created': True}
            if check_ok and receiver_ok:
                temporary = root / 'status.tmp'
                temporary.write_text(json.dumps(status, separators=(',', ':')))
                os.replace(temporary, root / 'status.json')
            sequence += 1
            next_time += 3
            while time.monotonic() < min(next_time, deadline) and not interrupted and not (root / 'finish').exists():
                time.sleep(min(0.2, max(0, min(next_time, deadline) - time.monotonic())))
        completed = not interrupted and sequence > 0
    finally:
        if receiver_url is not None and clock is not None:
            try:
                final_record = dict(last_record) if last_record is not None else {'type': 'launchd-observation-v1', 'run_id': data['run_id'], 'diagnostic_variant': data.get('diagnostic_variant'), 'observer_instance': label}
                final_record.update(sequence=sequence, phase='finished',
                                    outcome='completed' if completed else ('interrupted' if interrupted else 'error'),
                                    clock=clock.read(), prior_channels=channel_state.copy())
                final_envelope = {'sequence': sequence, **encrypt_record(final_record, root / 'observer-public.pem', env)}
                post_receiver(receiver_url, {'run_id': data['run_id'], 'snapshot': final_envelope}, root, env)
            except Exception:
                pass
        if check_id is not None:
            try:
                bootstrap.check_request('PATCH', api + '/' + str(check_id), {'status':'completed','conclusion':'success' if completed else 'failure','output':{'title':CHECK_NAME,'summary':json.dumps({'phase':'finished' if completed else 'interrupted','run_id':data['run_id'],'diagnostic_variant':data.get('diagnostic_variant'),'observations':sequence})}}, token, root, env)
            except Exception:
                pass
        shutil.rmtree(root, ignore_errors=True)
        # Removal precedes bootout because launchd may terminate this process immediately.
        subprocess.run(['/bin/launchctl', 'bootout', 'system/' + label], env=env, stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=10, check=False)


def own_parent(record):
    return record['observer']['ppid']


def status(directory):
    root = root_directory(directory)
    print((root / 'status.json').read_text())


def finish(directory, label):
    if not re.fullmatch(r'org\.codex\.ci306\.[a-f0-9]{24}', label): raise ValueError('invalid label')
    root = root_directory(directory)
    if plistlib.loads((root / 'observer.plist').read_bytes()).get('Label') != label:
        raise ValueError('label does not belong to observer')
    fd = os.open(root / 'finish', os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
    os.close(fd)


def cleanup(directory, label):
    if not re.fullmatch(r'org\.codex\.ci306\.[a-f0-9]{24}', label): raise ValueError('invalid label')
    root = root_directory(directory)
    if plistlib.loads((root / 'observer.plist').read_bytes()).get('Label') != label:
        raise ValueError('label does not belong to observer')
    subprocess.run(['/bin/launchctl','kill','SIGTERM','system/' + label], env=clean_environment(), stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=8, check=False)
    for _ in range(50):
        if not root.exists(): return
        time.sleep(0.2)
    subprocess.run(['/bin/launchctl','bootout','system/' + label], env=clean_environment(), stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=8, check=False)
    shutil.rmtree(root, ignore_errors=True)

if __name__ == '__main__':
    try:
        if sys.argv[1:] == ['--install']: install()
        elif len(sys.argv) == 3 and sys.argv[1] == '--daemon': daemon(sys.argv[2])
        elif len(sys.argv) == 3 and sys.argv[1] == '--status': status(sys.argv[2])
        elif len(sys.argv) == 4 and sys.argv[1] == '--finish': finish(sys.argv[2], sys.argv[3])
        elif len(sys.argv) == 4 and sys.argv[1] == '--cleanup': cleanup(sys.argv[2], sys.argv[3])
        else: raise ValueError('invalid arguments')
    except Exception as exc:
        print('launchd observer failed: ' + type(exc).__name__, file=sys.stderr)
        sys.exit(1)

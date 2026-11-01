#!/usr/bin/env python3
"""Independent launchd heartbeat with no process creation in the send loop."""
import base64
import http.client
import hmac
import importlib.util
import json
import os
from pathlib import Path
import plistlib
import re
import secrets
import shutil
import signal
import socket
import ssl
import stat
import subprocess
import sys
import tempfile
import time

HERE = Path(__file__).resolve().parent

def load(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module

observer = load('observer', HERE / 'launchd-observer-ci306.py')
common = observer.common
bootstrap = observer.bootstrap
PREFIX = 'ci306-forkless-'
FORMAT = 'forkless-hmac-json-v1'
DOMAIN = b'ci306-forkless-heartbeat-v1\x00'


def envelope(record, session_key):
    raw = json.dumps(record, sort_keys=True, separators=(',', ':'), ensure_ascii=True).encode('ascii')
    return {'sequence': record['sequence'], 'format': FORMAT, 'key': '', 'data': base64.b64encode(raw).decode('ascii'),
            'hmac_sha256': hmac.digest(session_key, DOMAIN + raw, 'sha256').hex()}


def verify(item, session_key, run_id, instance):
    try:
        if item['format'] != FORMAT or item['key'] != '' or type(item['sequence']) is not int or item['sequence'] < 0 or len(session_key) != 32:
            return False
        raw = base64.b64decode(item['data'], validate=True)
        if not hmac.compare_digest(hmac.digest(session_key, DOMAIN + raw, 'sha256').hex(), item['hmac_sha256']):
            return False
        data = json.loads(raw)
        return (data['type'] == 'forkless-heartbeat-v1' and data['format'] == FORMAT and data['phase'] in ('beat', 'finished')
                and data['run_id'] == run_id and data['instance'] == instance
                and type(data['sequence']) is int and data['sequence'] == item['sequence'])
    except (ValueError, KeyError, TypeError):
        return False


def validate(data):
    if data.get('repo') != 'Code-Hex/vz' or data.get('ref') != 'refs/heads/codex/diagnose-ci306-intel':
        raise ValueError('wrong repository or branch')
    if not (bootstrap.SAFE_TOKEN.fullmatch(data.get('token','')) and bootstrap.SHA.fullmatch(data.get('sha','')) and bootstrap.RUN_ID.fullmatch(data.get('run_id',''))):
        raise ValueError('invalid metadata')
    if not observer.valid_receiver_url(data.get('receiver_url')):
        raise ValueError('invalid receiver')
    if type(data.get('duration')) is not int or not 15 <= data['duration'] <= 300:
        raise ValueError('invalid duration')


def root_directory(value):
    path = Path(value)
    if path.parent != Path('/private/tmp') or not re.fullmatch(PREFIX + r'[a-zA-Z0-9_]+', path.name):
        raise ValueError('invalid directory')
    meta = path.lstat()
    if not path.is_dir() or path.is_symlink() or meta.st_uid != 0 or meta.st_mode & 0o077:
        raise ValueError('directory not root private')
    return path


def install():
    if os.geteuid() != 0: raise PermissionError('root required')
    data = json.load(sys.stdin)
    validate(data)
    os.umask(0o077)
    root = Path(tempfile.mkdtemp(prefix=PREFIX, dir='/private/tmp'))
    label = 'org.codex.ci306.' + secrets.token_hex(12)
    attempted = False
    try:
        for name in ('forkless-hb-ci306.py','launchd-observer-ci306.py','kernel-fbt-stream.py','external-bootstrap-observed.py','observer-public.pem'):
            shutil.copyfile(HERE / name, root / name)
        data['label'] = label
        (root / 'config.json').write_text(json.dumps(data, separators=(',', ':')))
        plist = {'Label':label, 'RunAtLoad':True, 'KeepAlive':False,
                 'ProgramArguments':['/usr/bin/python3', str(root / 'forkless-hb-ci306.py'), '--daemon', str(root)],
                 'StandardOutPath':'/dev/null','StandardErrorPath':'/dev/null'}
        with (root / 'observer.plist').open('wb') as stream: plistlib.dump(plist, stream)
        attempted = True
        subprocess.run(['/bin/launchctl','bootstrap','system',str(root / 'observer.plist')], env=observer.clean_environment(), stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=15, check=True)
        print(json.dumps({'directory':str(root),'label':label}))
    except BaseException:
        if attempted:
            subprocess.run(['/bin/launchctl','bootout','system/' + label], env=observer.clean_environment(), stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=10, check=False)
        shutil.rmtree(root, ignore_errors=True)
        raise


class PinnedHTTPSConnection(http.client.HTTPSConnection):
    def __init__(self, host, address, context, timeout=4):
        super().__init__(host, 443, timeout=timeout, context=context)
        self.address = address
    def connect(self):
        family, socktype, proto, _, sockaddr = self.address
        sock = socket.socket(family, socktype, proto)
        try:
            sock.settimeout(self.timeout)
            sock.connect(sockaddr)
            self.sock = self._context.wrap_socket(sock, server_hostname=self.host)
        except BaseException:
            sock.close()
            raise


def send_once(host, path, addresses, context, run_id, item, timeout=4):
    content = json.dumps({'run_id':run_id,'snapshot':item}, separators=(',', ':')).encode('ascii')
    conn = PinnedHTTPSConnection(host, addresses[item['sequence'] % len(addresses)], context, timeout)
    try:
        conn.request('POST', path, body=content, headers={'Content-Type':'application/json','Connection':'close'})
        response = conn.getresponse()
        data = response.read(4097)
        if len(data) > 4096 or not 200 <= response.status < 300:
            raise RuntimeError('receiver rejected heartbeat')
        return response.status
    finally:
        conn.close()


def daemon(directory):
    os.environ.pop('GITHUB_TOKEN', None)
    os.environ.pop('CI306_OBSERVER_RECEIVER', None)
    if os.geteuid() != 0: raise PermissionError('root required')
    root = root_directory(directory)
    label = plistlib.loads((root/'observer.plist').read_bytes())['Label']
    if not re.fullmatch(r'org\.codex\.ci306\.[a-f0-9]{24}', label): raise ValueError('invalid label')
    env = observer.clean_environment()
    state = {'hot':False, 'stop':False}
    def terminate(_signum, _frame): state['stop'] = True
    def alarm(_signum, _frame): raise TimeoutError('heartbeat request timed out')
    def audit(event, _args):
        if state['hot'] and event in ('subprocess.Popen','os.fork','os.forkpty','os.posix_spawn','os.system','_posixsubprocess.fork_exec'):
            raise RuntimeError('process creation forbidden in heartbeat loop')
    signal.signal(signal.SIGTERM, terminate)
    signal.signal(signal.SIGALRM, alarm)
    instance = secrets.token_hex(16)
    key = secrets.token_bytes(32)
    clock = None
    session = None
    try:
        config = root/'config.json'
        fd = os.open(config, os.O_RDONLY | os.O_NOFOLLOW)
        with os.fdopen(fd, 'rb') as stream:
            meta = os.fstat(stream.fileno())
            if meta.st_uid != 0 or not stat.S_ISREG(meta.st_mode) or stat.S_IMODE(meta.st_mode) != 0o600: raise PermissionError('insecure config')
            raw = stream.read(16385)
        if len(raw) > 16384: raise ValueError('config too large')
        data = json.loads(raw)
        del raw
        config.unlink()
        validate(data)
        if data['label'] != label: raise ValueError('label mismatch')
        if os.getppid() != 1: raise RuntimeError('heartbeat is not launchd child')
        token = data.pop('token')
        url = data.pop('receiver_url')
        host, path = url[8:].split('/',1)
        path = '/' + path
        deadline = time.monotonic() + data['duration']
        context = ssl.create_default_context()
        signal.setitimer(signal.ITIMER_REAL, 6)
        try:
            addresses = socket.getaddrinfo(host, 443, type=socket.SOCK_STREAM)
        finally:
            signal.setitimer(signal.ITIMER_REAL, 0)
        if not addresses: raise RuntimeError('receiver DNS unavailable')
        clock = common.ParentClock()
        startup = {'euid':os.geteuid(), 'pid':os.getpid(), 'ppid':os.getppid(), 'pgid':os.getpgrp()}
        session = {'type':'forkless-registration-v1', 'run_id':data['run_id'], 'instance':instance, 'key_hex':key.hex(), 'startup':startup}
        sealed = common.encrypt_record(session, root/'observer-public.pem', env)
        summary = json.dumps({'format':'forkless-registration-v1','run_id':data['run_id'],'instance':instance,'snapshot':sealed}, separators=(',', ':'))
        if len(summary.encode()) > 64000: raise ValueError('registration too large')
        api = 'https://api.github.com/repos/' + data['repo'] + '/check-runs'
        created = bootstrap.check_request('POST', api, {'name':'ci306-forkless-registration','head_sha':data['sha'],'status':'completed','conclusion':'success','output':{'title':'ci306-forkless-registration','summary':summary}}, token, root, env)
        check_id = int(created['id'])
        del token, session, sealed, created, summary, api, url
        sys.addaudithook(audit)
        state['hot'] = True
        sequence = 0
        next_time = time.monotonic()
        status_written = False
        last_send = {'outcome':'not_attempted'}
        while time.monotonic() < deadline and not state['stop']:
            record = {'type':'forkless-heartbeat-v1','format':FORMAT,'run_id':data['run_id'],'instance':instance,'sequence':sequence,
                      'phase':'beat','clock':clock.read(),'startup':startup,'prior_send':last_send}
            item = envelope(record, key)
            try:
                signal.setitimer(signal.ITIMER_REAL, 6)
                code = send_once(host, path, addresses, context, data['run_id'], item)
                last_send = {'outcome':'success','http_status':code}
                if not status_written:
                    (root/'status.json').write_text(json.dumps({'parent_pid':startup['ppid'],'check_id':check_id,'instance':instance,'first_post':True}))
                    status_written = True
            except Exception as exc:
                last_send = {'outcome':'error','error_type':type(exc).__name__}
            finally:
                signal.setitimer(signal.ITIMER_REAL, 0)
            sequence += 1
            next_time += 3
            while time.monotonic() < min(next_time, deadline) and not state['stop']:
                time.sleep(min(0.2, max(0, min(next_time, deadline)-time.monotonic())))
        end = {'type':'forkless-heartbeat-v1','format':FORMAT,'run_id':data['run_id'],'instance':instance,'sequence':sequence,
               'phase':'finished','clock':clock.read(),'startup':startup,'prior_send':last_send}
        try:
            signal.setitimer(signal.ITIMER_REAL, 6)
            send_once(host, path, addresses, context, data['run_id'], envelope(end,key))
        except Exception:
            pass
        finally:
            signal.setitimer(signal.ITIMER_REAL, 0)
    finally:
        state['hot'] = False
        shutil.rmtree(root, ignore_errors=True)
        subprocess.run(['/bin/launchctl','bootout','system/'+label], env=env, stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=10, check=False)


def status(directory):
    print((root_directory(directory)/'status.json').read_text())


def cleanup(directory, label):
    if not re.fullmatch(r'org\.codex\.ci306\.[a-f0-9]{24}', label): raise ValueError('invalid label')
    root = root_directory(directory)
    if plistlib.loads((root/'observer.plist').read_bytes()).get('Label') != label: raise ValueError('label mismatch')
    env = observer.clean_environment()
    subprocess.run(['/bin/launchctl','kill','SIGTERM','system/'+label], env=env, stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=8, check=False)
    for _ in range(60):
        if not root.exists(): return
        time.sleep(0.2)
    subprocess.run(['/bin/launchctl','bootout','system/'+label], env=env, stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=8, check=False)
    shutil.rmtree(root, ignore_errors=True)

if __name__ == '__main__':
    try:
        if sys.argv[1:] == ['--install']: install()
        elif len(sys.argv)==3 and sys.argv[1]=='--daemon': daemon(sys.argv[2])
        elif len(sys.argv)==3 and sys.argv[1]=='--status': status(sys.argv[2])
        elif len(sys.argv)==4 and sys.argv[1]=='--cleanup': cleanup(sys.argv[2],sys.argv[3])
        else: raise ValueError('invalid arguments')
    except Exception as exc:
        print('forkless heartbeat failed: '+type(exc).__name__, file=sys.stderr)
        sys.exit(1)

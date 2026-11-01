#!/usr/bin/env python3
"""Prove a root LaunchDaemon survives one controlled helper's exit, without VMs."""
import importlib.util
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import time

HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location('observer', HERE / 'launchd-observer-ci306.py')
observer = importlib.util.module_from_spec(spec)
spec.loader.exec_module(observer)


def get_check(url, token, env):
    config = ('silent\nshow-error\nfail\nheader = "Accept: application/vnd.github+json"\n'
              'header = "X-GitHub-Api-Version: 2022-11-28"\n'
              'header = "Authorization: Bearer ' + token + '"\n').encode('ascii')
    result = subprocess.run(['/usr/bin/curl', '--config', '-', '--connect-timeout', '3', '--max-time', '6', url],
                            input=config, env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=8, check=True)
    return json.loads(result.stdout)


def verify_cleanup(directory, label, env, timeout=15):
    if not re.fullmatch(r'org\.codex\.ci306\.[a-f0-9]{24}', label):
        raise ValueError('invalid launchd label')
    limited_env = dict(env)
    limited_env['LC_ALL'] = 'C'
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        directory_status = subprocess.run(['/usr/bin/sudo', '-n', '/bin/test', '-e', directory], env=limited_env,
                                          stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.PIPE, timeout=5, check=False)
        service = subprocess.run(['/usr/bin/sudo', '-n', '/bin/launchctl', 'print', 'system/' + label], env=limited_env,
                                 stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=5, check=False)
        if directory_status.returncode not in (0, 1) or (directory_status.returncode == 1 and directory_status.stderr):
            raise RuntimeError('observer directory check failed')
        service_missing = service.returncode != 0 and b'Could not find service' in service.stderr
        if directory_status.returncode == 1 and service_missing:
            return
        time.sleep(0.25)
    raise RuntimeError('observer directory or launchd service remains')


def main():
    token = os.environ.pop('GITHUB_TOKEN')
    repo = os.environ['GITHUB_REPOSITORY']
    ref = os.environ['GITHUB_REF']
    sha = os.environ['GITHUB_SHA']
    run_id = os.environ['GITHUB_RUN_ID']
    env = observer.clean_environment()
    helper = subprocess.Popen(['/usr/bin/python3', '-c', 'import time; time.sleep(60)'], env=env, stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, start_new_session=True)
    directory = label = check_id = None
    try:
        parent = observer.process_snapshot(os.getppid(), env)
        pids = list(dict.fromkeys(pid for pid in (helper.pid, os.getpid(), os.getppid(), parent.get('ppid')) if isinstance(pid, int) and pid > 1))
        data = {'repo':repo, 'ref':ref, 'sha':sha, 'run_id':run_id, 'token':token, 'pids':pids, 'helper_pid':helper.pid, 'duration':30}
        observer.validate_config(data)
        installed = subprocess.run(['/usr/bin/sudo','-n','/usr/bin/python3',str(HERE / 'launchd-observer-ci306.py'),'--install'], input=json.dumps(data, separators=(',', ':')).encode(), env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=25, check=True)
        del data['token']
        result = json.loads(installed.stdout)
        directory, label = result['directory'], result['label']
        deadline = time.monotonic() + 20
        initial = None
        while time.monotonic() < deadline:
            value = subprocess.run(['/usr/bin/sudo','-n','/usr/bin/python3',str(HERE / 'launchd-observer-ci306.py'),'--status',directory], env=env, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, timeout=5, check=False)
            if value.returncode == 0:
                initial = json.loads(value.stdout)
                if initial['parent_pid'] == 1 and initial['observed_helper_alive']:
                    break
            time.sleep(0.5)
        if initial is None or initial['parent_pid'] != 1 or not initial['observed_helper_alive']:
            raise RuntimeError('daemon ancestry or helper not observed')
        check_id = int(initial['check_id'])
        helper.terminate()
        helper.wait(timeout=5)
        gone = None
        deadline = time.monotonic() + 15
        while time.monotonic() < deadline:
            value = subprocess.run(['/usr/bin/sudo','-n','/usr/bin/python3',str(HERE / 'launchd-observer-ci306.py'),'--status',directory], env=env, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, timeout=5, check=False)
            if value.returncode == 0:
                gone = json.loads(value.stdout)
                if gone['parent_pid'] == 1 and gone['observed_helper_gone'] and gone['sequence'] > initial['sequence']:
                    break
            time.sleep(0.5)
        if gone is None or gone['parent_pid'] != 1 or not gone['observed_helper_gone'] or gone['sequence'] <= initial['sequence']:
            raise RuntimeError('daemon did not observe helper exit')
        api = 'https://api.github.com/repos/' + repo + '/check-runs/' + str(check_id)
        deadline = time.monotonic() + 45
        while time.monotonic() < deadline:
            result = get_check(api, token, env)
            if result.get('status') == 'completed':
                if result.get('conclusion') != 'success': raise RuntimeError('observer Check failed')
                verify_cleanup(directory, label, env)
                print(json.dumps({'status':'passed','parent_pid':1,'check_id':check_id,'alive_sequence':initial['sequence'],'gone_sequence':gone['sequence']}))
                return
            time.sleep(1)
        raise TimeoutError('observer Check did not finish')
    finally:
        if helper.poll() is None:
            helper.terminate()
            try: helper.wait(timeout=5)
            except subprocess.TimeoutExpired: helper.kill(); helper.wait()
        if directory and label:
            subprocess.run(['/usr/bin/sudo','-n','/usr/bin/python3',str(HERE / 'launchd-observer-ci306.py'),'--cleanup',directory,label], env=env, stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=25, check=False)

if __name__ == '__main__':
    try: main()
    except Exception as exc:
        print('launchd preflight failed: ' + type(exc).__name__, file=sys.stderr)
        sys.exit(1)

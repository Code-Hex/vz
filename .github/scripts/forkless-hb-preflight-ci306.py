#!/usr/bin/env python3
"""Start a forkless heartbeat while a controlled helper is blocked on a pipe."""
import importlib.util
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import time

HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location('hb', HERE/'forkless-hb-ci306.py')
hb = importlib.util.module_from_spec(spec)
spec.loader.exec_module(hb)
preflight_spec = importlib.util.spec_from_file_location('preflight', HERE/'launchd-preflight-ci306.py')
preflight = importlib.util.module_from_spec(preflight_spec)
preflight_spec.loader.exec_module(preflight)


def main():
    token = os.environ.pop('GITHUB_TOKEN')
    receiver = os.environ.pop('CI306_OBSERVER_RECEIVER', None)
    data = {'repo':os.environ['GITHUB_REPOSITORY'], 'ref':os.environ['GITHUB_REF'], 'sha':os.environ['GITHUB_SHA'],
            'run_id':os.environ['GITHUB_RUN_ID'], 'token':token, 'receiver_url':receiver, 'duration':30}
    hb.validate(data)
    env = hb.observer.clean_environment()
    target = subprocess.Popen(['/usr/bin/python3', '-c', 'import time;time.sleep(60)'], env=env,
                              stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, start_new_session=True)
    helper = None
    directory = label = None
    old_status = None
    try:
        helper = subprocess.Popen(['/usr/bin/sample', str(target.pid), '30', '100', '-mayDie', '-file', '/dev/null'],
                                  env=env, stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, start_new_session=True)
        os.kill(helper.pid, signal.SIGSTOP)
        helper_state = hb.observer.process_snapshot(helper.pid, env)
        if not helper_state['present'] or not helper_state['stat'].startswith('T'):
            raise RuntimeError('controlled sample helper did not stop')
        installed = subprocess.run(['/usr/bin/sudo','-n','/usr/bin/python3',str(HERE/'forkless-hb-ci306.py'),'--install'],
                                   input=json.dumps(data, separators=(',', ':')).encode(), env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=25, check=True)
        del data['token'], data['receiver_url'], token, receiver
        result = json.loads(installed.stdout)
        directory, label = result['directory'], result['label']
        deadline = time.monotonic() + 20
        while time.monotonic() < deadline:
            response = subprocess.run(['/usr/bin/sudo','-n','/usr/bin/python3',str(HERE/'forkless-hb-ci306.py'),'--status',directory], env=env,
                                      stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, timeout=5, check=False)
            if response.returncode == 0:
                old_status = json.loads(response.stdout)
                if old_status['parent_pid'] == 1 and old_status['first_post']:
                    break
            time.sleep(0.5)
        if old_status is None or old_status['parent_pid'] != 1 or not old_status['first_post']:
            raise RuntimeError('initial heartbeat not observed')
        wait_start_ns = time.time_ns()
        until = time.monotonic() + 10
        while time.monotonic() < until:
            if helper.poll() is not None: raise RuntimeError('controlled helper exited while blocked')
            time.sleep(min(0.2, max(0, until-time.monotonic())))
        helper_state = hb.observer.process_snapshot(helper.pid, env)
        if not helper_state['present'] or not helper_state['stat'].startswith('T'):
            raise RuntimeError('controlled sample helper did not remain stopped')
        release_ns = time.time_ns()
        os.kill(helper.pid, signal.SIGCONT)
        helper.terminate()
        helper.wait(timeout=5)
        print(json.dumps({'status':'local_preflight_passed','external_hmac_verification':'required',
                          'registration_check_id':old_status['check_id'],'instance':old_status['instance'],
                          'helper_block_start_wall_ns':wait_start_ns,'helper_release_wall_ns':release_ns}))
    finally:
        if helper is not None and helper.poll() is None:
            try: os.kill(helper.pid, signal.SIGCONT)
            except ProcessLookupError: pass
            helper.terminate()
            try: helper.wait(timeout=5)
            except subprocess.TimeoutExpired: helper.kill(); helper.wait()
        if target.poll() is None:
            target.terminate()
            try: target.wait(timeout=5)
            except subprocess.TimeoutExpired: target.kill(); target.wait()
        if directory and label:
            subprocess.run(['/usr/bin/sudo','-n','/usr/bin/python3',str(HERE/'forkless-hb-ci306.py'),'--cleanup',directory,label], env=env,
                           stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=25, check=False)
            preflight.verify_cleanup(directory,label,env)

if __name__ == '__main__':
    try: main()
    except Exception as exc:
        print('forkless preflight failed: '+type(exc).__name__, file=sys.stderr)
        sys.exit(1)

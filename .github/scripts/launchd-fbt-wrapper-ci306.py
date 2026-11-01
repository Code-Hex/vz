#!/usr/bin/env python3
"""Gate the runner-owned FBT controller on an independent launchd observer."""
import importlib.util
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import time

HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location('preflight', HERE / 'launchd-preflight-ci306.py')
preflight = importlib.util.module_from_spec(spec)
spec.loader.exec_module(preflight)
observer = preflight.observer


def ancestry(pid, env):
    snapshots = []
    visited = set()
    while pid > 1 and pid not in visited and len(snapshots) < 16:
        visited.add(pid)
        item = observer.process_snapshot(pid, env)
        snapshots.append(item)
        if not item['present']:
            break
        pid = item['ppid']
    return snapshots


def status(directory, env):
    result = subprocess.run(['/usr/bin/sudo', '-n', '/usr/bin/python3', str(HERE / 'launchd-observer-ci306.py'), '--status', directory], env=env, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, timeout=5, check=False)
    return json.loads(result.stdout) if result.returncode == 0 else None


def main():
    token = os.environ.pop('GITHUB_TOKEN')
    repo, ref, sha, run_id = (os.environ[name] for name in ('GITHUB_REPOSITORY', 'GITHUB_REF', 'GITHUB_SHA', 'GITHUB_RUN_ID'))
    env = observer.clean_environment()
    for name in ('GITHUB_REPOSITORY', 'GITHUB_REF', 'GITHUB_SHA', 'GITHUB_RUN_ID', 'RUNNER_TEMP'):
        env[name] = os.environ[name]
    controller = subprocess.Popen(['/usr/bin/python3', str(HERE / 'launchd-fbt-gate-ci306.py')], env=env, stdin=subprocess.PIPE, start_new_session=True)
    directory = label = None
    activated = orderly = False
    oldterm = signal.getsignal(signal.SIGTERM)
    def terminate(_signum, _frame):
        raise SystemExit(1)
    signal.signal(signal.SIGTERM, terminate)
    try:
        initial = ancestry(controller.pid, env)
        if not initial or not initial[0]['present'] or initial[0]['ppid'] != os.getpid():
            raise RuntimeError('gated controller ancestry unavailable')
        pids = [item['pid'] for item in initial]
        starts = {str(item['pid']): item['lstart'] for item in initial if item['present']}
        data = {'repo': repo, 'ref': ref, 'sha': sha, 'run_id': run_id, 'token': token, 'pids': pids,
                'initial_lstarts': starts, 'helper_pid': controller.pid, 'duration': 260}
        observer.validate_config(data)
        installed = subprocess.run(['/usr/bin/sudo', '-n', '/usr/bin/python3', str(HERE / 'launchd-observer-ci306.py'), '--install'],
                                   input=json.dumps(data, separators=(',', ':')).encode(), env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=25, check=True)
        del data['token']
        result = json.loads(installed.stdout)
        directory, label = result['directory'], result['label']
        deadline = time.monotonic() + 25
        first = None
        while time.monotonic() < deadline:
            if controller.poll() is not None:
                raise RuntimeError('gated controller exited before observer readiness')
            first = status(directory, env)
            if first and first['parent_pid'] == 1 and first['observed_helper_alive']:
                break
            time.sleep(0.5)
        if not first or first['parent_pid'] != 1 or not first['observed_helper_alive']:
            raise RuntimeError('independent observer not ready')
        activated = True
        controller.stdin.write(json.dumps({'token': token}, separators=(',', ':')).encode() + b'\n')
        controller.stdin.close()
        code = controller.wait()
        deadline = time.monotonic() + 15
        after = None
        while time.monotonic() < deadline:
            after = status(directory, env)
            if after and after['parent_pid'] == 1 and after['observed_helper_gone'] and after['sequence'] > first['sequence']:
                break
            time.sleep(0.5)
        if not after or after['parent_pid'] != 1 or not after['observed_helper_gone'] or after['sequence'] <= first['sequence']:
            raise RuntimeError('observer did not record controller exit')
        orderly = True
        return code
    finally:
        signal.signal(signal.SIGTERM, signal.SIG_IGN)
        if controller.poll() is None:
            try:
                os.killpg(controller.pid, signal.SIGTERM)
                controller.wait(timeout=15)
            except (ProcessLookupError, subprocess.TimeoutExpired):
                if controller.poll() is None:
                    try: os.killpg(controller.pid, signal.SIGKILL)
                    except ProcessLookupError: pass
                    controller.wait(timeout=5)
        if directory and label and (orderly or not activated):
            action = '--finish' if orderly else '--cleanup'
            result = subprocess.run(['/usr/bin/sudo', '-n', '/usr/bin/python3', str(HERE / 'launchd-observer-ci306.py'), action, directory, label], env=env, stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=25, check=False)
            preflight.verify_cleanup(directory, label, env)
            if orderly and result.returncode != 0:
                raise RuntimeError('observer finish request failed')
        signal.signal(signal.SIGTERM, oldterm)

if __name__ == '__main__':
    try: sys.exit(main())
    except Exception as exc:
        print('launchd FBT wrapper failed: ' + type(exc).__name__, file=sys.stderr)
        sys.exit(1)

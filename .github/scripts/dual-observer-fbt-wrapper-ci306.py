#!/usr/bin/env python3
"""Gate the unchanged runner-owned Go controller on two independent observers."""
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


def status(script, directory, env):
    result = subprocess.run(['/usr/bin/sudo', '-n', '/usr/bin/python3', str(HERE / script), '--status', directory],
                            env=env, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, timeout=5, check=False)
    return json.loads(result.stdout) if result.returncode == 0 else None


def start_service(script, data, env):
    result = subprocess.run(['/usr/bin/sudo', '-n', '/usr/bin/python3', str(HERE / script), '--install'],
                            input=json.dumps(data, separators=(',', ':')).encode(), env=env,
                            stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=25, check=True)
    value = json.loads(result.stdout)
    return {'script': script, 'directory': value['directory'], 'label': value['label']}


def distinct_services(ps, hb, controller_pid, wrapper_pgid):
    if not ps or not hb or ps.get('parent_pid') != 1 or hb.get('parent_pid') != 1:
        return False
    if ps.get('observer_euid') != 0 or hb.get('observer_euid') != 0:
        return False
    pids = [ps.get('observer_pid'), hb.get('observer_pid')]
    pgids = [ps.get('observer_pgid'), hb.get('observer_pgid')]
    if any(type(x) is not int or x <= 1 for x in pids + pgids):
        return False
    if len(set(pids)) != 2 or len(set(pgids)) != 2:
        return False
    return all(p != controller_pid for p in pids) and all(g not in (controller_pid, wrapper_pgid) for g in pgids)


def main():
    token = os.environ.pop('GITHUB_TOKEN')
    receiver = os.environ.pop('CI306_OBSERVER_RECEIVER', None) or None
    if receiver is None:
        raise ValueError('independent receiver is required')
    repo, ref, sha, run_id = (os.environ[name] for name in ('GITHUB_REPOSITORY', 'GITHUB_REF', 'GITHUB_SHA', 'GITHUB_RUN_ID'))
    env = observer.clean_environment()
    for name in ('GITHUB_REPOSITORY', 'GITHUB_REF', 'GITHUB_SHA', 'GITHUB_RUN_ID', 'RUNNER_TEMP'):
        env[name] = os.environ[name]
    controller = subprocess.Popen(['/usr/bin/python3', str(HERE / 'launchd-fbt-gate-ci306.py')], env=env, stdin=subprocess.PIPE, start_new_session=True)
    services = {}
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
        base = {'repo':repo, 'ref':ref, 'sha':sha, 'run_id':run_id, 'token':token, 'receiver_url':receiver, 'duration':300}
        ps_data = {**base, 'pids':pids, 'initial_lstarts':starts, 'helper_pid':controller.pid}
        observer.validate_config(ps_data)
        startup_deadline = time.monotonic() + 45
        services['hb'] = start_service('forkless-hb-ci306.py', base, env)
        services['ps'] = start_service('launchd-observer-ci306.py', ps_data, env)
        del base['token'], base['receiver_url'], ps_data['token'], ps_data['receiver_url']
        first = heartbeat = None
        while time.monotonic() < startup_deadline:
            if controller.poll() is not None:
                raise RuntimeError('gated controller exited before observer readiness')
            first = status(services['ps']['script'], services['ps']['directory'], env)
            heartbeat = status(services['hb']['script'], services['hb']['directory'], env)
            if first and heartbeat and first.get('observed_helper_alive') and heartbeat.get('first_post') and distinct_services(first, heartbeat, controller.pid, os.getpgrp()):
                break
            time.sleep(0.5)
        if time.monotonic() >= startup_deadline or not first or not heartbeat or not first.get('observed_helper_alive') or not heartbeat.get('first_post') or not distinct_services(first, heartbeat, controller.pid, os.getpgrp()):
            raise RuntimeError('independent observers not ready')
        controller.stdin.write(json.dumps({'token':token}, separators=(',', ':')).encode() + b'\n')
        controller.stdin.close()
        code = controller.wait()
        deadline = time.monotonic() + 15
        after = None
        while time.monotonic() < deadline:
            after = status(services['ps']['script'], services['ps']['directory'], env)
            if after and after['parent_pid'] == 1 and after['observed_helper_gone'] and after['sequence'] > first['sequence']:
                break
            time.sleep(0.5)
        if not after or after['parent_pid'] != 1 or not after['observed_helper_gone'] or after['sequence'] <= first['sequence']:
            raise RuntimeError('observer did not record controller exit')
        ps = services['ps']
        result = subprocess.run(['/usr/bin/sudo', '-n', '/usr/bin/python3', str(HERE / ps['script']), '--finish', ps['directory'], ps['label']],
                                env=env, stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=25, check=False)
        preflight.verify_cleanup(ps['directory'], ps['label'], env)
        if result.returncode != 0:
            raise RuntimeError('process observer finish request failed')
        api = 'https://api.github.com/repos/' + repo + '/check-runs/' + str(int(first['check_id']))
        check = preflight.get_check(api, token, env)
        if check.get('status') != 'completed' or check.get('conclusion') != 'success':
            raise RuntimeError('process observer Check did not complete successfully')
        hb = services['hb']
        result = subprocess.run(['/usr/bin/sudo', '-n', '/usr/bin/python3', str(HERE / hb['script']), '--cleanup', hb['directory'], hb['label']],
                                env=env, stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=25, check=False)
        preflight.verify_cleanup(hb['directory'], hb['label'], env)
        if result.returncode != 0:
            raise RuntimeError('forkless observer cleanup failed')
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
        signal.signal(signal.SIGTERM, oldterm)

if __name__ == '__main__':
    try: sys.exit(main())
    except Exception as exc:
        print('dual observer wrapper failed: ' + type(exc).__name__, file=sys.stderr)
        sys.exit(1)

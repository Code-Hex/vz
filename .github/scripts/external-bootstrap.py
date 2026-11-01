#!/usr/bin/env python3
"""Temporary bootstrap for an externally observed native diagnostic."""
import base64
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
SAFE_TOKEN = re.compile(r'[A-Za-z0-9_]+\Z')
REPOSITORY = re.compile(r'[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+\Z')
SHA = re.compile(r'[a-fA-F0-9]{40}\Z')
RUN_ID = re.compile(r'[0-9]+\Z')


def child_environment():
    env = {name: os.environ[name] for name in ('HOME', 'TMPDIR', 'LANG', 'LC_ALL') if name in os.environ}
    env.update({'PATH': '/usr/bin:/bin:/usr/sbin:/sbin', 'NAT': '1', 'DGRAM': '0',
                'CONNECT': '0', 'SOCKET_DEVICE': '0', 'COUNT': '10', 'CONSOLE': '0'})
    return env


def check_request(method, url, body, token, tempdir, env):
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
                                 '--data-binary', '@' + body_file, '--max-time', '20', url],
                                input=config, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                env=env, timeout=25, check=True)
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
        await_server(server, token)
        tunnel_log = open(tempdir / 'ci306-control-tunnel.log', 'wb')
        tunnel = subprocess.Popen([str(tunnel_bin), 'tunnel', '--no-autoupdate', '--url',
                                   'http://127.0.0.1:8765', '--protocol', 'http2'],
                                  stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.PIPE,
                                  env=env, start_new_session=True, close_fds=True)
        results = queue.Queue(maxsize=1)
        tunnel_reader = threading.Thread(target=drain_tunnel, args=(tunnel.stderr, tunnel_log, results), daemon=True)
        tunnel_reader.start()
        public_url = await_tunnel(tunnel, results)
        encrypted = encrypt_token(token, public_key, env)
        summary = json.dumps({'url': public_url, 'ciphertext': encrypted, 'run_id': run_id}, separators=(',', ':'))
        created = check_request('POST', api_url, {'name': CHECK_NAME, 'head_sha': sha, 'status': 'in_progress',
                                'output': {'title': CHECK_NAME, 'summary': summary}}, token_github, tempdir, env)
        check_id = int(created['id'])
        # No runner-side progress loop or timer after publication. The observer drives /start and /finish.
        completed = server.wait() == 0
        if not completed:
            raise RuntimeError('control server exited unsuccessfully')
    finally:
        signal.signal(signal.SIGTERM, signal.SIG_IGN)
        try:
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
                                              'summary': 'External observation finished.' if completed else 'External observation ended before completion.'}},
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

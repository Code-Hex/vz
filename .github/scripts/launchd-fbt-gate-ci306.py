#!/usr/bin/env python3
"""Wait for independent observer readiness before execing the runner-owned controller."""
import json
import os
from pathlib import Path
import re
import sys


def main():
    if os.environ.get('GITHUB_REPOSITORY') != 'Code-Hex/vz' or os.environ.get('GITHUB_REF') != 'refs/heads/codex/diagnose-ci306-intel':
        raise ValueError('incorrect diagnostic repository or branch')
    if 'GITHUB_TOKEN' in os.environ:
        raise ValueError('token must arrive through the release pipe')
    raw = sys.stdin.buffer.readline(4097)
    if not raw.endswith(b'\n') or len(raw) > 4096:
        raise ValueError('invalid release')
    payload = json.loads(raw)
    if not isinstance(payload, dict) or set(payload) != {'token'} or not isinstance(payload['token'], str) or not re.fullmatch(r'[A-Za-z0-9._~+/-]+={0,2}', payload['token']):
        raise ValueError('invalid release token')
    env = os.environ.copy()
    env['GITHUB_TOKEN'] = payload['token']
    controller = Path(__file__).resolve().parent / 'kernel-fbt-stream.py'
    os.execve('/usr/bin/python3', ['/usr/bin/python3', str(controller)], env)

if __name__ == '__main__':
    try: main()
    except Exception as exc:
        print('launchd gate failed: ' + type(exc).__name__, file=sys.stderr)
        sys.exit(1)

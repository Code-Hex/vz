#!/usr/bin/env python3
"""一時診断。VM を起動せず、確認済み Intel kernel の timeout 定数を一度読む。"""
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import platform
import re
import secrets
import subprocess
import tarfile
import time
import uuid

EXPECTED_UUID = 'F6A1B3D4-1EBB-3628-A1C5-89506C51DDEC'
PARSER_SHA = '9ee5193fa81863d3b02df52ff4c3cc844aba73d5295c451730c6b783f403da11'
FUNCTIONS = ('mp_cpus_call1', 'pmap_flush_tlbs')
WIDTHS = {'vm_kernel_slide': 64, 'LockTimeOutUsec': 32, 'TLBTimeOut': 64, 'debugger_entry_time': 64,
          'LockTimeOutTSC': 64, 'debugger_exit_time': 64, 'LastDebuggerEntryAllowance': 64,
          'lck_spinlock_timeout_in_progress': 64, 'pmap_tlb_flush_timeout': 32,
          'tscFreq': 64, 'virtualized': 32, 'LockTimeOut': 64,
          'disable_serial_output': 8, 'debugger_current_op': 32}
ENV = {'PATH': '/usr/bin:/bin:/usr/sbin:/sbin', 'LC_ALL': 'C', 'LANG': 'C'}


def guard_host(system, machine):
    if (system, machine) != ('Darwin', 'x86_64'):
        raise ValueError('Intel Darwin 専用。kernel 取得は実行しない')


def decode_kcdata(path, parser_path):
    if path.stat().st_size > 64 * 1024 * 1024:
        raise ValueError('stackshot size limit')
    if hashlib.sha256(parser_path.read_bytes()).hexdigest() != PARSER_SHA:
        raise ValueError('parser SHA mismatch')
    spec = importlib.util.spec_from_file_location('apple_kcdata', parser_path)
    parser = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(parser)
    buffers = list(parser.iterate_kcdatas(io.TextIOWrapper(io.BytesIO(path.read_bytes()))))
    if len(buffers) != 1:
        raise ValueError('one stackshot required')
    return json.loads('{' + buffers[0].GetJsonRepr() + '}')


def running_kernel(decoded):
    root = decoded['kcdata_stackshot']
    if '/RELEASE_X86_64' not in root['osversion']:
        raise ValueError('running kernel architecture/flavor mismatch')
    tasks = [t for t in root['task_snapshots'].values() if t['task_snapshot']['ts_pid'] == 0]
    if len(tasks) != 1:
        raise ValueError('kernel task missing or duplicated')
    # Apple はここに VM_KERNEL_UNSLIDE(vm_kernel_stext) を保存する。slide として使わない。
    image = tasks[0]['dyld_load_info'][0]
    kernel_uuid = str(uuid.UUID(bytes=bytes(image['imageUUID']))).upper()
    if kernel_uuid != EXPECTED_UUID:
        raise ValueError('running kernel UUID mismatch')
    return {'uuid': kernel_uuid, 'unslid_text_address': image['imageLoadAddress'],
            'osversion': root['osversion'], 'boot_args': root['boot_args'],
            'mach_absolute_time': root['mach_absolute_time'],
            'mach_timebase_info': root['mach_timebase_info']}


def parse_values(text):
    measured, symbols, meta = {}, {}, None
    complete = 0
    for line in text.splitlines():
        if line == 'COMPLETE':
            complete += 1
            continue
        match = re.fullmatch(r'META wall_ns=(\d+) mono_ns=(\d+)', line)
        if match:
            if meta is not None:
                raise ValueError('duplicate META')
            meta = dict(zip(('wall_ns', 'mono_ns'), map(int, match.groups())))
            continue
        match = re.fullmatch(r'SYMBOL (\w+) address=(0x[0-9a-f]+)', line)
        if match:
            name, address = match.groups()
            if name not in FUNCTIONS or name in symbols:
                raise ValueError('unexpected/duplicate symbol')
            symbols[name] = int(address, 16)
            continue
        match = re.fullmatch(r'VALUE (\w+) address=(0x[0-9a-f]+) width=(\d+) value=(\d+)', line)
        if not match:
            raise ValueError('unknown or error DTrace output')
        name, address, width, value = match.groups()
        if name not in WIDTHS or name in measured or int(width) != WIDTHS[name]:
            raise ValueError('unexpected/duplicate value or width')
        if int(value) >= 1 << int(width):
            raise ValueError('value outside width')
        measured[name] = {'address': int(address, 16), 'width': int(width), 'value': int(value)}
    if complete != 1 or meta is None or set(measured) != set(WIDTHS) or set(symbols) != set(FUNCTIONS):
        raise ValueError('incomplete measurement')
    addresses = list(symbols.values()) + [v['address'] for v in measured.values()]
    if not all(0xffffff8000000000 <= a < 0xffffffffffffffff for a in addresses):
        raise ValueError('non-kernel address')
    value = {k: v['value'] for k, v in measured.items()}
    if value['tscFreq'] <= 0 or value['LockTimeOutTSC'] <= 0:
        raise ValueError('invalid measured timeout/rate')
    if value['disable_serial_output'] not in (0, 1) or value['virtualized'] not in (0, 1):
        raise ValueError('invalid boolean')
    multiplier = 4 if value['disable_serial_output'] else 16
    return {'measured': measured, 'runtime_function_addresses': symbols, 'dtrace_time': meta,
            'derived': {'mp_multiplier': multiplier,
                        'mp_timeout_seconds': value['LockTimeOutTSC'] / value['tscFreq'] * multiplier,
                        'spin_timeout_seconds_from_usec': value['LockTimeOutUsec'] / 1000000,
                        'spin_timeout_seconds_from_tsc': value['LockTimeOutTSC'] / value['tscFreq']},
            'interpretation': '定数の実測と期限の計算。障害箇所・原因は示さない'}


def encrypt(raw, root, public):
    archive, password = root / 'raw.tar.gz', root / 'password'
    try:
        with tarfile.open(archive, 'w:gz') as out:
            for path in sorted(raw.iterdir()):
                if path.is_file() and (path.suffix == '.log' or path.name == 'summary.json'):
                    out.add(path, arcname='raw/' + path.name)
        password.write_text(secrets.token_hex(32))
        password.chmod(0o600)
        subprocess.run(['/usr/bin/openssl', 'enc', '-aes-256-cbc', '-pbkdf2', '-iter', '100000', '-salt',
                        '-in', str(archive), '-out', str(root / 'capture.enc'), '-pass', 'file:' + str(password)],
                       env=ENV, stdin=subprocess.DEVNULL, capture_output=True, timeout=20, check=True)
        subprocess.run(['/usr/bin/openssl', 'pkeyutl', '-encrypt', '-pubin', '-inkey', str(public),
                        '-pkeyopt', 'rsa_padding_mode:oaep', '-in', str(password), '-out', str(root / 'password.enc')],
                       env=ENV, stdin=subprocess.DEVNULL, capture_output=True, timeout=10, check=True)
    finally:
        password.unlink(missing_ok=True)
        archive.unlink(missing_ok=True)


def main():
    guard_host(platform.system(), platform.machine())
    if os.environ.get('GITHUB_REPOSITORY') != 'Code-Hex/vz' or os.environ.get('GITHUB_REF') != 'refs/heads/codex/diagnose-ci306-intel':
        raise ValueError('diagnostic repository/branch required')
    os.umask(0o077)
    scripts = Path(__file__).resolve().parent
    temp = Path(os.environ['RUNNER_TEMP']).resolve(strict=True)
    root = temp / 'timeout-policy'
    root.mkdir(mode=0o700, exist_ok=False)
    raw = root / 'raw'
    raw.mkdir(mode=0o700)
    results = []
    summary = {'status': 'failed', 'vm_started': False, 'dtrace_executions': 0, 'commands': results}

    def run(name, argv, timeout=10):
        started = time.monotonic()
        with (raw / (name + '.log')).open('wb') as log:
            proc = subprocess.Popen(argv, env=ENV, stdin=subprocess.DEVNULL, stdout=log,
                                    stderr=subprocess.STDOUT, start_new_session=True)
            try:
                code = proc.wait(timeout=timeout)
            except subprocess.TimeoutExpired:
                subprocess.run(['/usr/bin/sudo', '-n', '/bin/kill', '-KILL', '--', '-' + str(proc.pid)],
                               env=ENV, stdin=subprocess.DEVNULL, capture_output=True, timeout=5, check=False)
                proc.wait(timeout=5)
                results.append({'name': name, 'status': 'timeout'})
                raise TimeoutError(name)
        results.append({'name': name, 'returncode': code, 'seconds': time.monotonic() - started})
        if code != 0:
            raise RuntimeError(name + ' failed')
        return (raw / (name + '.log')).read_text()

    try:
        before = run('boot-session-before', ['/usr/sbin/sysctl', '-n', 'kern.bootsessionuuid']).strip()
        uuid.UUID(before)
        summary['boot_session_before'] = before
        run('boot', ['/usr/sbin/sysctl', 'kern.bootargs', 'kern.hv_vmm_present', 'kern.version'])
        file_uuid = run('file-uuid', ['/usr/bin/xcrun', 'dwarfdump', '--uuid', '/System/Library/Kernels/kernel'])
        if not re.fullmatch(r'UUID: ' + EXPECTED_UUID + r' \(x86_64\) /System/Library/Kernels/kernel\s*', file_uuid):
            raise ValueError('file kernel UUID mismatch')
        shot = raw / 'shot.kcdata'
        capture = json.loads(run('stackshot', ['/usr/bin/sudo', '-n', str(temp / 'stackshot-once-timeout'), str(shot)], 30))
        if capture.get('status') != 'captured' or capture.get('euid') != 0:
            raise ValueError('stackshot not captured as root')
        run('stackshot-owner', ['/usr/bin/sudo', '-n', '/usr/sbin/chown', str(os.getuid()), str(shot)])
        if capture['bytes'] != shot.stat().st_size:
            raise ValueError('stackshot size mismatch')
        summary['running_kernel'] = running_kernel(decode_kcdata(shot, temp / 'kcdata.py'))
        shot.unlink()
        summary['dtrace_executions'] = 1
        output = run('dtrace-values', ['/usr/bin/sudo', '-n', '/usr/sbin/dtrace', '-q', '-s',
                                     str(scripts / 'timeout-policy-live-ci306.d')], 15)
        summary['measurement'] = parse_values(output)
        after = run('boot-session-after', ['/usr/sbin/sysctl', '-n', 'kern.bootsessionuuid']).strip()
        summary['boot_session_after'] = after
        if after != before:
            raise ValueError('boot session changed')
        summary['status'] = 'measured'
    except Exception as error:
        summary['error'] = type(error).__name__ + ': ' + str(error)
    finally:
        (raw / 'summary.json').write_text(json.dumps(summary, indent=2, ensure_ascii=False))
        # 生 stackshot は選択失敗時も artifact に含めない。
        shot_path = raw / 'shot.kcdata'
        if shot_path.exists():
            subprocess.run(['/usr/bin/sudo', '-n', '/bin/rm', '-f', str(shot_path)], env=ENV,
                           stdin=subprocess.DEVNULL, capture_output=True, timeout=5, check=False)
        encrypt(raw, root, scripts / 'observer-public.pem')
    print(json.dumps({'status': summary['status'], 'vm_started': False,
                      'capture_sha256': hashlib.sha256((root / 'capture.enc').read_bytes()).hexdigest()}))
    return 0 if summary['status'] == 'measured' else 1


if __name__ == '__main__':
    raise SystemExit(main())

#!/usr/bin/env python3
"""Window two DTrace text streams without treating them as ktrace events."""
import argparse
import collections
import gzip
import json
import os
from pathlib import Path
import queue
import selectors
import threading
import time


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--output-dir', required=True)
    parser.add_argument('--fbt-fd', type=int, required=True)
    parser.add_argument('--profile-fd', type=int, required=True)
    parser.add_argument('--window-seconds', type=float, default=3)
    parser.add_argument('--duration-seconds', type=float, default=145)
    args = parser.parse_args()
    if not 0.01 <= args.window_seconds <= args.duration_seconds <= 600 or args.fbt_fd == args.profile_fd:
        parser.error('invalid durations or duplicate source')
    os.umask(0o077)
    dest = Path(args.output_dir)
    dest.mkdir(mode=0o700, parents=True, exist_ok=True)
    if any(dest.glob('window-*.json.gz')):
        raise RuntimeError('window directory must be empty')
    pending = queue.Queue(maxsize=3)
    errors = []
    done = threading.Event()
    def writer():
        try:
            while not done.is_set() or not pending.empty():
                try: item = pending.get(timeout=0.1)
                except queue.Empty: continue
                compressed = gzip.compress(json.dumps(item, separators=(',', ':'), ensure_ascii=True).encode(), compresslevel=1, mtime=0)
                target = dest / ('window-%06d.json.gz' % item['sequence'])
                tmp = target.with_suffix(target.suffix + '.partial')
                fd = os.open(tmp, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
                with os.fdopen(fd, 'wb') as out: out.write(compressed)
                os.rename(tmp, target)
                pending.task_done()
        except Exception as exc:
            errors.append(type(exc).__name__)
    thread = threading.Thread(target=writer, daemon=True)
    thread.start()
    selector = selectors.DefaultSelector()
    sources = {'fbt': args.fbt_fd, 'profile': args.profile_fd}
    for name, fd in sources.items(): selector.register(fd, selectors.EVENT_READ, name)
    partial = {name: b'' for name in sources}
    oversized = {name: False for name in sources}
    order = 0
    start = time.monotonic()
    deadline = start + args.duration_seconds
    next_boundary = start + args.window_seconds
    sequence = 0
    dropped_windows = []
    def new_window(seq):
        return {'sequence': seq, 'collector_start_monotonic': time.monotonic(), 'wall_start_ns': time.time_ns(),
                'sources': {name: {'input_bytes': 0, 'complete_lines': 0, 'dropped_lines': 0, 'dropped_bytes': 0,
                                   'oversized_lines': 0, 'retained_bytes': 0} for name in sources},
                'lines': {name: collections.deque() for name in sources}}
    window = new_window(sequence)
    def add(name, line):
        nonlocal order
        info = window['sources'][name]
        info['complete_lines'] += 1
        limit = 98304
        if len(line) > limit:
            info['dropped_lines'] += 1
            info['dropped_bytes'] += len(line)
            return
        while info['retained_bytes'] + len(line) > limit:
            _, old = window['lines'][name].popleft()
            info['retained_bytes'] -= len(old)
            info['dropped_lines'] += 1
            info['dropped_bytes'] += len(old)
        order += 1
        window['lines'][name].append((order, line))
        info['retained_bytes'] += len(line)
    def emit(reason):
        result = {k: v for k, v in window.items() if k != 'lines'}
        result.update(collector_end_monotonic=time.monotonic(), wall_end_ns=time.time_ns(), stop_reason=reason,
                      dropped_window_sequences=list(dropped_windows), partial_bytes_at_boundary={n: len(p) for n,p in partial.items()},
                      incomplete_oversized_line=dict(oversized), window_scope='collector_receive_time')
        lines = [(idx, name, line) for name, entries in window['lines'].items() for idx, line in entries]
        result['raw_lines'] = [name + '\t' + line.decode('utf-8', errors='replace') for _, name, line in sorted(lines)]
        try: pending.put_nowait(result)
        except queue.Full: dropped_windows.append(result['sequence'])
    reason = None
    while reason is None:
        now = time.monotonic()
        if now >= deadline: reason = 'collector_deadline'; break
        if errors: reason = 'writer_failed'; break
        if now >= next_boundary:
            emit('window')
            sequence += 1
            window = new_window(sequence)
            next_boundary = now + args.window_seconds
        events = selector.select(min(0.2, max(0, next_boundary-now), deadline-now))
        for key, _ in events:
            name = key.data
            block = os.read(key.fd, 65536)
            if not block:
                selector.unregister(key.fd)
                continue
            info = window['sources'][name]
            info['input_bytes'] += len(block)
            pieces = (partial[name] + block).split(b'\n')
            partial[name] = pieces.pop()
            for line in pieces:
                if oversized[name]:
                    info['complete_lines'] += 1
                    info['dropped_lines'] += 1
                    info['dropped_bytes'] += len(line)
                    info['oversized_lines'] += 1
                    oversized[name] = False
                elif len(line) > 65536:
                    info['complete_lines'] += 1
                    info['dropped_lines'] += 1
                    info['dropped_bytes'] += len(line)
                    info['oversized_lines'] += 1
                else: add(name, line)
            if len(partial[name]) > 65536:
                info['dropped_bytes'] += len(partial[name])
                partial[name] = b''
                oversized[name] = True
        if not selector.get_map(): reason = 'eof'
    if pending.full():
        try:
            lost = pending.get_nowait(); pending.task_done(); dropped_windows.append(lost['sequence'])
        except queue.Empty: pass
    emit(reason)
    selector.close()
    done.set()
    thread.join(timeout=2)
    return 1 if errors or thread.is_alive() or not pending.empty() else 0


if __name__ == '__main__':
    raise SystemExit(main())

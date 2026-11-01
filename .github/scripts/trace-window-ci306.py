"""固定 ktrace プロセスの stdout を、容量制限付きの短い私有ファイルへ分割する。"""
import argparse
import collections
import gzip
import json
import os
import pathlib
import queue
import selectors
import threading
import time


class Window:
    def __init__(self, sequence, limit):
        self.sequence = sequence
        self.limit = limit
        self.started = time.monotonic()
        self.wall_started_ns = time.time_ns()
        self.lines = collections.deque()
        self.retained = 0
        self.input_bytes = 0
        self.complete_lines = 0
        self.dropped_lines = 0
        self.oversized_lines = 0

    def add(self, line):
        self.complete_lines += 1
        if len(line) > self.limit:
            self.dropped_lines += 1
            return
        while self.retained + len(line) > self.limit:
            self.retained -= len(self.lines.popleft())
            self.dropped_lines += 1
        self.lines.append(line)
        self.retained += len(line)

    def snapshot(self, reason, partial, queue_drops):
        return dict(sequence=self.sequence, collector_start_monotonic=self.started,
            collector_end_monotonic=time.monotonic(), wall_start_ns=self.wall_started_ns,
            wall_end_ns=time.time_ns(), input_bytes=self.input_bytes,
            complete_lines=self.complete_lines, dropped_lines=self.dropped_lines,
            oversized_lines=self.oversized_lines, retained_bytes=self.retained,
            partial_bytes_at_boundary=partial, dropped_window_sequences=list(queue_drops),
            stop_reason=reason, window_scope='collector_receive_time',
            raw_lines=[line.decode('utf-8', errors='replace') for line in self.lines])


def run(args):
    os.umask(0o077)
    destination = pathlib.Path(args.output_dir)
    destination.mkdir(mode=0o700, parents=True, exist_ok=True)
    if any(destination.glob('window-*.json.gz')):
        raise RuntimeError('既存の窓ファイルがあるため再利用しません')
    pending = queue.Queue(maxsize=3)
    errors = []
    done = threading.Event()

    def write_windows():
        try:
            while not done.is_set() or not pending.empty():
                try:
                    snapshot = pending.get(timeout=0.1)
                except queue.Empty:
                    continue
                encoded = json.dumps(snapshot, separators=(',', ':'), ensure_ascii=True).encode()
                compressed = gzip.compress(encoded, compresslevel=1, mtime=0)
                name = f"window-{snapshot['sequence']:06d}.json.gz"
                target = destination / name
                temporary = destination / (name + '.partial')
                fd = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
                with os.fdopen(fd, 'wb') as output:
                    output.write(compressed)
                os.rename(temporary, target)
                pending.task_done()
        except Exception as exc:
            errors.append(type(exc).__name__)

    writer = threading.Thread(target=write_windows, daemon=True)
    writer.start()
    sequence = 0
    window = Window(sequence, args.retained_bytes)
    deadline = window.started + args.duration_seconds
    next_boundary = window.started + args.window_seconds
    partial = b''
    oversize = False
    queue_drops = []
    selection = selectors.DefaultSelector()
    selection.register(0, selectors.EVENT_READ)
    stop_reason = None

    def enqueue(snapshot):
        try:
            pending.put_nowait(snapshot)
        except queue.Full:
            queue_drops.append(snapshot['sequence'])

    while stop_reason is None:
        now = time.monotonic()
        if now >= deadline:
            stop_reason = 'collector_deadline'
            break
        if errors:
            stop_reason = 'writer_failed'
            break
        if now >= next_boundary:
            enqueue(window.snapshot('window', len(partial), queue_drops))
            sequence += 1
            window = Window(sequence, args.retained_bytes)
            next_boundary = now + args.window_seconds
        ready = selection.select(min(0.2, max(0, next_boundary - now), deadline - now))
        if not ready:
            continue
        block = os.read(0, 65536)
        if not block:
            stop_reason = 'eof'
            break
        window.input_bytes += len(block)
        pieces = (partial + block).split(b'\n')
        partial = pieces.pop()
        for line in pieces:
            if oversize:
                window.complete_lines += 1
                window.dropped_lines += 1
                window.oversized_lines += 1
                oversize = False
            elif len(line) > 65536:
                window.complete_lines += 1
                window.dropped_lines += 1
                window.oversized_lines += 1
            else:
                window.add(line)
        if len(partial) > 65536:
            partial = b''
            oversize = True
    final = window.snapshot(stop_reason, len(partial), queue_drops)
    final['incomplete_oversized_line'] = oversize
    if pending.full():
        try:
            dropped = pending.get_nowait()
            pending.task_done()
            queue_drops.append(dropped['sequence'])
        except queue.Empty:
            pass
    final['dropped_window_sequences'] = list(queue_drops)
    enqueue(final)
    selection.close()
    done.set()
    writer.join(timeout=2)
    if errors or writer.is_alive() or not pending.empty():
        return 1
    return 0


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--output-dir', required=True)
    parser.add_argument('--window-seconds', type=float, default=5)
    parser.add_argument('--duration-seconds', type=float, default=25)
    parser.add_argument('--retained-bytes', type=int, default=196608)
    args = parser.parse_args()
    if not 0.01 <= args.window_seconds <= args.duration_seconds <= 600:
        parser.error('0.01 <= window <= duration <= 600 が必要です')
    if not 256 <= args.retained_bytes <= 1048576:
        parser.error('retained-bytes は256〜1048576です')
    raise SystemExit(run(args))


if __name__ == '__main__':
    main()

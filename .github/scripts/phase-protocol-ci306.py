#!/usr/bin/env python3
"""Bounded native-output reader and persisted phase-gate protocol."""
import collections
import os
import queue
import re
import threading

PATTERN = re.compile(r'^PHASE_(READY|PERMITTED|COMPLETE) sequence=([0-9]+) iteration=([0-9]+) phase=([a-z-]+)$')
PHASES = {'start-peer', 'start-client', 'both-running', 'stop-client', 'release-client', 'stop-peer', 'release-peer', 'abort-exit'}


class PhaseProtocol:
    def __init__(self, mode):
        if mode not in ('0', '1'):
            raise ValueError('invalid phase mode')
        self.mode = mode
        self.last_sequence = 0
        self.pending = None
        self.history = []
    def accept(self, line):
        match = PATTERN.fullmatch(line)
        if not match:
            raise ValueError('malformed phase event')
        kind, seq, iteration, phase = match.groups()
        seq, iteration = int(seq), int(iteration)
        if iteration < 1 or phase not in PHASES:
            raise ValueError('invalid phase metadata')
        event = {'fact': kind.lower(), 'sequence': seq, 'iteration': iteration, 'phase': phase}
        if kind == 'READY':
            normal = ['start-peer', 'start-client', 'both-running', 'stop-client']
            normal += ['release-client', 'stop-peer'] if self.mode == '0' else ['stop-peer', 'release-client']
            normal += ['release-peer']
            if phase != 'abort-exit' and (iteration != (seq - 1) // 7 + 1 or phase != normal[(seq - 1) % 7]):
                raise ValueError('unexpected phase order')
            preceding = self.pending or (self.history[-1] if self.history else None)
            if phase == 'abort-exit' and (preceding is None or iteration != preceding['iteration']):
                raise ValueError('unexpected abort')
            if seq != self.last_sequence + 1 or (self.pending is not None and not (phase == 'abort-exit' and self.pending['fact'] == 'permitted')):
                raise ValueError('unexpected readiness')
            self.last_sequence = seq
            self.pending = event
        else:
            if self.pending is None or any(event[key] != self.pending[key] for key in ('sequence', 'iteration', 'phase')):
                raise ValueError('phase event does not match readiness')
            if kind == 'PERMITTED' and self.pending['fact'] != 'ready':
                raise ValueError('unexpected permit acknowledgement')
            if kind == 'COMPLETE' and self.pending['fact'] != 'permitted':
                raise ValueError('completion without permit acknowledgement')
            self.pending = None if kind == 'COMPLETE' else event
        self.history.append(event)
        return event
    def release(self, event, persist, wait, fd):
        if self.pending != event or event['fact'] != 'ready':
            raise ValueError('phase is not awaiting permission')
        persist(event)
        wait(10 if event['phase'] == 'both-running' else 1)
        data = ('PERMIT %d\n' % event['sequence']).encode('ascii')
        if os.write(fd, data) != len(data):
            raise OSError('short permit write')


class NativeReader:
    def __init__(self):
        self.lock = threading.Lock()
        self.data = bytearray()
        self.total = 0
        self.events = queue.Queue(maxsize=256)
        self.failed = threading.Event()
    def drain(self, stream):
        partial = b''
        try:
            while True:
                block = os.read(stream.fileno(), 16384)
                if not block: break
                with self.lock:
                    self.total += len(block)
                    self.data.extend(block)
                    del self.data[:-65536]
                lines = (partial + block).split(b'\n')
                partial = lines.pop()
                if len(partial) > 16384:
                    self.failed.set()
                    partial = b''
                for line in lines:
                    if len(line) > 16384:
                        self.failed.set()
                    elif line.startswith(b'PHASE_'):
                        try: self.events.put_nowait(line.decode('ascii'))
                        except (UnicodeError, queue.Full): self.failed.set()
            if partial.startswith(b'PHASE_'):
                self.failed.set()
        except OSError:
            self.failed.set()
        finally:
            stream.close()
    def read(self):
        with self.lock:
            return {'tail': self.data.decode('utf-8', 'replace'), 'total_bytes': self.total, 'dropped_bytes': self.total - len(self.data)}

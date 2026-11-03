import argparse
import json
import os
import signal
import subprocess
import time


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--case", required=True)
    parser.add_argument("--go", required=True)
    parser.add_argument("--target", required=True)
    parser.add_argument("--count", type=int, required=True)
    parser.add_argument("--processes", type=int, default=1)
    parser.add_argument("--interval", type=float, default=10)
    parser.add_argument("--quit-after", type=float, default=240)
    parser.add_argument("--kill-delay", type=float, default=30)
    parser.add_argument("command", nargs=argparse.REMAINDER)
    args = parser.parse_args()
    command = args.command
    if command and command[0] == "--":
        command = command[1:]
    if not command or min(args.count, args.processes) < 1:
        parser.error("command, count, and processes must be provided")
    if min(args.interval, args.quit_after, args.kill_delay) <= 0:
        parser.error("observer durations must be positive")
    started = time.monotonic()

    def emit(phase, process_number, pid, exit_code=None):
        print(json.dumps({
            "case": args.case,
            "go": args.go,
            "target": args.target,
            "count": args.count,
            "processNumber": process_number,
            "phase": phase,
            "monotonicDuration": round(time.monotonic() - started, 6),
            "pid": pid,
            "exitCode": exit_code,
        }), flush=True)

    for number in range(1, args.processes + 1):
        env = os.environ.copy()
        env.update({
            "CI312_CASE": args.case,
            "CI312_GO": args.go,
            "CI312_TARGET": args.target,
            "CI312_COUNT": str(args.count),
            "CI312_PROCESS_NUMBER": str(number),
        })
        child = subprocess.Popen(command, env=env, start_new_session=True)
        emit("start", number, child.pid)
        child_started = time.monotonic()
        heartbeat = child_started + args.interval
        quit_at = child_started + args.quit_after
        kill_at = quit_at + args.kill_delay
        expired = False
        killed = False
        while child.poll() is None:
            now = time.monotonic()
            if not expired and now >= quit_at:
                emit("quit", number, child.pid)
                expired = True
                try:
                    os.killpg(child.pid, signal.SIGQUIT)
                except ProcessLookupError:
                    pass
            if expired and not killed and now >= kill_at:
                emit("kill", number, child.pid)
                killed = True
                try:
                    os.killpg(child.pid, signal.SIGKILL)
                except ProcessLookupError:
                    pass
            if now >= heartbeat:
                emit("heartbeat", number, child.pid)
                heartbeat = now + args.interval
            next_signal = kill_at if expired else quit_at
            timeout = max(0.001, min(0.1, heartbeat - now, next_signal - now))
            try:
                child.wait(timeout=timeout)
            except subprocess.TimeoutExpired:
                pass
        exit_code = child.wait()
        emit("exit", number, child.pid, exit_code)
        if expired:
            return 124
        if exit_code:
            return exit_code if exit_code > 0 else 128 - exit_code
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

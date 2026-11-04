import datetime
import pathlib
import signal
import subprocess
import sys


def stop(signum, frame):
    raise SystemExit(128 + signum)


signal.signal(signal.SIGTERM, stop)
signal.signal(signal.SIGINT, stop)
directory = pathlib.Path(sys.argv[1])
with (directory / "native-tests.log").open("a", buffering=1) as output:
    def record(message):
        now = datetime.datetime.now(datetime.timezone.utc).isoformat()
        print(f"[native-control] utc={now} {message}", file=output)

    for iteration in range(1, 4):
        child = None
        try:
            child = subprocess.Popen(sys.argv[2:], stdout=output, stderr=subprocess.STDOUT)
            record(f"iteration={iteration} child_pid={child.pid} started")
            try:
                # main.m allows 30 seconds per guest and 10 seconds per stop.
                status = child.wait(timeout=90)
            except subprocess.TimeoutExpired:
                record(f"iteration={iteration} child_pid={child.pid} deadline; killing child")
                raise SystemExit(1)
            record(f"iteration={iteration} child_pid={child.pid} exit={status}")
            if status:
                raise SystemExit(1)
        finally:
            if child is not None and child.poll() is None:
                child.kill()
                child.wait()

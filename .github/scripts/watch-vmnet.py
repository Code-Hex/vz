import datetime
import pathlib
import signal
import sys
import time


directory = pathlib.Path(sys.argv[1])
stopping = False
previous = {}


def stop(signum, frame):
    global stopping
    stopping = True


signal.signal(signal.SIGTERM, stop)
signal.signal(signal.SIGINT, stop)

# The reader can block on Actions output without blocking this collector.
with (directory / "host-health.log").open("w", buffering=1) as output:
    def record(message):
        now = datetime.datetime.now(datetime.timezone.utc).isoformat()
        print(f"[host-witness] utc={now} {message}", file=output)

    while not stopping:
        record(f"heartbeat monotonic={time.monotonic():.3f}")
        for path in directory.glob("*-wrapper-*.log"):
            lines = path.read_text(errors="replace").splitlines()
            if not lines:
                continue
            if previous.get(path) != lines[-1]:
                previous[path] = lines[-1]
                record(lines[-1])
        time.sleep(1)
    record("collector stopped")

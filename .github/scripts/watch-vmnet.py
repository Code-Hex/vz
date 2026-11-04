import datetime
import pathlib
import re
import signal
import subprocess
import sys
import time


directory = pathlib.Path(sys.argv[1])
stopping = False
sampled = {}
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
            if "test exited" in lines[-1]:
                continue
            match = re.search(r"child_pid=(\d+).*binary=\"([^\"]+)\"", lines[-1])
            if match is None or match[1] == "0":
                continue
            pid, binary = match.groups()
            captures = sampled.setdefault(pid, [])
            if len(captures) == 2 or (captures and time.monotonic() - captures[-1] < 15):
                continue
            captures.append(time.monotonic())
            capture_name = f"sample-{pid}-{len(captures)}"
            try:
                process = subprocess.run(
                    ["/bin/ps", "-p", pid, "-o", "pid,ppid,stat,etime,time,command"],
                    capture_output=True, text=True, timeout=3,
                )
                record(f"process exit={process.returncode} {process.stdout.strip()} {process.stderr.strip()}")
                if process.returncode != 0 or binary not in process.stdout:
                    continue
                record(f"sample begin child_pid={pid} capture={len(captures)}")
                with (directory / f"{capture_name}.log").open("w") as sample_output:
                    sample = subprocess.run(
                        ["/usr/bin/sample", pid, "1", "10", "-mayDie",
                         "-file", str(directory / f"{capture_name}.txt")],
                        stdout=sample_output, stderr=subprocess.STDOUT, timeout=5,
                    )
                record(f"sample end child_pid={pid} capture={len(captures)} exit={sample.returncode}")
                print((directory / f"{capture_name}.log").read_text(errors="replace"), file=output)
                sample_path = directory / f"{capture_name}.txt"
                if sample_path.exists():
                    print(sample_path.read_text(errors="replace"), file=output)
            except (OSError, subprocess.TimeoutExpired) as error:
                record(f"sample child_pid={pid} error={error}")
        time.sleep(1)
    record("collector stopped")

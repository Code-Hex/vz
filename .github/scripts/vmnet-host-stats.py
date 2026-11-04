import datetime
import os
import pathlib
import subprocess
import sys
import time


os.setsid()
directory = pathlib.Path(sys.argv[1])
with (directory / "host-stats.log").open("w", buffering=1) as output:
    started = time.monotonic()
    while time.monotonic() - started < 240:
        print(datetime.datetime.now(datetime.timezone.utc).isoformat(), file=output)
        for command in [
            ["/usr/bin/vm_stat"],
            ["/usr/sbin/sysctl", "vm.swapusage", "kern.num_files", "kern.maxfiles", "hw.memsize"],
            ["/bin/ps", "-axo", "pid,ppid,rss,vsz,pcpu,comm"],
        ]:
            try:
                process = subprocess.run(command, capture_output=True, text=True, timeout=2)
                text = process.stdout
                if command[0] == "/bin/ps":
                    selected = []
                    for line in text.splitlines():
                        words = line.split()
                        if words and (words[-1].rsplit("/", 1)[-1] in ("go", "compile", "link") or any(
                            name in line for name in ("PID", "vz.test", "Virtualization", "Runner.", "kernel_task", "codesign")
                        )):
                            selected.append(line)
                    text = "\n".join(selected)
                print(command, "exit", process.returncode, file=output)
                print(text, process.stderr, file=output)
            except (OSError, subprocess.TimeoutExpired) as error:
                print(command, error, file=output)
        time.sleep(1)
    print(datetime.datetime.now(datetime.timezone.utc).isoformat(), "collector completed", file=output)

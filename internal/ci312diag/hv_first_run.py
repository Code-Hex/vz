import argparse
from contextlib import ExitStack
import json
import os
from pathlib import Path
import re
import signal
import subprocess
import sys
import time


RUN_UNTIL = "_ZN13hv_vmx_vcpu_t26THREAD_TRAP_vcpu_run_untilEPS_y"
ENTER_GUEST = "_ZN13hv_vmx_vcpu_t11enter_guestEPjS0_"
STREAMS = ("trace.log", "trace.stderr.log", "vm.log", "vm.stderr.log", "launcher.jsonl")


def publish(directory, seconds):
    os.set_blocking(sys.stdout.fileno(), False)
    positions = dict.fromkeys(STREAMS, 0)
    skipped = unwritten = written = 0
    deadline = time.monotonic() + seconds
    with (directory / "publisher.jsonl").open("x", encoding="utf-8") as journal:
        while True:
            for name in STREAMS:
                path = directory / name
                if not path.exists():
                    continue
                size = path.stat().st_size
                offset = max(positions[name], size - 65536)
                skipped += offset - positions[name]
                with path.open("rb") as stream:
                    stream.seek(offset)
                    data = stream.read(min(65536, size - offset))
                positions[name] = offset + len(data)
                if not data:
                    continue
                packet = f"\nHVTRACE_FILE {name}\n".encode() + data
                try:
                    sent = os.write(sys.stdout.fileno(), packet)
                except (BlockingIOError, BrokenPipeError):
                    sent = 0
                written += sent
                unwritten += len(packet) - sent
            journal.write(json.dumps({"monotonic": time.monotonic(), "writtenBytes": written,
                "skippedFileBytes": skipped, "unwrittenOutputBytes": unwritten}) + "\n")
            journal.flush()
            if (directory / "publisher.stop").exists() or time.monotonic() >= deadline:
                return 0
            time.sleep(min(1, max(0, deadline - time.monotonic())))


def capture(command, path, cleanup, record, timeout=60):
    with path.open("xb") as output:
        process = subprocess.Popen(command, stdin=subprocess.DEVNULL, stdout=output,
            stderr=subprocess.STDOUT, start_new_session=True)
        record("preflight.start", file=path.name, pid=process.pid)
        try:
            process.wait(timeout=timeout)
        except subprocess.TimeoutExpired:
            record("preflight.timeout", file=path.name, pid=process.pid, timeout=timeout)
            stop(process, command[0] == "sudo", cleanup, record)
            raise
    record("preflight.exit", file=path.name, pid=process.pid, exitCode=process.returncode)
    if process.returncode:
        raise RuntimeError(f"preflight failed: {path.name}, exit={process.returncode}")
    return path.read_text(errors="replace")


def stop(process, root, output, record):
    if process is None:
        return
    for sig in (signal.SIGTERM, signal.SIGKILL):
        if process.returncode is not None or process.poll() is not None:
            record("cleanup.leader.exit", pid=process.pid, exitCode=process.returncode)
            return
        record("cleanup.signal", pid=process.pid, signal=int(sig), root=root)
        if root:
            result = subprocess.run(["sudo", "-n", "/bin/kill", f"-{sig.value}", "--", f"-{process.pid}"],
                stdin=subprocess.DEVNULL, stdout=output, stderr=subprocess.STDOUT, timeout=3)
            record("cleanup.signal.return", pid=process.pid, exitCode=result.returncode)
        else:
            try:
                os.killpg(process.pid, sig)
            except ProcessLookupError:
                pass
        try:
            process.wait(timeout=2)
            record("cleanup.leader.exit", pid=process.pid, exitCode=process.returncode)
            return
        except subprocess.TimeoutExpired:
            pass
    raise RuntimeError(f"owned leader did not exit: {process.pid}")


def run(binary, directory, script):
    directory.mkdir(parents=True, exist_ok=True)
    trace = vm = publisher = None
    result = 1
    with ExitStack() as files:
        journal = files.enter_context((directory / "launcher.jsonl").open("x", encoding="utf-8"))
        outputs = {name: files.enter_context((directory / name).open("xb")) for name in STREAMS[:-1]}
        cleanup = files.enter_context((directory / "cleanup.log").open("xb"))

        def record(phase, **values):
            journal.write(json.dumps({"phase": phase, "monotonic": time.monotonic(), **values}) + "\n")
            journal.flush()

        try:
            uid = os.getuid()
            record("runner", pid=os.getpid(), uid=uid, gid=os.getgid())
            if uid == 0 or binary.name != "hv-control":
                raise RuntimeError("VM must run as the original non-root runner with execname hv-control")
            root_id = capture(["sudo", "-n", "id"], directory / "sudo-id.log", cleanup, record)
            if "uid=0(" not in root_id:
                raise RuntimeError("sudo did not establish root")
            capture(["csrutil", "status"], directory / "csrutil.log", cleanup, record)
            for function in (RUN_UNTIL, ENTER_GUEST):
                for kind in ("entry", "return"):
                    spec = f"fbt:com.apple.driver.AppleHV:{function}:{kind}"
                    text = capture(["sudo", "-n", "/usr/sbin/dtrace", "-xmangled", "-l", "-n", spec],
                        directory / f"probe-{function}-{kind}.log", cleanup, record)
                    expected = ["fbt", "com.apple.driver.AppleHV", function, kind]
                    rows = [line.split() for line in text.splitlines()]
                    if not any(len(row) >= 5 and row[0].isdigit()
                            and row[1:4] == expected[:3] and row[-1] == kind for row in rows):
                        raise RuntimeError(f"exact probe missing: {spec}")
            capture(["sudo", "-n", "/usr/sbin/dtrace", "-xmangled", "-e", "-s", str(script), str(uid)],
                directory / "trace-compile.log", cleanup, record)
            trace = subprocess.Popen(["sudo", "-n", "/usr/sbin/dtrace", "-xmangled", "-q", "-s", str(script), str(uid)],
                stdin=subprocess.DEVNULL, stdout=outputs["trace.log"], stderr=outputs["trace.stderr.log"],
                start_new_session=True)
            record("trace.start", pid=trace.pid)
            ready_deadline = time.monotonic() + 60
            while True:
                text = (directory / "trace.log").read_text(errors="replace")
                if trace.poll() is not None:
                    record("trace.exit.before.vm", exitCode=trace.returncode)
                    raise RuntimeError("DTrace exited before VM start")
                if time.monotonic() >= ready_deadline:
                    raise RuntimeError("DTrace READY and baseline not received")
                if "HVTRACE READY " in text and "HVTRACE BASELINE " in text:
                    break
                time.sleep(0.05)
            record("trace.baseline.received")
            publisher = subprocess.Popen([sys.executable, str(Path(__file__).resolve()), "--publish",
                "--output-dir", str(directory), "--publish-seconds", "35"],
                stdin=subprocess.DEVNULL, stderr=cleanup, start_new_session=True)
            record("publisher.start", pid=publisher.pid)
            vm = subprocess.Popen([str(binary), "park", "1"], stdin=subprocess.DEVNULL,
                stdout=outputs["vm.log"], stderr=outputs["vm.stderr.log"], start_new_session=True)
            record("vm.start", pid=vm.pid, uid=uid, mode="park", iterations=1)
            deadline = time.monotonic() + 25
            while trace.poll() is None:
                if time.monotonic() >= deadline:
                    raise RuntimeError("launcher trace deadline exceeded")
                if vm.poll() is not None and vm.returncode:
                    raise RuntimeError(f"VM failed: exit={vm.returncode}")
                time.sleep(0.05)
            text = (directory / "trace.log").read_text(errors="replace")
            errors = (directory / "trace.stderr.log").read_text(errors="replace")
            drops = bool(re.search(r"\b(?:drop|drops|dropped)\b", errors, re.IGNORECASE))
            record("trace.exit", exitCode=trace.returncode, drops=drops)
            if trace.returncode or drops or "HVTRACE ERROR " in text or "HVTRACE WINDOW_END " not in text:
                raise RuntimeError("trace window incomplete or DTrace reported errors/drops")
            if "HVTRACE run_until.entry " not in text:
                raise RuntimeError("initial run_until entry not captured")
            vm.poll()
            record("trace.window.complete", vmExitCode=vm.returncode, vmStillRunning=vm.returncode is None)
            result = 124 if vm.returncode is None else vm.returncode
        except Exception as error:
            record("failure", error=str(error))
        finally:
            for process, root in ((vm, False), (trace, True)):
                try:
                    stop(process, root, cleanup, record)
                except Exception as error:
                    record("cleanup.failure", error=str(error))
                    result = 1
            (directory / "publisher.stop").touch()
            try:
                if publisher is not None:
                    publisher.wait(timeout=2)
                    if publisher.returncode:
                        result = 1
            except subprocess.TimeoutExpired:
                stop(publisher, False, cleanup, record)
                result = 1
            record("launcher.exit", exitCode=result)
    return result


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--binary", type=Path)
    parser.add_argument("--output-dir", type=Path, required=True)
    parser.add_argument("--script", type=Path, default=Path(__file__).with_suffix(".d").resolve())
    parser.add_argument("--publish", action="store_true")
    parser.add_argument("--publish-seconds", type=float, default=35)
    args = parser.parse_args()
    if args.publish:
        return publish(args.output_dir, args.publish_seconds)
    if args.binary is None:
        parser.error("--binary is required")
    return run(args.binary.resolve(), args.output_dir.resolve(), args.script.resolve())


if __name__ == "__main__":
    raise SystemExit(main())

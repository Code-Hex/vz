import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import tempfile
import time
import unittest


OBSERVER = Path(__file__).with_name("observe.py")


class ObserverTest(unittest.TestCase):
    def setUp(self):
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        self.directory = Path(directory.name)
        self.output_dir = self.directory / "output"

    def command(self, source, processes=1, quit_after=10, kill_delay=1):
        return [
            sys.executable, str(OBSERVER),
            "--case", "fake", "--go", "none", "--target", "fake",
            "--count", "1", "--processes", str(processes),
            "--interval", "0.05", "--quit-after", str(quit_after), "--kill-delay", str(kill_delay),
            "--output-dir", str(self.output_dir),
            "--", sys.executable, "-c", source,
        ]

    def records(self):
        return [json.loads(line) for line in (self.output_dir / "observer.jsonl").read_text().splitlines()]

    def test_exit_and_streams(self):
        result = subprocess.run(self.command("import sys; print('child-out', flush=True); print('child-err', file=sys.stderr, flush=True)"), capture_output=True, text=True, timeout=15)
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout, "")
        self.assertEqual(result.stderr, "")
        self.assertIn("child-out", (self.output_dir / "process-1.stdout.log").read_text())
        self.assertIn("child-err", (self.output_dir / "process-1.stderr.log").read_text())
        records = self.records()
        lifecycle = [r for r in records if r["phase"] in ("start", "exit")]
        self.assertEqual([r["phase"] for r in lifecycle], ["start", "exit"])
        self.assertEqual(lifecycle[-1]["exitCode"], 0)

    def test_nonzero_stops_repetition(self):
        result = subprocess.run(self.command("import time; time.sleep(0.15); raise SystemExit(7)", processes=3), capture_output=True, text=True, timeout=15)
        self.assertEqual(result.returncode, 7)
        records = self.records()
        starts = [r for r in records if r["phase"] == "start"]
        exits = [r for r in records if r["phase"] == "exit"]
        self.assertEqual([r["processNumber"] for r in starts], [1])
        self.assertEqual([r["processNumber"] for r in exits], [1])
        self.assertEqual(exits[0]["exitCode"], 7)

    def test_success_starts_fresh_processes(self):
        result = subprocess.run(self.command("pass", processes=3), capture_output=True, text=True, timeout=15)
        self.assertEqual(result.returncode, 0)
        starts = [r for r in self.records() if r["phase"] == "start"]
        self.assertEqual([r["processNumber"] for r in starts], [1, 2, 3])
        self.assertEqual(len({r["pid"] for r in starts}), 3)

    def test_quit_cannot_be_reported_as_pass(self):
        source = "import signal,time; time.sleep(0.25); signal.signal(signal.SIGQUIT, lambda *args: exit(0)); time.sleep(10)"
        result = subprocess.run(self.command(source, quit_after=2), capture_output=True, text=True, timeout=15)
        self.assertEqual(result.returncode, 124)
        records = self.records()
        self.assertEqual(sum(r["phase"] == "quit" for r in records), 1)
        self.assertEqual(records[-1]["exitCode"], 0)

    def test_quit_then_kill(self):
        source = "import signal,time; time.sleep(0.25); signal.signal(signal.SIGQUIT, signal.SIG_IGN); time.sleep(10)"
        result = subprocess.run(self.command(source, quit_after=2), capture_output=True, text=True, timeout=15)
        self.assertEqual(result.returncode, 124)
        records = self.records()
        self.assertEqual(sum(r["phase"] == "quit" for r in records), 1)
        self.assertEqual(sum(r["phase"] == "kill" for r in records), 1)
        self.assertEqual(records[-1]["exitCode"], -9)

    def test_start_is_journaled_before_exit(self):
        with tempfile.TemporaryDirectory() as directory:
            release = Path(directory) / "release"
            source = f"import pathlib,time; release=pathlib.Path({str(release)!r})\nwhile not release.exists(): time.sleep(0.01)"
            child = subprocess.Popen(self.command(source), stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
            try:
                journal = self.output_dir / "observer.jsonl"
                deadline = time.monotonic() + 5
                while time.monotonic() < deadline:
                    if journal.exists() and journal.read_text().endswith("\n"):
                        break
                    time.sleep(0.01)
                first = self.records()[0]
                self.assertEqual(first["phase"], "start")
                self.assertIsNone(child.poll())
                release.touch()
                _, stderr = child.communicate(timeout=15)
                self.assertEqual(child.returncode, 0, stderr)
            finally:
                release.touch()
                if child.poll() is None:
                    child.kill()
                    child.wait()

    def test_unread_runner_pipe_does_not_block_deadline(self):
        state_file = self.directory / "child.json"
        source = f"""import json,os,pathlib,signal,time
signal.signal(signal.SIGQUIT, signal.SIG_IGN)
pathlib.Path({str(state_file)!r}).write_text(json.dumps({{"pid": os.getpid(), "pgid": os.getpgrp(), "ppid": os.getppid()}}))
os.write(1, b"x" * (1024 * 1024))
time.sleep(60)
"""
        observer = subprocess.Popen(self.command(source, quit_after=2, kill_delay=1), stdout=subprocess.PIPE, stderr=subprocess.DEVNULL)
        started = time.monotonic()
        try:
            self.assertEqual(observer.wait(timeout=6), 124)
            self.assertLess(time.monotonic() - started, 6)
            records = self.records()
            self.assertEqual([r["phase"] for r in records if r["phase"] in ("quit", "kill", "exit")], ["quit", "kill", "exit"])
            self.assertEqual(records[-1]["exitCode"], -signal.SIGKILL)
            self.assertEqual((self.output_dir / "process-1.stdout.log").stat().st_size, 1024 * 1024)
        finally:
            if observer.poll() is None and state_file.exists():
                state = json.loads(state_file.read_text())
                self.assertEqual(state["ppid"], observer.pid)
                self.assertEqual(state["pid"], state["pgid"])
                try:
                    os.killpg(state["pgid"], signal.SIGKILL)
                except ProcessLookupError:
                    pass
            try:
                observer.communicate(timeout=3)
            except subprocess.TimeoutExpired:
                observer.kill()
                observer.communicate(timeout=3)


if __name__ == "__main__":
    unittest.main()

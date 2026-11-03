import json
from pathlib import Path
import subprocess
import sys
import unittest


OBSERVER = Path(__file__).with_name("observe.py")


class ObserverTest(unittest.TestCase):
    def command(self, source, processes=1):
        return [
            sys.executable, str(OBSERVER),
            "--case", "fake", "--go", "none", "--target", "fake",
            "--count", "1", "--processes", str(processes),
            "--interval", "0.05", "--quit-after", "0.2", "--kill-delay", "0.1",
            "--", sys.executable, "-c", source,
        ]

    def records(self, result):
        return [json.loads(line) for line in result.stdout.splitlines() if line.startswith("{")]

    def test_exit_and_streams(self):
        result = subprocess.run(self.command("import sys; print('child-out', flush=True); print('child-err', file=sys.stderr, flush=True)"), capture_output=True, text=True, timeout=3)
        self.assertEqual(result.returncode, 0)
        self.assertIn("child-out", result.stdout)
        self.assertIn("child-err", result.stderr)
        records = self.records(result)
        self.assertEqual([r["phase"] for r in records], ["start", "exit"])
        self.assertEqual(records[-1]["exitCode"], 0)

    def test_nonzero_stops_repetition(self):
        result = subprocess.run(self.command("raise SystemExit(7)", processes=3), capture_output=True, text=True, timeout=3)
        self.assertEqual(result.returncode, 7)
        self.assertEqual([r["processNumber"] for r in self.records(result)], [1, 1])

    def test_success_starts_fresh_processes(self):
        result = subprocess.run(self.command("pass", processes=3), capture_output=True, text=True, timeout=3)
        self.assertEqual(result.returncode, 0)
        starts = [r for r in self.records(result) if r["phase"] == "start"]
        self.assertEqual([r["processNumber"] for r in starts], [1, 2, 3])
        self.assertEqual(len({r["pid"] for r in starts}), 3)

    def test_quit_cannot_be_reported_as_pass(self):
        source = "import signal,time; signal.signal(signal.SIGQUIT, lambda *args: exit(0)); time.sleep(3)"
        result = subprocess.run(self.command(source), capture_output=True, text=True, timeout=3)
        self.assertEqual(result.returncode, 124)
        records = self.records(result)
        self.assertEqual(sum(r["phase"] == "quit" for r in records), 1)
        self.assertEqual(records[-1]["exitCode"], 0)

    def test_quit_then_kill(self):
        source = "import signal,time; signal.signal(signal.SIGQUIT, signal.SIG_IGN); time.sleep(3)"
        result = subprocess.run(self.command(source), capture_output=True, text=True, timeout=3)
        self.assertEqual(result.returncode, 124)
        records = self.records(result)
        self.assertEqual(sum(r["phase"] == "quit" for r in records), 1)
        self.assertEqual(sum(r["phase"] == "kill" for r in records), 1)
        self.assertEqual(records[-1]["exitCode"], -9)

    def test_start_is_flushed_before_exit(self):
        child = subprocess.Popen(self.command("import time; time.sleep(0.15)"), stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        try:
            first = json.loads(child.stdout.readline())
            self.assertEqual(first["phase"], "start")
            self.assertIsNone(child.poll())
            _, stderr = child.communicate(timeout=3)
            self.assertEqual(child.returncode, 0, stderr)
        finally:
            if child.poll() is None:
                child.kill()
                child.wait()


if __name__ == "__main__":
    unittest.main()

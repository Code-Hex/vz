#!/usr/bin/env python3
import collections
import json
import os
import re
import signal
import subprocess
import sys
import tempfile
import threading
import time
import urllib.error
import urllib.request


class Observations:
    def __init__(self):
        self.lock = threading.Lock()
        self.last_test = "なし"
        self.lines = collections.deque(maxlen=12)

    def record(self, line):
        match = re.match(r"^=== RUN\s+([^/\s]+)\s*$", line)
        with self.lock:
            if match:
                self.last_test = match.group(1)[:160]
            if "VZTRACE" in line or re.match(r"^\s*(?:--- FAIL:|FAIL(?:\s|$)|panic:|fatal error:)", line):
                self.lines.append(line.rstrip().encode("utf-8")[-500:].decode("utf-8", errors="ignore"))

    def snapshot(self):
        with self.lock:
            return self.last_test, list(self.lines)


def read_output(stream, path, observations):
    with open(path, "w", encoding="utf-8") as log:
        for line in stream:
            log.write(line)
            log.flush()
            observations.record(line)


def watchdog(process, finished, timeout=360):
    if not finished.wait(timeout):
        try:
            os.killpg(process.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass


def summary(observations, started, exit_code=None):
    last_test, lines = observations.snapshot()
    result = f"経過: {int(time.monotonic() - started)} 秒\n最後のトップレベルテスト: {last_test}"
    if exit_code is not None:
        result += f"\n終了コード: {exit_code}"
    if lines:
        result += "\n直近の診断行:\n" + "\n".join(lines)
    encoded = result.encode("utf-8")
    if len(encoded) > 8192:
        result = encoded[:8192].decode("utf-8", errors="ignore")
    return result


def check_request(method, url, token, body):
    data = json.dumps(body, ensure_ascii=False).encode("utf-8")
    request = urllib.request.Request(
        url,
        data=data,
        method=method,
        headers={
            "Authorization": "Bearer " + token,
            "Accept": "application/vnd.github+json",
            "Content-Type": "application/json",
            "X-GitHub-Api-Version": "2022-11-28",
        },
    )
    with urllib.request.urlopen(request, timeout=5) as response:
        return json.load(response)


def main():
    required = ("GITHUB_TOKEN", "GITHUB_REPOSITORY", "GITHUB_SHA", "GITHUB_RUN_ID", "TRACE_NAME")
    if any(not os.environ.get(key) for key in required):
        print("診断に必要な環境変数がありません。", file=sys.stderr)
        return 1
    if os.environ['GITHUB_REPOSITORY'] != 'Code-Hex/vz' or os.environ.get('GITHUB_REF') != 'refs/heads/codex/diagnose-ci306-intel':
        print('診断専用branch以外では実行できません。', file=sys.stderr)
        return 1

    token = os.environ["GITHUB_TOKEN"]
    repository = os.environ["GITHUB_REPOSITORY"]
    api = "https://api.github.com/repos/" + repository + "/check-runs"
    run_url = "https://github.com/" + repository + "/actions/runs/" + os.environ["GITHUB_RUN_ID"]
    observations = Observations()
    started = time.monotonic()
    try:
        check = check_request("POST", api, token, {
            "name": os.environ["TRACE_NAME"],
            "head_sha": os.environ["GITHUB_SHA"],
            "status": "in_progress",
            "details_url": run_url,
            "output": {"title": "テスト診断", "summary": summary(observations, started)},
        })
        check_url = api + "/" + str(check["id"])
    except (OSError, ValueError, KeyError, urllib.error.URLError):
        print("Check Run を作成できません。テストを開始しません。", file=sys.stderr)
        return 1

    cwd = os.path.realpath(os.getcwd())
    child_env = os.environ.copy()
    for key, value in list(child_env.items()):
        if key in ("GITHUB_TOKEN", "GH_TOKEN", "ACTIONS_RUNTIME_TOKEN") or value == token:
            child_env.pop(key, None)
    child_env["PWD"] = cwd
    command = ["go", "test", "-p", "1", "-exec", "go run " + os.path.join(cwd, "cmd/codesign"),
               ".", "-timeout", "5m", "-v", "-count=2"]
    finished = threading.Event()
    log_dir = os.environ.get("RUNNER_TEMP") or tempfile.gettempdir()
    log_fd, log_path = tempfile.mkstemp(prefix="ci306-", suffix=".log", dir=log_dir)
    os.close(log_fd)
    try:
        process = subprocess.Popen(command, cwd=cwd, env=child_env, stdout=subprocess.PIPE,
                                   stderr=subprocess.STDOUT, text=True, encoding="utf-8", errors="replace",
                                   bufsize=1, start_new_session=True)
    except OSError:
        print("テストプロセスを開始できません。", file=sys.stderr)
        try:
            check_request("PATCH", check_url, token, {"status": "completed", "conclusion": "failure",
                          "output": {"title": "テスト診断", "summary": "テストプロセスを開始できません。"}})
        except (OSError, ValueError, urllib.error.URLError):
            print("Check Run を更新できません。", file=sys.stderr)
        return 1

    reader = threading.Thread(target=read_output, args=(process.stdout, log_path, observations), daemon=True)
    killer = threading.Thread(target=watchdog, args=(process, finished), daemon=True)
    reader.start()
    killer.start()
    next_update = time.monotonic() + 10
    while process.poll() is None:
        if time.monotonic() >= next_update:
            try:
                check_request("PATCH", check_url, token, {"output": {"title": "テスト診断",
                              "summary": summary(observations, started)}})
            except (OSError, ValueError, urllib.error.URLError):
                print("Check Run を更新できません。", file=sys.stderr)
            next_update = time.monotonic() + 10
        time.sleep(0.2)

    finished.set()
    reader.join(timeout=2)
    killer.join(timeout=1)
    exit_code = process.returncode if process.returncode >= 0 else 128 - process.returncode
    final_summary = summary(observations, started, exit_code)
    try:
        check_request("PATCH", check_url, token, {"status": "completed",
                      "conclusion": "success" if exit_code == 0 else "failure",
                      "output": {"title": "テスト診断", "summary": final_summary}})
    except (OSError, ValueError, urllib.error.URLError):
        print("Check Run の最終更新が失敗しました。", file=sys.stderr)
    print(final_summary)
    return exit_code


if __name__ == "__main__":
    sys.exit(main())

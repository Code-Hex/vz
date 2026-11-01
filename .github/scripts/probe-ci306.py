#!/usr/bin/env python3
import collections
import http.client
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
    def __init__(self, started=None):
        self.lock = threading.Lock()
        self.started = time.monotonic() if started is None else started
        self.last_test = "なし"
        self.test_counts = collections.Counter()
        self.test_total = 0
        self.first_test_elapsed = None
        self.last_result = "なし"
        self.lines = collections.deque(maxlen=12)
        self.checkpoint = "なし"

    def record(self, line):
        match = re.match(r"^=== RUN\s+([^/\s]+)\s*$", line)
        with self.lock:
            if match:
                name = match.group(1)[:160]
                self.test_counts[name] += 1
                self.test_total += 1
                if self.first_test_elapsed is None:
                    self.first_test_elapsed = time.monotonic() - self.started
                self.last_test = f"{name} #{self.test_counts[name]}"
            if re.match(r"^(?:ok\s|FAIL(?:\s|$)|--- FAIL:)", line):
                self.last_result = line.rstrip().encode("utf-8")[-500:].decode("utf-8", errors="ignore")
            if "VZTRACE" in line or "network_device_integration_test.go:" in line or "virtualization_test.go:" in line or re.match(r"^\s*(?:--- FAIL:|FAIL(?:\s|$)|panic:|fatal error:)", line):
                self.lines.append(line.rstrip().encode("utf-8")[-500:].decode("utf-8", errors="ignore"))

    def snapshot(self):
        with self.lock:
            return self.last_test, list(self.lines), self.test_total, self.first_test_elapsed, self.last_result

    def set_checkpoint(self, request):
        with self.lock:
            self.checkpoint = f"VZTRACE checkpoint {request['id']} {request['label']}"

    def get_checkpoint(self):
        with self.lock:
            return self.checkpoint


def read_output(stream, path, observations):
    with open(path, "w", encoding="utf-8") as log:
        for line in stream:
            log.write(line)
            log.flush()
            observations.record(line)
            print(line.rstrip(), flush=True)


def watchdog(process, finished, timeout=360):
    if not finished.wait(timeout):
        try:
            os.killpg(process.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass


def summary(observations, started, exit_code=None):
    last_test, lines, total, first_elapsed, last_result = observations.snapshot()
    first = "未観測" if first_elapsed is None else f"{first_elapsed:.1f} 秒"
    result = (f"経過: {int(time.monotonic() - started)} 秒\n最初のテスト開始: {first}\n"
              f"トップレベルテスト開始回数: {total}\n最後のトップレベルテスト: {last_test}\n"
              f"最後の ok/FAIL 行: {last_result}")
    if exit_code != 0:
        result += f"\n{observations.get_checkpoint()}"
    if exit_code is not None:
        result += f"\n終了コード: {exit_code}"
    if lines and exit_code != 0:
        result += "\n直近の診断行:\n" + "\n".join(lines)
    encoded = result.encode("utf-8")
    if len(encoded) > 8192:
        result = encoded[:8192].decode("utf-8", errors="ignore")
    return result


def read_checkpoint(directory):
    try:
        with open(os.path.join(directory, "request.json"), encoding="utf-8") as request_file:
            request = json.load(request_file)
    except FileNotFoundError:
        return None
    if not isinstance(request.get("id"), int) or not isinstance(request.get("label"), str):
        raise ValueError("invalid checkpoint request")
    return request


def write_ack(directory, checkpoint_id):
    tmp = os.path.join(directory, f"ack-{checkpoint_id}.tmp")
    with open(tmp, "w", encoding="utf-8") as ack:
        ack.write(str(checkpoint_id))
    os.replace(tmp, os.path.join(directory, "ack"))


def check_request(method, url, token, body):
    configuration = '\n'.join([
        'header = "Authorization: Bearer ' + token + '"',
        'header = "Accept: application/vnd.github+json"',
        'header = "Content-Type: application/json"',
        'header = "X-GitHub-Api-Version: 2022-11-28"',
    ])
    response = subprocess.run([
        '/usr/bin/curl', '--silent', '--show-error', '--fail',
        '--connect-timeout', '3', '--max-time', '6',
        '--request', method, '--config', '-',
        '--data-binary', json.dumps(body, ensure_ascii=False), url,
    ], input=configuration, text=True, stdout=subprocess.PIPE,
        stderr=subprocess.PIPE, timeout=8)
    if response.returncode:
        raise OSError('Check request failed with curl exit ' + str(response.returncode))
    return json.loads(response.stdout)


def report_error(context, error):
    detail = type(error).__name__
    if isinstance(error, urllib.error.HTTPError):
        detail += f" status={error.code}"
    print(f"{context}: {detail}", file=sys.stderr)


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
    started = time.monotonic()
    observations = Observations(started)
    try:
        check = check_request("POST", api, token, {
            "name": os.environ["TRACE_NAME"],
            "head_sha": os.environ["GITHUB_SHA"],
            "status": "in_progress",
            "details_url": run_url,
            "output": {"title": "テスト診断", "summary": summary(observations, started)},
        })
        check_url = api + "/" + str(check["id"])
    except (OSError, ValueError, KeyError, urllib.error.URLError, http.client.HTTPException) as error:
        report_error("Check Run 作成失敗。テスト未開始", error)
        return 1

    cwd = os.path.realpath(os.environ.get("TRACE_SOURCE_DIR") or os.getcwd())
    child_env = os.environ.copy()
    for key, value in list(child_env.items()):
        if key in ("GITHUB_TOKEN", "GH_TOKEN", "ACTIONS_RUNTIME_TOKEN") or value == token:
            child_env.pop(key, None)
    child_env["PWD"] = cwd
    log_dir = os.environ.get("RUNNER_TEMP") or tempfile.gettempdir()
    child_env.pop("CI306_BARRIER_DIR", None)
    barrier_dir = None
    if os.environ.get("CI306_NO_BARRIERS") != "1":
        barrier_dir = tempfile.mkdtemp(prefix="ci306-barrier-", dir=log_dir)
        child_env["CI306_BARRIER_DIR"] = barrier_dir
    command = ["go", "test", "-p", "1", "-exec", "go run " + os.path.join(cwd, "cmd/codesign"),
               ".", "-timeout", "5m", "-v", "-run", os.environ.get("TRACE_PATTERN", "^TestRuntimeNetworkConnection$"),
               "-count=" + os.environ.get("TRACE_COUNT", "5")]
    if os.environ.get('TRACE_NATIVE') == '1':
        command = ['/bin/bash', '-c', 'for ((i=1;i<=10;i++)); do "$1" "$2" "$3" || exit $?; done', 'native-probe',
                   os.path.join(os.environ['RUNNER_TEMP'], 'native-ci306'), os.path.join(cwd, 'testdata/Image'),
                   os.path.join(cwd, 'testdata/initramfs.cpio.gz')]
    if os.environ.get('TRACE_NATIVE_PAIR') == '1':
        command = [os.path.join(os.environ['RUNNER_TEMP'], 'native-pair-ci306'),
                   os.path.join(cwd, 'testdata/Image'), os.path.join(cwd, 'testdata/initramfs.cpio.gz')]
    finished = threading.Event()
    log_fd, log_path = tempfile.mkstemp(prefix="ci306-", suffix=".log", dir=log_dir)
    os.close(log_fd)
    try:
        process = subprocess.Popen(command, cwd=cwd, env=child_env, stdout=subprocess.PIPE,
                                   stderr=subprocess.STDOUT, text=True, encoding="utf-8", errors="replace",
                                   bufsize=1, start_new_session=True)
    except OSError as error:
        report_error("テストプロセス開始失敗", error)
        try:
            check_request("PATCH", check_url, token, {"status": "completed", "conclusion": "failure",
                          "output": {"title": "テスト診断", "summary": "テストプロセスを開始できません。"}})
        except (OSError, ValueError, urllib.error.URLError, http.client.HTTPException) as error:
            report_error("Check Run 更新失敗", error)
        return 1

    reader = None
    killer = None
    try:
        reader = threading.Thread(target=read_output, args=(process.stdout, log_path, observations), daemon=True)
        killer = threading.Thread(target=watchdog, args=(process, finished), daemon=True)
        reader.start()
        killer.start()
        last_snapshot = None
        last_update = time.monotonic()
        acknowledged = 0
        while process.poll() is None:
            try:
                checkpoint = read_checkpoint(barrier_dir) if barrier_dir is not None else None
                if checkpoint is not None and checkpoint["id"] > acknowledged:
                    observations.set_checkpoint(checkpoint)
                    check_request("PATCH", check_url, token, {"output": {"title": "テスト診断",
                                  "summary": summary(observations, started)}})
                    write_ack(barrier_dir, checkpoint["id"])
                    acknowledged = checkpoint["id"]
                    last_snapshot = observations.snapshot()
                    last_update = time.monotonic()
            except (OSError, ValueError, urllib.error.URLError, http.client.HTTPException) as error:
                report_error("診断バリア更新失敗。ack 保留", error)
            now = time.monotonic()
            current = observations.snapshot()
            if now - last_update >= 10 or (current != last_snapshot and now - last_update >= 1):
                try:
                    check_request("PATCH", check_url, token, {"output": {"title": "テスト診断",
                                  "summary": summary(observations, started)}})
                    last_snapshot = current
                except (OSError, ValueError, urllib.error.URLError, http.client.HTTPException) as error:
                    report_error("Check Run 更新失敗", error)
                last_update = time.monotonic()
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
        except (OSError, ValueError, urllib.error.URLError, http.client.HTTPException) as error:
            report_error("Check Run 最終更新失敗", error)
        print(final_summary)
        return exit_code
    finally:
        try:
            os.killpg(process.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
        try:
            process.wait(timeout=2)
        except subprocess.TimeoutExpired:
            print("診断子プロセスの終了待機がタイムアウトしました。", file=sys.stderr)
        finished.set()
        if reader is not None and reader.ident is not None:
            reader.join(timeout=2)
        if killer is not None and killer.ident is not None:
            killer.join(timeout=1)



if __name__ == "__main__":
    try:
        sys.exit(main())
    except Exception as error:
        report_error("予期しない診断エラー", error)
        sys.exit(1)

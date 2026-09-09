"""Codex subscription-CLI executor adapter.

Dict shapes follow src/praxis_contracts/schemas/v1/capability-advertisement.schema.json and
src/praxis_contracts/schemas/v1/capability.schema.json.
"""

from __future__ import annotations

import os
import re
import shutil
import subprocess
import uuid

from praxis_executors.interface import (
    Executor,
    ExecutionHandle,
    ExecutionRequest,
    ExecutionResult,
    ExecutorAvailability,
    ExecutorError,
    ExecutorStatus,
)

_SPEC_VERSION = "1.0.0"
_CLI_NAME = "codex"

_CREDENTIAL_PATTERN = re.compile(r"sk-[A-Za-z0-9_-]{20,}")
_REDACTED = "***REDACTED***"


def _redact(text: str) -> str:
    return _CREDENTIAL_PATTERN.sub(_REDACTED, text)


# Investigation (this session, `codex --help`/`codex exec --help` on the real
# `/opt/homebrew/bin/codex` binary, version 0.153.4): beyond `OPENAI_API_KEY`,
# a search of the binary's embedded strings surfaces `OPENAI_ORGANIZATION`,
# `OPENAI_PROJECT`, and `OPENAI_BASE_URL` as the org/project-scoping and
# base-URL-override variables the plan asked to look for. These route
# requests to OpenAI's metered API the same way `OPENAI_API_KEY` does, so
# they are stripped alongside it.
_ENV_VARS_TO_STRIP = ("OPENAI_API_KEY", "OPENAI_ORGANIZATION", "OPENAI_PROJECT", "OPENAI_BASE_URL")


def _subprocess_env() -> dict[str, str]:
    env = os.environ.copy()
    for key in _ENV_VARS_TO_STRIP:
        env.pop(key, None)
    return env


class CodexCliExecutor(Executor):
    """Runs prompts through the locally-installed `codex` subscription CLI as a subprocess."""

    def __init__(self, executor_id: str) -> None:
        self._executor_id = executor_id
        self._processes: dict[str, subprocess.Popen] = {}
        self._results: dict[str, ExecutionResult] = {}
        self._cancelled: set[str] = set()

    def capabilities(self) -> dict:
        return {
            "spec_version": _SPEC_VERSION,
            "executor_id": self._executor_id,
            "capabilities": [
                {
                    "spec_version": _SPEC_VERSION,
                    "satisfies": [
                        {"kind": "coding"},
                        {"kind": "shell"},
                        {"kind": "filesystem"},
                    ],
                    "auth_transport": "subscription_cli",
                }
            ],
        }

    def health(self) -> ExecutorAvailability:
        cli_path = shutil.which(_CLI_NAME)
        if cli_path is None:
            return ExecutorAvailability.UNAVAILABLE
        self._probe_version(cli_path)
        authenticated = self._detect_authenticated(cli_path)
        if authenticated is False:
            # Deliberately no fallback branch here: an unauthenticated CLI
            # must resolve straight to UNAVAILABLE, never a metered API key.
            return ExecutorAvailability.UNAVAILABLE
        if authenticated is True:
            return ExecutorAvailability.AVAILABLE
        return ExecutorAvailability.DEGRADED

    def _probe_version(self, cli_path: str) -> None:
        try:
            subprocess.run([cli_path, "--version"], capture_output=True, text=True, timeout=5)
        except (OSError, subprocess.TimeoutExpired):
            pass

    def _detect_authenticated(self, cli_path: str) -> bool | None:
        # Investigation (this session, real `codex` binary on PATH): `codex
        # --help` documents no `codex auth` subcommand, but `codex login
        # status` is a real, safe, side-effect-free subcommand that reports
        # auth state -- it only re-reads the on-disk credential file
        # (~/.codex/auth.json) and returned in ~20ms when run live here,
        # printing "Logged in using ChatGPT" to stderr with exit code 0.
        # `codex login status --help` documents no `--json` flag and no
        # dedicated exit-code contract for the unauthenticated case, so the
        # unauthenticated branch is identified from the "Not logged in"
        # string embedded in the same binary rather than a live logout
        # (logging out would revoke this environment's real credentials).
        # Both states are therefore distinguished by matching that text.
        try:
            probe = subprocess.run(
                [cli_path, "login", "status"], capture_output=True, text=True, timeout=5
            )
        except (OSError, subprocess.TimeoutExpired):
            return None
        output = f"{probe.stdout}\n{probe.stderr}".lower()
        if "not logged in" in output:
            return False
        if "logged in" in output:
            return True
        return None

    def launch(self, request: ExecutionRequest) -> ExecutionHandle:
        if "prompt" not in request.parameters:
            raise ExecutorError("request.parameters is missing required key 'prompt'")
        cli_path = shutil.which(_CLI_NAME)
        if cli_path is None:
            raise ExecutorError("codex CLI is not available on PATH")
        extra_args = request.parameters.get("extra_args", [])
        # `codex exec` is Codex's documented non-interactive one-shot
        # subcommand (confirmed via `codex exec --help` on the real binary
        # in this session), mirroring `claude_cli.py`'s `-p` invocation.
        argv = [cli_path, "exec", request.parameters["prompt"], *extra_args]
        try:
            process = subprocess.Popen(
                argv,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                env=_subprocess_env(),
            )
        except OSError as exc:
            raise ExecutorError(f"failed to launch codex CLI: {_redact(str(exc))}") from exc
        handle_id = uuid.uuid4().hex
        self._processes[handle_id] = process
        return ExecutionHandle(handle_id=handle_id)

    def _process_for(self, handle: ExecutionHandle) -> subprocess.Popen:
        process = self._processes.get(handle.handle_id)
        if process is None:
            raise ExecutorError(f"unknown execution handle: {handle.handle_id!r}")
        return process

    def status(self, handle: ExecutionHandle) -> ExecutorStatus:
        returncode = self._process_for(handle).poll()
        if returncode is None:
            return ExecutorStatus.RUNNING
        return self._terminal_status(handle.handle_id, returncode)

    def _terminal_status(self, handle_id: str, returncode: int) -> ExecutorStatus:
        if handle_id in self._cancelled:
            return ExecutorStatus.CANCELLED
        return ExecutorStatus.SUCCEEDED if returncode == 0 else ExecutorStatus.FAILED

    def cancel(self, handle: ExecutionHandle) -> None:
        process = self._process_for(handle)
        if process.poll() is None:
            self._cancelled.add(handle.handle_id)
            process.terminate()

    def result(self, handle: ExecutionHandle) -> ExecutionResult:
        process = self._process_for(handle)
        cached = self._results.get(handle.handle_id)
        if cached is not None:
            return cached
        if process.poll() is None:
            raise ExecutorError("cannot fetch result while execution is still RUNNING")
        stdout, stderr = process.communicate()
        returncode = process.returncode
        result = ExecutionResult(
            status=self._terminal_status(handle.handle_id, returncode),
            evidence={"process-exit-status": returncode == 0},
            payload={
                "stdout": _redact(stdout),
                "stderr": _redact(stderr),
                "returncode": returncode,
            },
        )
        self._results[handle.handle_id] = result
        return result

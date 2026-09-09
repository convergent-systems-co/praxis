"""Claude subscription-CLI executor adapter.

Dict shapes follow src/praxis_contracts/schemas/v1/capability-advertisement.schema.json and
src/praxis_contracts/schemas/v1/capability.schema.json.
"""

from __future__ import annotations

import json
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
_CLI_NAME = "claude"

_CREDENTIAL_PATTERN = re.compile(r"sk-ant-[A-Za-z0-9_-]{10,}")
_REDACTED = "***REDACTED***"


def _redact(text: str) -> str:
    return _CREDENTIAL_PATTERN.sub(_REDACTED, text)


class ClaudeCliExecutor(Executor):
    """Runs prompts through the locally-installed `claude` subscription CLI as a subprocess."""

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
                        {"kind": "reasoning"},
                        {"kind": "tools"},
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
        try:
            result = subprocess.run(
                [cli_path, "auth", "status", "--json"],
                capture_output=True,
                text=True,
                timeout=5,
            )
            return json.loads(result.stdout)["loggedIn"]
        except (
            subprocess.TimeoutExpired,
            OSError,
            json.JSONDecodeError,
            KeyError,
            TypeError,
        ):
            return None

    def launch(self, request: ExecutionRequest) -> ExecutionHandle:
        if "prompt" not in request.parameters:
            raise ExecutorError("request.parameters is missing required key 'prompt'")
        cli_path = shutil.which(_CLI_NAME)
        if cli_path is None:
            raise ExecutorError("claude CLI is not available on PATH")
        extra_args = request.parameters.get("extra_args", [])
        argv = [cli_path, "-p", request.parameters["prompt"], *extra_args]
        try:
            process = subprocess.Popen(
                argv,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
            )
        except OSError as exc:
            raise ExecutorError(f"failed to launch claude CLI: {_redact(str(exc))}") from exc
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

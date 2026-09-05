"""Subprocess executor adapter: the first real (non-fake) Executor backend.

Dict shapes follow schemas/v1/capability-advertisement.schema.json and
schemas/v1/capability.schema.json.
"""

from __future__ import annotations

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


class SubprocessExecutor(Executor):
    """Runs request.parameters["command"] (list[str]) as a real OS subprocess. Advertises the
    capability kind(s) given at construction time -- generic, no vendor/model coupling."""

    def __init__(self, executor_id: str, satisfies_kinds: list[str]) -> None:
        self._executor_id = executor_id
        self._satisfies_kinds = list(satisfies_kinds)
        self._processes: dict[str, subprocess.Popen] = {}
        self._results: dict[str, ExecutionResult] = {}

    def capabilities(self) -> dict:
        return {
            "spec_version": _SPEC_VERSION,
            "executor_id": self._executor_id,
            "capabilities": [
                {
                    "spec_version": _SPEC_VERSION,
                    "satisfies": [{"kind": kind} for kind in self._satisfies_kinds],
                }
            ],
        }

    def health(self) -> ExecutorAvailability:
        # A subprocess launcher has no external dependency (network, API, ...)
        # to be degraded against, so it is always reported as available.
        return ExecutorAvailability.AVAILABLE

    def launch(self, request: ExecutionRequest) -> ExecutionHandle:
        if "command" not in request.parameters:
            raise ExecutorError("request.parameters is missing required key 'command'")
        command = request.parameters["command"]
        try:
            process = subprocess.Popen(
                command,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
            )
        except OSError as exc:
            raise ExecutorError(f"failed to launch command {command!r}: {exc}") from exc
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
        return ExecutorStatus.SUCCEEDED if returncode == 0 else ExecutorStatus.FAILED

    def cancel(self, handle: ExecutionHandle) -> None:
        process = self._process_for(handle)
        if process.poll() is None:
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
            status=ExecutorStatus.SUCCEEDED if returncode == 0 else ExecutorStatus.FAILED,
            evidence={"process-exit-status": returncode == 0},
            payload={"stdout": stdout, "stderr": stderr, "returncode": returncode},
        )
        self._results[handle.handle_id] = result
        return result

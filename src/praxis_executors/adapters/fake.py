"""Deterministic fake capability executor adapter.

Dict shapes follow schemas/v1/capability-advertisement.schema.json and
schemas/v1/capability.schema.json.

`script` maps an opaque request key (the caller-chosen
`request.parameters["request_key"]`, or the promise `kind` if absent) to a
fully predetermined ExecutionResult -- no randomness, no wall-clock, no
external call, matching the determinism guarantee of
praxis_runtime.testing.fake_executor.FakeExecutor.
"""

from __future__ import annotations

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


class FakeCapabilityExecutor(Executor):
    """Executor backed by a fixed script of request key -> ExecutionResult."""

    def __init__(
        self, executor_id: str, capabilities: list[dict], script: dict[str, ExecutionResult]
    ) -> None:
        self._executor_id = executor_id
        self._capabilities = list(capabilities)
        self._script = dict(script)
        self._results: dict[str, ExecutionResult] = {}

    def capabilities(self) -> dict:
        return {
            "spec_version": _SPEC_VERSION,
            "executor_id": self._executor_id,
            "capabilities": self._capabilities,
        }

    def health(self) -> ExecutorAvailability:
        return ExecutorAvailability.AVAILABLE

    def launch(self, request: ExecutionRequest) -> ExecutionHandle:
        request_key = request.parameters.get("request_key", request.promise.get("kind"))
        if request_key not in self._script:
            raise ExecutorError(f"no scripted result for request key: {request_key!r}")
        handle_id = uuid.uuid4().hex
        self._results[handle_id] = self._script[request_key]
        return ExecutionHandle(handle_id=handle_id)

    def _result_for(self, handle: ExecutionHandle) -> ExecutionResult:
        result = self._results.get(handle.handle_id)
        if result is None:
            raise ExecutorError(f"unknown execution handle: {handle.handle_id!r}")
        return result

    def status(self, handle: ExecutionHandle) -> ExecutorStatus:
        return self._result_for(handle).status

    def cancel(self, handle: ExecutionHandle) -> None:
        self._result_for(handle)

    def result(self, handle: ExecutionHandle) -> ExecutionResult:
        return self._result_for(handle)

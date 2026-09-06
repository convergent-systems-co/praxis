"""Executor registry: register adapters, select one for a requirement, and
drive a selected adapter's launch/poll/result lifecycle.

This module has no dependency on praxis_runtime. `ExecutionResult.evidence`
is a flat claim dict, not the `list[dict]` of proof-record documents
`TransitionEngine.apply(node_id, event_type, evidence=...)` requires; a
caller with run/graph/node context must convert it first (see
`ExecutionResult`'s docstring in `interface.py`) -- wiring that call and
conversion is the caller's responsibility, not the registry's.
"""

from __future__ import annotations

from typing import Callable

from . import matching
from .interface import Executor, ExecutionRequest, ExecutionResult, ExecutorAvailability, ExecutorStatus

_TERMINAL_STATUSES = frozenset(
    {ExecutorStatus.SUCCEEDED, ExecutorStatus.FAILED, ExecutorStatus.CANCELLED}
)


class RegistryError(Exception):
    """Raised for registry-level failures: identity conflicts or no selection."""


class ExecutorRegistry:
    """Tracks registered Executors and mediates selection and execution."""

    def __init__(self) -> None:
        self._executors: dict[str, Executor] = {}

    def register(self, executor_id: str, executor: Executor) -> None:
        if executor_id in self._executors:
            raise RegistryError(f"executor_id '{executor_id}' is already registered")
        self._executors[executor_id] = executor

    def unregister(self, executor_id: str) -> None:
        self._executors.pop(executor_id, None)

    def advertisements(self, *, healthy_only: bool = True) -> list[dict]:
        result = []
        for executor_id, executor in self._executors.items():
            try:
                health = executor.health()
            except Exception:
                continue
            if healthy_only and health is not ExecutorAvailability.AVAILABLE:
                continue
            result.append(executor.capabilities())
        return result

    def select(
        self,
        requirement: dict,
        *,
        is_eligible: Callable[[str], bool] | None = None,
    ) -> matching.MatchResult:
        return matching.match(requirement, self.advertisements(), is_eligible=is_eligible)

    def execute(
        self,
        requirement: dict,
        request: ExecutionRequest,
        *,
        is_eligible: Callable[[str], bool] | None = None,
        poll: Callable[[], None] | None = None,
    ) -> ExecutionResult:
        result = self.select(requirement, is_eligible=is_eligible)
        if result.selected is None:
            raise RegistryError(
                f"no executor selected for requirement: {result.unsatisfied!r}"
            )

        executor = self._executors[result.selected.executor_id]
        handle = executor.launch(request)

        while True:
            current_status = executor.status(handle)
            if current_status in _TERMINAL_STATUSES:
                break
            if poll is not None:
                poll()

        return executor.result(handle)

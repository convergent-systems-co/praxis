"""Tests for T2 (remediation-1): ExecutorRegistry.select() must default to
policy.AuthTransportPolicy() eligibility when no `is_eligible` is supplied,
so `select()`/`execute()`/`execute_with_proof_records()` never pick an
executor whose advertisement lacks `auth_transport` or advertises an
unsafe-by-default transport (`metered_api`/`api_key`) unless a caller
explicitly opts in via its own `is_eligible` callable.

This file is intentionally separate from tests/test_executor_registry.py
(owned by a concurrent task in this bundle, which updates the pre-existing
registry tests that assumed no default policy) to avoid touching that
task's footprint.
"""

from __future__ import annotations

from praxis_executors.interface import (
    Executor,
    ExecutionHandle,
    ExecutionRequest,
    ExecutionResult,
    ExecutorAvailability,
    ExecutorStatus,
)
from praxis_executors.registry import ExecutorRegistry


class _StubExecutor(Executor):
    """Minimal Executor test double: only capabilities()/health() matter here."""

    def __init__(self, executor_id: str, advertisement: dict) -> None:
        self._executor_id = executor_id
        self._advertisement = advertisement

    def capabilities(self) -> dict:
        return self._advertisement

    def health(self) -> ExecutorAvailability:
        return ExecutorAvailability.AVAILABLE

    def launch(self, request: ExecutionRequest) -> ExecutionHandle:
        raise AssertionError("launch() must not be called: select() should reject first")

    def status(self, handle: ExecutionHandle) -> ExecutorStatus:
        raise AssertionError("status() must not be called: select() should reject first")

    def cancel(self, handle: ExecutionHandle) -> None:
        raise AssertionError("cancel() must not be called: select() should reject first")

    def result(self, handle: ExecutionHandle) -> ExecutionResult:
        raise AssertionError("result() must not be called: select() should reject first")


def _advertisement(executor_id: str, *, auth_transport: str | None) -> dict:
    capability = {
        "spec_version": "1.0.0",
        "id": f"cap-{executor_id}",
        "satisfies": [{"kind": "kind-a"}],
    }
    if auth_transport is not None:
        capability["auth_transport"] = auth_transport
    return {
        "spec_version": "1.0.0",
        "executor_id": executor_id,
        "capabilities": [capability],
    }


def _requirement() -> dict:
    return {
        "spec_version": "1.0.0",
        "requirements": [
            {
                "promise": {"spec_version": "1.0.0", "kind": "kind-a"},
                "constraint": "required",
            }
        ],
    }


def test_select_default_eligibility_rejects_executor_missing_auth_transport():
    registry = ExecutorRegistry()
    registry.register(
        "executor-no-auth-transport",
        _StubExecutor(
            "executor-no-auth-transport",
            _advertisement("executor-no-auth-transport", auth_transport=None),
        ),
    )

    result = registry.select(_requirement())

    assert result.selected is None


def test_select_default_eligibility_rejects_metered_api_advertisement():
    registry = ExecutorRegistry()
    registry.register(
        "executor-metered",
        _StubExecutor(
            "executor-metered",
            _advertisement("executor-metered", auth_transport="metered_api"),
        ),
    )

    result = registry.select(_requirement())

    assert result.selected is None

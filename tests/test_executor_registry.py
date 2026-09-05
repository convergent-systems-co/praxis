"""Tests for ExecutorRegistry: register/unregister identity rules, health-gated
advertisements, select() as a thin pass-through to matching.match, and
execute()'s launch/poll/result lifecycle and error-propagation behavior.

Uses a hand-rolled `_ScriptedExecutor` test double rather than
FakeCapabilityExecutor (src/praxis_executors/adapters/fake.py), because that
adapter's health() is hardcoded AVAILABLE and its status()/result() are
single-shot terminal-on-launch (see its module docstring) -- this suite needs
independent control of health() (including making it raise) and a multi-step
status() sequence (QUEUED -> RUNNING -> SUCCEEDED) to exercise the registry's
poll loop.
"""

from __future__ import annotations

import pytest

from praxis_executors.interface import (
    Executor,
    ExecutionHandle,
    ExecutionRequest,
    ExecutionResult,
    ExecutorAvailability,
    ExecutorStatus,
)
from praxis_executors.registry import ExecutorRegistry, RegistryError


def _advertisement(executor_id: str, kind: str) -> dict:
    return {
        "spec_version": "1.0.0",
        "executor_id": executor_id,
        "capabilities": [
            {
                "spec_version": "1.0.0",
                "id": f"cap-{executor_id}",
                "satisfies": [{"kind": kind}],
            }
        ],
    }


def _requirement(kind: str) -> dict:
    return {
        "spec_version": "1.0.0",
        "requirements": [
            {
                "promise": {"spec_version": "1.0.0", "kind": kind},
                "constraint": "required",
            }
        ],
    }


class _HealthError(Exception):
    """Distinct exception type so tests can tell 'raised' apart from any
    exception the registry itself might raise for other reasons."""


class _ScriptedExecutor(Executor):
    """Executor test double with fully controllable health/status/result."""

    def __init__(
        self,
        executor_id: str,
        *,
        advertisement: dict,
        health: ExecutorAvailability | Exception = ExecutorAvailability.AVAILABLE,
        status_sequence: list[ExecutorStatus] | None = None,
        final_result: ExecutionResult | None = None,
    ) -> None:
        self.executor_id = executor_id
        self._advertisement = advertisement
        self._health = health
        self._status_sequence = list(status_sequence or [ExecutorStatus.SUCCEEDED])
        self._final_result = final_result
        self.launch_calls: list[ExecutionRequest] = []
        self.status_calls = 0

    def capabilities(self) -> dict:
        return self._advertisement

    def health(self) -> ExecutorAvailability:
        if isinstance(self._health, Exception):
            raise self._health
        return self._health

    def launch(self, request: ExecutionRequest) -> ExecutionHandle:
        self.launch_calls.append(request)
        return ExecutionHandle(handle_id=f"handle-{self.executor_id}")

    def status(self, handle: ExecutionHandle) -> ExecutorStatus:
        index = min(self.status_calls, len(self._status_sequence) - 1)
        outcome = self._status_sequence[index]
        self.status_calls += 1
        return outcome

    def cancel(self, handle: ExecutionHandle) -> None:
        pass

    def result(self, handle: ExecutionHandle) -> ExecutionResult:
        return self._final_result


def test_register_raises_registry_error_on_duplicate_executor_id():
    registry = ExecutorRegistry()
    first = _ScriptedExecutor("executor-a", advertisement=_advertisement("executor-a", "kind-a"))
    second = _ScriptedExecutor("executor-a", advertisement=_advertisement("executor-a", "kind-a"))
    registry.register("executor-a", first)

    with pytest.raises(RegistryError):
        registry.register("executor-a", second)


def test_unregister_then_register_same_id_succeeds():
    registry = ExecutorRegistry()
    executor = _ScriptedExecutor("executor-a", advertisement=_advertisement("executor-a", "kind-a"))
    registry.register("executor-a", executor)

    registry.unregister("executor-a")
    registry.register("executor-a", executor)  # must not raise: id was actually freed

    ids = {advertisement["executor_id"] for advertisement in registry.advertisements(healthy_only=False)}
    assert ids == {"executor-a"}


def test_advertisements_healthy_only_true_excludes_non_available():
    registry = ExecutorRegistry()
    registry.register(
        "executor-available",
        _ScriptedExecutor(
            "executor-available",
            advertisement=_advertisement("executor-available", "kind-a"),
            health=ExecutorAvailability.AVAILABLE,
        ),
    )
    registry.register(
        "executor-degraded",
        _ScriptedExecutor(
            "executor-degraded",
            advertisement=_advertisement("executor-degraded", "kind-a"),
            health=ExecutorAvailability.DEGRADED,
        ),
    )
    registry.register(
        "executor-unavailable",
        _ScriptedExecutor(
            "executor-unavailable",
            advertisement=_advertisement("executor-unavailable", "kind-a"),
            health=ExecutorAvailability.UNAVAILABLE,
        ),
    )

    healthy_ids = {ad["executor_id"] for ad in registry.advertisements(healthy_only=True)}

    assert healthy_ids == {"executor-available"}


def test_advertisements_healthy_only_false_includes_non_available():
    registry = ExecutorRegistry()
    registry.register(
        "executor-degraded",
        _ScriptedExecutor(
            "executor-degraded",
            advertisement=_advertisement("executor-degraded", "kind-a"),
            health=ExecutorAvailability.DEGRADED,
        ),
    )

    all_ids = {ad["executor_id"] for ad in registry.advertisements(healthy_only=False)}

    assert all_ids == {"executor-degraded"}


def test_advertisements_excludes_executor_whose_health_call_raises():
    registry = ExecutorRegistry()
    registry.register(
        "executor-broken",
        _ScriptedExecutor(
            "executor-broken",
            advertisement=_advertisement("executor-broken", "kind-a"),
            health=_HealthError("probe failed"),
        ),
    )
    registry.register(
        "executor-fine",
        _ScriptedExecutor(
            "executor-fine",
            advertisement=_advertisement("executor-fine", "kind-a"),
            health=ExecutorAvailability.AVAILABLE,
        ),
    )

    # Must not propagate _HealthError -- a health probe failure excludes only
    # that one executor, not the whole registry call.
    healthy_ids = {ad["executor_id"] for ad in registry.advertisements(healthy_only=True)}

    assert healthy_ids == {"executor-fine"}


def test_select_returns_match_result_matching_direct_match_call():
    from praxis_executors.matching import match

    registry = ExecutorRegistry()
    registry.register(
        "executor-a",
        _ScriptedExecutor("executor-a", advertisement=_advertisement("executor-a", "kind-a")),
    )
    registry.register(
        "executor-b",
        _ScriptedExecutor("executor-b", advertisement=_advertisement("executor-b", "kind-b")),
    )
    requirement = _requirement("kind-a")

    result = registry.select(requirement)

    expected = match(requirement, registry.advertisements())
    assert result == expected
    assert result.selected is not None
    assert result.selected.executor_id == "executor-a"


def test_select_honors_is_eligible_callback():
    registry = ExecutorRegistry()
    registry.register(
        "executor-a",
        _ScriptedExecutor("executor-a", advertisement=_advertisement("executor-a", "kind-a")),
    )
    requirement = _requirement("kind-a")

    result = registry.select(requirement, is_eligible=lambda executor_id: False)

    assert result.selected is None


def test_execute_raises_registry_error_embedding_unsatisfied_when_no_selection():
    registry = ExecutorRegistry()
    registry.register(
        "executor-a",
        _ScriptedExecutor("executor-a", advertisement=_advertisement("executor-a", "kind-a")),
    )
    requirement = _requirement("kind-missing")
    request = ExecutionRequest(promise={"spec_version": "1.0.0", "kind": "kind-missing"})

    with pytest.raises(RegistryError) as exc_info:
        registry.execute(requirement, request)

    result = registry.select(requirement)
    assert result.selected is None
    assert result.unsatisfied  # sanity: the fixture really is unsatisfiable
    for unsatisfied_promise in result.unsatisfied:
        assert str(unsatisfied_promise) in str(exc_info.value) or repr(unsatisfied_promise) in str(
            exc_info.value
        )


def test_execute_launches_polls_to_terminal_and_returns_result_unchanged():
    scripted_result = ExecutionResult(
        status=ExecutorStatus.SUCCEEDED,
        evidence={"process-exit-status": True},
        payload={"output": "done"},
    )
    executor = _ScriptedExecutor(
        "executor-a",
        advertisement=_advertisement("executor-a", "kind-a"),
        status_sequence=[ExecutorStatus.QUEUED, ExecutorStatus.RUNNING, ExecutorStatus.SUCCEEDED],
        final_result=scripted_result,
    )
    registry = ExecutorRegistry()
    registry.register("executor-a", executor)
    requirement = _requirement("kind-a")
    request = ExecutionRequest(promise={"spec_version": "1.0.0", "kind": "kind-a"})

    poll_calls = []
    result = registry.execute(requirement, request, poll=lambda: poll_calls.append(None))

    assert result == scripted_result
    assert executor.launch_calls == [request]
    # Loop must run until it observes the terminal SUCCEEDED status, i.e. once
    # per entry in the scripted sequence -- not stop early on QUEUED/RUNNING.
    assert executor.status_calls == 3
    # The loop polls status() once per non-terminal entry (QUEUED, RUNNING)
    # and calls poll() after each; it breaks before polling again once
    # status() reports the terminal SUCCEEDED, so poll() runs exactly twice.
    assert len(poll_calls) == 2


def test_execute_works_without_a_poll_callback():
    scripted_result = ExecutionResult(status=ExecutorStatus.SUCCEEDED)
    executor = _ScriptedExecutor(
        "executor-a",
        advertisement=_advertisement("executor-a", "kind-a"),
        status_sequence=[ExecutorStatus.SUCCEEDED],
        final_result=scripted_result,
    )
    registry = ExecutorRegistry()
    registry.register("executor-a", executor)
    requirement = _requirement("kind-a")
    request = ExecutionRequest(promise={"spec_version": "1.0.0", "kind": "kind-a"})

    result = registry.execute(requirement, request)

    assert result == scripted_result

"""Tests for `praxis run`'s executor dispatch (`praxis_cli.run_dispatch`):
registry construction, the `--executor auto` path, the explicit
`--executor <id>` refusals, and the explicit dispatch path.

Every test injects its own adapters rather than calling
`praxis_cli.adapters.build_adapters()`: the shipped adapters probe a real
`claude`/`ollama`/subprocess environment, so a suite built on them asserts
about the machine it runs on rather than about dispatch. `FakeCapabilityExecutor`
(src/praxis_executors/adapters/fake.py) is the injected adapter, driven by a
real `script` keyed on the promise `kind`, so `launch`/`status`/`result` walk
the same lifecycle a real adapter does. `_RecordingExecutor` adds the two
things dispatch has to be observed on and the fake does not offer: a record of
every `launch` call, so a refusal can be proven to have happened *before*
anything launched (criterion 18), and an injectable `ExecutorError` out of
`launch`, so the no-traceback guarantee (criterion 19) can be exercised.

Advertisement fixtures follow schemas/v1/capability-advertisement.schema.json;
requirement fixtures follow schemas/v1/requirement.schema.json. The
`auth_transport` values exercise the default `AuthTransportPolicy` that
`registry.select` installs (`registry.py:65-76`): "local" is recognized and
safe, "metered_api" is unsafe-by-default and so is refused for an explicit
choice and reported `policy_excluded` for an auto one.
"""

from __future__ import annotations

import pytest

from praxis_cli import run_dispatch
from praxis_cli.match_cmd import _POLICY_EXCLUDED_SUFFIX, run_match
from praxis_cli.run_dispatch import (
    DispatchRefused,
    build_registry,
    check_explicit_choice,
    dispatch_auto,
    dispatch_explicit,
)
from praxis_executors.adapters.fake import FakeCapabilityExecutor
from praxis_executors.interface import (
    ExecutionRequest,
    ExecutionResult,
    ExecutorError,
    ExecutorStatus,
)
from praxis_executors.registry import ExecutorRegistry, RegistryError

_SPEC_VERSION = "1.0.0"

_RUN_CONTEXT = {"run_id": "run-1", "graph_version": _SPEC_VERSION, "node_id": "n1"}

# Volatile per-record fields: `proof_id` is a fresh uuid4 and `produced_at` a
# wall-clock stamp (`proof.py:44-57`), so two record lists are compared on
# everything except these two.
_VOLATILE_RECORD_KEYS = ("proof_id", "produced_at")


def _capability(kind: str, auth_transport: str = "local", *, cost: float | None = None) -> dict:
    entry: dict = {"kind": kind}
    if cost is not None:
        entry["parameters"] = {"cost": cost}
    return {
        "spec_version": _SPEC_VERSION,
        "id": f"cap-{kind}",
        "satisfies": [entry],
        "auth_transport": auth_transport,
    }


def _requirement(*kinds: str) -> dict:
    return {
        "spec_version": _SPEC_VERSION,
        "requirements": [
            {
                "promise": {"spec_version": _SPEC_VERSION, "kind": kind},
                "constraint": "required",
            }
            for kind in kinds
        ],
    }


def _request(kind: str) -> ExecutionRequest:
    return ExecutionRequest(promise={"spec_version": _SPEC_VERSION, "kind": kind})


class _RecordingExecutor(FakeCapabilityExecutor):
    """A `FakeCapabilityExecutor` that records every `launch` and can be made
    to fail in it.

    `launch_calls` is what proves a refusal happened before anything ran;
    `launch_error` is the `ExecutorError` a real adapter raises when the
    backing CLI or service cannot be started at all.
    """

    def __init__(
        self,
        executor_id: str,
        capabilities: list[dict],
        script: dict[str, ExecutionResult],
        *,
        launch_error: BaseException | None = None,
    ) -> None:
        super().__init__(executor_id, capabilities, script)
        self.launch_calls: list[ExecutionRequest] = []
        self._launch_error = launch_error

    def launch(self, request: ExecutionRequest):
        self.launch_calls.append(request)
        if self._launch_error is not None:
            raise self._launch_error
        return super().launch(request)


class _SlowExecutor(_RecordingExecutor):
    """A `_RecordingExecutor` that reports `RUNNING` for its first
    `running_reads` `status` calls.

    The shipped fake is terminal on the first read, so it can never show what a
    dispatch does while an execution is still in flight -- which is the only
    moment the poll callback exists for.
    """

    def __init__(self, *args, running_reads: int = 2, **kwargs) -> None:
        super().__init__(*args, **kwargs)
        self._running_reads = running_reads
        self.status_calls = 0

    def status(self, handle):
        self.status_calls += 1
        if self.status_calls <= self._running_reads:
            return ExecutorStatus.RUNNING
        return super().status(handle)


def _slow_adapter(
    executor_id: str = "executor-local-1", kind: str = "kind-local", *, running_reads: int = 2
) -> _SlowExecutor:
    return _SlowExecutor(
        executor_id,
        [_capability(kind)],
        {kind: ExecutionResult(status=ExecutorStatus.SUCCEEDED, evidence={"test-pass": True})},
        running_reads=running_reads,
    )


def _succeeding_adapter(
    executor_id: str = "executor-local-1",
    kind: str = "kind-local",
    *,
    auth_transport: str = "local",
    cost: float | None = None,
    evidence: dict | None = None,
) -> _RecordingExecutor:
    return _RecordingExecutor(
        executor_id,
        [_capability(kind, auth_transport, cost=cost)],
        {
            kind: ExecutionResult(
                status=ExecutorStatus.SUCCEEDED,
                evidence=evidence if evidence is not None else {"test-pass": True},
                payload={"output": "done"},
            )
        },
    )


def _terminal_adapter(
    status: ExecutorStatus,
    executor_id: str = "executor-local-1",
    kind: str = "kind-local",
) -> _RecordingExecutor:
    return _RecordingExecutor(
        executor_id,
        [_capability(kind)],
        {kind: ExecutionResult(status=status, evidence={"test-pass": False})},
    )


def _without_volatile_keys(records: list[dict]) -> list[dict]:
    return [
        {key: value for key, value in record.items() if key not in _VOLATILE_RECORD_KEYS}
        for record in records
    ]


def test_build_registry_registers_every_adapter_in_the_mapping():
    adapters = {
        "executor-a": _succeeding_adapter("executor-a", "kind-a"),
        "executor-b": _succeeding_adapter("executor-b", "kind-b"),
    }

    registry = build_registry(adapters)

    assert isinstance(registry, ExecutorRegistry)
    advertised_ids = {
        advertisement["executor_id"]
        for advertisement in registry.advertisements(healthy_only=False)
    }
    assert advertised_ids == {"executor-a", "executor-b"}


def test_build_registry_keys_on_the_mapping_key_not_the_advertised_executor_id():
    """The key an adapter is registered under is the mapping's key, even when
    the adapter advertises a different `executor_id` for itself.

    `advertisements()` returns `capabilities()` verbatim (`registry.py:53-63`),
    so it reports the advertised id either way and cannot tell the two apart.
    `unregister` is keyed exactly as `register` was (`registry.py:50-51`), so
    removing by the mapping key is what distinguishes them.
    """
    adapter = _succeeding_adapter("executor-advertised-1", "kind-local")
    # Sanity: the two ids really differ, or the assertions below prove nothing.
    assert adapter.capabilities()["executor_id"] != "executor-mapping-1"

    registry = build_registry({"executor-mapping-1": adapter})

    assert [
        advertisement["executor_id"]
        for advertisement in registry.advertisements(healthy_only=False)
    ] == ["executor-advertised-1"]
    registry.unregister("executor-mapping-1")
    assert registry.advertisements(healthy_only=False) == []


def test_dispatch_auto_succeeds_and_returns_proof_records():
    adapter = _succeeding_adapter(evidence={"test-pass": True, "lint-clean": False})
    registry = build_registry({"executor-local-1": adapter})

    outcome = dispatch_auto(
        registry, _requirement("kind-local"), _request("kind-local"), **_RUN_CONTEXT
    )

    assert outcome.succeeded is True
    assert adapter.launch_calls  # the selected adapter really ran
    # One proof-record document per claimed proof_type, graded from the claim
    # and stamped with the executor `select()` actually chose.
    assert [record["proof_type"] for record in outcome.records] == ["test-pass", "lint-clean"]
    assert [record["status"] for record in outcome.records] == ["pass", "fail"]
    assert {record["executor_id"] for record in outcome.records} == {"executor-local-1"}
    assert {record["node_id"] for record in outcome.records} == {"n1"}
    assert {record["run_id"] for record in outcome.records} == {"run-1"}


def test_dispatch_auto_without_selection_reports_unsatisfied_and_policy_excluded():
    metered = _succeeding_adapter(
        "executor-metered-1", "kind-metered", auth_transport="metered_api"
    )
    local = _succeeding_adapter("executor-local-1", "kind-local")
    registry = build_registry({"executor-metered-1": metered, "executor-local-1": local})
    requirement = _requirement("kind-metered", "kind-absent")

    outcome = dispatch_auto(registry, requirement, _request("kind-metered"), **_RUN_CONTEXT)

    assert outcome.succeeded is False
    assert outcome.records == []
    assert metered.launch_calls == []
    # The message has to carry the registry's own reasons, and mark the
    # policy-excluded kind with the token `executors match` prints, so the two
    # commands report a policy exclusion identically.
    reason_by_kind = {entry.kind: entry for entry in registry.select(requirement).unsatisfied}
    assert reason_by_kind["kind-metered"].policy_excluded is True
    assert (
        reason_by_kind["kind-metered"].reason + _POLICY_EXCLUDED_SUFFIX
    ) in outcome.message
    assert reason_by_kind["kind-absent"].reason in outcome.message


@pytest.mark.parametrize("status", [ExecutorStatus.FAILED, ExecutorStatus.CANCELLED])
def test_dispatch_auto_converts_non_succeeded_terminal_status_into_failed_outcome(status):
    registry = build_registry({"executor-local-1": _terminal_adapter(status)})

    outcome = dispatch_auto(
        registry, _requirement("kind-local"), _request("kind-local"), **_RUN_CONTEXT
    )

    assert outcome.succeeded is False
    assert status.value in outcome.message


def test_dispatch_auto_converts_executor_error_into_failed_outcome():
    adapter = _RecordingExecutor(
        "executor-local-1",
        [_capability("kind-local")],
        {},
        launch_error=ExecutorError("backing cli is not installed"),
    )
    registry = build_registry({"executor-local-1": adapter})

    # Must not propagate: an ExecutorError never escapes as a traceback.
    outcome = dispatch_auto(
        registry, _requirement("kind-local"), _request("kind-local"), **_RUN_CONTEXT
    )

    assert outcome.succeeded is False
    assert "backing cli is not installed" in outcome.message


def test_dispatch_auto_reraises_a_registry_error_it_cannot_explain():
    """A `RegistryError` that is not a no-selection propagates.

    `dispatch_auto` re-derives the unsatisfied reasons from a second `select()`
    and only reports them when that selection also chose nothing. The guard is
    observable only when `select()` does choose while
    `execute_with_proof_records` still raised, so the registry here raises for a
    reason other than no selection -- reporting that as an unsatisfied
    requirement would blame the graph for a registry fault.
    """

    class _RaisingRegistry(ExecutorRegistry):
        def execute_with_proof_records(self, *args, **kwargs):
            raise RegistryError("registry is misconfigured")

    registry = _RaisingRegistry()
    registry.register("executor-local-1", _succeeding_adapter())
    requirement = _requirement("kind-local")
    # Sanity: selection succeeds, so the raise cannot be a no-selection.
    assert registry.select(requirement).selected is not None

    with pytest.raises(RegistryError, match="registry is misconfigured"):
        dispatch_auto(registry, requirement, _request("kind-local"), **_RUN_CONTEXT)


def test_check_explicit_choice_accepts_an_eligible_executor_that_satisfies_the_requirement():
    adapters = {"executor-local-1": _succeeding_adapter()}

    assert (
        check_explicit_choice(adapters, "executor-local-1", _requirement("kind-local")) is None
    )


def test_check_explicit_choice_refuses_unknown_id_naming_every_known_id():
    adapters = {
        "executor-a": _succeeding_adapter("executor-a", "kind-a"),
        "executor-b": _succeeding_adapter("executor-b", "kind-b"),
    }

    with pytest.raises(DispatchRefused) as exc_info:
        check_explicit_choice(adapters, "executor-typo", _requirement("kind-a"))

    message = str(exc_info.value)
    assert "executor-typo" in message
    for known_id in adapters:
        assert known_id in message


def test_check_explicit_choice_refuses_policy_denied_transport_and_launches_nothing():
    adapter = _succeeding_adapter(
        "executor-metered-1", "kind-metered", auth_transport="metered_api"
    )
    adapters = {"executor-metered-1": adapter}

    with pytest.raises(DispatchRefused) as exc_info:
        check_explicit_choice(adapters, "executor-metered-1", _requirement("kind-metered"))

    message = str(exc_info.value)
    assert "executor-metered-1" in message
    assert "metered_api" in message
    # The refusal is what "refused, not silently overridden" means: the check
    # runs before anything launches.
    assert adapter.launch_calls == []


def test_check_explicit_choice_refuses_an_executor_advertising_no_capabilities():
    """An adapter that advertises nothing is denied outright by
    `AuthTransportPolicy` (`policy.py:50-52`), so the refusal names that reason
    rather than an empty list of transports."""
    adapter = _RecordingExecutor("executor-empty-1", [], {})
    adapters = {"executor-empty-1": adapter}

    with pytest.raises(DispatchRefused) as exc_info:
        check_explicit_choice(adapters, "executor-empty-1", _requirement("kind-local"))

    message = str(exc_info.value)
    assert "executor-empty-1" in message
    assert "advertises no capabilities" in message
    assert adapter.launch_calls == []


def test_check_explicit_choice_refuses_executor_that_does_not_satisfy_the_requirement():
    adapter = _succeeding_adapter("executor-local-1", "kind-local")
    adapters = {"executor-local-1": adapter}

    with pytest.raises(DispatchRefused) as exc_info:
        check_explicit_choice(adapters, "executor-local-1", _requirement("kind-other"))

    message = str(exc_info.value)
    assert "executor-local-1" in message
    assert "kind-other" in message
    assert adapter.launch_calls == []


def test_dispatch_explicit_produces_the_same_proof_records_as_the_auto_path():
    evidence = {"test-pass": True, "lint-clean": False}
    requirement = _requirement("kind-local")
    request = _request("kind-local")

    auto_adapter = _succeeding_adapter(evidence=evidence)
    auto_registry = build_registry({"executor-local-1": auto_adapter})
    auto_outcome = dispatch_auto(auto_registry, requirement, request, **_RUN_CONTEXT)

    explicit_adapter = _succeeding_adapter(evidence=evidence)
    explicit_adapters = {"executor-local-1": explicit_adapter}
    explicit_registry = build_registry(explicit_adapters)
    explicit_outcome = dispatch_explicit(
        explicit_registry,
        explicit_adapters,
        "executor-local-1",
        request,
        **_RUN_CONTEXT,
    )

    assert explicit_outcome.succeeded is True
    assert explicit_adapter.launch_calls == [request]
    assert _without_volatile_keys(explicit_outcome.records) == _without_volatile_keys(
        auto_outcome.records
    )


def test_dispatch_explicit_runs_the_named_executor_not_the_one_auto_would_rank_first():
    requirement = _requirement("kind-local")
    request = _request("kind-local")
    cheap = _succeeding_adapter("executor-cheap-1", "kind-local", cost=1)
    pricey = _succeeding_adapter("executor-pricey-1", "kind-local", cost=9)
    adapters = {"executor-cheap-1": cheap, "executor-pricey-1": pricey}
    registry = build_registry(adapters)

    # Sanity: auto ranks the cheaper candidate first, so the explicit choice
    # below is genuinely the one auto-matching would not have made.
    assert registry.select(requirement).selected.executor_id == "executor-cheap-1"

    outcome = dispatch_explicit(
        registry, adapters, "executor-pricey-1", request, **_RUN_CONTEXT
    )

    assert outcome.succeeded is True
    assert cheap.launch_calls == []
    assert pricey.launch_calls == [request]
    assert {record["executor_id"] for record in outcome.records} == {"executor-pricey-1"}


def test_dispatch_explicit_stamps_records_with_the_mapping_key_not_the_advertised_id():
    """A proof record is attributed to the id the user named on `--executor`.

    That id is the mapping key, which is also what `build_registry` registers
    under and what `executors match` prints. An adapter's self-reported
    `executor_id` is its own claim about itself, and the two are only ever the
    same by convention.
    """
    adapter = _succeeding_adapter("executor-advertised-1", "kind-local")
    adapters = {"executor-mapping-1": adapter}
    # Sanity: the two ids really differ, or the assertion below proves nothing.
    assert adapter.capabilities()["executor_id"] != "executor-mapping-1"

    outcome = dispatch_explicit(
        build_registry(adapters),
        adapters,
        "executor-mapping-1",
        _request("kind-local"),
        **_RUN_CONTEXT,
    )

    assert outcome.succeeded is True
    assert adapter.launch_calls  # the named adapter is the one that ran
    assert {record["executor_id"] for record in outcome.records} == {"executor-mapping-1"}


@pytest.mark.parametrize("status", [ExecutorStatus.FAILED, ExecutorStatus.CANCELLED])
def test_dispatch_explicit_converts_non_succeeded_terminal_status_into_failed_outcome(status):
    adapters = {"executor-local-1": _terminal_adapter(status)}
    registry = build_registry(adapters)

    outcome = dispatch_explicit(
        registry, adapters, "executor-local-1", _request("kind-local"), **_RUN_CONTEXT
    )

    assert outcome.succeeded is False
    assert status.value in outcome.message


def test_dispatch_explicit_converts_executor_error_into_failed_outcome():
    adapters = {
        "executor-local-1": _RecordingExecutor(
            "executor-local-1",
            [_capability("kind-local")],
            {},
            launch_error=ExecutorError("backing cli is not installed"),
        )
    }
    registry = build_registry(adapters)

    outcome = dispatch_explicit(
        registry, adapters, "executor-local-1", _request("kind-local"), **_RUN_CONTEXT
    )

    assert outcome.succeeded is False
    assert "backing cli is not installed" in outcome.message


def test_run_and_executors_match_word_a_policy_exclusion_identically(capsys):
    """The no-selection reasons `run` reports are the lines `executors match`
    prints, marker included.

    Asserted against that command's real output rather than against its
    module-private formatter, so the two commands are pinned together through
    the surface a user actually sees.
    """
    adapters = {
        "executor-metered-1": _succeeding_adapter(
            "executor-metered-1", "kind-metered", auth_transport="metered_api"
        )
    }
    registry = build_registry(adapters)

    outcome = dispatch_auto(
        registry, _requirement("kind-metered"), _request("kind-metered"), **_RUN_CONTEXT
    )

    run_match(adapters, capabilities=["kind-metered"], explain=False)
    printed = capsys.readouterr().out.splitlines()
    assert printed[0] == "no executor selected"
    # Every reason line the other command printed, verbatim -- the marker is a
    # suffix of one of them, so a drifted spelling fails here.
    assert printed[1:]
    for line in printed[1:]:
        assert line.endswith(_POLICY_EXCLUDED_SUFFIX)
        assert line in outcome.message


@pytest.mark.parametrize("status", [ExecutorStatus.FAILED, ExecutorStatus.CANCELLED])
def test_dispatch_keeps_the_proof_records_a_non_succeeded_execution_produced(status):
    """A failed execution's evidence is still graded and reported.

    The node's transition takes `fail`, which carries no evidence, but a caller
    that discarded these records could say nothing about *what* failed.
    """
    auto_adapters = {"executor-local-1": _terminal_adapter(status)}
    auto_outcome = dispatch_auto(
        build_registry(auto_adapters),
        _requirement("kind-local"),
        _request("kind-local"),
        **_RUN_CONTEXT,
    )

    explicit_adapters = {"executor-local-1": _terminal_adapter(status)}
    explicit_outcome = dispatch_explicit(
        build_registry(explicit_adapters),
        explicit_adapters,
        "executor-local-1",
        _request("kind-local"),
        **_RUN_CONTEXT,
    )

    assert auto_outcome.succeeded is False
    assert [record["proof_type"] for record in auto_outcome.records] == ["test-pass"]
    assert [record["status"] for record in auto_outcome.records] == ["fail"]
    # Both paths report the same thing about the same failure.
    assert _without_volatile_keys(explicit_outcome.records) == _without_volatile_keys(
        auto_outcome.records
    )


def test_dispatch_auto_waits_on_the_poll_callback_while_the_execution_is_running():
    adapter = _slow_adapter()
    polls: list[int] = []

    outcome = dispatch_auto(
        build_registry({"executor-local-1": adapter}),
        _requirement("kind-local"),
        _request("kind-local"),
        **_RUN_CONTEXT,
        poll=lambda: polls.append(adapter.status_calls),
    )

    assert outcome.succeeded is True
    # One wait per non-terminal read, and none after the terminal one: the loop
    # never spins on `status` unwaited.
    assert polls == [1, 2]


def test_dispatch_explicit_waits_on_the_poll_callback_while_the_execution_is_running():
    adapter = _slow_adapter()
    adapters = {"executor-local-1": adapter}
    polls: list[int] = []

    outcome = dispatch_explicit(
        build_registry(adapters),
        adapters,
        "executor-local-1",
        _request("kind-local"),
        **_RUN_CONTEXT,
        poll=lambda: polls.append(adapter.status_calls),
    )

    assert outcome.succeeded is True
    assert polls == [1, 2]


def test_dispatch_explicit_sleeps_between_polls_when_the_caller_names_no_callback(monkeypatch):
    """The default is a sleep, not a busy-wait: an adapter that stays
    non-terminal must not cost a core."""
    sleeps: list[float] = []
    monkeypatch.setattr(run_dispatch.time, "sleep", lambda seconds: sleeps.append(seconds))
    adapter = _slow_adapter()
    adapters = {"executor-local-1": adapter}

    outcome = dispatch_explicit(
        build_registry(adapters),
        adapters,
        "executor-local-1",
        _request("kind-local"),
        **_RUN_CONTEXT,
    )

    assert outcome.succeeded is True
    assert sleeps == [run_dispatch._POLL_INTERVAL_SECONDS] * 2

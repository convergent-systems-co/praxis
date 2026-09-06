"""Integration test: alternate-executor retry fallback, proving PolicyGate's
decision layer composes with `praxis_executors`' registry/policy/matching
layer and a real `TransitionEngine` -- not just at the decision layer alone
(that is `test_policy_gate_core.py`'s job).

Uses a generic document-review domain (no software-development vocabulary,
per the epic's constraint), following the same non-development-domain
convention as `tests/test_end_to_end_fake_executor.py`'s sample graph,
rather than `test_executor_end_to_end.py`'s code-execution/text-generation
kinds.
"""

from __future__ import annotations

from dataclasses import replace
from pathlib import Path

from conftest import _PassthroughGrader
from praxis_evidence.graders import GraderRegistry
from praxis_evidence.proof import build_proof_record
from praxis_evidence.types import proof_record_to_document
from praxis_executors.adapters.fake import FakeCapabilityExecutor
from praxis_executors.interface import ExecutionRequest, ExecutionResult, ExecutorStatus
from praxis_executors.policy import DenyListPolicy, as_eligibility_callable
from praxis_executors.registry import ExecutorRegistry
from praxis_policy.budgets import BudgetLedger
from praxis_policy.gate import PolicyGate, PolicyOutcome
from praxis_policy.profiles import BUILTIN_PROFILES
from praxis_runtime.events import EventLog
from praxis_runtime.graph import Graph, Node
from praxis_runtime.state import RunStateStore
from praxis_runtime.transitions import NodeStatus, TransitionEngine

_SPEC_VERSION = "1.0.0"
_KIND = "document-review"
_TRANSIENT_PAYLOAD = {"failure_class": "transient"}


def _fallback_allowing_profile():
    return replace(
        BUILTIN_PROFILES["standard"],
        allow_alternate_executor_retry=True,
        default_retry_budget=3,
        default_repair_budget=1,
    )


def _requirement() -> dict:
    return {
        "spec_version": _SPEC_VERSION,
        "requirements": [
            {"promise": {"spec_version": _SPEC_VERSION, "kind": _KIND}, "constraint": "required"}
        ],
    }


def _reviewer(executor_id: str, result: ExecutionResult) -> FakeCapabilityExecutor:
    return FakeCapabilityExecutor(
        executor_id=executor_id,
        capabilities=[{"spec_version": _SPEC_VERSION, "satisfies": [{"kind": _KIND}]}],
        script={_KIND: result},
    )


def _single_node_graph() -> Graph:
    return Graph(
        spec_version=_SPEC_VERSION,
        nodes={
            "n1": Node(
                id="n1",
                kind="task",
                metadata={
                    "evidence_requirement": {
                        "spec_version": _SPEC_VERSION,
                        "evidence": [
                            {"proof_type": "peer-attestation", "constraint": "required"}
                        ],
                    }
                },
            )
        },
        edges=[],
        entry_node="n1",
        terminal_nodes={"n1"},
    )


def _make_engine(tmp_path: Path) -> TransitionEngine:
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    registry = GraderRegistry()
    registry.register("peer-attestation", "deterministic", _PassthroughGrader())
    return TransitionEngine(_single_node_graph(), store, log, grader_registry=registry)


def _proof_records(evidence: dict, *, node_id: str, executor_id: str) -> list[dict]:
    """Convert a flat `ExecutionResult.evidence` claim dict into the
    `list[dict]` of proof-record documents `TransitionEngine.apply` requires
    -- the conversion a caller with run/graph/node context must do, since
    `praxis_executors` deliberately has none (see `ExecutionResult`'s
    docstring)."""
    records = []
    for proof_type, claim in evidence.items():
        record = build_proof_record(
            run_id="run-1",
            graph_version=_SPEC_VERSION,
            node_id=node_id,
            proof_type=proof_type,
            executor_id=executor_id,
            grader_kind="deterministic",
            status="pass" if claim else "fail",
        )
        records.append(proof_record_to_document(record))
    return records


def test_alternate_executor_retry_recovers_a_transient_failure_end_to_end(tmp_path: Path):
    registry = ExecutorRegistry()
    registry.register(
        "reviewer-a",
        _reviewer(
            "reviewer-a",
            ExecutionResult(status=ExecutorStatus.FAILED, payload=_TRANSIENT_PAYLOAD),
        ),
    )
    registry.register(
        "reviewer-b",
        _reviewer(
            "reviewer-b",
            ExecutionResult(
                status=ExecutorStatus.SUCCEEDED, evidence={"peer-attestation": True}
            ),
        ),
    )
    requirement = _requirement()
    request = ExecutionRequest(promise={"spec_version": _SPEC_VERSION, "kind": _KIND})

    first_match = registry.select(requirement)
    assert first_match.selected is not None
    first_executor_id = first_match.selected.executor_id

    first_result = registry.execute(requirement, request)
    assert first_result.status is ExecutorStatus.FAILED

    profile = _fallback_allowing_profile()
    ledger = BudgetLedger()
    gate = PolicyGate(profile, ledger)

    # On this first failure `previously_tried_executor_ids` is empty, so the
    # `self._profile.allow_alternate_executor_retry and
    # previously_tried_executor_ids and not repair-exhausted` guard in
    # `PolicyGate.decide_on_failure` (src/praxis_policy/gate.py) is skipped
    # and the priority order falls through to its last branch,
    # RETRY_SAME_EXECUTOR -- alternate-executor retry only ever applies
    # starting on the second attempt, once an executor has actually been
    # tried.
    first_decision = gate.decide_on_failure(
        "n1", {}, first_result.payload, previously_tried_executor_ids=frozenset()
    )
    assert first_decision.outcome is PolicyOutcome.RETRY_SAME_EXECUTOR

    second_decision = gate.decide_on_failure(
        "n1",
        {},
        first_result.payload,
        previously_tried_executor_ids=frozenset({first_executor_id}),
    )
    assert second_decision.outcome is PolicyOutcome.RETRY_ALTERNATE_EXECUTOR
    assert second_decision.excluded_executor_ids == frozenset({first_executor_id})

    deny_list = DenyListPolicy(denied_executor_ids=second_decision.excluded_executor_ids)
    is_eligible = as_eligibility_callable(deny_list, registry.advertisements())
    second_match = registry.select(requirement, is_eligible=is_eligible)

    assert second_match.selected is not None
    assert second_match.selected.executor_id != first_executor_id

    second_result = registry.execute(requirement, request, is_eligible=is_eligible)
    assert second_result.status is ExecutorStatus.SUCCEEDED

    engine = _make_engine(tmp_path)
    engine.apply("n1", "start")
    engine.apply("n1", second_decision.event_type)
    engine.apply("n1", "resume")
    evidence = _proof_records(
        second_result.evidence, node_id="n1", executor_id=second_match.selected.executor_id
    )
    state = engine.apply("n1", "complete", evidence=evidence)

    assert state.cursors["n1"].status == NodeStatus.TERMINAL_SUCCESS.value

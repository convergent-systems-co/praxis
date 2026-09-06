"""Executor assignment / capability projection for the dashboard.

`build_executor_assignments` reads stored proof-record documents out of
event payloads (shape: schemas/v1/proof-record.schema.json, produced by
src/praxis_evidence/types.py::proof_record_to_document) -- one
ExecutorAssignmentView per document found under payload["evidence"].

`build_capability_views` projects an optional live
praxis_executors.registry.ExecutorRegistry.advertisements() snapshot
(shape: schemas/v1/capability-advertisement.schema.json) into
CapabilityView entries, mirroring the cost-hint convention of
praxis_executors.matching._cost_hint (first present value among
cost/risk/latency parameters across an advertisement's satisfies entries).
"""

from __future__ import annotations

from praxis_dashboard.executor_view import (
    CapabilityView,
    ExecutorAssignmentView,
    build_capability_views,
    build_executor_assignments,
)
from praxis_runtime.events import Event

_SPEC_VERSION = "1.0.0"


def _event(payload: dict, *, seq: int = 0, event_id: str = "evt-0") -> Event:
    return Event(
        spec_version=_SPEC_VERSION,
        seq=seq,
        run_id="run-1",
        node_id="n1",
        event_type="proof_recorded",
        payload=payload,
        event_id=event_id,
    )


def _proof_record_document(**overrides) -> dict:
    document = {
        "spec_version": _SPEC_VERSION,
        "proof_id": "proof-1",
        "run_id": "run-1",
        "graph_version": "1.0.0",
        "node_id": "n1",
        "proof_type": "test-pass",
        "executor_id": "executor-1",
        "grader_kind": "deterministic",
        "status": "pass",
    }
    document.update(overrides)
    return document


def test_event_with_no_evidence_key_contributes_no_view():
    event = _event({"node_id": "n1"})

    assert build_executor_assignments([event]) == ()


def test_event_with_one_proof_record_produces_one_matching_view():
    document = _proof_record_document()
    event = _event({"evidence": [document]})

    views = build_executor_assignments([event])

    assert views == (
        ExecutorAssignmentView(
            node_id="n1",
            proof_type="test-pass",
            executor_id="executor-1",
            grader_kind="deterministic",
            status="pass",
        ),
    )


def test_two_proof_records_across_two_events_both_surface():
    first = _event(
        {"evidence": [_proof_record_document(proof_id="proof-1", node_id="n1")]},
        seq=0,
        event_id="evt-0",
    )
    second = _event(
        {
            "evidence": [
                _proof_record_document(
                    proof_id="proof-2",
                    node_id="n2",
                    executor_id="executor-2",
                    status="fail",
                )
            ]
        },
        seq=1,
        event_id="evt-1",
    )

    views = build_executor_assignments([first, second])

    assert views == (
        ExecutorAssignmentView(
            node_id="n1",
            proof_type="test-pass",
            executor_id="executor-1",
            grader_kind="deterministic",
            status="pass",
        ),
        ExecutorAssignmentView(
            node_id="n2",
            proof_type="test-pass",
            executor_id="executor-2",
            grader_kind="deterministic",
            status="fail",
        ),
    )


def test_build_capability_views_none_yields_empty_tuple():
    assert build_capability_views(None) == ()


def test_advertisement_with_cost_parameter_surfaces_cost_hint():
    advertisement = {
        "spec_version": "1.0.0",
        "executor_id": "executor-7f3a",
        "capabilities": [
            {
                "spec_version": "1.0.0",
                "id": "cap-primary",
                "satisfies": [
                    {
                        "kind": "text-generation",
                        "parameters": {"cost": 0.5},
                    }
                ],
            }
        ],
    }

    views = build_capability_views([advertisement])

    assert views == (
        CapabilityView(
            executor_id="executor-7f3a",
            satisfied_kinds=("text-generation",),
            cost_hint=0.5,
        ),
    )


def test_advertisement_with_no_cost_risk_or_latency_yields_none_cost_hint():
    advertisement = {
        "spec_version": "1.0.0",
        "executor_id": "executor-9",
        "capabilities": [
            {
                "spec_version": "1.0.0",
                "id": "cap-primary",
                "satisfies": [
                    {
                        "kind": "code-execution",
                        "parameters": {"max_context_tokens": 32768},
                    }
                ],
            }
        ],
    }

    views = build_capability_views([advertisement])

    assert views == (
        CapabilityView(
            executor_id="executor-9",
            satisfied_kinds=("code-execution",),
            cost_hint=None,
        ),
    )

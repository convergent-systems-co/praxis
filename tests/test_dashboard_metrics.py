"""Cost/time/retry metrics projection for the dashboard.

`build_node_metrics` derives retry/handoff counts directly from raw
`event_type` occurrences in the event log (no dependency on
`praxis_policy.BudgetLedger`, which is in-memory-only and not durable), and
`evidence_confidence` from any stored proof-record document's optional
`confidence` key (shape: schemas/v1/proof-record.schema.json, produced by
src/praxis_evidence/types.py::proof_record_to_document) found under
`payload["evidence"]`. No wall-clock timing metric is produced: neither
event.schema.json nor run-state.schema.json declares a timestamp property.
"""

from __future__ import annotations

from praxis_dashboard.metrics import NodeMetrics, build_node_metrics
from praxis_runtime.events import Event

_SPEC_VERSION = "1.0.0"


def _event(
    node_id: str, event_type: str, payload: dict | None = None, *, seq: int = 0, event_id: str = "evt-0"
) -> Event:
    return Event(
        spec_version=_SPEC_VERSION,
        seq=seq,
        run_id="run-1",
        node_id=node_id,
        event_type=event_type,
        payload=payload or {},
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


def test_node_with_two_blocks_and_one_handoff_counts_each():
    events = [
        _event("n1", "block", seq=0, event_id="evt-0"),
        _event("n1", "block", seq=1, event_id="evt-1"),
        _event("n1", "handoff", seq=2, event_id="evt-2"),
    ]

    metrics = build_node_metrics(events)

    assert metrics == (
        NodeMetrics(node_id="n1", retry_count=2, handoff_count=1, evidence_confidence={}),
    )


def test_stored_proof_with_confidence_surfaces_by_proof_type():
    document = _proof_record_document(proof_type="X", confidence=0.9)
    events = [_event("n1", "proof_recorded", {"evidence": [document]})]

    metrics = build_node_metrics(events)

    assert metrics == (
        NodeMetrics(node_id="n1", retry_count=0, handoff_count=0, evidence_confidence={"X": 0.9}),
    )


def test_proof_with_no_confidence_key_contributes_nothing():
    document = _proof_record_document(proof_type="X")
    assert "confidence" not in document
    events = [_event("n1", "proof_recorded", {"evidence": [document]})]

    metrics = build_node_metrics(events)

    assert metrics == (
        NodeMetrics(node_id="n1", retry_count=0, handoff_count=0, evidence_confidence={}),
    )


def test_node_with_no_block_or_handoff_events_shows_zero_counts_present():
    events = [_event("n1", "started", seq=0, event_id="evt-0")]

    metrics = build_node_metrics(events)

    assert metrics == (
        NodeMetrics(node_id="n1", retry_count=0, handoff_count=0, evidence_confidence={}),
    )

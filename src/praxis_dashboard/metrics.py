"""Cost/time/retry metrics projection for the dashboard.

`build_node_metrics` derives `retry_count`/`handoff_count` directly from raw
`event_type` occurrences in the event log ("block" and "handoff"
respectively) rather than importing the optional `praxis_policy` package's
`BudgetLedger`, which only tracks in-memory counts for whichever single
process constructed it and is not itself durable -- the dashboard is a
separate, possibly-later-attaching reader that cannot rely on that in-memory
state existing at all.

No wall-clock timing metric is produced: neither schemas/v1/event.schema.json
nor schemas/v1/run-state.schema.json declares a timestamp property (verified
against both). "Time" is out of scope for this bundle beyond whatever a
stored proof record's own optional `produced_at` string happens to record --
surfaced, unparsed, alongside confidence, not synthesized here.

`evidence_confidence` is built from stored proof-record documents (shape:
schemas/v1/proof-record.schema.json, produced by
src/praxis_evidence/types.py::proof_record_to_document) found under each
event's `payload["evidence"]`: for every document with a non-`None`
`"confidence"` key, `evidence_confidence[proof_type]` is set to that value,
last one wins if a `proof_type` recurs -- matching how `stored_evidence_for`
(src/praxis_dashboard/evidence_view.py) treats "most recent" as authoritative.
"""

from __future__ import annotations

from dataclasses import dataclass, field

from praxis_runtime.events import Event


@dataclass(frozen=True)
class NodeMetrics:
    node_id: str
    retry_count: int
    handoff_count: int
    evidence_confidence: dict[str, float] = field(default_factory=dict)


def build_node_metrics(events: list[Event]) -> tuple[NodeMetrics, ...]:
    events_by_node: dict[str, list[Event]] = {}
    for event in events:
        events_by_node.setdefault(event.node_id, []).append(event)

    metrics = []
    for node_id, node_events in events_by_node.items():
        retry_count = sum(1 for event in node_events if event.event_type == "block")
        handoff_count = sum(1 for event in node_events if event.event_type == "handoff")

        evidence_confidence: dict[str, float] = {}
        for event in node_events:
            for document in event.payload.get("evidence") or []:
                confidence = document.get("confidence")
                if confidence is not None:
                    evidence_confidence[document["proof_type"]] = confidence

        metrics.append(
            NodeMetrics(
                node_id=node_id,
                retry_count=retry_count,
                handoff_count=handoff_count,
                evidence_confidence=evidence_confidence,
            )
        )

    return tuple(metrics)

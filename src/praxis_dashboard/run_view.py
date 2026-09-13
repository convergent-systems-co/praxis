"""Current-run view: one entry per node with a cursor in the run state.

This module composes projections other modules already built rather than
re-deriving them: `attempts` comes from the passed-in
praxis_dashboard.metrics.NodeMetrics tuple, and the evidence fields are
passed through from the passed-in praxis_dashboard.evidence_view.EvidenceView
tuple unchanged (including `satisfied is None`, which means "no requirement"
or "not yet attempted" -- grading belongs to `build_evidence_view`, and
re-grading here could disagree with it).

`attempts` is `1 + NodeMetrics.retry_count` for a node that has any metrics
entry (an entry exists only for a node the event log has seen, i.e. one that
was started) and `0` for a node still `PENDING` with no events. It is never
read from praxis_policy.budgets.BudgetLedger: that ledger holds in-memory
counts belonging to whichever process constructed it, so a dashboard
attaching later cannot see it at all. The `"block"` events behind
`retry_count` are counted once, by `metrics.build_node_metrics`, and not
re-scanned here.

`executor_id` and `duration` come from the most recent stored proof-record
documents for the node (shape: schemas/v1/proof-record.schema.json), read via
`evidence_view.stored_evidence_for` -- the same `payload["evidence"]` source
`executor_view.build_executor_assignments` reads. A node with no stored
evidence reports `executor_id is None`, never a placeholder id.

`duration` is a stored record's optional `produced_at` string, passed through
unparsed -- the treatment metrics.py's docstring already documents. Nothing
here synthesizes a wall-clock figure, and no timestamp property is added to
schemas/v1/event.schema.json or schemas/v1/run-state.schema.json; when no
durable record carries a time value, `duration is None` and `duration_note`
says so.

Both note fields are always a non-empty sentence, in the recorded case as
well as the not-recorded one, so a renderer can display either unconditionally
and never has to treat an empty string as a special case. `NOT_RECORDED` is
the prefix that distinguishes them: a note starts with it exactly when its
value is `None`.

`model` is never derived from an `executor_id`. Both
schemas/v1/proof-record.schema.json and
schemas/v1/capability-advertisement.schema.json describe that field
identically -- "An opaque identifier for the executor. Must not encode a
vendor or model name." (verified against both) -- and docs/ontology.md's core
architectural rule holds for every schema in src/praxis_contracts/schemas/v1/.
No durable record this repository stores declares a model property either:
proof-record.schema.json sets `"additionalProperties": false` over a property
list with no such field, so a validated proof record cannot carry one. No
schema field is added to change that, so `model` is `None` with the reason in
`model_note`.
"""

from __future__ import annotations

from dataclasses import dataclass

import praxis_runtime.events
import praxis_runtime.graph
import praxis_runtime.state
from praxis_dashboard import evidence_view as evidence_view_module
from praxis_dashboard import metrics as metrics_module

NOT_RECORDED = "not recorded"

_NO_MODEL_NOTE = (
    f"{NOT_RECORDED}: no durable record carries a model name, and the executor "
    "id is opaque by contract, so none can be inferred from it"
)

_NO_DURATION_NOTE = (
    f"{NOT_RECORDED}: no stored proof record for this node carries a "
    "produced_at value, and no time is synthesized here"
)

_DURATION_NOTE = (
    "recorded: the produced_at value of a stored proof record, passed through "
    "unparsed"
)


@dataclass(frozen=True)
class RunNodeView:
    node_id: str
    kind: str
    state: str
    attempts: int
    executor_id: str | None
    evidence_satisfied: bool | None
    evidence_reasons: tuple[str, ...]
    evidence_stale_warning: str | None
    model: str | None
    model_note: str
    duration: str | None
    duration_note: str


def _latest_record(records: list[dict]) -> dict | None:
    return records[-1] if records else None


def _produced_at(records: list[dict]) -> str | None:
    # Last one wins if several stored records carry the key, matching how
    # metrics.build_node_metrics and evidence_view.stored_evidence_for both
    # treat the most recent value as authoritative.
    produced_at = None
    for record in records:
        value = record.get("produced_at")
        if value is not None:
            produced_at = value
    return produced_at


def build_run_view(
    graph: "praxis_runtime.graph.Graph",
    run_state: "praxis_runtime.state.RunState",
    events: "list[praxis_runtime.events.Event]",
    node_metrics: "tuple[metrics_module.NodeMetrics, ...]",
    evidence_views: "tuple[evidence_view_module.EvidenceView, ...]",
) -> tuple[RunNodeView, ...]:
    metrics_by_node = {entry.node_id: entry for entry in node_metrics}
    evidence_by_node = {view.node_id: view for view in evidence_views}

    views = []
    for node_id, cursor in run_state.cursors.items():
        node_metrics_entry = metrics_by_node.get(node_id)
        attempts = 0 if node_metrics_entry is None else 1 + node_metrics_entry.retry_count

        records = evidence_view_module.stored_evidence_for(node_id, events)
        latest = _latest_record(records)
        produced_at = _produced_at(records)

        evidence = evidence_by_node.get(node_id)

        views.append(
            RunNodeView(
                node_id=node_id,
                kind=graph.nodes[node_id].kind,
                state=cursor.status,
                attempts=attempts,
                executor_id=latest["executor_id"] if latest else None,
                evidence_satisfied=evidence.satisfied if evidence else None,
                evidence_reasons=evidence.reasons if evidence else (),
                evidence_stale_warning=evidence.stale_warning if evidence else None,
                model=None,
                model_note=_NO_MODEL_NOTE,
                duration=produced_at,
                duration_note=_DURATION_NOTE if produced_at is not None else _NO_DURATION_NOTE,
            )
        )
    return tuple(views)

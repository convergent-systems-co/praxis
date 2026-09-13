"""Recovery/escalation projection for the dashboard.

`build_recovery_views` is derived purely from the graph, the run-state
cursors and the event log -- it never touches `TransitionEngine`, a
`LeaseStore` or a live executor -- so a replayed run projects exactly like a
live one.

The escalation edge token is the hyphenated `"on-failure"`, matching
`src/praxis_runtime/transitions.py::TransitionEngine._advance_successors`
(lines 238 and 246) and `src/overlays/development/graph.py`. That
spelling is authoritative: `schemas/v1/graph.schema.json` leaves an edge's
`kind` an open string, so an `"on_failure"` edge (the underscore spelling
used in some prose) is a perfectly valid edge that the runtime silently
never fires -- and this projection must agree with the runtime rather than
advertise an escalation target that can never be reached.

Three independent sources feed a view, and any subset of them may be
present:

* `"on-failure"` edges out of the node -> `escalation_targets`;
* `"policy-*"` audit receipts (appended by the optional `praxis_policy`
  package's `receipts.record_policy_decision`) -> steps carrying
  `policy_outcome`, `reason`, `excluded_executor_ids` and `detail`;
* the raw `"block"`/`"handoff"`/`"resume"`/`"accept"`/`"fail"` transition
  events, plus the `BLOCKED`/`HANDOFF` cursor statuses that
  `praxis_dashboard.projection` already treats as blockers.

`praxis_policy` is never imported: issue #8's policy layer is optional and a
run that never used it must still project correctly, so only the documented
event-type/payload convention is relied on -- the same approach
`projection._blocked_reason` takes.

`policy_outcome` reverses `receipts.record_policy_decision`'s own derivation
(`f"policy-{outcome.value.replace('_', '-')}"`) instead of hardcoding a list
of outcomes. Every `PolicyOutcome` value in `src/praxis_policy/gate.py`
("authorized", "human_required", "denied", "retry_same_executor",
"retry_alternate_executor", verified at implementation time) is lowercase
with `_` word separators and no internal `-`, so the round trip is exact,
and a future outcome is recovered without a change here.

`excluded_executor_ids` is surfaced per step and never collapsed across
steps: it is what makes an alternate-executor retry legible as such rather
than as an unexplained second attempt. `detail` is every remaining payload
key, passed through unread (`unresolved_scopes`, `denied_scopes`,
`retries_used`, `max_retries`, ...), so an outcome-specific key this module
has never heard of still reaches the dashboard.

`reason` is left as `None` when a step recorded none rather than filled with
placeholder prose; the "reason not recorded" wording stays where it already
lives, in `projection.next_actions`.

A node earns an entry when it has any one of those three things to show; the
three are independently sufficient, because a policy receipt can land on a
node the run has not entered yet (`record_policy_decision` takes its
`node_id` from the caller, and an authority or budget gate is evaluated
before the node starts, while `TransitionEngine` has seeded a cursor only
for the entry node). Such a node reports `status` `"pending"` -- the status
`TransitionEngine` would give it -- rather than being dropped, which would
hide exactly the denial the dashboard exists to surface. A run with no
escalation edge, no policy receipt and no blocked/handoff cursor yields `()`
-- never an error and never a fabricated escalation -- mirroring
`build_snapshot`'s `lease_store=None -> resources=()`.
"""

from __future__ import annotations

from dataclasses import dataclass

import praxis_runtime.events
import praxis_runtime.graph
import praxis_runtime.state
import praxis_runtime.transitions

from praxis_dashboard import metrics, projection

_ESCALATION_EDGE_KIND = "on-failure"
_POLICY_EVENT_PREFIX = "policy-"

# The recovery-relevant transition events: every event_type in
# transitions.py::_TRANSITIONS that leaves or re-enters a non-happy path.
# "start"/"complete"/"interrupt" are excluded -- the first two are ordinary
# progress, and "interrupt" is crash recovery rather than an escalation.
_LIFECYCLE_EVENT_TYPES = frozenset({"block", "handoff", "resume", "accept", "fail"})

# Payload keys record_policy_decision gives a dedicated field on the step.
_PROMOTED_PAYLOAD_KEYS = frozenset({"reason", "excluded_executor_ids"})

_HANDOFF_STATUS = praxis_runtime.transitions.NodeStatus.HANDOFF.value

# The status reported for a graph node the run has not reached, matching the
# status TransitionEngine seeds a cursor with (transitions.py:134).
_UNREACHED_STATUS = praxis_runtime.transitions.NodeStatus.PENDING.value


@dataclass(frozen=True)
class RecoveryStepView:
    seq: int
    event_type: str
    policy_outcome: str | None
    reason: str | None
    excluded_executor_ids: tuple[str, ...]
    detail: dict


@dataclass(frozen=True)
class RecoveryView:
    node_id: str
    status: str
    attempts: int
    awaiting_human: bool
    escalation_targets: tuple[str, ...]
    steps: tuple[RecoveryStepView, ...]


def _policy_outcome(event_type: str) -> str | None:
    if not event_type.startswith(_POLICY_EVENT_PREFIX):
        return None
    return event_type[len(_POLICY_EVENT_PREFIX) :].replace("-", "_")


def _step(event: "praxis_runtime.events.Event") -> RecoveryStepView:
    return RecoveryStepView(
        seq=event.seq,
        event_type=event.event_type,
        policy_outcome=_policy_outcome(event.event_type),
        reason=event.payload.get("reason"),
        excluded_executor_ids=tuple(event.payload.get("excluded_executor_ids") or ()),
        detail={
            key: value
            for key, value in event.payload.items()
            if key not in _PROMOTED_PAYLOAD_KEYS
        },
    )


def build_recovery_views(
    graph: "praxis_runtime.graph.Graph",
    run_state: "praxis_runtime.state.RunState",
    events: "list[praxis_runtime.events.Event]",
    node_metrics: "tuple[metrics.NodeMetrics, ...]",
) -> tuple[RecoveryView, ...]:
    escalation_targets: dict[str, list[str]] = {}
    for edge in graph.edges:
        if edge.kind != _ESCALATION_EDGE_KIND:
            continue
        escalation_targets.setdefault(edge.source, []).append(edge.target)

    steps_by_node: dict[str, list[RecoveryStepView]] = {}
    nodes_with_receipts: set[str] = set()
    for event in sorted(events, key=lambda event: event.seq):
        is_policy_receipt = event.event_type.startswith(_POLICY_EVENT_PREFIX)
        if not is_policy_receipt and event.event_type not in _LIFECYCLE_EVENT_TYPES:
            continue
        if is_policy_receipt:
            nodes_with_receipts.add(event.node_id)
        steps_by_node.setdefault(event.node_id, []).append(_step(event))

    retry_counts = {node.node_id: node.retry_count for node in node_metrics}

    # Graph order first, so the projection is stable across runs, then any
    # node the run knows about that the graph does not declare (a cursor or a
    # receipt naming an unknown node -- surfaced rather than silently
    # dropped), in first-seen order.
    candidates = list(graph.nodes)
    for node_id in (*run_state.cursors, *steps_by_node):
        if node_id not in graph.nodes and node_id not in candidates:
            candidates.append(node_id)

    views = []
    for node_id in candidates:
        cursor = run_state.cursors.get(node_id)
        status = cursor.status if cursor is not None else _UNREACHED_STATUS
        targets = tuple(dict.fromkeys(escalation_targets.get(node_id, ())))
        is_blocker = status in projection._BLOCKER_STATUSES
        if not targets and node_id not in nodes_with_receipts and not is_blocker:
            continue

        views.append(
            RecoveryView(
                node_id=node_id,
                status=status,
                # The same 1 + retry_count derivation the run view uses, from
                # the passed-in NodeMetrics rather than a second scan of the
                # log for "block" events.
                attempts=1 + retry_counts.get(node_id, 0),
                # Currently at HANDOFF, not "ever handed off": a node that was
                # handed off and has since been accepted is no longer waiting
                # on a human.
                awaiting_human=status == _HANDOFF_STATUS,
                escalation_targets=targets,
                steps=tuple(steps_by_node.get(node_id, ())),
            )
        )
    return tuple(views)

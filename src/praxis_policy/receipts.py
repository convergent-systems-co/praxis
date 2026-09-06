"""Auditable policy-decision receipts.

`record_policy_decision` turns a `praxis_policy.gate.PolicyDecision` into an
append-only `"policy-*"` event on the run's `EventLog`. This is purely
additive: the appended event never participates in `TransitionEngine`'s
`_TRANSITIONS` legality table and never mutates `RunState` -- it is
audit-only, appended alongside (before or after, caller's choice) whatever
real transition event the decision's own `event_type` produces when the
caller applies it via `TransitionEngine.apply`.
"""

from __future__ import annotations

import uuid
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    import praxis_runtime.events
    from praxis_policy.gate import PolicyDecision


def record_policy_decision(
    event_log: "praxis_runtime.events.EventLog",
    *,
    run_id: str,
    node_id: str,
    decision: "PolicyDecision",
) -> "praxis_runtime.events.Event":
    """Append an audit-only `"policy-*"` event recording `decision`.

    `event_type` is derived from `decision.outcome.value` (e.g.
    `"human_required"` -> `"policy-human-required"`); every `PolicyOutcome`
    value is already lowercase with `_` word separators, so replacing `_`
    with `-` always satisfies event.schema.json's `event_type` pattern
    `^[a-z0-9]+(-[a-z0-9]+)*$`.

    `seq=0` below is a placeholder: `EventLog.append` ignores/reassigns it
    from the log's own state (`src/praxis_runtime/events.py::EventLog.append`),
    matching the existing convention for constructing an `Event` before
    appending it.
    """
    from praxis_runtime.events import Event

    event = Event(
        spec_version="1.0.0",
        seq=0,
        run_id=run_id,
        node_id=node_id,
        event_type=f"policy-{decision.outcome.value.replace('_', '-')}",
        payload={
            "reason": decision.reason,
            "excluded_executor_ids": sorted(decision.excluded_executor_ids),
            **decision.detail,
        },
        event_id=uuid.uuid4().hex,
    )
    return event_log.append(event)

"""Compatibility adapter between the legacy `develop` skill's state
vocabulary and Praxis (T8, docs/overlays/development-compat.md).

The current `develop` skill (`~/.ai/skills/develop/`) is a standalone
graph-shaped orchestration system with its own status/event vocabulary; it
does not run on Praxis today. This module is the narrowest translation layer
needed to reason about that legacy state in Praxis terms -- it does not make
the legacy skill execute through Praxis.

`legacy_status_to_node_status` covers every enum value of
`~/.ai/skills/develop/contracts/run-state.schema.json`'s `cursor.status`
(`active`, `complete`, `waiting_human`) and top-level `status`
(`running`, `handoff`, `complete`, `human_required`), fanning them onto
`praxis_runtime.transitions.NodeStatus` by the closest matching Praxis
transition semantics:

- `active` / `running` (a cursor or run actively being worked) -> `RUNNING`
- `complete` (terminal, either level) -> `TERMINAL_SUCCESS`
- `handoff` (paused session, resumed by `checkpoint.py resume`) -> `HANDOFF`,
  Praxis's own paused-for-a-fresh-context status
- `waiting_human` / `human_required` (paused pending a human decision) ->
  `BLOCKED`, Praxis's own paused-pending-external-resolution status; this is
  not `HANDOFF` because resuming it needs a human decision, not just a fresh
  agent context
An unrecognized legacy status fails closed with `ValueError` rather than
guessing.

`legacy_event_to_proof_type` covers a representative slice of
`~/.ai/skills/develop/GRAPH.yaml`'s `events` list -- never a full graph
transliteration, per this task's scope note -- mapping evidence-bearing
events onto the development overlay's own declared proof types
(`overlays.development.manifest.DEVELOPMENT_MANIFEST`) and everything else
(bookkeeping events, or events this slice doesn't name) to `None`.
"""

from __future__ import annotations

from praxis_runtime.transitions import NodeStatus

_STATUS_MAP: dict[str, NodeStatus] = {
    "active": NodeStatus.RUNNING,
    "running": NodeStatus.RUNNING,
    "complete": NodeStatus.TERMINAL_SUCCESS,
    "handoff": NodeStatus.HANDOFF,
    "waiting_human": NodeStatus.BLOCKED,
    "human_required": NodeStatus.BLOCKED,
}

_EVENT_PROOF_TYPE_MAP: dict[str, str | None] = {
    "VERIFY_DONE": "development.test-pass",
    "REVIEW_APPROVED": "development.review-approved",
    "PERSONA_DISPATCHED": None,
}


def legacy_status_to_node_status(legacy_status: str) -> NodeStatus:
    try:
        return _STATUS_MAP[legacy_status]
    except KeyError:
        raise ValueError(f"unrecognized legacy develop status: {legacy_status!r}") from None


def legacy_event_to_proof_type(legacy_event: str) -> str | None:
    return _EVENT_PROOF_TYPE_MAP.get(legacy_event)

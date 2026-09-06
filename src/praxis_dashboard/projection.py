"""Core run/node projection for the dashboard.

`build_node_views` is read-only against `TransitionEngine`: the only method
ever called on it here is `.legal_next(node_id)`
(src/praxis_runtime/transitions.py::TransitionEngine.legal_next), which
itself only calls `.current_state()` -- never `.apply(...)`. `.apply(...)` is
the engine's single mutation entrypoint (see the module docstring of
src/praxis_runtime/transitions.py), so this module never mutates a run.

`blocked_reason` is sourced from the audit-only `"policy-*"` event
convention documented in src/praxis_policy/receipts.py::record_policy_decision
(event_type prefixed `"policy-"`, payload carrying a `"reason"` key) --
without importing `praxis_policy` itself, since issue #8's policy layer is
optional and a run that never used it must still project correctly.
"""

from __future__ import annotations

from dataclasses import dataclass

import praxis_runtime.events
import praxis_runtime.graph
import praxis_runtime.state
import praxis_runtime.transitions

_BLOCKER_STATUSES = {
    praxis_runtime.transitions.NodeStatus.BLOCKED.value,
    praxis_runtime.transitions.NodeStatus.HANDOFF.value,
}

_TERMINAL_STATUSES = {
    praxis_runtime.transitions.NodeStatus.TERMINAL_SUCCESS.value,
    praxis_runtime.transitions.NodeStatus.TERMINAL_FAILED.value,
}


@dataclass(frozen=True)
class NodeView:
    node_id: str
    kind: str
    status: str
    legal_next_events: tuple[str, ...]
    is_blocker: bool
    blocked_reason: str | None


@dataclass(frozen=True)
class RunSummary:
    run_id: str
    total_nodes: int
    counts_by_status: dict[str, int]
    is_complete: bool


def _blocked_reason(events: "list[praxis_runtime.events.Event]", node_id: str) -> str | None:
    for event in reversed(events):
        if event.node_id == node_id and event.event_type.startswith("policy-"):
            return event.payload.get("reason")
    return None


def build_node_views(
    graph: "praxis_runtime.graph.Graph",
    run_state: "praxis_runtime.state.RunState",
    engine: "praxis_runtime.transitions.TransitionEngine",
    events: "list[praxis_runtime.events.Event]",
) -> tuple[NodeView, ...]:
    views = []
    for node_id, cursor in run_state.cursors.items():
        node = graph.nodes[node_id]
        is_blocker = cursor.status in _BLOCKER_STATUSES
        views.append(
            NodeView(
                node_id=node_id,
                kind=node.kind,
                status=cursor.status,
                legal_next_events=tuple(sorted(engine.legal_next(node_id))),
                is_blocker=is_blocker,
                blocked_reason=_blocked_reason(events, node_id) if is_blocker else None,
            )
        )
    return tuple(views)


def build_run_summary(run_state: "praxis_runtime.state.RunState") -> RunSummary:
    counts_by_status = {status.value: 0 for status in praxis_runtime.transitions.NodeStatus}
    for cursor in run_state.cursors.values():
        counts_by_status[cursor.status] += 1

    is_complete = all(
        cursor.status in _TERMINAL_STATUSES for cursor in run_state.cursors.values()
    )

    return RunSummary(
        run_id=run_state.run_id,
        total_nodes=len(run_state.cursors),
        counts_by_status=counts_by_status,
        is_complete=is_complete,
    )


def next_actions(node_views: tuple[NodeView, ...]) -> tuple[str, ...]:
    actions = []
    for view in node_views:
        if view.legal_next_events:
            actions.append(
                f"{view.node_id} can be advanced via: {', '.join(sorted(view.legal_next_events))}"
            )
        if view.is_blocker:
            suffix = f": {view.blocked_reason}" if view.blocked_reason else " (reason not recorded)"
            actions.append(f"{view.node_id} is {view.status}{suffix}")
    return tuple(actions)

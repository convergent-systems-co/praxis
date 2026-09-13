"""Tests for the read-only recovery/escalation projection used by the dashboard.

Uses real Graph/RunState/TransitionEngine/Event objects, never hand-rolled
fakes: the multi-node graphs with an "on-failure" edge are built in memory the
same way tests/test_transitions.py::_on_failure_graph builds them, and the
no-escalation baseline uses examples/sample-graph.json like
tests/test_dashboard_projection.py.

The "policy-*" events appended below mirror the shape produced by
src/praxis_policy/receipts.py::record_policy_decision (event_type
f"policy-{outcome.value.replace('_', '-')}", payload carrying "reason",
"excluded_executor_ids", and the decision's outcome-specific detail keys)
without importing praxis_policy -- that package is optional and the projection
must not depend on it.
"""

from __future__ import annotations

from pathlib import Path

from praxis_runtime.events import Event, EventLog
from praxis_runtime.graph import Edge, Graph, Node, load_graph
from praxis_runtime.state import RunStateStore
from praxis_runtime.transitions import NodeStatus, TransitionEngine

from praxis_dashboard.metrics import build_node_metrics
from praxis_dashboard.recovery_view import build_recovery_views

SAMPLE_GRAPH_PATH = Path(__file__).resolve().parent.parent / "examples" / "sample-graph.json"


def _escalation_graph(*, on_failure_kind: str = "on-failure") -> Graph:
    """A graph whose entry node escalates to "recover" when it fails.

    `on_failure_kind` is a parameter only so one test can pin the underscore
    spelling `"on_failure"` as *not* an escalation edge.
    """
    return Graph(
        spec_version="1.0.0",
        nodes={
            "draft": Node(id="draft", kind="task"),
            "recover": Node(id="recover", kind="task"),
            "publish": Node(id="publish", kind="task"),
        },
        edges=[
            Edge(source="draft", target="recover", kind=on_failure_kind),
            Edge(source="draft", target="publish", kind="sequential"),
        ],
        entry_node="draft",
        terminal_nodes={"recover", "publish"},
    )


def _engine_for(graph: Graph, tmp_path: Path) -> tuple[EventLog, TransitionEngine]:
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    return log, TransitionEngine(graph, store, log)


def _append_policy_event(
    log: EventLog,
    *,
    run_id: str,
    node_id: str,
    event_type: str,
    reason: str,
    excluded_executor_ids: list[str] | None = None,
    detail: dict | None = None,
    event_id: str | None = None,
) -> None:
    log.append(
        Event(
            spec_version="1.0.0",
            seq=0,
            run_id=run_id,
            node_id=node_id,
            event_type=event_type,
            payload={
                "reason": reason,
                "excluded_executor_ids": excluded_executor_ids or [],
                **(detail or {}),
            },
            event_id=event_id or f"{event_type}-{node_id}",
        )
    )


def _views_for(graph: Graph, log: EventLog, engine: TransitionEngine):
    events = log.read_all()
    return build_recovery_views(
        graph, engine.current_state(), events, build_node_metrics(events)
    )


def test_run_without_escalation_edge_or_policy_event_yields_empty(tmp_path: Path):
    graph = load_graph(SAMPLE_GRAPH_PATH)
    log, engine = _engine_for(graph, tmp_path)
    engine.apply("intake", "start")
    engine.apply("intake", "complete")

    assert not any(edge.kind == "on-failure" for edge in graph.edges)

    assert _views_for(graph, log, engine) == ()


def test_on_failure_edge_alone_yields_entry_with_no_steps(tmp_path: Path):
    graph = _escalation_graph()
    log, engine = _engine_for(graph, tmp_path)

    views = _views_for(graph, log, engine)

    draft_view = next(view for view in views if view.node_id == "draft")
    assert draft_view.escalation_targets == ("recover",)
    assert draft_view.steps == ()
    assert draft_view.attempts == 1
    assert draft_view.awaiting_human is False


def test_alternate_executor_retry_names_target_outcome_and_excluded_executors(
    tmp_path: Path,
):
    graph = _escalation_graph()
    log, engine = _engine_for(graph, tmp_path)
    engine.apply("draft", "start")
    engine.apply("draft", "block")
    _append_policy_event(
        log,
        run_id=engine.current_state().run_id,
        node_id="draft",
        event_type="policy-retry-alternate-executor",
        reason="executor exceeded its budget",
        excluded_executor_ids=["executor-slow"],
        detail={"retries_used": 1, "max_retries": 3},
    )

    views = _views_for(graph, log, engine)

    draft_view = next(view for view in views if view.node_id == "draft")
    assert draft_view.status == NodeStatus.BLOCKED.value
    assert draft_view.escalation_targets == ("recover",)
    # 1 + retry_count, from the passed-in NodeMetrics (one "block" event).
    assert draft_view.attempts == 2
    assert draft_view.awaiting_human is False

    policy_step = next(
        step for step in draft_view.steps if step.event_type == "policy-retry-alternate-executor"
    )
    assert policy_step.policy_outcome == "retry_alternate_executor"
    assert policy_step.reason == "executor exceeded its budget"
    assert policy_step.excluded_executor_ids == ("executor-slow",)
    # "detail" is every payload key other than reason/excluded_executor_ids.
    assert policy_step.detail == {"retries_used": 1, "max_retries": 3}


def test_raw_lifecycle_events_become_steps_in_event_log_order(tmp_path: Path):
    graph = _escalation_graph()
    log, engine = _engine_for(graph, tmp_path)
    engine.apply("draft", "start")
    engine.apply("draft", "block")
    engine.apply("draft", "resume")
    engine.apply("draft", "handoff")
    engine.apply("draft", "accept")
    engine.apply("draft", "fail")

    views = _views_for(graph, log, engine)

    draft_view = next(view for view in views if view.node_id == "draft")
    assert tuple(step.event_type for step in draft_view.steps) == (
        "block",
        "resume",
        "handoff",
        "accept",
        "fail",
    )
    assert [step.seq for step in draft_view.steps] == sorted(
        step.seq for step in draft_view.steps
    )
    assert all(step.policy_outcome is None for step in draft_view.steps)
    assert all(step.excluded_executor_ids == () for step in draft_view.steps)


def test_handoff_node_is_awaiting_human_with_its_policy_outcome(tmp_path: Path):
    graph = load_graph(SAMPLE_GRAPH_PATH)
    log, engine = _engine_for(graph, tmp_path)
    engine.apply("intake", "start")
    engine.apply("intake", "handoff")
    _append_policy_event(
        log,
        run_id=engine.current_state().run_id,
        node_id="intake",
        event_type="policy-human-required",
        reason="authority scope not granted",
        detail={"unresolved_scopes": ["publish"]},
    )

    views = _views_for(graph, log, engine)

    intake_view = next(view for view in views if view.node_id == "intake")
    assert intake_view.status == NodeStatus.HANDOFF.value
    assert intake_view.awaiting_human is True

    policy_step = next(
        step for step in intake_view.steps if step.event_type == "policy-human-required"
    )
    assert policy_step.policy_outcome == "human_required"
    assert policy_step.detail == {"unresolved_scopes": ["publish"]}


def test_blocker_status_alone_yields_entry_without_escalation_target(tmp_path: Path):
    graph = load_graph(SAMPLE_GRAPH_PATH)
    log, engine = _engine_for(graph, tmp_path)
    engine.apply("intake", "start")
    engine.apply("intake", "block")

    views = _views_for(graph, log, engine)

    intake_view = next(view for view in views if view.node_id == "intake")
    assert intake_view.escalation_targets == ()
    assert intake_view.awaiting_human is False
    assert tuple(step.event_type for step in intake_view.steps) == ("block",)


def test_underscore_on_failure_edge_contributes_no_escalation_target(tmp_path: Path):
    # Assumption 7: the escalation token is hyphenated, matching
    # transitions.py::_advance_successors; graph.schema.json leaves edge "kind"
    # an open string, so an "on_failure" edge must silently not escalate.
    graph = _escalation_graph(on_failure_kind="on_failure")
    log, engine = _engine_for(graph, tmp_path)
    engine.apply("draft", "start")
    engine.apply("draft", "block")

    views = _views_for(graph, log, engine)

    draft_view = next(view for view in views if view.node_id == "draft")
    assert draft_view.escalation_targets == ()

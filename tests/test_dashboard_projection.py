"""Tests for the read-only run/node projection used by the dashboard.

Uses the same fixture pattern as tests/test_end_to_end_fake_executor.py:
real Graph/RunState/TransitionEngine/Event objects built from
examples/sample-graph.json and praxis_runtime.testing.fake_executor.FakeExecutor,
never hand-rolled fakes for those types.

The "policy-*" event constructed in several tests below mirrors the shape
produced by src/praxis_policy/receipts.py::record_policy_decision (event_type
prefixed "policy-", payload carrying a "reason" key) without importing
praxis_policy -- the projection module must not depend on that optional
package.
"""

from __future__ import annotations

from pathlib import Path

from praxis_runtime.events import Event, EventLog
from praxis_runtime.graph import load_graph
from praxis_runtime.state import RunStateStore
from praxis_runtime.testing.fake_executor import FakeExecutor
from praxis_runtime.transitions import NodeStatus, TransitionEngine

from praxis_dashboard.projection import build_node_views, build_run_summary, next_actions

SAMPLE_GRAPH_PATH = Path(__file__).resolve().parent.parent / "examples" / "sample-graph.json"


def _make_engine(tmp_path: Path):
    graph = load_graph(SAMPLE_GRAPH_PATH)
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log)
    return graph, log, engine


def _append_policy_event(
    log: EventLog, *, run_id: str, node_id: str, reason: str, event_id: str | None = None
) -> None:
    log.append(
        Event(
            spec_version="1.0.0",
            seq=0,
            run_id=run_id,
            node_id=node_id,
            event_type="policy-human-required",
            payload={"reason": reason, "excluded_executor_ids": []},
            event_id=event_id or f"policy-{node_id}",
        )
    )


def test_pending_node_shows_start_in_legal_next_events(tmp_path: Path):
    graph, log, engine = _make_engine(tmp_path)
    state = engine.current_state()

    views = build_node_views(graph, state, engine, log.read_all())

    intake_view = next(v for v in views if v.node_id == "intake")
    assert intake_view.kind == "intake"
    assert "start" in intake_view.legal_next_events


def test_blocked_node_via_real_transition_is_blocker(tmp_path: Path):
    graph, log, engine = _make_engine(tmp_path)
    engine.apply("intake", "start")
    engine.apply("intake", "block")
    state = engine.current_state()

    views = build_node_views(graph, state, engine, log.read_all())

    intake_view = next(v for v in views if v.node_id == "intake")
    assert intake_view.status == NodeStatus.BLOCKED.value
    assert intake_view.is_blocker is True


def test_blocked_reason_is_none_without_policy_event(tmp_path: Path):
    graph, log, engine = _make_engine(tmp_path)
    engine.apply("intake", "start")
    engine.apply("intake", "block")
    state = engine.current_state()

    views = build_node_views(graph, state, engine, log.read_all())

    intake_view = next(v for v in views if v.node_id == "intake")
    assert intake_view.blocked_reason is None


def test_blocked_reason_comes_from_most_recent_policy_event(tmp_path: Path):
    graph, log, engine = _make_engine(tmp_path)
    engine.apply("intake", "start")
    engine.apply("intake", "block")
    state = engine.current_state()
    _append_policy_event(
        log, run_id=state.run_id, node_id="intake", reason="manual review required"
    )

    views = build_node_views(graph, state, engine, log.read_all())

    intake_view = next(v for v in views if v.node_id == "intake")
    assert intake_view.blocked_reason == "manual review required"


def test_blocked_reason_uses_most_recent_of_multiple_policy_events(tmp_path: Path):
    graph, log, engine = _make_engine(tmp_path)
    engine.apply("intake", "start")
    engine.apply("intake", "block")
    state = engine.current_state()
    _append_policy_event(
        log,
        run_id=state.run_id,
        node_id="intake",
        reason="first reviewer flagged",
        event_id="policy-intake-1",
    )
    _append_policy_event(
        log,
        run_id=state.run_id,
        node_id="intake",
        reason="second reviewer flagged",
        event_id="policy-intake-2",
    )

    views = build_node_views(graph, state, engine, log.read_all())

    intake_view = next(v for v in views if v.node_id == "intake")
    assert intake_view.blocked_reason == "second reviewer flagged"


def test_handoff_status_is_blocker(tmp_path: Path):
    graph, log, engine = _make_engine(tmp_path)
    engine.apply("intake", "start")
    engine.apply("intake", "handoff")
    state = engine.current_state()

    views = build_node_views(graph, state, engine, log.read_all())

    intake_view = next(v for v in views if v.node_id == "intake")
    assert intake_view.status == NodeStatus.HANDOFF.value
    assert intake_view.is_blocker is True


def test_is_complete_false_when_only_some_fanout_branches_terminal(tmp_path: Path):
    graph, log, engine = _make_engine(tmp_path)
    engine.apply("intake", "start")
    engine.apply("intake", "complete")
    engine.apply("review-legal", "start")
    engine.apply("review-legal", "complete")
    state = engine.current_state()

    assert state.cursors["review-legal"].status == NodeStatus.TERMINAL_SUCCESS.value
    assert state.cursors["review-editorial"].status != NodeStatus.TERMINAL_SUCCESS.value

    summary = build_run_summary(state)

    assert summary.is_complete is False


def test_build_run_summary_zero_fills_every_node_status(tmp_path: Path):
    graph, log, engine = _make_engine(tmp_path)
    state = engine.current_state()

    summary = build_run_summary(state)

    for status in NodeStatus:
        assert status.value in summary.counts_by_status
    assert summary.counts_by_status[NodeStatus.PENDING.value] == 1
    assert summary.total_nodes == 1
    assert summary.is_complete is False


def test_build_run_summary_is_complete_true_once_run_finishes(tmp_path: Path):
    graph, log, engine = _make_engine(tmp_path)
    script = {node_id: {"event_type": "complete", "evidence": None} for node_id in graph.nodes}

    mid_state = engine.current_state()
    assert build_run_summary(mid_state).is_complete is False

    final_state = FakeExecutor(engine, script).run_to_completion()

    assert build_run_summary(final_state).is_complete is True


def test_next_actions_reports_runnable_node_and_blocker_with_reason(tmp_path: Path):
    graph, log, engine = _make_engine(tmp_path)
    engine.apply("intake", "start")
    engine.apply("intake", "complete")
    engine.apply("review-legal", "start")
    engine.apply("review-legal", "block")
    state = engine.current_state()
    _append_policy_event(
        log, run_id=state.run_id, node_id="review-legal", reason="manual review required"
    )

    views = build_node_views(graph, state, engine, log.read_all())
    actions = next_actions(views)

    review_legal_view = next(v for v in views if v.node_id == "review-legal")
    assert review_legal_view.kind == "review"

    assert any(
        action == "review-editorial can be advanced via: start" for action in actions
    )
    assert any(
        action == "review-legal is blocked: manual review required" for action in actions
    )

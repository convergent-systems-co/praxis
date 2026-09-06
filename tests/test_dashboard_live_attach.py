"""Live-attach update test: proves DashboardSource.poll_live tracks a run
step-by-step, not just at the end, and that being observed never affects the
run it is attached to.

Drives `examples/sample-graph.json` (the same fan-out/join graph
`tests/test_end_to_end_fake_executor.py` uses) partway through -- `intake`
then `review-legal` only, mirroring that test's own mid-run assertions --
with a real `TransitionEngine`. A single, long-lived `DashboardSource` is
constructed once, before any transition is applied, and reused for every
poll: since it is never the object that drives the run, every poll_live()
call in this test observes updates made through a genuinely different
`TransitionEngine` instance, which is also the shape of the real cross-process
scenario (a dashboard process attached to a run another process is driving).
"""

from __future__ import annotations

from pathlib import Path

from praxis_dashboard.sources import DashboardSource
from praxis_runtime.events import EventLog
from praxis_runtime.graph import load_graph
from praxis_runtime.state import RunStateStore
from praxis_runtime.transitions import NodeStatus, TransitionEngine

SAMPLE_GRAPH_PATH = Path(__file__).resolve().parent.parent / "examples" / "sample-graph.json"


def _node_view(snapshot, node_id: str):
    return next(v for v in snapshot.nodes if v.node_id == node_id)


def _drive_to_terminal(engine: TransitionEngine, node_id: str) -> None:
    if "start" in engine.legal_next(node_id):
        engine.apply(node_id, "start")
    engine.apply(node_id, "complete")


def test_poll_live_tracks_run_step_by_step_without_affecting_it(tmp_path: Path):
    graph = load_graph(SAMPLE_GRAPH_PATH)
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log)

    # Constructed before any transition is applied, and reused for every
    # poll below -- this is the "long-lived" option T7 allows, chosen here
    # specifically because it also proves observation across a different
    # engine instance (see module docstring).
    source = DashboardSource(SAMPLE_GRAPH_PATH, tmp_path)

    # Step 1: intake starts running.
    engine.apply("intake", "start")
    snap = source.poll_live()
    assert _node_view(snap, "intake").status == NodeStatus.RUNNING.value
    assert snap.run_summary.counts_by_status["running"] == 1
    assert snap.run_summary.counts_by_status["pending"] == 0

    # Step 2: intake completes -- its two fan-out successors appear as
    # pending immediately, before either review has started.
    engine.apply("intake", "complete")
    snap = source.poll_live()
    assert _node_view(snap, "intake").status == NodeStatus.TERMINAL_SUCCESS.value
    assert _node_view(snap, "review-legal").status == NodeStatus.PENDING.value
    assert _node_view(snap, "review-editorial").status == NodeStatus.PENDING.value
    assert snap.run_summary.counts_by_status["terminal_success"] == 1
    assert snap.run_summary.counts_by_status["pending"] == 2

    # Step 3: review-legal starts running; review-editorial is untouched.
    engine.apply("review-legal", "start")
    snap = source.poll_live()
    assert _node_view(snap, "review-legal").status == NodeStatus.RUNNING.value
    assert _node_view(snap, "review-editorial").status == NodeStatus.PENDING.value
    assert snap.run_summary.counts_by_status["running"] == 1
    assert snap.run_summary.counts_by_status["terminal_success"] == 1
    assert snap.run_summary.counts_by_status["pending"] == 1

    # Step 4: review-legal completes. The join target "decision" must not
    # appear yet -- review-editorial, its other incoming source, has not
    # reached TERMINAL_SUCCESS -- mirroring
    # test_end_to_end_fake_executor.py's own mid_state assertions.
    engine.apply("review-legal", "complete")
    snap = source.poll_live()
    assert _node_view(snap, "review-legal").status == NodeStatus.TERMINAL_SUCCESS.value
    assert _node_view(snap, "review-editorial").status == NodeStatus.PENDING.value
    assert all(v.node_id != "decision" for v in snap.nodes)
    assert snap.run_summary.counts_by_status["terminal_success"] == 2
    assert snap.run_summary.counts_by_status["pending"] == 1

    # Having been polled at every step above must not have affected the run:
    # the same external engine can still legally drive the remaining nodes
    # to completion.
    _drive_to_terminal(engine, "review-editorial")
    _drive_to_terminal(engine, "decision")
    _drive_to_terminal(engine, "revise")
    _drive_to_terminal(engine, "approve")
    _drive_to_terminal(engine, "archive")

    final_state = engine.current_state()
    assert set(final_state.cursors) == set(graph.nodes)
    for node_id in graph.nodes:
        assert final_state.cursors[node_id].status == NodeStatus.TERMINAL_SUCCESS.value

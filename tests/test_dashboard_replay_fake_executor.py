"""Replay-after-exit and fake-executor functional test.

Drives examples/sample-graph.json (a fan-out/join graph) to full completion
with praxis_runtime.testing.fake_executor.FakeExecutor against a real
run_directory on tmp_path -- the same convention (run-state.json +
an events/ subdirectory) used by tests/test_end_to_end_fake_executor.py and
tests/test_dashboard_sources.py -- then explicitly closes the EventLog used
to drive the run, simulating the owning process exiting with no live
TransitionEngine/EventLog instance kept around.

A fresh DashboardSource constructed afterward must still be able to replay
the run purely from durable records (replay_snapshot) and to poll it live
(poll_live), which for this run exercises the checkpoint-present path since
TransitionEngine.apply's own state_store.save wrote run-state.json on every
transition. Both attach modes must agree on every node's terminal status,
proving the dashboard remains functional for a deterministic fake-executor
run after the process that ran it has gone away.
"""

from __future__ import annotations

from pathlib import Path

from praxis_dashboard.sources import DashboardSource
from praxis_runtime.events import EventLog
from praxis_runtime.graph import load_graph
from praxis_runtime.state import RunStateStore
from praxis_runtime.testing.fake_executor import FakeExecutor
from praxis_runtime.transitions import NodeStatus, TransitionEngine

SAMPLE_GRAPH_PATH = Path(__file__).resolve().parent.parent / "examples" / "sample-graph.json"


def test_replay_after_process_exit_agrees_with_live_poll_for_completed_run(tmp_path: Path):
    graph = load_graph(SAMPLE_GRAPH_PATH)
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log)

    script = {
        node_id: {"event_type": "complete", "evidence": None} for node_id in graph.nodes
    }
    FakeExecutor(engine, script).run_to_completion()

    # Simulate process exit: close the EventLog (releases its file handle)
    # and drop every reference to the live engine/store/log -- nothing from
    # the run above is kept around for the fresh DashboardSource below.
    log.close()
    del engine, store, log

    source = DashboardSource(SAMPLE_GRAPH_PATH, tmp_path)

    replayed = source.replay_snapshot()

    assert replayed.mode == "replay"
    assert replayed.run_summary.is_complete is True
    for node_view in replayed.nodes:
        assert node_view.status == NodeStatus.TERMINAL_SUCCESS.value

    # (tmp_path / "run-state.json").exists() is True here (unlike
    # test_dashboard_sources.py's replay test, which deletes it to prove
    # replay ignores it) -- this run's checkpoint was written by
    # TransitionEngine.apply's own state_store.save, so poll_live below
    # exercises the checkpoint-present path rather than the no-checkpoint
    # fallback.
    live = source.poll_live()

    assert live.mode == "live"
    replayed_statuses = {v.node_id: v.status for v in replayed.nodes}
    live_statuses = {v.node_id: v.status for v in live.nodes}
    assert live_statuses == replayed_statuses
    for status in live_statuses.values():
        assert status == NodeStatus.TERMINAL_SUCCESS.value

"""End-to-end acceptance test: a non-development sample graph run to
completion with the deterministic fake-executor harness.

`examples/sample-graph.json` is a generic document-review pipeline (no
software-development vocabulary) with a fan-out (intake splits into two
parallel reviews) and a join (both reviews must complete before approval),
exercising both edge kinds end to end through the same public
load_graph/TransitionEngine/FakeExecutor surface real callers use.
"""

from __future__ import annotations

from pathlib import Path

from praxis_runtime.events import EventLog
from praxis_runtime.graph import load_graph
from praxis_runtime.state import RunStateStore
from praxis_runtime.testing.fake_executor import FakeExecutor
from praxis_runtime.transitions import NodeStatus, TransitionEngine

SAMPLE_GRAPH_PATH = Path(__file__).resolve().parent.parent / "examples" / "sample-graph.json"


class _CountingEngine:
    """Wraps a TransitionEngine to count applied transitions, so the test
    can assert the event log's count matches transitions applied without
    duplicating TransitionEngine's own bookkeeping.
    """

    def __init__(self, engine: TransitionEngine) -> None:
        self._engine = engine
        self.apply_count = 0

    def current_state(self):
        return self._engine.current_state()

    def legal_next(self, node_id: str):
        return self._engine.legal_next(node_id)

    def apply(self, node_id: str, event_type: str, *, evidence: dict | None = None):
        self.apply_count += 1
        return self._engine.apply(node_id, event_type, evidence=evidence)


def _drive_node_to_terminal(engine, script: dict, node_id: str) -> None:
    """Step a single node from its current status to a terminal status,
    applying the mechanical "start" transition first if legal.
    """
    legal = engine.legal_next(node_id)
    if "start" in legal:
        engine.apply(node_id, "start")
    scripted = script[node_id]
    engine.apply(node_id, scripted["event_type"], evidence=scripted.get("evidence"))


def test_sample_graph_node_kinds_do_not_reuse_edge_kind_vocabulary():
    # Node "kind" (domain role) and edge "kind" (topology) are separate
    # vocabularies -- a node must not be labeled with an edge-kind term like
    # "fan-out"/"join"/"sequential", which would conflate the two.
    graph = load_graph(SAMPLE_GRAPH_PATH)
    edge_kind_terms = {edge.kind for edge in graph.edges}
    for node in graph.nodes.values():
        assert node.kind not in edge_kind_terms


def test_sample_graph_runs_to_completion_with_fake_executor(tmp_path: Path):
    graph = load_graph(SAMPLE_GRAPH_PATH)
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log)
    counting_engine = _CountingEngine(engine)

    script = {
        node_id: {"event_type": "complete", "evidence": None} for node_id in graph.nodes
    }

    # Drive the fan-out through only one branch (review-legal) and confirm the
    # join target ("decision") is not created until the other branch
    # (review-editorial) also reaches TERMINAL_SUCCESS -- the "join" half of
    # the fan-out/join acceptance criterion.
    _drive_node_to_terminal(counting_engine, script, "intake")
    _drive_node_to_terminal(counting_engine, script, "review-legal")

    mid_state = counting_engine.current_state()
    assert "decision" not in mid_state.cursors
    assert mid_state.cursors["review-legal"].status == NodeStatus.TERMINAL_SUCCESS.value
    assert mid_state.cursors["review-editorial"].status == NodeStatus.PENDING.value

    executor = FakeExecutor(counting_engine, script)
    final_state = executor.run_to_completion()

    assert set(final_state.cursors) == set(graph.nodes)
    for node_id in graph.nodes:
        assert final_state.cursors[node_id].status == NodeStatus.TERMINAL_SUCCESS.value

    assert len(log.read_all()) == counting_engine.apply_count

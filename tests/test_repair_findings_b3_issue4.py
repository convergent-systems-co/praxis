"""Regression tests for repair-findings.md (bundle b3-issue4).

Each test reproduces one finding before its fix and must pass after it:

1. `replay._fold_events` opens a scratch `EventLog` but never closes it (no
   `close()` call, no context-manager use), leaking the underlying file
   handle every time `replay()`/`resume()` folds pending events. Only masked
   today by CPython's refcounting GC closing the handle as an implementation
   detail.
2. `examples/sample-graph.json`'s "decision" node has two outgoing edges
   both labeled `kind: "sequential"`. `TransitionEngine._advance_successors`
   treats every non-join outgoing edge identically (all fire unconditionally
   once the source completes), so two "sequential" edges from one source
   behave exactly like fan-out, not like an exclusive branch -- the example
   doesn't actually model a decision despite the node's kind/id.
"""

from __future__ import annotations

import json
from collections import defaultdict
from pathlib import Path

from praxis_runtime.events import EventLog
from praxis_runtime.graph import Edge, Graph, Node
from praxis_runtime.replay import replay
from praxis_runtime.state import RunStateStore
from praxis_runtime.transitions import TransitionEngine

_SPEC_VERSION = "1.0.0"
REPO_ROOT = Path(__file__).resolve().parent.parent
SAMPLE_GRAPH_PATH = REPO_ROOT / "examples" / "sample-graph.json"


def _linear_graph() -> Graph:
    return Graph(
        spec_version=_SPEC_VERSION,
        nodes={
            "n1": Node(id="n1", kind="task"),
            "n2": Node(id="n2", kind="task"),
        },
        edges=[Edge(source="n1", target="n2", kind="sequential")],
        entry_node="n1",
        terminal_nodes={"n2"},
    )


def test_replay_closes_its_scratch_event_log(monkeypatch, tmp_path: Path):
    graph = _linear_graph()
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log)

    engine.apply("n1", "start")
    engine.apply("n1", "complete")

    closed_handles = []
    original_close = EventLog.close

    def tracking_close(self):
        closed_handles.append(self)
        original_close(self)

    monkeypatch.setattr(EventLog, "close", tracking_close)

    replay(log, graph)

    assert closed_handles, (
        "replay()'s scratch EventLog (opened inside _fold_events) was never "
        "closed -- this leaks a file descriptor per replay()/resume() call "
        "with pending events"
    )


def test_sample_graph_does_not_mislabel_fan_out_as_sequential():
    document = json.loads(SAMPLE_GRAPH_PATH.read_text())

    outgoing_by_source = defaultdict(list)
    for edge in document["edges"]:
        outgoing_by_source[edge["source"]].append(edge)

    for source, edges in outgoing_by_source.items():
        if len(edges) <= 1:
            continue
        sequential_targets = [edge["target"] for edge in edges if edge["kind"] == "sequential"]
        assert not sequential_targets, (
            f"node {source!r} has {len(edges)} outgoing edges but labels "
            f"{sequential_targets} as 'sequential' -- TransitionEngine fires "
            "every non-join outgoing edge unconditionally once the source "
            "completes, so this behaves exactly like fan-out, not an "
            "exclusive branch, despite the label implying otherwise"
        )

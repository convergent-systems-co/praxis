"""Document the evidence-gate limitation of an executor escalation ladder.

An attempt carrying an evidence requirement cannot be failed by the ladder:
the runtime grades evidence on `fail` too, so an unsatisfied gate prevents the
`on-failure` edge from creating the next review rung. The supported ladder
surface is deliberately evidence-free and is documented in
`docs/orchestration.md`.
"""

from __future__ import annotations

from dataclasses import replace
from pathlib import Path

import pytest

from praxis_orchestration import build_escalation_ladder
from praxis_runtime.graph import Graph, Node
from praxis_runtime.events import EventLog
from praxis_runtime.state import RunStateStore
from praxis_runtime.transitions import TransitionEngine, TransitionError


def _engine(tmp_path: Path, nodes: list[Node], edges):
    return TransitionEngine(
        Graph(
            spec_version="1.0.0",
            nodes={node.id: node for node in nodes},
            edges=edges,
            entry_node="review-first",
            terminal_nodes={"review-human"},
        ),
        RunStateStore(tmp_path / "run-state.json"),
        EventLog(tmp_path / "events"),
    )


def test_evidence_requirement_deadlocks_failure_edge(tmp_path: Path):
    nodes, edges = build_escalation_ladder(
        attempt_node_ids=["review-first", "review-second"],
        human_node_id="review-human",
    )
    requirement_node = replace(
        nodes[0],
        metadata={
            "evidence_requirement": {
                "spec_version": "1.0.0",
                "evidence": [
                    {"proof_type": "peer-attestation", "constraint": "required"}
                ],
            }
        },
    )
    engine = _engine(tmp_path, [requirement_node, *nodes[1:]], edges)
    engine.apply("review-first", "start")

    with pytest.raises(TransitionError):
        engine.apply("review-first", "fail")

    assert "review-second" not in engine.current_state().cursors


def test_evidence_free_ladder_failure_creates_next_rung(tmp_path: Path):
    nodes, edges = build_escalation_ladder(
        attempt_node_ids=["review-first", "review-second"],
        human_node_id="review-human",
    )
    engine = _engine(tmp_path, nodes, edges)
    engine.apply("review-first", "start")
    engine.apply("review-first", "fail")

    assert engine.current_state().cursors["review-second"].status == "pending"

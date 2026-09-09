"""Pins T10's edge-kind fix in isolation from tests/test_overlay_development.py
(owned by a concurrent task): bundle_verify->repair_bundle,
final_review->repair_bundle, and repair_bundle->awaiting_human must be
`on-failure` edges, not `sequential`, so TransitionEngine._advance_successors
only routes to repair_bundle/awaiting_human when the source reaches
TERMINAL_FAILED (see src/praxis_runtime/transitions.py's on-failure handling).
"""

from __future__ import annotations

from overlays.development.graph import build_development_graph

_EXPECTED_ON_FAILURE_EDGES = {
    ("bundle_verify", "repair_bundle"),
    ("final_review", "repair_bundle"),
    ("repair_bundle", "awaiting_human"),
}


def test_repair_and_awaiting_human_edges_are_on_failure_kind():
    graph = build_development_graph()

    on_failure_edges = {
        (edge.source, edge.target) for edge in graph.edges if edge.kind == "on-failure"
    }

    assert on_failure_edges == _EXPECTED_ON_FAILURE_EDGES


def test_no_sequential_edges_target_repair_bundle_or_awaiting_human():
    graph = build_development_graph()

    offending = [
        edge
        for edge in graph.edges
        if edge.kind == "sequential"
        and (edge.source, edge.target) in _EXPECTED_ON_FAILURE_EDGES
    ]

    assert offending == []

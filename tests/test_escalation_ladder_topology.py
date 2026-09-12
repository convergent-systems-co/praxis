"""Topology and transport-policy contracts for escalation ladders."""

from __future__ import annotations

import json
from pathlib import Path

import pytest

from praxis_orchestration import ATTEMPT_TIERS, build_escalation_ladder
from praxis_orchestration.escalation import tier_transport_policy
from praxis_runtime.graph import load_graph


def test_three_rung_builder_returns_a_hyphenated_failure_chain(tmp_path: Path):
    attempt_ids = ["review-first", "review-second", "review-third"]
    nodes, edges = build_escalation_ladder(
        attempt_node_ids=attempt_ids, human_node_id="review-human"
    )

    assert [node.id for node in nodes] == [*attempt_ids, "review-human"]
    assert len(nodes) == 4
    assert len(edges) == 3
    assert [(edge.source, edge.target) for edge in edges] == [
        ("review-first", "review-second"),
        ("review-second", "review-third"),
        ("review-third", "review-human"),
    ]
    assert {edge.kind for edge in edges} == {"on-failure"}
    assert all("evidence_requirement" not in node.metadata for node in nodes)

    instance = {
        "spec_version": "1.0.0",
        "nodes": [
            {"id": node.id, "kind": node.kind, "metadata": node.metadata}
            for node in nodes
        ]
        + [{"id": "review-entry", "kind": "review-entry"}],
        "edges": [
            {"source": "review-entry", "target": "review-first", "kind": "sequential"}
        ]
        + [
            {"source": edge.source, "target": edge.target, "kind": edge.kind}
            for edge in edges
        ],
        "entry_node": "review-entry",
        "terminal_nodes": ["review-human"],
    }
    path = tmp_path / "review-graph.json"
    path.write_text(json.dumps(instance))
    graph = load_graph(path)
    assert graph.edges[-1].kind == "on-failure"


def test_tier_table_and_policy_never_opt_into_paid_transports():
    assert ATTEMPT_TIERS == (
        frozenset({"local"}),
        frozenset({"subscription_cli"}),
        None,
    )
    for index in range(len(ATTEMPT_TIERS)):
        policy = tier_transport_policy(index)
        assert "metered_api" not in (policy.allowed_auth_transports or frozenset())
        assert "api_key" not in (policy.allowed_auth_transports or frozenset())


@pytest.mark.parametrize(
    "attempt_ids,human_id",
    [([], "review-human"), (["review-first", "review-first"], "review-human"), (["review-first"], "review-first")],
)
def test_builder_rejects_malformed_identifiers(attempt_ids, human_id):
    with pytest.raises(ValueError):
        build_escalation_ladder(
            attempt_node_ids=attempt_ids, human_node_id=human_id
        )

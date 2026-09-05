"""Graph loader and validator behavior.

load_graph reads a JSON graph document, validates it against
graph.schema.json via praxis_contracts.validator.validate_document, then
checks graph-level invariants JSON Schema can't express: edges reference
existing node ids, entry_node exists, every node is reachable from
entry_node, and terminal_nodes reference existing node ids. Any violation
fails closed with GraphValidationError.
"""

from __future__ import annotations

import copy
import json
from pathlib import Path

import pytest

from praxis_runtime.graph import GraphValidationError, load_graph

VALID_GRAPH = {
    "spec_version": "1.0.0",
    "nodes": [
        {"id": "start", "kind": "start"},
        {"id": "middle", "kind": "task"},
        {"id": "end", "kind": "end"},
    ],
    "edges": [
        {"source": "start", "target": "middle", "kind": "sequential"},
        {"source": "middle", "target": "end", "kind": "sequential"},
    ],
    "entry_node": "start",
    "terminal_nodes": ["end"],
}


def _write_graph(tmp_path: Path, instance: dict) -> Path:
    path = tmp_path / "graph.json"
    path.write_text(json.dumps(instance))
    return path


def test_well_formed_graph_loads(tmp_path: Path):
    path = _write_graph(tmp_path, VALID_GRAPH)

    graph = load_graph(path)

    assert graph.spec_version == "1.0.0"
    assert graph.entry_node == "start"
    assert graph.terminal_nodes == {"end"}
    assert set(graph.nodes.keys()) == {"start", "middle", "end"}
    assert graph.nodes["start"].kind == "start"
    assert len(graph.edges) == 2


def test_dangling_edge_reference_fails_closed(tmp_path: Path):
    instance = copy.deepcopy(VALID_GRAPH)
    instance["edges"].append(
        {"source": "middle", "target": "nonexistent", "kind": "sequential"}
    )
    path = _write_graph(tmp_path, instance)

    with pytest.raises(GraphValidationError):
        load_graph(path)


def test_unreachable_node_fails_closed(tmp_path: Path):
    instance = copy.deepcopy(VALID_GRAPH)
    instance["nodes"].append({"id": "orphan", "kind": "task"})
    path = _write_graph(tmp_path, instance)

    with pytest.raises(GraphValidationError):
        load_graph(path)


def test_entry_node_dangling_reference_fails_closed(tmp_path: Path):
    instance = copy.deepcopy(VALID_GRAPH)
    instance["entry_node"] = "nonexistent"
    path = _write_graph(tmp_path, instance)

    with pytest.raises(GraphValidationError) as excinfo:
        load_graph(path)

    assert "entry_node references unknown node" in str(excinfo.value)


def test_terminal_nodes_dangling_reference_fails_closed(tmp_path: Path):
    instance = copy.deepcopy(VALID_GRAPH)
    instance["terminal_nodes"].append("nonexistent")
    path = _write_graph(tmp_path, instance)

    with pytest.raises(GraphValidationError):
        load_graph(path)


def test_version_mismatch_surfaces_distinct_message(tmp_path: Path):
    instance = copy.deepcopy(VALID_GRAPH)
    instance["spec_version"] = "2.0.0"
    path = _write_graph(tmp_path, instance)

    with pytest.raises(GraphValidationError) as excinfo:
        load_graph(path)

    assert "version mismatch" in str(excinfo.value).lower()

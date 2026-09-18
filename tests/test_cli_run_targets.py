"""Target resolution for ``praxis run <target>`` through graph plugins."""

from __future__ import annotations

import copy
import json
from pathlib import Path

import pytest

from praxis_cli.run_targets import TargetError, available_graph_targets, resolve_target
from praxis_runtime.graph import Graph, GraphValidationError, load_graph

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


def test_builtin_graph_targets_are_discovered_from_plugins():
    targets = set(available_graph_targets())
    assert {"trivial", "development"} <= targets


def test_existing_graph_document_resolves_to_that_graph(tmp_path: Path):
    path = _write_graph(tmp_path, VALID_GRAPH)
    graph = resolve_target(str(path))
    assert isinstance(graph, Graph)
    assert set(graph.nodes) == {"start", "middle", "end"}


def test_invalid_graph_document_raises_target_error_carrying_loader_message(tmp_path: Path):
    instance = copy.deepcopy(VALID_GRAPH)
    instance["edges"].append({"source": "middle", "target": "nonexistent", "kind": "sequential"})
    path = _write_graph(tmp_path, instance)
    with pytest.raises(GraphValidationError) as loader_exc:
        load_graph(path)
    with pytest.raises(TargetError) as exc:
        resolve_target(str(path))
    assert str(loader_exc.value) in str(exc.value)


def test_unparseable_graph_document_raises_target_error(tmp_path: Path):
    path = tmp_path / "graph.json"
    path.write_text("this is not JSON")
    with pytest.raises(TargetError):
        resolve_target(str(path))


@pytest.mark.parametrize("graph_id", ("trivial", "development"))
def test_builtin_graph_plugin_ids_resolve_to_graphs(graph_id: str):
    graph = resolve_target(graph_id)
    assert isinstance(graph, Graph)
    assert graph.nodes
    assert graph.entry_node in graph.nodes


def test_unknown_target_names_installed_graph_ids_and_says_it_was_not_a_path():
    with pytest.raises(TargetError) as exc:
        resolve_target("no-such-graph")
    message = str(exc.value)
    for graph_id in available_graph_targets():
        assert graph_id in message
    assert "path" in message.lower()


def test_directory_target_raises_target_error(tmp_path: Path):
    directory = tmp_path / "graphs"
    directory.mkdir()
    with pytest.raises(TargetError):
        resolve_target(str(directory))


def test_unknown_target_writes_nothing_to_working_directory(tmp_path: Path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    with pytest.raises(TargetError):
        resolve_target("no-such-graph")
    assert list(tmp_path.iterdir()) == []

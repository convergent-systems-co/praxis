"""Target resolution for `praxis run <target>`.

`resolve_target` accepts either a path to a graph document or one of a fixed
set of known overlay ids, and returns a `Graph` either way. Every failure --
an unparseable or invalid document, a path that is a directory, an id that is
neither -- surfaces as `TargetError` carrying a message the CLI can print, so
a caller never has to catch loader-specific or OS-level exceptions.

Resolution itself writes nothing: no run directory, no file in the working
directory.
"""

from __future__ import annotations

import copy
import json
import os
import subprocess
import sys
from pathlib import Path

import pytest

from praxis_cli.run_targets import KNOWN_OVERLAY_IDS, TargetError, resolve_target
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


def test_known_overlay_ids_are_trivial_and_development():
    assert KNOWN_OVERLAY_IDS == ("trivial", "development")


def test_existing_graph_document_resolves_to_that_graph(tmp_path: Path):
    path = _write_graph(tmp_path, VALID_GRAPH)

    graph = resolve_target(str(path))

    assert isinstance(graph, Graph)
    assert set(graph.nodes) == {"start", "middle", "end"}
    assert graph.entry_node == "start"
    assert graph.terminal_nodes == {"end"}


def test_invalid_graph_document_raises_target_error_carrying_loader_message(tmp_path: Path):
    instance = copy.deepcopy(VALID_GRAPH)
    instance["edges"].append(
        {"source": "middle", "target": "nonexistent", "kind": "sequential"}
    )
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


# Node ids each overlay produces and the other does not, so an id bound to the
# wrong builder -- or to a graph built here instead of by the overlay -- fails.
OVERLAY_MARKER_NODE_IDS = {
    "trivial": frozenset({"draft", "publish"}),
    "development": frozenset({"write_tdd", "bundle_verify", "commit_task"}),
}


@pytest.mark.parametrize("overlay_id", KNOWN_OVERLAY_IDS)
def test_each_known_overlay_id_resolves_to_a_graph(overlay_id: str):
    graph = resolve_target(overlay_id)

    assert isinstance(graph, Graph)
    assert graph.nodes
    assert graph.entry_node in graph.nodes


@pytest.mark.parametrize("overlay_id", KNOWN_OVERLAY_IDS)
def test_each_known_overlay_id_resolves_to_that_overlays_own_nodes(overlay_id: str):
    graph = resolve_target(overlay_id)

    assert OVERLAY_MARKER_NODE_IDS[overlay_id] <= set(graph.nodes)
    for other_id, other_markers in OVERLAY_MARKER_NODE_IDS.items():
        if other_id != overlay_id:
            assert not (other_markers & set(graph.nodes))


@pytest.mark.parametrize("overlay_id", KNOWN_OVERLAY_IDS)
def test_each_known_overlay_id_resolves_to_the_graph_its_builder_returns(overlay_id: str):
    # The expectation comes from the overlay itself, so an implementation that
    # synthesises a graph instead of calling the builder cannot match it.
    from overlays.development.graph import build_development_graph
    from overlays.trivial.overlay import build_trivial_graph

    builders = {"trivial": build_trivial_graph, "development": build_development_graph}
    expected = builders[overlay_id]()

    graph = resolve_target(overlay_id)

    assert set(graph.nodes) == set(expected.nodes)
    assert graph.entry_node == expected.entry_node
    assert graph.terminal_nodes == expected.terminal_nodes


def test_known_overlay_ids_resolve_to_distinct_graphs():
    graphs = [resolve_target(overlay_id) for overlay_id in KNOWN_OVERLAY_IDS]

    node_id_sets = [frozenset(graph.nodes) for graph in graphs]
    assert len(set(node_id_sets)) == len(KNOWN_OVERLAY_IDS)


def test_unknown_target_names_every_known_id_and_says_it_was_not_a_path():
    with pytest.raises(TargetError) as exc:
        resolve_target("no-such-overlay")

    message = str(exc.value)
    for overlay_id in KNOWN_OVERLAY_IDS:
        assert overlay_id in message
    assert "path" in message.lower()


def test_directory_target_raises_target_error_not_is_a_directory_error(tmp_path: Path):
    directory = tmp_path / "graphs"
    directory.mkdir()

    with pytest.raises(TargetError):
        resolve_target(str(directory))


def test_unknown_target_writes_nothing_to_the_working_directory(tmp_path: Path, monkeypatch):
    monkeypatch.chdir(tmp_path)

    with pytest.raises(TargetError):
        resolve_target("no-such-overlay")

    assert list(tmp_path.iterdir()) == []


def test_importing_the_module_does_not_import_the_overlay_builders():
    # The builders are imported inside `resolve_target`, so importing this
    # module costs only what a target actually needs. Run in a fresh
    # interpreter, since this session has already imported the overlays.
    import praxis_cli

    source_root = str(Path(praxis_cli.__file__).resolve().parent.parent)
    program = (
        "import sys; import praxis_cli.run_targets; "
        "print(any(m.startswith('overlays') for m in sys.modules))"
    )

    result = subprocess.run(
        [sys.executable, "-c", program],
        capture_output=True,
        text=True,
        env={**os.environ, "PYTHONPATH": source_root},
    )

    assert result.returncode == 0, result.stderr
    assert result.stdout.strip() == "False"

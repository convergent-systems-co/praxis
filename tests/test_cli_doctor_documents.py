"""Tests for `praxis_cli.doctor_documents` -- `doctor` check 3, criterion 10:
the validity of the documents named by `--graph` and `--overlay-manifest`.

Three things are held here that nothing else in the suite holds:

1. The skip path. With no document flags the check must *state* that it skipped
   and must leave the exit code alone, which under criterion 7's vocabulary
   (`praxis_cli.doctor_report.Verdict`) means the verdict `ok` -- not `warn`,
   which would still print a non-`ok` block, and not `fail`, which would make
   `doctor` exit 1 on a machine with nothing wrong with it. The check also does
   no document *discovery*: a graph sitting in the working directory is not a
   document the user asked about (spec: explicitly out of scope).

2. Delegation to the real loaders. The check owns no validation logic of its
   own -- `praxis_runtime.graph.load_graph` (`src/praxis_runtime/graph.py:50-101`)
   and `praxis_overlay.manifest.load_manifest`
   (`src/praxis_overlay/manifest.py:65-84`) decide what is valid, and the tests
   below compare the reported message against what those loaders actually raise
   rather than against a copied literal, so a change in loader wording cannot
   leave `doctor` printing a stale message.

3. Every document is attempted. One unreadable file must not hide the rest, so
   a failing document still leaves a report line for each of its neighbours,
   and the check's single verdict is `fail` if *any* document failed.

`load_graph` takes a path and reads the file itself; `load_manifest` takes an
already-parsed document, so the check is the one that turns a manifest path
into JSON, which is why a non-JSON manifest and a non-JSON graph fail through
two different code paths and are tested separately.
"""

from __future__ import annotations

import copy
import json
from pathlib import Path

import pytest

from praxis_cli.doctor_documents import check_documents
from praxis_cli.doctor_report import CheckResult
from praxis_overlay.manifest import OverlayManifestError, load_manifest
from praxis_runtime.graph import GraphValidationError, load_graph

# A minimal graph the schema and the structural checks both accept: two nodes,
# one edge, every node reachable from `entry_node`.
VALID_GRAPH_DOCUMENT = {
    "spec_version": "1.0.0",
    "nodes": [
        {"id": "intake", "kind": "intake"},
        {"id": "archive", "kind": "archive"},
    ],
    "edges": [{"source": "intake", "target": "archive", "kind": "sequential"}],
    "entry_node": "intake",
    "terminal_nodes": ["archive"],
}

# The same shape `tests/test_overlay_manifest.py` pins: every `declares.*` entry
# carries the overlay's own namespace as its prefix.
VALID_MANIFEST_DOCUMENT = {
    "spec_version": "1.0.0",
    "overlay_id": "development-overlay",
    "namespace": "development",
    "version": "0.1.0",
    "description": "Ports the current develop skill's graph/policy semantics onto Praxis.",
    "declares": {
        "capability_kinds": ["development.code-generation"],
        "proof_types": ["development.test-pass"],
        "resource_types": ["development.filesystem"],
        "authority_scopes": ["development.merge-authority"],
    },
    "requested_capability_kinds": ["development.code-generation"],
}


def _write_json(path, document) -> str:
    path.write_text(json.dumps(document))
    return str(path)


def _valid_graph(tmp_path, name: str = "graph.json") -> str:
    return _write_json(tmp_path / name, VALID_GRAPH_DOCUMENT)


def _invalid_graph(tmp_path, name: str = "broken.json") -> str:
    # Structurally invalid rather than schema-invalid: the shape is fine, but an
    # edge names a node that does not exist.
    document = copy.deepcopy(VALID_GRAPH_DOCUMENT)
    document["edges"][0]["target"] = "ghost"
    return _write_json(tmp_path / name, document)


def _valid_manifest(tmp_path, name: str = "overlay.json") -> str:
    return _write_json(tmp_path / name, VALID_MANIFEST_DOCUMENT)


def _invalid_manifest(tmp_path, name: str = "bad-overlay.json") -> str:
    # The cross-field invariant `load_manifest` enforces and the schema cannot:
    # the declared strings no longer carry this overlay's namespace as prefix.
    document = copy.deepcopy(VALID_MANIFEST_DOCUMENT)
    document["namespace"] = "elsewhere"
    return _write_json(tmp_path / name, document)


def _field(result: CheckResult, key: str) -> str:
    return dict(result.fields)[key]


# The skip path -- criterion 10


def test_no_documents_reports_the_skip_explicitly(tmp_path):
    result = check_documents([], [])

    assert result.fields == [("status", "skipped (no document supplied)")]


def test_no_documents_leaves_the_exit_code_alone(tmp_path):
    # `ok` is the only verdict in criterion 7's vocabulary that both states the
    # skip and keeps `doctor` at exit 0.
    assert check_documents([], []).verdict == "ok"


def test_check_is_named_documents():
    assert check_documents([], []).name == "documents"


def test_result_is_a_check_result():
    assert isinstance(check_documents([], []), CheckResult)


def test_no_documents_does_not_discover_documents_lying_around(tmp_path, monkeypatch):
    # Document discovery is explicitly out of scope: a graph in the working
    # directory is not a document the user asked `doctor` to check.
    _valid_graph(tmp_path)
    _valid_manifest(tmp_path)
    monkeypatch.chdir(tmp_path)

    result = check_documents([], [])

    assert result.fields == [("status", "skipped (no document supplied)")]


# --graph


def test_valid_graph_reports_node_and_edge_counts(tmp_path):
    path = _valid_graph(tmp_path)

    result = check_documents([path], [])

    assert result.fields == [(f"graph {path}", "nodes=2 edges=1 ok")]
    assert result.verdict == "ok"


def test_graph_counts_come_from_the_loaded_graph(tmp_path):
    # Seven nodes and eight edges, so a count that is really "length of the raw
    # JSON list I happened to read first" cannot pass by coincidence.
    document = copy.deepcopy(VALID_GRAPH_DOCUMENT)
    document["nodes"] = [{"id": f"n{index}", "kind": "step"} for index in range(7)]
    document["edges"] = [
        {"source": "n0", "target": f"n{index}", "kind": "fan-out"} for index in range(1, 7)
    ] + [
        {"source": "n1", "target": "n2", "kind": "sequential"},
        {"source": "n3", "target": "n4", "kind": "sequential"},
    ]
    document["entry_node"] = "n0"
    document["terminal_nodes"] = ["n6"]
    path = _write_json(tmp_path / "wide.json", document)

    result = check_documents([path], [])

    assert _field(result, f"graph {path}") == "nodes=7 edges=8 ok"


def test_graph_node_count_is_the_loaded_graphs_not_the_raw_lists(tmp_path):
    # The one shape where the two sources genuinely disagree. `load_graph` keys
    # nodes into a dict by id (`src/praxis_runtime/graph.py:58-61`) and the
    # schema puts no uniqueness constraint on `id`, so a repeated id is a valid
    # document whose raw `nodes` list is longer than the graph it loads to.
    # Without this case a count read straight off the JSON list would agree with
    # the loaded count everywhere and pass.
    document = copy.deepcopy(VALID_GRAPH_DOCUMENT)
    document["nodes"] = [
        {"id": "intake", "kind": "intake"},
        {"id": "archive", "kind": "archive"},
        {"id": "archive", "kind": "archive"},
    ]
    path = _write_json(tmp_path / "duplicate-id.json", document)

    loaded = load_graph(path)
    assert len(document["nodes"]) != len(loaded.nodes)  # guards the case itself

    result = check_documents([path], [])

    assert _field(result, f"graph {path}") == (
        f"nodes={len(loaded.nodes)} edges={len(loaded.edges)} ok"
    )


def test_structurally_invalid_graph_reports_the_loaders_own_message(tmp_path):
    path = _invalid_graph(tmp_path)

    with pytest.raises(GraphValidationError) as raised:
        load_graph(path)
    expected_message = str(raised.value)

    result = check_documents([path], [])

    assert result.fields == [(f"graph {path}", expected_message)]
    assert result.verdict == "fail"


def test_graph_that_is_not_json_fails_rather_than_raising(tmp_path):
    path = tmp_path / "notes.txt"
    path.write_text("this is not JSON at all\n")

    result = check_documents([str(path)], [])

    assert result.verdict == "fail"
    assert _field(result, f"graph {path}")


def test_missing_graph_file_fails_rather_than_raising(tmp_path):
    path = str(tmp_path / "absent.json")

    result = check_documents([path], [])

    assert result.verdict == "fail"
    assert _field(result, f"graph {path}")


# --overlay-manifest


def test_valid_manifest_reports_its_id_and_namespace(tmp_path):
    path = _valid_manifest(tmp_path)

    result = check_documents([], [path])

    assert result.fields == [
        (f"overlay {path}", "overlay_id=development-overlay namespace=development ok")
    ]
    assert result.verdict == "ok"


def test_manifest_details_come_from_the_loaded_manifest(tmp_path):
    document = copy.deepcopy(VALID_MANIFEST_DOCUMENT)
    document["overlay_id"] = "research-overlay"
    document["namespace"] = "research"
    document["declares"] = {
        "capability_kinds": ["research.literature-review"],
        "proof_types": ["research.citation-checked"],
        "resource_types": ["research.corpus"],
        "authority_scopes": ["research.publish-authority"],
    }
    document["requested_capability_kinds"] = ["research.literature-review"]
    path = _write_json(tmp_path / "research.json", document)

    manifest = load_manifest(copy.deepcopy(document))
    result = check_documents([], [path])

    assert _field(result, f"overlay {path}") == (
        f"overlay_id={manifest.overlay_id} namespace={manifest.namespace} ok"
    )


def test_manifest_with_a_wrong_namespace_prefix_fails(tmp_path):
    path = _invalid_manifest(tmp_path)

    with pytest.raises(OverlayManifestError) as raised:
        load_manifest(json.loads(Path(path).read_text()))
    expected_message = str(raised.value)

    result = check_documents([], [path])

    assert result.fields == [(f"overlay {path}", expected_message)]
    assert result.verdict == "fail"


def test_manifest_that_is_not_json_fails_rather_than_raising(tmp_path):
    # `load_manifest` takes a parsed document, so the check does the reading and
    # owns this failure itself.
    path = tmp_path / "overlay.yaml"
    path.write_text("namespace: development\n")

    result = check_documents([], [str(path)])

    assert result.verdict == "fail"
    assert _field(result, f"overlay {path}")


def test_missing_manifest_file_fails_rather_than_raising(tmp_path):
    path = str(tmp_path / "absent-overlay.json")

    result = check_documents([], [path])

    assert result.verdict == "fail"
    assert _field(result, f"overlay {path}")


# Every document is attempted


def test_a_failing_graph_does_not_hide_the_graphs_after_it(tmp_path):
    bad = _invalid_graph(tmp_path)
    good = _valid_graph(tmp_path)

    result = check_documents([bad, good], [])

    assert [key for key, _ in result.fields] == [f"graph {bad}", f"graph {good}"]
    assert _field(result, f"graph {good}") == "nodes=2 edges=1 ok"
    assert result.verdict == "fail"


def test_one_bad_and_one_good_document_give_two_lines_and_one_fail_verdict(tmp_path):
    bad_graph = _invalid_graph(tmp_path)
    good_manifest = _valid_manifest(tmp_path)

    result = check_documents([bad_graph], [good_manifest])

    assert len(result.fields) == 2
    assert _field(result, f"overlay {good_manifest}") == (
        "overlay_id=development-overlay namespace=development ok"
    )
    assert result.verdict == "fail"


def test_a_failing_graph_does_not_hide_the_manifests(tmp_path):
    missing_graph = str(tmp_path / "absent.json")
    bad_manifest = _invalid_manifest(tmp_path)
    good_manifest = _valid_manifest(tmp_path)

    result = check_documents([missing_graph], [bad_manifest, good_manifest])

    assert [key for key, _ in result.fields] == [
        f"graph {missing_graph}",
        f"overlay {bad_manifest}",
        f"overlay {good_manifest}",
    ]
    assert result.verdict == "fail"


def test_every_document_valid_gives_one_line_each_and_an_ok_verdict(tmp_path):
    first = _valid_graph(tmp_path, "one.json")
    second = _valid_graph(tmp_path, "two.json")
    manifest = _valid_manifest(tmp_path)

    result = check_documents([first, second], [manifest])

    assert [key for key, _ in result.fields] == [
        f"graph {first}",
        f"graph {second}",
        f"overlay {manifest}",
    ]
    assert result.verdict == "ok"


def test_documents_are_reported_graphs_first_then_overlays(tmp_path):
    graph = _valid_graph(tmp_path)
    manifest = _valid_manifest(tmp_path)

    result = check_documents([graph], [manifest])

    assert result.fields[0][0].startswith("graph ")
    assert result.fields[1][0].startswith("overlay ")

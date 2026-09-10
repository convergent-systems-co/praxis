"""Hermetic end-to-end tests for `praxis run`."""

from __future__ import annotations

import json
from pathlib import Path

from praxis_executors.adapters.fake import FakeCapabilityExecutor
from praxis_executors.interface import ExecutionResult, ExecutorStatus
from praxis_cli.run_cmd import run_run


_SPEC_VERSION = "1.0.0"
_KIND = "document-review"


def _graph(path: Path, *, requirement: dict | None = None, second: bool = True):
    nodes = [{"id": "review-first", "kind": "review"}]
    edges = []
    if requirement is not None:
        nodes[0]["metadata"] = {"requirement": requirement}
    if second:
        nodes.append({"id": "review-second", "kind": "review"})
        edges.append(
            {"source": "review-first", "target": "review-second", "kind": "sequential"}
        )
    document = {
        "spec_version": _SPEC_VERSION,
        "nodes": nodes,
        "edges": edges,
        "entry_node": "review-first",
        "terminal_nodes": [nodes[-1]["id"]],
    }
    path.write_text(json.dumps(document))


def _requirement() -> dict:
    return {
        "spec_version": _SPEC_VERSION,
        "requirements": [
            {"promise": {"spec_version": _SPEC_VERSION, "kind": _KIND}, "constraint": "required"}
        ],
    }


def _adapter(executor_id: str, transport: str = "local"):
    return FakeCapabilityExecutor(
        executor_id=executor_id,
        capabilities=[
            {
                "spec_version": _SPEC_VERSION,
                "satisfies": [{"kind": _KIND}],
                "auth_transport": transport,
            }
        ],
        script={_KIND: ExecutionResult(status=ExecutorStatus.SUCCEEDED)},
    )


def test_auto_run_writes_dashboard_compatible_layout_and_completes(tmp_path: Path):
    graph_path = tmp_path / "review-graph.json"
    _graph(graph_path, requirement=_requirement())
    run_dir = tmp_path / "run"

    assert run_run(
        {"reviewer": _adapter("reviewer")},
        target=str(graph_path),
        run_dir=str(run_dir),
    ) == 0
    state = json.loads((run_dir / "run-state.json").read_text())
    assert {cursor["status"] for cursor in state["cursors"].values()} == {"terminal_success"}
    assert (run_dir / "events" / "events.jsonl").is_file()


def test_explicit_executor_wins_over_auto_ranking(tmp_path: Path):
    graph_path = tmp_path / "review-graph.json"
    _graph(graph_path, requirement=_requirement(), second=False)
    run_dir = tmp_path / "run"
    first = _adapter("review-a")
    second = _adapter("review-z")

    assert run_run(
        {"review-a": first, "review-z": second},
        target=str(graph_path),
        executor="review-z",
        run_dir=str(run_dir),
    ) == 0
    assert len(second._results) == 1
    assert not first._results


def test_unknown_target_and_populated_run_dir_are_refused_without_state(tmp_path: Path):
    unknown_dir = tmp_path / "unknown"
    assert run_run({}, target="missing-target", run_dir=str(unknown_dir)) == 1
    assert not unknown_dir.exists()

    graph_path = tmp_path / "review-graph.json"
    _graph(graph_path, second=False)
    populated = tmp_path / "populated"
    populated.mkdir()
    state_path = populated / "run-state.json"
    state_path.write_text("keep")
    assert run_run({}, target=str(graph_path), run_dir=str(populated)) == 1
    assert state_path.read_text() == "keep"


def test_explicit_paid_transport_is_refused_before_launch(tmp_path: Path):
    graph_path = tmp_path / "review-graph.json"
    _graph(graph_path, requirement=_requirement(), second=False)
    run_dir = tmp_path / "run"
    paid = _adapter("paid-reviewer", "metered_api")

    assert run_run(
        {"paid-reviewer": paid},
        target=str(graph_path),
        executor="paid-reviewer",
        run_dir=str(run_dir),
    ) == 1
    assert not run_dir.exists()
    assert not paid._results


def test_node_without_requirement_completes_without_dispatch(tmp_path: Path):
    graph_path = tmp_path / "review-graph.json"
    _graph(graph_path, second=False)
    run_dir = tmp_path / "run"

    assert run_run({}, target=str(graph_path), run_dir=str(run_dir)) == 0

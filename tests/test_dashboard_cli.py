"""CLI entrypoint.

`parse_args` is a thin `argparse` wrapper with no positional arguments --
just `--graph`/`--run-dir` (required), `--lease-dir` (optional), `--host`/
`--port` (defaulted), and `--replay-only` (a store_true flag) -- so T13 and
manual testing can rely on this exact flag surface.

`main`'s `--replay-only` path is driven against a real completed
fake-executor run directory (same convention as
tests/test_dashboard_replay_fake_executor.py: a `RunStateStore` +
`EventLog` on `tmp_path`, driven to completion with
`praxis_runtime.testing.fake_executor.FakeExecutor`) and asserted to print
valid JSON to stdout and return 0 promptly -- it must never import/start
`server`, so the test doesn't even need to patch `server.serve` to prove
this: if a live server were started, `main` would block on
`serve_forever()` and the test itself would hang rather than return.
"""

from __future__ import annotations

import json
from pathlib import Path
from unittest.mock import MagicMock

from praxis_dashboard.cli import main, parse_args
from praxis_runtime.events import EventLog
from praxis_runtime.graph import load_graph
from praxis_runtime.state import RunStateStore
from praxis_runtime.testing.fake_executor import FakeExecutor
from praxis_runtime.transitions import NodeStatus, TransitionEngine

SAMPLE_GRAPH_PATH = Path(__file__).resolve().parent.parent / "examples" / "sample-graph.json"


def test_parse_args_defaults_for_omitted_flags():
    args = parse_args(["--graph", "g.json", "--run-dir", "r"])

    assert args.graph == "g.json"
    assert args.run_dir == "r"
    assert args.lease_dir is None
    assert args.host == "127.0.0.1"
    assert args.port == 0
    assert args.replay_only is False


def test_main_replay_only_prints_snapshot_json_and_returns_0_without_serving(
    tmp_path, capsys, monkeypatch
):
    graph = load_graph(SAMPLE_GRAPH_PATH)
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log)

    script = {node_id: {"event_type": "complete", "evidence": None} for node_id in graph.nodes}
    FakeExecutor(engine, script).run_to_completion()
    log.close()
    del engine, store, log

    mock_serve = MagicMock()
    monkeypatch.setattr("praxis_dashboard.server.serve", mock_serve)

    exit_code = main(
        [
            "--graph",
            str(SAMPLE_GRAPH_PATH),
            "--run-dir",
            str(tmp_path),
            "--replay-only",
        ]
    )

    assert exit_code == 0
    mock_serve.assert_not_called()
    captured = capsys.readouterr()
    document = json.loads(captured.out)
    assert document["mode"] == "replay"
    assert document["run_summary"]["total_nodes"] == len(graph.nodes)
    assert document["run_summary"]["is_complete"] is True
    assert {node["node_id"] for node in document["nodes"]} == set(graph.nodes)
    assert all(
        node["status"] == NodeStatus.TERMINAL_SUCCESS.value for node in document["nodes"]
    )


def test_main_forwards_lease_dir_to_dashboard_source(tmp_path, monkeypatch):
    captured_kwargs = {}

    class RecordingSource:
        def __init__(self, graph_path, run_dir, *, lease_directory=None, **kwargs):
            captured_kwargs["lease_directory"] = lease_directory

        def replay_snapshot(self):
            return None

    monkeypatch.setattr("praxis_dashboard.cli.DashboardSource", RecordingSource)
    monkeypatch.setattr(
        "praxis_dashboard.cli.snapshot.snapshot_to_document", lambda snap: {"mode": "replay"}
    )

    lease_dir = tmp_path / "leases"

    exit_code = main(
        [
            "--graph",
            "g.json",
            "--run-dir",
            "r",
            "--lease-dir",
            str(lease_dir),
            "--replay-only",
        ]
    )

    assert exit_code == 0
    assert captured_kwargs["lease_directory"] == Path(lease_dir)

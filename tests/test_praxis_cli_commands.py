"""Routing tests for the `doctor` and `run` top-level CLI tokens."""

from __future__ import annotations

from praxis_cli import doctor_cmd, run_cmd
from praxis_cli.main import main


def test_main_routes_doctor_flags_and_return_code(monkeypatch):
    captured = {}

    def fake(build, *, graphs, overlay_manifests):
        captured.update(graphs=graphs, manifests=overlay_manifests)
        return 17

    monkeypatch.setattr(doctor_cmd, "run_doctor", fake)
    assert main(["doctor", "--graph", "a.json", "--overlay-manifest", "m.json"]) == 17
    assert captured == {"graphs": ["a.json"], "manifests": ["m.json"]}


def test_main_routes_run_flags_and_return_code(monkeypatch):
    captured = {}

    monkeypatch.setattr(run_cmd, "run_run", lambda adapters, **kwargs: captured.update(kwargs) or 23)
    assert main(
        [
            "run",
            "trivial",
            "--executor",
            "reviewer",
            "--capability",
            "document-review",
            "--run-dir",
            "run-dir",
            "--run-id",
            "run-1",
        ]
    ) == 23
    assert captured == {
        "target": "trivial",
        "executor": "reviewer",
        "capabilities": ["document-review"],
        "run_dir": "run-dir",
        "run_id": "run-1",
    }

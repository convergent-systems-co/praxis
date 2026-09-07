"""Acceptance proof for task T7 (bundle b12-issue13).

T7's deliverable is a *captured* Praxis-candidate real run: a real
(non-`tmp_path`) run directory under
`benchmark/parity/runs/run-<UTC-timestamp>-development-overlay/` holding the
`state.json`/`events.jsonl` produced by driving
`overlays.development.graph.build_development_graph()` through a real
`TransitionEngine`/`RunStateStore`/`EventLog`/`FakeExecutor` to
`TERMINAL_SUCCESS`, plus a `report.md` filled in against
`benchmark/report-format/real-run-report-format.md`'s template.

This module lives under `tests/` (not `benchmark/parity/runs/`) specifically
so it is collected by a plain `pytest` invocation -- `pyproject.toml` sets
`testpaths = ["tests"]`, and a test file that is the only automated check
that these committed run artifacts stay consistent with the overlay code
must actually run as part of the standard suite, not sit uncollected next to
the artifacts it verifies.
"""

from __future__ import annotations

import re
import time
from pathlib import Path

from overlays.development.graph import build_development_graph
from overlays.development.overlay import register_development_overlay
from praxis_overlay.registry import OverlayRegistry
from praxis_runtime.events import EventLog
from praxis_runtime.state import RunStateStore
from praxis_runtime.testing.fake_executor import FakeExecutor
from praxis_runtime.transitions import NodeStatus, TransitionEngine

_RUNS_DIR = Path(__file__).resolve().parent.parent / "benchmark" / "parity" / "runs"
_TEST_PASS = "development.test-pass"
_REVIEW_APPROVED = "development.review-approved"

_GITIGNORE_PATH = Path(__file__).resolve().parent.parent / ".gitignore"


def _proof_record(*, node_id: str, proof_type: str, graph_version: str) -> dict:
    return {
        "spec_version": "1.0.0",
        "proof_id": f"{node_id}-{proof_type}-pass",
        "run_id": "t7-reproducibility-check",
        "graph_version": graph_version,
        "node_id": node_id,
        "proof_type": proof_type,
        "executor_id": "test-harness",
        "grader_kind": "deterministic",
        "status": "pass",
    }


def _all_passing_script(graph, terminal_node_id: str) -> dict:
    script = {node_id: {"event_type": "complete", "evidence": None} for node_id in graph.nodes}
    script[terminal_node_id] = {
        "event_type": "complete",
        "evidence": [
            _proof_record(
                node_id=terminal_node_id, proof_type=_TEST_PASS, graph_version=graph.spec_version
            ),
            _proof_record(
                node_id=terminal_node_id, proof_type=_REVIEW_APPROVED, graph_version=graph.spec_version
            ),
        ],
    }
    return script


def _find_run_dir() -> Path:
    candidates = sorted(_RUNS_DIR.glob("run-*-development-overlay"))
    assert candidates, (
        "expected a committed run-<UTC-timestamp>-development-overlay/ directory "
        f"under {_RUNS_DIR}, found none"
    )
    assert len(candidates) == 1, f"expected exactly one captured run directory, found {candidates}"
    return candidates[0]


def _parse_front_matter(report_text: str) -> dict[str, str]:
    match = re.match(r"^---\n(.*?)\n---\n", report_text, flags=re.DOTALL)
    assert match, "report.md must start with a --- delimited front matter block"
    fields: dict[str, str] = {}
    for line in match.group(1).splitlines():
        if not line.strip():
            continue
        key, _, value = line.partition(":")
        fields[key.strip()] = value.strip()
    return fields


def test_captured_run_directory_exists_with_real_committed_artifacts():
    run_dir = _find_run_dir()

    assert (run_dir / "state.json").is_file(), "state.json must be a committed artifact, not tmp_path"
    assert (run_dir / "events.jsonl").is_file(), "events.jsonl must be a committed artifact, not tmp_path"
    assert (run_dir / "report.md").is_file()


def test_event_log_lock_sidecar_is_gitignored():
    # praxis_runtime.events.EventLog creates an `events.jsonl.lock` sidecar the moment it is
    # opened -- including read-only replays of this test's own committed run directory a few
    # tests below -- so the file reappears on disk as an unavoidable runtime side effect. The
    # report-format spec (benchmark/report-format/real-run-report-format.md) requires only
    # state.json/events.jsonl/report.md as committed artifacts, so this sidecar must be
    # gitignored rather than asserted absent from the working tree (which it cannot reliably be
    # once any code reads the committed run's events).
    gitignore_text = _GITIGNORE_PATH.read_text()
    assert "events.jsonl.lock" in gitignore_text, (
        "events.jsonl.lock must be gitignored so EventLog's sidecar lock file is never "
        "accidentally committed alongside a captured run's real state.json/events.jsonl"
    )


def test_captured_state_reached_terminal_success_for_every_overlay_node():
    run_dir = _find_run_dir()
    registry = OverlayRegistry()
    register_development_overlay(registry)
    graph = build_development_graph()

    state = RunStateStore(run_dir / "state.json").load()
    assert state is not None

    assert set(state.cursors) == set(graph.nodes)
    for node_id, cursor in state.cursors.items():
        assert cursor.status == NodeStatus.TERMINAL_SUCCESS.value, (node_id, cursor.status)


def test_captured_events_are_non_empty_and_share_the_run_id():
    run_dir = _find_run_dir()
    state = RunStateStore(run_dir / "state.json").load()
    assert state is not None

    log = EventLog(run_dir)
    try:
        events = log.read_all()
    finally:
        log.close()

    assert events, "events.jsonl must record at least one real event from the captured run"
    assert all(event.run_id == state.run_id for event in events)


def test_report_front_matter_and_body_match_the_template_and_committed_state():
    run_dir = _find_run_dir()
    state = RunStateStore(run_dir / "state.json").load()
    assert state is not None

    log = EventLog(run_dir)
    try:
        event_count = len(log.read_all())
    finally:
        log.close()

    report_text = (run_dir / "report.md").read_text()
    front_matter = _parse_front_matter(report_text)

    assert front_matter.get("scenario") == "02-feature-implementation"
    assert front_matter.get("develop_version", "").strip('"') == (
        "n/a (Praxis development overlay, not the legacy skill)"
    )
    assert front_matter.get("develop_version_source"), "must explain this run is the Praxis-side counterpart"
    assert front_matter.get("run_id") in {run_dir.name, state.run_id}

    # Cross-links back to T4's migration doc and cites the real committed event count somewhere
    # (metrics table or Gaps section) rather than a value recalled from memory.
    assert "docs/parity/state-event-migration.md" in report_text
    assert str(event_count) in report_text

    # The Outcome/Gaps sections must plainly disclaim this as a deterministic proxy run, not a
    # live timing-comparable capture, and mark persona-latency/tool-calls/cost rows not applicable.
    assert "not applicable" in report_text.lower()
    assert "deterministic" in report_text.lower()
    assert "fake-executor" in report_text.lower() or "fake executor" in report_text.lower()


def test_capture_is_reproducible_by_replaying_the_same_script(tmp_path):
    run_dir = _find_run_dir()
    committed_state = RunStateStore(run_dir / "state.json").load()
    assert committed_state is not None

    committed_log = EventLog(run_dir)
    try:
        committed_event_count = len(committed_log.read_all())
    finally:
        committed_log.close()

    registry = OverlayRegistry()
    activated = register_development_overlay(registry)
    graph = build_development_graph()
    terminal_node_id = next(iter(graph.terminal_nodes))
    script = _all_passing_script(graph, terminal_node_id)

    store = RunStateStore(tmp_path / "state.json")
    log = EventLog(tmp_path / "events")
    try:
        engine = TransitionEngine(graph, store, log, grader_registry=activated.grader_registry)
        started = time.monotonic()
        final_state = FakeExecutor(engine, script).run_to_completion()
        elapsed_seconds = time.monotonic() - started
        assert elapsed_seconds >= 0.0, "monotonic replay timing must not go backwards"

        for node_id in graph.nodes:
            assert final_state.cursors[node_id].status == committed_state.cursors[node_id].status
        assert len(log.read_all()) == committed_event_count
    finally:
        log.close()

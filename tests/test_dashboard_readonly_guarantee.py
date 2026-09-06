"""Read-only/no-mutation guarantee: direct proof that the dashboard cannot
create legal state transitions by itself.

`TransitionEngine.apply`, `EventLog.append`, `RunStateStore.save`,
`praxis_runtime.resources.leases.acquire`/`.release`/`.renew`, and the
`LeaseStore.save` primitive those three wrappers call are the entrypoints
that can legally mutate a run (see the module docstrings of
`praxis_runtime.transitions`, `.events`, `.state`, and `.resources.leases`).
`LeaseStore.save` is patched in addition to the module-level
acquire/release/renew wrappers because patching only the wrappers leaves the
underlying method reachable by any code that constructs a `LeaseStore`
directly and calls `.save` on it, bypassing the wrappers entirely. This test
patches all of them so a call against the run this test is attached to
raises, then drives a `praxis_dashboard.sources.DashboardSource` through
`poll_live()` twice and `replay_snapshot()` once and lets any resulting
`AssertionError` propagate as a normal test failure -- the absence of a
failure *is* the proof that none of them was ever reached against that
run.

Two independent scoping choices keep this precise instead of producing a
false failure against code this test isn't exercising:

1. Each dashboard read is wrapped in its own `pytest.MonkeyPatch.context()`
   block, whose `with`-exit undoes the patch immediately afterward. Advancing
   the real run in between those reads happens through a *separate*,
   unpatched `TransitionEngine` -- patching `TransitionEngine.apply` patches
   the class itself, so a patch left active while that external engine also
   calls `.apply(...)` would wrongly flag the test's own fixture code, not
   the dashboard.
2. The four instance-bound methods (`TransitionEngine.apply`,
   `EventLog.append`, `RunStateStore.save`, `LeaseStore.save`) only raise
   when the instance they're called on is bound to *this test's* real
   `run-state.json`/`events`/`leases` paths. `replay_snapshot()` legitimately
   reconstructs state via `praxis_runtime.replay.replay`, which folds
   recorded events over its own scratch `TransitionEngine`/`EventLog`/
   `RunStateStore` confined to a `tempfile.TemporaryDirectory()` (see
   `praxis_runtime.replay`'s module docstring) -- reusing the very same
   `apply`/`append`/`save` methods this test patches, but never against the
   real run directory. Without this check, that already-audited-safe scratch
   fold would trip the patch and produce a failure that proves nothing about
   the dashboard.

Uses `examples/sample-graph.json` -- the same generic document-review
pipeline (no software-development vocabulary) `tests/test_dashboard_sources.py`
and `tests/test_end_to_end_fake_executor.py` already drive.
"""

from __future__ import annotations

from pathlib import Path

import pytest

from praxis_dashboard.sources import DashboardSource
from praxis_runtime.events import EventLog
from praxis_runtime.graph import load_graph
from praxis_runtime.resources import leases as leases_module
from praxis_runtime.resources.leases import LeaseStore
from praxis_runtime.state import RunStateStore
from praxis_runtime.testing.fake_executor import FakeExecutor
from praxis_runtime.transitions import NodeStatus, TransitionEngine

SAMPLE_GRAPH_PATH = Path(__file__).resolve().parent.parent / "examples" / "sample-graph.json"

_MUTATION_MESSAGE = "dashboard must not mutate state"

_ORIGINAL_ENGINE_APPLY = TransitionEngine.apply
_ORIGINAL_LOG_APPEND = EventLog.append
_ORIGINAL_STORE_SAVE = RunStateStore.save
_ORIGINAL_LEASE_STORE_SAVE = LeaseStore.save


def _forbidden(*_args, **_kwargs):
    raise AssertionError(_MUTATION_MESSAGE)


def _patch_mutating_entrypoints(
    mp: pytest.MonkeyPatch,
    real_state_path: Path,
    real_events_dir: Path,
    real_lease_dir: Path,
) -> None:
    def _forbid_engine_apply(self, *args, **kwargs):
        if self._state_store._path == real_state_path:
            raise AssertionError(_MUTATION_MESSAGE)
        return _ORIGINAL_ENGINE_APPLY(self, *args, **kwargs)

    def _forbid_log_append(self, *args, **kwargs):
        if self._directory == real_events_dir:
            raise AssertionError(_MUTATION_MESSAGE)
        return _ORIGINAL_LOG_APPEND(self, *args, **kwargs)

    def _forbid_store_save(self, *args, **kwargs):
        if self._path == real_state_path:
            raise AssertionError(_MUTATION_MESSAGE)
        return _ORIGINAL_STORE_SAVE(self, *args, **kwargs)

    def _forbid_lease_store_save(self, *args, **kwargs):
        if self._path == real_lease_dir:
            raise AssertionError(_MUTATION_MESSAGE)
        return _ORIGINAL_LEASE_STORE_SAVE(self, *args, **kwargs)

    mp.setattr(TransitionEngine, "apply", _forbid_engine_apply)
    mp.setattr(EventLog, "append", _forbid_log_append)
    mp.setattr(RunStateStore, "save", _forbid_store_save)
    mp.setattr(LeaseStore, "save", _forbid_lease_store_save)
    mp.setattr(leases_module, "acquire", _forbidden)
    mp.setattr(leases_module, "release", _forbidden)
    mp.setattr(leases_module, "renew", _forbidden)


def test_dashboard_source_never_mutates_the_run_it_observes(tmp_path: Path):
    real_state_path = tmp_path / "run-state.json"
    real_events_dir = tmp_path / "events"
    real_lease_dir = tmp_path / "leases"

    graph = load_graph(SAMPLE_GRAPH_PATH)
    store = RunStateStore(real_state_path)
    log = EventLog(real_events_dir)
    engine = TransitionEngine(graph, store, log)

    # Advance the item through its first step, unpatched, before any
    # dashboard read happens, so the first poll_live() has real data to show.
    engine.apply("intake", "start")
    engine.apply("intake", "complete")

    source = DashboardSource(SAMPLE_GRAPH_PATH, tmp_path, lease_directory=real_lease_dir)

    with pytest.MonkeyPatch.context() as mp:
        _patch_mutating_entrypoints(mp, real_state_path, real_events_dir, real_lease_dir)
        first_snapshot = source.poll_live()

    first_statuses = {view.node_id: view.status for view in first_snapshot.nodes}
    assert first_statuses["intake"] == NodeStatus.TERMINAL_SUCCESS.value
    assert first_statuses["review-legal"] == NodeStatus.PENDING.value
    assert first_statuses["review-editorial"] == NodeStatus.PENDING.value

    # A further transition, applied externally (unpatched) in between the
    # two poll_live() calls.
    engine.apply("review-legal", "start")
    engine.apply("review-legal", "complete")
    engine.apply("review-editorial", "start")
    engine.apply("review-editorial", "complete")

    with pytest.MonkeyPatch.context() as mp:
        _patch_mutating_entrypoints(mp, real_state_path, real_events_dir, real_lease_dir)
        second_snapshot = source.poll_live()

    second_statuses = {view.node_id: view.status for view in second_snapshot.nodes}
    assert second_statuses["review-legal"] == NodeStatus.TERMINAL_SUCCESS.value
    assert second_statuses["review-editorial"] == NodeStatus.TERMINAL_SUCCESS.value
    assert second_statuses["decision"] == NodeStatus.PENDING.value

    # Drive the item to completion, again through the external, unpatched
    # engine.
    script = {node_id: {"event_type": "complete", "evidence": None} for node_id in graph.nodes}
    FakeExecutor(engine, script).run_to_completion()

    with pytest.MonkeyPatch.context() as mp:
        _patch_mutating_entrypoints(mp, real_state_path, real_events_dir, real_lease_dir)
        final_snapshot = source.replay_snapshot()

    assert final_snapshot.mode == "replay"
    assert all(
        view.status == NodeStatus.TERMINAL_SUCCESS.value for view in final_snapshot.nodes
    )

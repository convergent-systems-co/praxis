"""Live-attach and replay sources for the dashboard.

`DashboardSource` is the only place in the dashboard that touches disk
directly: everything downstream of it (`.snapshot`, `.projection`, etc.) is
pure and read-only over already-loaded objects. Both `poll_live` and
`replay_snapshot` open a fresh `RunStateStore`/`EventLog` on every call
(simpler than holding them open across polls, and both classes already
re-derive their state from disk on every read per their own docstrings, so
either choice is equally correct) and never construct anything that calls
`TransitionEngine.apply`, `EventLog.append`, or `RunStateStore.save` against
the real `run_directory` -- so a `DashboardSource` can never mutate the run
it is attached to.

`poll_live` reads whatever checkpoint/event-log state exists right now. When
no checkpoint has been written yet, it builds the same `PENDING`-at-
`entry_node` fallback `TransitionEngine.current_state()` uses when unchecked,
duplicated directly here (a five-line, independently-documented duplication,
same pattern as `evidence_view.stored_evidence_for`'s duplication of
`TransitionEngine._stored_evidence`) since that fallback is a private
implementation detail of `TransitionEngine`.

`replay_snapshot` reconstructs state purely from the event log via
`praxis_runtime.replay.replay`, ignoring any checkpoint file entirely -- this
is what makes it work after the owning process has exited. A
`TransitionEngine` is still needed to compute `legal_next` for the projection
layer, so a scratch `RunStateStore` is seeded with the replayed state by
writing its document directly to a `tempfile.TemporaryDirectory()`-scoped
path (mirroring how `praxis_runtime.replay._fold_events` seeds its own
scratch store) -- never through `RunStateStore.save`, and never over
`run_directory`'s real checkpoint file. Nothing in this module calls `.save()`
on that scratch store again afterward: it is a one-shot seed solely so
`TransitionEngine.legal_next` has a state to read.
"""

from __future__ import annotations

import json
import tempfile
import uuid
from pathlib import Path

import praxis_evidence.graders
import praxis_executors.registry
import praxis_runtime.graph
import praxis_runtime.replay
from praxis_runtime.events import EventLog
from praxis_runtime.resources.leases import LeaseStore
from praxis_runtime.state import Cursor, RunState, RunStateStore
from praxis_runtime.transitions import NodeStatus, TransitionEngine

from . import snapshot

_SPEC_VERSION = "1.0.0"


class DashboardSourceError(Exception):
    """Raised fail-closed when the graph or run directory cannot be read/validated."""


def _entry_pending_state(graph: "praxis_runtime.graph.Graph") -> RunState:
    """Mirrors TransitionEngine.current_state()'s unchecked fallback (private to that class)."""
    entry = graph.entry_node
    return RunState(
        spec_version=_SPEC_VERSION,
        run_id=uuid.uuid4().hex,
        cursors={entry: Cursor(node_id=entry, status=NodeStatus.PENDING.value)},
        last_applied_seq=-1,
    )


def _document_from_state(state: RunState) -> dict:
    """Mirrors state.py's own private _to_document/replay.py's _seed_document."""
    return {
        "spec_version": state.spec_version,
        "run_id": state.run_id,
        "cursors": {
            node_id: {"node_id": cursor.node_id, "status": cursor.status}
            for node_id, cursor in state.cursors.items()
        },
        "last_applied_seq": state.last_applied_seq,
    }


class DashboardSource:
    def __init__(
        self,
        graph_path: Path,
        run_directory: Path,
        *,
        lease_directory: "Path | None" = None,
        executor_registry: "praxis_executors.registry.ExecutorRegistry | None" = None,
        grader_registry: "praxis_evidence.graders.GraderRegistry | None" = None,
    ) -> None:
        # load_graph raises GraphValidationError fail-closed; it is never
        # caught here, so a malformed graph never produces a placeholder.
        self._graph = praxis_runtime.graph.load_graph(graph_path)
        self._run_directory = Path(run_directory)
        self._lease_directory = Path(lease_directory) if lease_directory is not None else None
        self._executor_registry = executor_registry
        self._grader_registry = grader_registry

    def _advertisements(self) -> "list[dict] | None":
        if self._executor_registry is None:
            return None
        return self._executor_registry.advertisements()

    def _lease_store(self) -> "LeaseStore | None":
        if self._lease_directory is None:
            return None
        return LeaseStore(self._lease_directory)

    def poll_live(self) -> "snapshot.DashboardSnapshot":
        store = RunStateStore(self._run_directory / "run-state.json")
        with EventLog(self._run_directory / "events") as log:
            state = store.load()
            if state is None:
                state = _entry_pending_state(self._graph)
            events = log.read_all()
            engine = TransitionEngine(self._graph, store, log, grader_registry=self._grader_registry)

            return snapshot.build_snapshot(
                self._graph,
                state,
                events,
                engine,
                lease_store=self._lease_store(),
                advertisements=self._advertisements(),
                grader_registry=self._grader_registry,
                mode="live",
            )

    def replay_snapshot(self) -> "snapshot.DashboardSnapshot":
        with EventLog(self._run_directory / "events") as log:
            state = praxis_runtime.replay.replay(log, self._graph)
            events = log.read_all()

            with tempfile.TemporaryDirectory() as scratch_dir:
                scratch_path = Path(scratch_dir) / "run-state.json"
                scratch_path.write_text(json.dumps(_document_from_state(state)))
                scratch_store = RunStateStore(scratch_path)
                engine = TransitionEngine(
                    self._graph, scratch_store, log, grader_registry=self._grader_registry
                )

                return snapshot.build_snapshot(
                    self._graph,
                    state,
                    events,
                    engine,
                    lease_store=self._lease_store(),
                    advertisements=self._advertisements(),
                    grader_registry=self._grader_registry,
                    mode="replay",
                )

"""Resume/replay support.

replay() reconstructs a RunState purely from an EventLog's persisted events
and the graph's transition rules, independent of any checkpoint file. It
folds the events over a scratch TransitionEngine (backed by a throwaway
event log and state store confined to a temporary directory) so the real
fan-out/join/evidence logic in TransitionEngine.apply is reused rather than
duplicated, then corrects the resulting last_applied_seq to the real event
seq numbers (the scratch log renumbers from zero, which only matters for
that one field -- transition legality never depends on seq). The fold uses
_ReplayEngine, a TransitionEngine that skips evidence re-validation, since
evidence supplied to the original apply() call is not persisted onto the
Event and so cannot be re-checked on replay.

resume() is the process-restart entrypoint: it loads the last checkpoint (if
any), replays only the events appended after it via the same fold, persists
the reconciled state as the new checkpoint, and returns a TransitionEngine
bound to the real state_store and event_log. This covers a crash between an
event append and its checkpoint save, since the reconciliation happens
before the returned engine is used for anything else.
"""

from __future__ import annotations

import dataclasses
import json
import tempfile
import uuid
from pathlib import Path

from praxis_runtime.events import Event, EventLog
from praxis_runtime.graph import Graph, Node
from praxis_runtime.state import Cursor, RunState, RunStateStore
from praxis_runtime.transitions import NodeStatus, TransitionEngine

_SPEC_VERSION = "1.0.0"


class _ReplayEngine(TransitionEngine):
    """TransitionEngine used only for folding already-recorded events.

    Events in the log were legally applied once, and evidence supplied at
    that time is not persisted onto the Event itself (payload is not
    populated with it). Re-deriving evidence is therefore impossible, and
    re-checking it against an empty payload would reject a replay of a
    transition that legitimately succeeded. Skip the check here; legality
    was already enforced when the event was first appended.
    """

    def _check_evidence(self, node: Node, evidence: dict | None) -> None:
        return None

    def _validate_against_log(self, state: RunState, events: list[Event]) -> None:
        # The scratch event log used to fold historical events always
        # renumbers from seq 0, while the seed RunState's last_applied_seq
        # reflects the real log this replay is reconstructing -- an
        # intentional, by-design mismatch, not a stale checkpoint.
        # TransitionEngine's ahead-of-log guard exists to catch a genuinely
        # corrupt/stale checkpoint against its *own* backing log, which does
        # not apply to this internal seed/scratch-log pairing.
        return None


def _seed_document(state: RunState) -> dict:
    return {
        "spec_version": state.spec_version,
        "run_id": state.run_id,
        "cursors": {
            node_id: {"node_id": cursor.node_id, "status": cursor.status}
            for node_id, cursor in state.cursors.items()
        },
        "last_applied_seq": state.last_applied_seq,
    }


def _entry_state(graph: Graph, run_id: str) -> RunState:
    entry = graph.entry_node
    return RunState(
        spec_version=_SPEC_VERSION,
        run_id=run_id,
        cursors={entry: Cursor(node_id=entry, status=NodeStatus.PENDING.value)},
        last_applied_seq=-1,
    )


def _fold_events(graph: Graph, seed_state: RunState, events: list[Event]) -> RunState:
    if not events:
        return seed_state

    with tempfile.TemporaryDirectory() as tmp_dir:
        tmp_path = Path(tmp_dir)
        state_path = tmp_path / "run-state.json"
        # Written directly (bypassing RunStateStore.save's schema validation)
        # because seed_state may carry the last_applied_seq=-1 sentinel for a
        # run that has never been checkpointed, which the schema forbids.
        state_path.write_text(json.dumps(_seed_document(seed_state)))

        scratch_store = RunStateStore(state_path)
        with EventLog(tmp_path / "events") as scratch_log:
            engine = _ReplayEngine(graph, scratch_store, scratch_log)

            state = seed_state
            for event in events:
                state = engine.apply(event.node_id, event.event_type)

    return dataclasses.replace(state, last_applied_seq=events[-1].seq)


def replay(event_log: EventLog, graph: Graph) -> RunState:
    events = event_log.read_all()
    if not events:
        return _entry_state(graph, uuid.uuid4().hex)

    seed_state = _entry_state(graph, events[0].run_id)
    return _fold_events(graph, seed_state, events)


def resume(graph: Graph, state_store: RunStateStore, event_log: EventLog) -> TransitionEngine:
    checkpoint = state_store.load()
    last_applied_seq = checkpoint.last_applied_seq if checkpoint is not None else -1

    pending = [event for event in event_log.read_all() if event.seq > last_applied_seq]
    if pending:
        seed_state = checkpoint if checkpoint is not None else _entry_state(graph, pending[0].run_id)
        reconciled = _fold_events(graph, seed_state, pending)
        state_store.save(reconciled)

    return TransitionEngine(graph, state_store, event_log)

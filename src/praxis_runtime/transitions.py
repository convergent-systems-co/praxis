"""Deterministic transition engine.

TransitionEngine.apply is the single mutation entrypoint for a run: every
status change is checked against the node's current status via the fixed,
per-status _TRANSITIONS table before anything is written, and a rejected
transition never appends an event or persists a checkpoint (fail-closed, no
partial write). The graph's edges play no part in that per-node legality
check -- they are consulted only afterward, once a transition to
TERMINAL_SUCCESS is committed, to decide which successor cursors to create
next. Fan-out edges each create an independent successor cursor as soon as
their source completes; join edges only create their shared successor
cursor once every incoming edge's source has reported TERMINAL_SUCCESS.
Evidence supplied to apply() -- a list of raw proof-record documents -- is
persisted onto the committed Event's payload under the "evidence" key,
giving a durable audit trail of what evidence satisfied a gate. A node's own
evidence_requirement (if any) is graded via praxis_evidence.gates.evaluate_gate;
for a node reached via one or more join edges, each incoming source's own
gate result is re-derived from that source's stored evidence and its own
requirement, then combined with this node's own result via
praxis_evidence.aggregate.aggregate_gate_results, so a join can never advance
past an upstream branch whose gate is unsatisfied even if that branch already
reached TERMINAL_SUCCESS. apply() holds an exclusive flock on a sidecar file next
to the run-state checkpoint for the duration of its read-check-append-save
sequence, so two TransitionEngine instances (same process or different
processes) pointed at the same checkpoint serialize their applies instead
of both legally checking a transition against the same stale state and
racing to append conflicting events or overwrite each other's checkpoint
save.
"""

from __future__ import annotations

import enum
import fcntl
import uuid
from pathlib import Path

import praxis_evidence.graders
from praxis_evidence.aggregate import aggregate_gate_results
from praxis_evidence.gates import evaluate_gate
from praxis_evidence.types import GateResult
from praxis_runtime.events import Event, EventLog
from praxis_runtime.graph import Edge, Graph, Node
from praxis_runtime.state import Cursor, RunState, RunStateStore

_SPEC_VERSION = "1.0.0"


class NodeStatus(enum.Enum):
    PENDING = "pending"
    RUNNING = "running"
    BLOCKED = "blocked"
    HANDOFF = "handoff"
    RECOVERING = "recovering"
    TERMINAL_SUCCESS = "terminal_success"
    TERMINAL_FAILED = "terminal_failed"


_TERMINAL_STATUSES = {NodeStatus.TERMINAL_SUCCESS, NodeStatus.TERMINAL_FAILED}

_TRANSITIONS: dict[NodeStatus, dict[str, NodeStatus]] = {
    NodeStatus.PENDING: {"start": NodeStatus.RUNNING},
    NodeStatus.RUNNING: {
        "complete": NodeStatus.TERMINAL_SUCCESS,
        "fail": NodeStatus.TERMINAL_FAILED,
        "block": NodeStatus.BLOCKED,
        "handoff": NodeStatus.HANDOFF,
        "interrupt": NodeStatus.RECOVERING,
    },
    NodeStatus.BLOCKED: {"resume": NodeStatus.RUNNING},
    NodeStatus.HANDOFF: {"accept": NodeStatus.RUNNING},
    NodeStatus.RECOVERING: {
        "resume": NodeStatus.RUNNING,
        "fail": NodeStatus.TERMINAL_FAILED,
    },
}


class TransitionError(Exception):
    """Raised fail-closed when a requested transition cannot be applied."""


class TransitionEngine:
    def __init__(
        self,
        graph: Graph,
        state_store: RunStateStore,
        event_log: EventLog,
        *,
        grader_registry: "praxis_evidence.graders.GraderRegistry | None" = None,
    ) -> None:
        self._graph = graph
        self._state_store = state_store
        self._event_log = event_log
        self._grader_registry = grader_registry or praxis_evidence.graders.default_registry()

    def current_state(self) -> RunState:
        state = self._state_store.load()
        if state is not None:
            self._validate_against_log(state, self._event_log.read_all())
            return state
        entry = self._graph.entry_node
        return RunState(
            spec_version=_SPEC_VERSION,
            run_id=uuid.uuid4().hex,
            cursors={entry: Cursor(node_id=entry, status=NodeStatus.PENDING.value)},
            last_applied_seq=-1,
        )

    def _validate_against_log(self, state: RunState, events: list[Event]) -> None:
        max_seq = events[-1].seq if events else -1
        if state.last_applied_seq > max_seq:
            raise TransitionError(
                f"checkpoint last_applied_seq={state.last_applied_seq} is ahead of "
                f"the event log (max seq={max_seq})"
            )

    def legal_next(self, node_id: str) -> set[str]:
        state = self.current_state()
        cursor = state.cursors.get(node_id)
        if cursor is None:
            return set()
        return set(_TRANSITIONS.get(NodeStatus(cursor.status), {}).keys())

    def apply(
        self, node_id: str, event_type: str, *, evidence: list[dict] | None = None
    ) -> RunState:
        lock_path = self._lock_path()
        lock_path.parent.mkdir(parents=True, exist_ok=True)
        with open(lock_path, "a", encoding="utf-8") as lock_handle:
            fcntl.flock(lock_handle, fcntl.LOCK_EX)
            try:
                return self._apply_locked(node_id, event_type, evidence=evidence)
            finally:
                fcntl.flock(lock_handle, fcntl.LOCK_UN)

    def _lock_path(self) -> Path:
        state_path = self._state_store._path
        return state_path.with_name(state_path.name + ".lock")

    def _apply_locked(
        self, node_id: str, event_type: str, *, evidence: list[dict] | None = None
    ) -> RunState:
        state = self.current_state()
        cursor = state.cursors.get(node_id)
        if cursor is None:
            raise TransitionError(f"node {node_id!r} has no active cursor")

        current_status = NodeStatus(cursor.status)
        new_status = _TRANSITIONS.get(current_status, {}).get(event_type)
        if new_status is None:
            raise TransitionError(
                f"illegal transition {event_type!r} from status {current_status.value!r} "
                f"for node {node_id!r}"
            )

        if new_status in _TERMINAL_STATUSES:
            node = self._graph.nodes.get(node_id)
            if node is not None:
                self._check_evidence(node, evidence)

        stored_event = self._event_log.append(
            Event(
                spec_version=_SPEC_VERSION,
                seq=0,
                run_id=state.run_id,
                node_id=node_id,
                event_type=event_type,
                payload={"evidence": evidence} if evidence is not None else {},
                event_id=uuid.uuid4().hex,
            )
        )

        new_cursors = dict(state.cursors)
        new_cursors[node_id] = Cursor(node_id=node_id, status=new_status.value)
        if new_status == NodeStatus.TERMINAL_SUCCESS:
            self._advance_successors(node_id, new_cursors)

        new_state = RunState(
            spec_version=state.spec_version,
            run_id=state.run_id,
            cursors=new_cursors,
            last_applied_seq=stored_event.seq,
        )
        self._state_store.save(new_state)
        return new_state

    def _advance_successors(self, node_id: str, cursors: dict[str, Cursor]) -> None:
        outgoing: list[Edge] = [edge for edge in self._graph.edges if edge.source == node_id]
        for edge in outgoing:
            if edge.target in cursors:
                continue
            if edge.kind == "join" and not self._join_ready(edge.target, cursors):
                continue
            cursors[edge.target] = Cursor(node_id=edge.target, status=NodeStatus.PENDING.value)

    def _join_ready(self, target: str, cursors: dict[str, Cursor]) -> bool:
        incoming = [edge for edge in self._graph.edges if edge.target == target]
        return all(
            cursors.get(edge.source) is not None
            and cursors[edge.source].status == NodeStatus.TERMINAL_SUCCESS.value
            for edge in incoming
        )

    def _check_evidence(self, node: Node, evidence: list[dict] | None) -> None:
        requirement = node.metadata.get("evidence_requirement")
        if not requirement:
            return

        result = evaluate_gate(
            requirement,
            evidence or [],
            graph_version=self._graph.spec_version,
            registry=self._grader_registry,
        )

        join_sources = [
            edge.source
            for edge in self._graph.edges
            if edge.target == node.id and edge.kind == "join"
        ]
        if join_sources:
            source_results = [self._source_gate_result(source) for source in join_sources]
            result = aggregate_gate_results(node.id, [*source_results, result])

        if not result.satisfied:
            raise TransitionError("; ".join(result.reasons))

    def _source_gate_result(self, source_node_id: str) -> GateResult:
        source_node = self._graph.nodes.get(source_node_id)
        requirement = source_node.metadata.get("evidence_requirement") if source_node else None
        if not requirement:
            return GateResult(node_id=source_node_id, satisfied=True, reasons=(), evaluated=())

        return evaluate_gate(
            requirement,
            self._stored_evidence(source_node_id),
            graph_version=self._graph.spec_version,
            registry=self._grader_registry,
        )

    def _stored_evidence(self, node_id: str) -> list[dict]:
        for event in reversed(self._event_log.read_all()):
            if event.node_id == node_id and "evidence" in event.payload:
                return event.payload["evidence"] or []
        return []

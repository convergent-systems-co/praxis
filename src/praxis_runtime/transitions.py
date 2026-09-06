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
Evidence supplied to apply() is persisted onto the committed Event's payload
under the "evidence" key, giving a durable audit trail of what evidence
satisfied a gate. apply() holds an exclusive flock on a sidecar file next
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
import time
import uuid
from pathlib import Path

from praxis_runtime.events import Event, EventLog
from praxis_runtime.graph import Edge, Graph, Node
from praxis_runtime.resources import claims, leases, observed, policy
from praxis_runtime.resources.adapters.filesystem import paths_overlap
from praxis_runtime.state import Cursor, RunState, RunStateStore

_SPEC_VERSION = "1.0.0"


def _default_identifier_overlap(a: str, b: str) -> bool:
    # Mirrors leases.acquire's own default conflict_fn: identifier equality,
    # with the workspace-wide "*" fallback always treated as overlapping.
    return a == b or a == "*" or b == "*"


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
        resource_lease_store: "leases.LeaseStore | None" = None,
        resource_policy: "policy.ResourceAccessPolicy" = policy.ResourceAccessPolicy.STRICT,
        resource_ttl: float = 60.0,
    ) -> None:
        self._graph = graph
        self._state_store = state_store
        self._event_log = event_log
        self._resource_lease_store = resource_lease_store
        self._resource_policy = resource_policy
        self._resource_ttl = resource_ttl

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

    def apply(self, node_id: str, event_type: str, *, evidence: dict | None = None) -> RunState:
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
        self, node_id: str, event_type: str, *, evidence: dict | None = None
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

        node = self._graph.nodes.get(node_id)

        # Evidence is checked before resource claims are settled: settling a
        # terminal transition's resource claims revalidates and releases the
        # node's leases, and a transition the evidence gate is about to
        # reject must never have already given up that ownership (fail
        # closed, no partial write).
        if new_status in _TERMINAL_STATUSES:
            if node is not None:
                self._check_evidence(node, evidence)

        resource_leases_payload = None
        if node is not None:
            resource_leases_payload = self._check_resource_claims(node, event_type, new_status)

        payload: dict = {}
        if evidence is not None:
            payload["evidence"] = evidence
        if resource_leases_payload is not None:
            payload["resource_leases"] = resource_leases_payload

        stored_event = self._event_log.append(
            Event(
                spec_version=_SPEC_VERSION,
                seq=0,
                run_id=state.run_id,
                node_id=node_id,
                event_type=event_type,
                payload=payload,
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

    def _check_evidence(self, node: Node, evidence: dict | None) -> None:
        requirement = node.metadata.get("evidence_requirement")
        if not requirement:
            return
        required_keys = [
            item["proof_type"]
            for item in requirement.get("evidence", [])
            if item.get("constraint") == "required"
        ]
        provided = evidence or {}
        missing = [key for key in required_keys if key not in provided]
        if missing:
            raise TransitionError(
                f"node {node.id!r} is missing required evidence: {missing}"
            )

    def _check_resource_claims(
        self, node: Node, event_type: str, new_status: NodeStatus
    ) -> dict[str, int] | None:
        if self._resource_lease_store is None:
            return None

        is_terminal = new_status in _TERMINAL_STATUSES
        if event_type != "start" and not is_terminal:
            # block/handoff/interrupt/resume/accept never consult resource
            # claims, so the (potentially expensive, schema-validating)
            # parse below is skipped for them rather than run and discarded.
            return None

        document = node.metadata.get("resource_claims")
        declared_claims = claims.parse_claims(document) if document else []

        if event_type == "start":
            if not declared_claims:
                return None
            return self._acquire_resource_claims(node, declared_claims)

        if is_terminal:
            observed_document = node.metadata.get("observed_resources")
            observed_claims = (
                observed.parse_observed_resources(observed_document) if observed_document else []
            )
            # Every check below is read-only (authorize_access and
            # leases.revalidate never persist anything); only once every
            # declared claim and every observed resource has been validated
            # do the mutating acquire/release calls below run. Otherwise a
            # later check failing (e.g. a stale-epoch declared claim) after
            # an earlier mutation already ran (e.g. a dynamic-grant lease
            # acquire+release) would leave that mutation persisted for a
            # transition that ultimately never commits (fail-closed, no
            # partial write).
            dynamic_grants = self._check_observed_resources(node, declared_claims, observed_claims)
            resource_leases = None
            if declared_claims:
                resource_leases = self._start_event_resource_leases(node.id)
                self._revalidate_declared_claims(node, declared_claims, resource_leases)

            if dynamic_grants:
                self._record_dynamic_grants(node, dynamic_grants)
            if declared_claims:
                self._release_declared_claims(node, declared_claims, resource_leases)

        return None

    @staticmethod
    def _lease_key(resource_type: str, identifier: str) -> str:
        return f"{resource_type}\x1f{identifier}"

    @staticmethod
    def _lease_conflict_fn(resource_type: str):
        # Filesystem identifiers are path globs, so two differently-spelled
        # identifiers (e.g. "src/a/**" and "src/a/file.py") can still name
        # overlapping filesystem footprints. leases.acquire otherwise only
        # detects exact-identifier conflicts, so filesystem claims must be
        # checked with the adapter's own glob-aware paths_overlap instead of
        # plain equality -- other resource types keep leases.acquire's
        # exact-identifier default.
        if resource_type == "filesystem":
            return paths_overlap
        return None

    def _acquire_resource_claims(
        self, node: Node, declared_claims: list[claims.ResourceClaim]
    ) -> dict[str, int]:
        acquired: list[tuple[claims.ResourceClaim, leases.Lease]] = []
        try:
            for claim in declared_claims:
                lease = leases.acquire(
                    self._resource_lease_store,
                    claim.resource_type,
                    claim.identifier,
                    owner=node.id,
                    now=time.time(),
                    ttl=self._resource_ttl,
                    access_mode=claim.access_mode,
                    conflict_fn=self._lease_conflict_fn(claim.resource_type),
                )
                acquired.append((claim, lease))
        except leases.LeaseError as exc:
            for claim, lease in acquired:
                try:
                    leases.release(
                        self._resource_lease_store,
                        lease.resource_type,
                        lease.identifier,
                        lease.owner,
                        lease.epoch,
                        access_mode=claim.access_mode,
                    )
                except leases.LeaseError:
                    # Best-effort rollback: the original acquisition failure
                    # below is what must be reported. A LeaseError raised
                    # here would otherwise propagate unwrapped, breaking the
                    # single-exception-type (TransitionError) contract this
                    # method's callers rely on.
                    pass
            raise TransitionError(str(exc)) from exc

        return {
            self._lease_key(lease.resource_type, lease.identifier): lease.epoch
            for _, lease in acquired
        }

    def _start_event_resource_leases(self, node_id: str) -> dict[str, int]:
        # A single reversed scan of the event log locates node_id's own most
        # recent "start" event once per terminal transition; _acquired_epoch
        # below then does an in-memory dict lookup per declared claim instead
        # of each claim re-scanning the whole log itself (that scan is O(log
        # length) and was previously repeated once per declared claim, in
        # both _revalidate_declared_claims and _release_declared_claims).
        for event in reversed(self._event_log.read_all()):
            if event.node_id == node_id and event.event_type == "start":
                return event.payload.get("resource_leases", {})
        raise TransitionError(f"no start event recorded for node {node_id!r}")

    def _acquired_epoch(
        self,
        resource_leases: dict[str, int],
        node_id: str,
        resource_type: str,
        identifier: str,
    ) -> int:
        key = self._lease_key(resource_type, identifier)
        epoch = resource_leases.get(key)
        if epoch is None:
            raise TransitionError(
                f"no recorded lease acquisition for ({resource_type!r}, "
                f"{identifier!r}) on node {node_id!r}"
            )
        return epoch

    def _revalidate_declared_claims(
        self,
        node: Node,
        declared_claims: list[claims.ResourceClaim],
        resource_leases: dict[str, int],
    ) -> None:
        """Read-only check: raises if any declared claim's lease is no longer
        live. Never releases -- see the ordering note in _check_resource_claims."""
        for claim in declared_claims:
            # Use the epoch recorded at acquire time (from the node's own
            # "start" event), not whatever epoch happens to be stored right
            # now: reloading the current epoch here and revalidating against
            # itself would be tautological and could never detect that the
            # lease moved to a new generation between acquire and settle.
            epoch = self._acquired_epoch(
                resource_leases, node.id, claim.resource_type, claim.identifier
            )
            try:
                leases.revalidate(
                    self._resource_lease_store,
                    claim.resource_type,
                    claim.identifier,
                    owner=node.id,
                    epoch=epoch,
                    now=time.time(),
                    access_mode=claim.access_mode,
                )
            except leases.LeaseError as exc:
                raise TransitionError(str(exc)) from exc

    def _release_declared_claims(
        self,
        node: Node,
        declared_claims: list[claims.ResourceClaim],
        resource_leases: dict[str, int],
    ) -> None:
        """Mutating: only called after every check in this terminal
        transition has already passed (see _check_resource_claims)."""
        for claim in declared_claims:
            epoch = self._acquired_epoch(
                resource_leases, node.id, claim.resource_type, claim.identifier
            )
            try:
                leases.release(
                    self._resource_lease_store,
                    claim.resource_type,
                    claim.identifier,
                    node.id,
                    epoch,
                    access_mode=claim.access_mode,
                )
            except leases.LeaseError as exc:
                raise TransitionError(str(exc)) from exc

    def _check_observed_resources(
        self,
        node: Node,
        declared_claims: list[claims.ResourceClaim],
        observed_claims: list[claims.ResourceClaim],
    ) -> list[claims.ResourceClaim]:
        """Read-only check: raises under an undeclared/conflicting access,
        otherwise returns the observed claims that were dynamically granted
        (not covered by a declared claim) and so still need their grant
        recorded as a lease by _record_dynamic_grants."""
        dynamic_grants = []
        for requested in observed_claims:
            active_claims = self._active_foreign_claims(node.id, requested)
            try:
                granted = policy.authorize_access(
                    declared_claims, requested, self._resource_policy, active_claims
                )
            except policy.UndeclaredResourceError as exc:
                raise TransitionError(str(exc)) from exc

            if granted is requested:
                dynamic_grants.append(requested)
        return dynamic_grants

    def _record_dynamic_grants(
        self, node: Node, dynamic_grants: list[claims.ResourceClaim]
    ) -> None:
        """Mutating: only called after every check in this terminal
        transition has already passed (see _check_resource_claims)."""
        for requested in dynamic_grants:
            # authorize_access granted this dynamically (it was not covered
            # by a declared claim): record the grant as a real lease so a
            # concurrent scheduler's own authorization check -- which
            # consults the lease store via _active_foreign_claims, not this
            # call's in-memory active_claims snapshot -- can see it.
            # Discarding the granted claim here would let two nodes each
            # pass the conflict check without either ever registering the
            # resource as held. The node is terminating in this same call,
            # so once that visibility has been recorded the lease is
            # revalidated and released immediately -- otherwise it would
            # leak a hold on the resource for up to resource_ttl seconds
            # after the owning node has already terminated.
            try:
                lease = leases.acquire(
                    self._resource_lease_store,
                    requested.resource_type,
                    requested.identifier,
                    owner=node.id,
                    now=time.time(),
                    ttl=self._resource_ttl,
                    access_mode=requested.access_mode,
                    conflict_fn=self._lease_conflict_fn(requested.resource_type),
                )
                leases.revalidate(
                    self._resource_lease_store,
                    lease.resource_type,
                    lease.identifier,
                    owner=lease.owner,
                    epoch=lease.epoch,
                    now=time.time(),
                    access_mode=requested.access_mode,
                )
                leases.release(
                    self._resource_lease_store,
                    lease.resource_type,
                    lease.identifier,
                    lease.owner,
                    lease.epoch,
                    access_mode=requested.access_mode,
                )
            except leases.LeaseError as exc:
                raise TransitionError(str(exc)) from exc

    def _active_foreign_claims(
        self, node_id: str, requested: claims.ResourceClaim
    ) -> list[claims.ResourceClaim]:
        # Mirrors leases.acquire's own overlap scan (active_writer_leases /
        # active_reader_leases with the resource type's conflict_fn) instead
        # of a single exact-key load(): an exact-key lookup misses both a
        # foreign lease held under a different-but-overlapping identifier
        # (e.g. the "*" workspace-wide fallback, or a glob-overlapping
        # filesystem path) and any reader lease, which is stored under a
        # separate per-owner file that load() never sees.
        now = time.time()
        overlaps = self._lease_conflict_fn(requested.resource_type) or _default_identifier_overlap
        foreign: list[claims.ResourceClaim] = []
        for lease in self._resource_lease_store.active_writer_leases(requested.resource_type, now):
            if lease.owner == node_id:
                continue
            if lease.identifier == requested.identifier or overlaps(lease.identifier, requested.identifier):
                # The lease store does not record which access mode a
                # canonical writer lease was granted under (write vs.
                # exclusive), so it is treated conservatively as EXCLUSIVE:
                # fail closed rather than assume it would have been
                # compatible.
                foreign.append(
                    claims.ResourceClaim(
                        resource_type=requested.resource_type,
                        identifier=requested.identifier,
                        access_mode=claims.AccessMode.EXCLUSIVE.value,
                    )
                )
        for lease in self._resource_lease_store.active_reader_leases(requested.resource_type, now):
            if lease.owner == node_id:
                continue
            if lease.identifier == requested.identifier or overlaps(lease.identifier, requested.identifier):
                foreign.append(
                    claims.ResourceClaim(
                        resource_type=requested.resource_type,
                        identifier=requested.identifier,
                        access_mode=claims.AccessMode.READ.value,
                    )
                )
        return foreign

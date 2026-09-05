"""Deterministic parking/retry scheduler.

ResourceScheduler.request grants a claim immediately (returns True) unless it
conflicts, per its conflict_fn (claims_conflict by default), with a
currently granted claim held by a different node_id; otherwise the request
is parked (FIFO) and False is returned. ResourceScheduler.release removes a
node's grant and then grants, in FIFO park order, every parked request that
no longer conflicts with anything currently granted, returning the newly
granted node_ids in the order granted. ResourceScheduler.pending returns the
current park queue in order.

conflict_fn is pluggable so a domain adapter whose identifiers need a
different notion of conflict than claims_conflict's exact-identifier match
can supply its own — e.g. the filesystem adapter's footprint_conflict, which
is glob-aware (see praxis_runtime.resources.adapters.filesystem).
"""

from __future__ import annotations

from collections.abc import Callable
from dataclasses import dataclass

from praxis_runtime.resources.claims import ResourceClaim, claims_conflict

ConflictFn = Callable[[ResourceClaim, ResourceClaim], bool]


@dataclass(frozen=True)
class ParkedRequest:
    node_id: str
    claim: ResourceClaim


class ResourceScheduler:
    def __init__(self, conflict_fn: ConflictFn = claims_conflict) -> None:
        self._conflict_fn = conflict_fn
        self._grants: dict[str, ResourceClaim] = {}
        self._parked: list[ParkedRequest] = []

    def request(self, node_id: str, claim: ResourceClaim) -> bool:
        if self._conflicts_with_grants(node_id, claim):
            self._parked.append(ParkedRequest(node_id=node_id, claim=claim))
            return False
        self._grants[node_id] = claim
        return True

    def release(self, node_id: str, claim: ResourceClaim) -> list[str]:
        self._grants.pop(node_id, None)

        granted: list[str] = []
        still_parked: list[ParkedRequest] = []
        for parked in self._parked:
            if self._conflicts_with_grants(parked.node_id, parked.claim):
                still_parked.append(parked)
            else:
                self._grants[parked.node_id] = parked.claim
                granted.append(parked.node_id)
        self._parked = still_parked

        return granted

    def pending(self) -> list[ParkedRequest]:
        return list(self._parked)

    def _conflicts_with_grants(self, node_id: str, claim: ResourceClaim) -> bool:
        return any(
            self._conflict_fn(claim, granted_claim)
            for granted_node_id, granted_claim in self._grants.items()
            if granted_node_id != node_id
        )

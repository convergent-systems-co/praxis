"""Deterministic parking/retry scheduler.

ResourceScheduler.request grants a claim immediately (returns True) unless it
conflicts, per claims_conflict, with a currently granted claim held by a
different node_id; otherwise the request is parked (FIFO) and False is
returned. ResourceScheduler.release removes a node's grant and then grants,
in FIFO park order, every parked request that no longer conflicts with
anything currently granted, returning the newly granted node_ids in the
order granted. ResourceScheduler.pending returns the current park queue in
order.
"""

from __future__ import annotations

from praxis_runtime.resources.claims import AccessMode, ResourceClaim
from praxis_runtime.resources.scheduler import ParkedRequest, ResourceScheduler


def _claim(resource_type: str, identifier: str, access_mode: str) -> ResourceClaim:
    return ResourceClaim(
        resource_type=resource_type,
        identifier=identifier,
        access_mode=access_mode,
    )


def test_two_compatible_read_requests_from_different_nodes_are_both_granted():
    scheduler = ResourceScheduler()
    claim_a = _claim("compute-slot", "gpu-0", AccessMode.READ.value)
    claim_b = _claim("compute-slot", "gpu-0", AccessMode.READ.value)

    assert scheduler.request("node-a", claim_a) is True
    assert scheduler.request("node-b", claim_b) is True
    assert scheduler.pending() == []


def test_conflicting_write_request_is_parked_and_appears_in_pending():
    scheduler = ResourceScheduler()
    holder_claim = _claim("compute-slot", "gpu-0", AccessMode.WRITE.value)
    waiter_claim = _claim("compute-slot", "gpu-0", AccessMode.WRITE.value)

    assert scheduler.request("holder", holder_claim) is True
    assert scheduler.request("waiter", waiter_claim) is False
    assert scheduler.pending() == [ParkedRequest(node_id="waiter", claim=waiter_claim)]


def test_release_grants_parked_requests_in_fifo_order():
    scheduler = ResourceScheduler()
    holder_claim = _claim("compute-slot", "*", AccessMode.EXCLUSIVE.value)
    zeta_claim = _claim("compute-slot", "gpu-0", AccessMode.WRITE.value)
    alpha_claim = _claim("compute-slot", "gpu-1", AccessMode.WRITE.value)

    scheduler.request("holder", holder_claim)
    scheduler.request("zeta", zeta_claim)
    scheduler.request("alpha", alpha_claim)

    granted = scheduler.release("holder", holder_claim)

    assert granted == ["zeta", "alpha"]
    assert scheduler.pending() == []


def test_release_with_no_parked_requests_leaves_pending_empty():
    scheduler = ResourceScheduler()
    claim = _claim("compute-slot", "gpu-0", AccessMode.WRITE.value)

    scheduler.request("holder", claim)
    granted = scheduler.release("holder", claim)

    assert granted == []
    assert scheduler.pending() == []

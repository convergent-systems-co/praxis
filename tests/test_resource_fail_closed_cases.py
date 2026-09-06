"""Cross-cutting resource-claim/lease acceptance tests.

Exercises the resource-claim acceptance criteria end to end, importing
directly from claims.py/policy.py/scheduler.py rather than re-deriving them
from a single module's own unit-test suite: two compatible read claims
running concurrently, conflicting write/mutate claims serializing
deterministically through both static planning and the runtime scheduler,
an undeclared resource request failing closed under a strict policy, a
policy that allows deterministic dynamic acquisition when safe and blocks it
when unsafe, a malformed resource-claim document failing closed at parse
time, and a workspace-wide fallback claim blocking an otherwise-unrelated
concurrent claim of the same resource type.
"""

from __future__ import annotations

import pytest

from praxis_contracts.validator import ContractValidationError
from praxis_runtime.resources.claims import AccessMode, ResourceClaim, parse_claims, plan_claims
from praxis_runtime.resources.policy import (
    ResourceAccessPolicy,
    UndeclaredResourceError,
    authorize_access,
)
from praxis_runtime.resources.scheduler import ParkedRequest, ResourceScheduler

DOCUMENT_MISSING_RESOURCE_TYPE = {
    "spec_version": "1.0.0",
    "claims": [
        {
            "quantity": 1,
            "identifier": "table-1",
            "access_mode": "write",
        }
    ],
}

DOCUMENT_WITH_INVALID_ACCESS_MODE = {
    "spec_version": "1.0.0",
    "claims": [
        {
            "resource_type": "dataset",
            "quantity": 1,
            "identifier": "table-1",
            "access_mode": "delete",
        }
    ],
}


def _claim(resource_type: str, identifier: str, access_mode: str) -> ResourceClaim:
    return ResourceClaim(resource_type=resource_type, identifier=identifier, access_mode=access_mode)


# -- (a) Compatible read claims run concurrently ----------------------------


def test_two_compatible_read_claims_run_concurrently_via_scheduler():
    scheduler = ResourceScheduler()
    read_a = _claim("dataset", "table-1", AccessMode.READ.value)
    read_b = _claim("dataset", "table-1", AccessMode.READ.value)

    assert scheduler.request("reader-a", read_a) is True
    assert scheduler.request("reader-b", read_b) is True
    assert scheduler.pending() == []


# -- (b) Conflicting write/mutate claims serialize deterministically -------


def test_conflicting_write_claims_serialize_deterministically():
    claim_a = _claim("dataset", "table-1", AccessMode.WRITE.value)
    claim_b = _claim("dataset", "table-1", AccessMode.WRITE.value)

    # Static planning agrees the two nodes contend for the same resource,
    # in deterministic (lexicographic) order regardless of insertion order.
    conflicts = plan_claims({"task-b": [claim_b], "task-a": [claim_a]})
    assert conflicts == [("task-a", "task-b")]

    # The runtime scheduler serializes the same pair: the second requester
    # is parked, not silently granted alongside the first, and is only
    # granted once the first releases -- in deterministic FIFO order.
    scheduler = ResourceScheduler()
    assert scheduler.request("task-a", claim_a) is True
    assert scheduler.request("task-b", claim_b) is False
    assert scheduler.pending() == [ParkedRequest(node_id="task-b", claim=claim_b)]

    granted = scheduler.release("task-a", claim_a)
    assert granted == ["task-b"]
    assert scheduler.pending() == []


# -- (c) Undeclared resource access cannot silently mutate under STRICT -----


def test_undeclared_mutate_request_under_strict_cannot_silently_proceed():
    requested = _claim("dataset", "table-1", AccessMode.EXCLUSIVE.value)

    with pytest.raises(UndeclaredResourceError):
        authorize_access(
            declared=[],
            requested=requested,
            policy=ResourceAccessPolicy.STRICT,
            active_claims=[],
        )


# -- (d) DYNAMIC policy allows deterministic acquisition when safe, blocks when unsafe --


def test_dynamic_policy_allows_when_safe_and_blocks_when_unsafe():
    safe_request = _claim("dataset", "table-2", AccessMode.WRITE.value)
    unrelated_active = [_claim("dataset", "table-1", AccessMode.WRITE.value)]

    granted = authorize_access(
        declared=[],
        requested=safe_request,
        policy=ResourceAccessPolicy.DYNAMIC,
        active_claims=unrelated_active,
    )
    assert granted == safe_request

    unsafe_request = _claim("dataset", "table-1", AccessMode.WRITE.value)
    conflicting_active = [_claim("dataset", "table-1", AccessMode.READ.value)]

    with pytest.raises(UndeclaredResourceError):
        authorize_access(
            declared=[],
            requested=unsafe_request,
            policy=ResourceAccessPolicy.DYNAMIC,
            active_claims=conflicting_active,
        )


# -- (e) Malformed resource-claim documents fail closed at parse time -------


def test_malformed_document_missing_resource_type_fails_closed():
    with pytest.raises(ContractValidationError):
        parse_claims(DOCUMENT_MISSING_RESOURCE_TYPE)


def test_malformed_document_with_invalid_access_mode_fails_closed():
    with pytest.raises(ContractValidationError):
        parse_claims(DOCUMENT_WITH_INVALID_ACCESS_MODE)


# -- (f) Workspace-wide fallback claim blocks an unrelated concurrent claim --


def test_wildcard_fallback_claim_blocks_unrelated_concurrent_claim_of_same_resource_type():
    scheduler = ResourceScheduler()
    fallback_claim = _claim("dataset", "*", AccessMode.EXCLUSIVE.value)
    unrelated_claim = _claim("dataset", "table-99", AccessMode.WRITE.value)

    assert scheduler.request("holder", fallback_claim) is True
    assert scheduler.request("unrelated", unrelated_claim) is False
    assert scheduler.pending() == [ParkedRequest(node_id="unrelated", claim=unrelated_claim)]

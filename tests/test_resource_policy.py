"""Undeclared-resource-access policy.

authorize_access checks whether a requested claim is already covered by a
declared claim (same resource_type/identifier, and the declared claim's
access_mode permits the requested access_mode: EXCLUSIVE/WRITE declared
covers WRITE+READ requests, READ declared covers only READ requests) and
returns that declared claim unchanged if so.

Otherwise, under ResourceAccessPolicy.STRICT it always raises
UndeclaredResourceError (undeclared access is a planning defect). Under
ResourceAccessPolicy.DYNAMIC it grants the requested claim only if it does
not conflict (per claims_conflict) with any claim in active_claims;
if it does conflict, it raises UndeclaredResourceError.
"""

from __future__ import annotations

import pytest

from praxis_runtime.resources.claims import AccessMode, ResourceClaim
from praxis_runtime.resources.policy import (
    ResourceAccessPolicy,
    UndeclaredResourceError,
    authorize_access,
)


def _claim(resource_type: str, identifier: str, access_mode: str) -> ResourceClaim:
    return ResourceClaim(
        resource_type=resource_type,
        identifier=identifier,
        access_mode=access_mode,
    )


def test_undeclared_write_request_under_strict_raises():
    requested = _claim("compute-slot", "gpu-0", AccessMode.WRITE.value)

    with pytest.raises(UndeclaredResourceError):
        authorize_access(
            declared=[],
            requested=requested,
            policy=ResourceAccessPolicy.STRICT,
            active_claims=[],
        )


def test_declared_read_claim_does_not_authorize_undeclared_write_even_under_strict():
    declared = [_claim("compute-slot", "gpu-0", AccessMode.READ.value)]
    requested = _claim("compute-slot", "gpu-0", AccessMode.WRITE.value)

    with pytest.raises(UndeclaredResourceError):
        authorize_access(
            declared=declared,
            requested=requested,
            policy=ResourceAccessPolicy.STRICT,
            active_claims=[],
        )


def test_undeclared_request_under_dynamic_with_no_conflict_is_granted():
    requested = _claim("compute-slot", "gpu-0", AccessMode.WRITE.value)

    result = authorize_access(
        declared=[],
        requested=requested,
        policy=ResourceAccessPolicy.DYNAMIC,
        active_claims=[_claim("compute-slot", "gpu-1", AccessMode.WRITE.value)],
    )

    assert result == requested


def test_undeclared_request_under_dynamic_that_conflicts_with_active_claim_raises():
    requested = _claim("compute-slot", "gpu-0", AccessMode.WRITE.value)
    active_claims = [_claim("compute-slot", "gpu-0", AccessMode.READ.value)]

    with pytest.raises(UndeclaredResourceError):
        authorize_access(
            declared=[],
            requested=requested,
            policy=ResourceAccessPolicy.DYNAMIC,
            active_claims=active_claims,
        )


@pytest.mark.parametrize("policy", [ResourceAccessPolicy.STRICT, ResourceAccessPolicy.DYNAMIC])
def test_request_covered_by_declared_claim_is_granted_without_consulting_active_claims(policy):
    declared_claim = _claim("compute-slot", "gpu-0", AccessMode.WRITE.value)
    requested = _claim("compute-slot", "gpu-0", AccessMode.READ.value)
    # This active claim would conflict with `requested` if it were ever
    # consulted; it must not be, since `requested` is already covered.
    conflicting_active_claims = [_claim("compute-slot", "gpu-0", AccessMode.EXCLUSIVE.value)]

    result = authorize_access(
        declared=[declared_claim],
        requested=requested,
        policy=policy,
        active_claims=conflicting_active_claims,
    )

    assert result == declared_claim

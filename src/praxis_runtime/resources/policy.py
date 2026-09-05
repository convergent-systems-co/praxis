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

import enum

from praxis_runtime.resources.claims import AccessMode, ResourceClaim, claims_conflict


class ResourceAccessPolicy(enum.Enum):
    STRICT = "strict"
    DYNAMIC = "dynamic"


class UndeclaredResourceError(Exception):
    pass


def _covers(declared: ResourceClaim, requested: ResourceClaim) -> bool:
    if declared.resource_type != requested.resource_type:
        return False
    if declared.identifier != requested.identifier:
        return False
    if declared.access_mode in (AccessMode.EXCLUSIVE.value, AccessMode.WRITE.value):
        return requested.access_mode in (AccessMode.WRITE.value, AccessMode.READ.value)
    if declared.access_mode == AccessMode.READ.value:
        return requested.access_mode == AccessMode.READ.value
    return False


def authorize_access(
    declared: list[ResourceClaim],
    requested: ResourceClaim,
    policy: ResourceAccessPolicy,
    active_claims: list[ResourceClaim],
) -> ResourceClaim:
    for declared_claim in declared:
        if _covers(declared_claim, requested):
            return declared_claim

    if policy == ResourceAccessPolicy.STRICT:
        raise UndeclaredResourceError(
            f"undeclared access to {requested.resource_type}:{requested.identifier}"
        )

    if any(claims_conflict(requested, active_claim) for active_claim in active_claims):
        raise UndeclaredResourceError(
            f"undeclared access to {requested.resource_type}:{requested.identifier} "
            "conflicts with an active claim"
        )

    return requested

"""Claim model and deterministic conflict detection.

parse_claims validates a resource-claim document against
resource-claim.schema.json and returns one ResourceClaim per schedulable
entry (an entry with both identifier and access_mode set); budget-only
entries (no identifier) are not schedulable resources and are skipped.

claims_conflict is the single source of truth for whether two claims
contend for the same resource: claims of different resource_type never
conflict, two READ claims never conflict, and otherwise two claims
conflict when their identifier matches or either side is the workspace-wide
fallback identifier "*".

plan_claims applies claims_conflict across every pair of node ids in a
claim-set mapping and returns the conflicting pairs sorted deterministically
(a < b lexicographically within a pair, pairs sorted ascending).
"""

from __future__ import annotations

from praxis_runtime.resources.claims import (
    AccessMode,
    ResourceClaim,
    claims_conflict,
    parse_claims,
    plan_claims,
)

VALID_DOCUMENT = {
    "spec_version": "1.0.0",
    "claims": [
        {
            "resource_type": "compute-slot",
            "quantity": 1,
            "identifier": "gpu-0",
            "access_mode": "write",
        },
        {
            "resource_type": "memory",
            "quantity": 4,
            "unit": "GB",
        },
    ],
}

DOCUMENT_WITH_ACCESS_MODE_ONLY = {
    "spec_version": "1.0.0",
    "claims": [
        {
            "resource_type": "compute-slot",
            "quantity": 1,
            "access_mode": "read",
        },
    ],
}


def _claim(resource_type: str, identifier: str, access_mode: str) -> ResourceClaim:
    return ResourceClaim(
        resource_type=resource_type,
        identifier=identifier,
        access_mode=access_mode,
    )


def test_parse_claims_returns_one_claim_per_schedulable_entry():
    claims = parse_claims(VALID_DOCUMENT)

    assert len(claims) == 1
    claim = claims[0]
    assert isinstance(claim, ResourceClaim)
    assert claim.resource_type == "compute-slot"
    assert claim.identifier == "gpu-0"
    assert claim.access_mode == AccessMode.WRITE.value
    assert claim.quantity == 1


def test_parse_claims_skips_budget_only_entries_without_identifier():
    claims = parse_claims(VALID_DOCUMENT)

    assert all(claim.resource_type != "memory" for claim in claims)


def test_parse_claims_skips_entries_missing_identifier_even_with_access_mode():
    assert parse_claims(DOCUMENT_WITH_ACCESS_MODE_ONLY) == []


def test_two_read_claims_on_same_identifier_do_not_conflict():
    a = _claim("compute-slot", "gpu-0", AccessMode.READ.value)
    b = _claim("compute-slot", "gpu-0", AccessMode.READ.value)

    assert claims_conflict(a, b) is False


def test_write_conflicts_with_read_on_same_identifier():
    a = _claim("compute-slot", "gpu-0", AccessMode.WRITE.value)
    b = _claim("compute-slot", "gpu-0", AccessMode.READ.value)

    assert claims_conflict(a, b) is True


def test_write_conflicts_with_write_on_same_identifier():
    a = _claim("compute-slot", "gpu-0", AccessMode.WRITE.value)
    b = _claim("compute-slot", "gpu-0", AccessMode.WRITE.value)

    assert claims_conflict(a, b) is True


def test_different_identifiers_same_resource_type_do_not_conflict():
    a = _claim("compute-slot", "gpu-0", AccessMode.WRITE.value)
    b = _claim("compute-slot", "gpu-1", AccessMode.WRITE.value)

    assert claims_conflict(a, b) is False


def test_different_resource_types_never_conflict():
    a = _claim("compute-slot", "gpu-0", AccessMode.EXCLUSIVE.value)
    b = _claim("memory", "gpu-0", AccessMode.EXCLUSIVE.value)

    assert claims_conflict(a, b) is False


def test_wildcard_identifier_conflicts_with_specific_identifier_same_resource_type():
    a = _claim("compute-slot", "*", AccessMode.WRITE.value)
    b = _claim("compute-slot", "gpu-7", AccessMode.READ.value)

    assert claims_conflict(a, b) is True


def test_wildcard_read_does_not_conflict_with_read_of_specific_identifier():
    a = _claim("compute-slot", "*", AccessMode.READ.value)
    b = _claim("compute-slot", "gpu-7", AccessMode.READ.value)

    assert claims_conflict(a, b) is False


def test_plan_claims_returns_deterministically_ordered_conflicting_pairs():
    claim_sets = {
        "zeta": [_claim("gpu", "g1", AccessMode.WRITE.value)],
        "mid": [_claim("disk", "d1", AccessMode.EXCLUSIVE.value)],
        "beta": [_claim("disk", "d1", AccessMode.WRITE.value)],
        "alpha": [_claim("gpu", "g1", AccessMode.WRITE.value)],
    }

    result = plan_claims(claim_sets)

    assert result == [("alpha", "zeta"), ("beta", "mid")]

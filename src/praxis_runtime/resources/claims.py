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

import enum
from dataclasses import dataclass
from itertools import combinations
from pathlib import Path

from praxis_contracts.validator import validate_document

SCHEMA_PATH = (
    Path(__file__).resolve().parent.parent.parent.parent / "schemas" / "v1" / "resource-claim.schema.json"
)


class AccessMode(enum.Enum):
    READ = "read"
    WRITE = "write"
    EXCLUSIVE = "exclusive"


@dataclass(frozen=True)
class ResourceClaim:
    resource_type: str
    identifier: str
    access_mode: str
    quantity: float = 1
    scope: str | None = None


def parse_claims(document: dict) -> list[ResourceClaim]:
    validate_document(document, SCHEMA_PATH)

    claims = []
    for entry in document["claims"]:
        identifier = entry.get("identifier")
        access_mode = entry.get("access_mode")
        if identifier is None or access_mode is None:
            continue
        claims.append(
            ResourceClaim(
                resource_type=entry["resource_type"],
                identifier=identifier,
                access_mode=access_mode,
                quantity=entry.get("quantity", 1),
                scope=entry.get("scope"),
            )
        )
    return claims


def claims_conflict(a: ResourceClaim, b: ResourceClaim) -> bool:
    if a.resource_type != b.resource_type:
        return False
    if a.access_mode == AccessMode.READ.value and b.access_mode == AccessMode.READ.value:
        return False
    return a.identifier == b.identifier or a.identifier == "*" or b.identifier == "*"


def plan_claims(claim_sets: dict[str, list[ResourceClaim]]) -> list[tuple[str, str]]:
    conflicting_pairs = []
    for node_a, node_b in combinations(sorted(claim_sets), 2):
        has_conflict = any(
            claims_conflict(claim_a, claim_b)
            for claim_a in claim_sets[node_a]
            for claim_b in claim_sets[node_b]
        )
        if has_conflict:
            conflicting_pairs.append((node_a, node_b))
    return conflicting_pairs

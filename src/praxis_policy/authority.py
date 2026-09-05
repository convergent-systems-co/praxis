"""Authority-boundary evaluation for a node's declared authority requirement.

An `authority_requirement` (validated against
`schemas/v1/authority-requirement.schema.json`) reuses the ontology's
three-value `required`/`preferred`/`prohibited` constraint vocabulary — the
same vocabulary `schemas/v1/evidence-requirement.schema.json` uses for proof
constraints (see docs/ontology.md, "this same three-value constraint
vocabulary ... is reused").
"""

from __future__ import annotations

import enum
from dataclasses import dataclass
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    import praxis_policy.profiles


class AuthorityOutcome(enum.Enum):
    AUTO_APPROVED = "auto_approved"
    HUMAN_REQUIRED = "human_required"
    DENIED = "denied"


@dataclass(frozen=True)
class AuthorityDecision:
    outcome: AuthorityOutcome
    unresolved_scopes: frozenset[str]
    denied_scopes: frozenset[str]


def evaluate_authority(
    requirement: dict | None,
    profile: "praxis_policy.profiles.PolicyProfile",
    *,
    granted_scopes: frozenset[str] = frozenset(),
) -> AuthorityDecision:
    scopes = requirement.get("scopes") if requirement else None
    if not scopes:
        return AuthorityDecision(
            outcome=AuthorityOutcome.AUTO_APPROVED,
            unresolved_scopes=frozenset(),
            denied_scopes=frozenset(),
        )

    prohibited = {s["scope"] for s in scopes if s["constraint"] == "prohibited"}
    denied = prohibited & granted_scopes
    if denied:
        return AuthorityDecision(
            outcome=AuthorityOutcome.DENIED,
            unresolved_scopes=frozenset(),
            denied_scopes=frozenset(denied),
        )

    required = {s["scope"] for s in scopes if s["constraint"] == "required"}
    allowed = profile.auto_approved_authority_scopes | granted_scopes
    unresolved = required - allowed
    if unresolved:
        return AuthorityDecision(
            outcome=AuthorityOutcome.HUMAN_REQUIRED,
            unresolved_scopes=frozenset(unresolved),
            denied_scopes=frozenset(),
        )

    return AuthorityDecision(
        outcome=AuthorityOutcome.AUTO_APPROVED,
        unresolved_scopes=frozenset(),
        denied_scopes=frozenset(),
    )

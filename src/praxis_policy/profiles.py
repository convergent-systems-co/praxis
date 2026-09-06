"""Policy profiles: named strictness bundles and fail-closed minimum enforcement.

A profile controls which authority scopes are auto-approved and the default
retry/repair budgets a node gets. `resolve_profile` lets a user select a
profile at or above a node's declared minimum strictness (`policy_requirement`
at `node.metadata["policy_requirement"]`, schemas/v1/policy-requirement.schema.json)
but never below it, and every lookup fails closed (raises `PolicyProfileError`)
rather than silently falling back to a default.
"""

from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path

from praxis_contracts.validator import ContractValidationError, validate_document

_STRICTNESS_ORDER = ("fast", "standard", "strict", "regulated")  # index == strictness rank

_SCHEMA_PATH = Path(__file__).resolve().parent.parent.parent / "schemas" / "v1" / "policy-profile.schema.json"


class PolicyProfileError(Exception):
    """Raised whenever a profile lookup or document validation fails closed."""


@dataclass(frozen=True)
class PolicyProfile:
    name: str
    strictness: int
    auto_approved_authority_scopes: frozenset[str]
    allow_alternate_executor_retry: bool
    default_retry_budget: int
    default_repair_budget: int
    default_max_cost: float | None = None
    default_max_time_seconds: float | None = None


def _profile(
    name: str,
    strictness: int,
    *,
    allow_alternate_executor_retry: bool,
    default_retry_budget: int,
    default_repair_budget: int,
) -> PolicyProfile:
    return PolicyProfile(
        name=name,
        strictness=strictness,
        auto_approved_authority_scopes=frozenset(),
        allow_alternate_executor_retry=allow_alternate_executor_retry,
        default_retry_budget=default_retry_budget,
        default_repair_budget=default_repair_budget,
    )


BUILTIN_PROFILES: dict[str, PolicyProfile] = {
    "fast": _profile(
        "fast",
        0,
        allow_alternate_executor_retry=True,
        default_retry_budget=5,
        default_repair_budget=2,
    ),
    "standard": _profile(
        "standard",
        1,
        allow_alternate_executor_retry=True,
        default_retry_budget=3,
        default_repair_budget=1,
    ),
    "strict": _profile(
        "strict",
        2,
        allow_alternate_executor_retry=False,
        default_retry_budget=1,
        default_repair_budget=1,
    ),
    "regulated": _profile(
        "regulated",
        3,
        allow_alternate_executor_retry=False,
        default_retry_budget=0,
        default_repair_budget=0,
    ),
}


def profile_from_document(document: dict) -> PolicyProfile:
    try:
        validate_document(document, _SCHEMA_PATH)
    except ContractValidationError as exc:
        raise PolicyProfileError(f"invalid policy profile document: {exc.reason}") from exc

    return PolicyProfile(
        name=document["name"],
        strictness=document["strictness"],
        auto_approved_authority_scopes=frozenset(document["auto_approved_authority_scopes"]),
        allow_alternate_executor_retry=document.get("allow_alternate_executor_retry", False),
        default_retry_budget=document["default_retry_budget"],
        default_repair_budget=document["default_repair_budget"],
        default_max_cost=document.get("default_max_cost"),
        default_max_time_seconds=document.get("default_max_time_seconds"),
    )


def resolve_profile(
    selected_name: str,
    node_minimum_name: str | None = None,
    *,
    profiles: dict[str, PolicyProfile] | None = None,
) -> PolicyProfile:
    available = profiles if profiles is not None else BUILTIN_PROFILES

    if selected_name not in available:
        raise PolicyProfileError(f"unknown policy profile: {selected_name!r}")
    selected = available[selected_name]

    if node_minimum_name is not None:
        if node_minimum_name not in available:
            raise PolicyProfileError(f"unknown policy profile: {node_minimum_name!r}")
        minimum = available[node_minimum_name]
        if selected.strictness < minimum.strictness:
            raise PolicyProfileError(
                f"policy profile {selected_name!r} is weaker than the node's declared "
                f"minimum {node_minimum_name!r}"
            )

    return selected

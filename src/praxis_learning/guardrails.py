"""Fail-closed guardrails for the bounded-learning subsystem.

These checks keep learned heuristics from injecting authority/policy/security/
graph-legality configuration, or targeting those subsystems directly, and
force every learned-heuristic promotion policy to demand human review, per
docs/policy.md's zero-auto-approval default.
"""

from __future__ import annotations

from typing import TYPE_CHECKING

if TYPE_CHECKING:
    import praxis_eval.types

_FORBIDDEN_CONFIGURATION_KEYS = frozenset(
    {
        "authority_requirement",
        "authority",
        "policy_requirement",
        "policy_profile",
        "policy_floor",
        "security_invariant",
        "graph_legality",
        "transition",
        "transitions",
        "node_status",
        "event_type",
    }
)

_FORBIDDEN_TARGETS = frozenset(
    {
        "authority",
        "policy",
        "policy-floor",
        "security-invariant",
        "graph-legality",
        "runtime-transition",
    }
)

_REQUIRED_PROMOTION_AUTHORITY_SCOPE = "learned-heuristic-promotion"


class GuardrailViolation(Exception):
    pass


def _walk(node: object) -> None:
    if isinstance(node, dict):
        for key, value in node.items():
            if isinstance(key, str) and key.lower() in _FORBIDDEN_CONFIGURATION_KEYS:
                raise GuardrailViolation(
                    f"forbidden configuration key: {key!r}"
                )
            _walk(value)
    elif isinstance(node, (list, tuple)):
        for item in node:
            _walk(item)


def check_configuration(configuration: dict) -> None:
    if not isinstance(configuration, dict):
        raise GuardrailViolation("configuration must be a dict")
    _walk(configuration)


def check_target(target: str | None) -> None:
    if target is not None and target.strip().lower() in _FORBIDDEN_TARGETS:
        raise GuardrailViolation(f"forbidden target: {target!r}")


def require_authority_review(policy: "praxis_eval.types.PromotionPolicy") -> None:
    authority_requirement = policy.authority_requirement
    if not isinstance(authority_requirement, dict):
        raise GuardrailViolation(
            "promotion policy must declare an authority_requirement"
        )
    scopes = authority_requirement.get("scopes", [])
    if not isinstance(scopes, list):
        scopes = []
    for scope in scopes:
        if (
            isinstance(scope, dict)
            and scope.get("scope") == _REQUIRED_PROMOTION_AUTHORITY_SCOPE
            and scope.get("constraint") == "required"
        ):
            return
    raise GuardrailViolation(
        "promotion policy must require authority review "
        f"(scope={_REQUIRED_PROMOTION_AUTHORITY_SCOPE!r})"
    )

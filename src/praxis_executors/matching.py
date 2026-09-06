"""Capability matching: rank capability advertisements against a requirement.

Dict shapes follow schemas/v1/requirement.schema.json and
schemas/v1/capability-advertisement.schema.json.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Callable


@dataclass(frozen=True)
class MatchCandidate:
    executor_id: str
    capability_id: str | None
    satisfied_kinds: frozenset[str]


@dataclass(frozen=True)
class UnsatisfiedPromise:
    kind: str
    constraint: str  # "required" or "prohibited"
    reason: str


@dataclass(frozen=True)
class MatchResult:
    selected: MatchCandidate | None
    ranked: list[MatchCandidate]
    unsatisfied: list[UnsatisfiedPromise]


# Generic, vendor-neutral numeric hints read from a satisfied kind's
# `parameters`. Checked in this priority order; the first one present wins.
_COST_HINT_KEYS = ("cost", "risk", "latency")


def _satisfied_kinds(advertisement: dict) -> frozenset[str]:
    kinds: set[str] = set()
    for capability in advertisement["capabilities"]:
        for entry in capability["satisfies"]:
            kinds.add(entry["kind"])
    return frozenset(kinds)


def _capability_id(advertisement: dict, matched_kinds: frozenset[str]) -> str | None:
    for capability in advertisement["capabilities"]:
        if "id" not in capability:
            continue
        if any(entry["kind"] in matched_kinds for entry in capability["satisfies"]):
            return capability["id"]
    return None


def _cost_hint(advertisement: dict, matched_kinds: frozenset[str]) -> float:
    for key in _COST_HINT_KEYS:
        for capability in advertisement["capabilities"]:
            for entry in capability["satisfies"]:
                if entry["kind"] not in matched_kinds:
                    continue
                parameters = entry.get("parameters")
                if parameters and key in parameters:
                    return parameters[key]
    return 0


def _ordered_unique_kinds(requirement: dict, constraint: str) -> list[str]:
    kinds: list[str] = []
    for item in requirement["requirements"]:
        if item["constraint"] == constraint:
            kind = item["promise"]["kind"]
            if kind not in kinds:
                kinds.append(kind)
    return kinds


def match(
    requirement: dict,
    advertisements: list[dict],
    *,
    is_eligible: Callable[[str], bool] | None = None,
) -> MatchResult:
    required_kinds = _ordered_unique_kinds(requirement, "required")
    preferred_kinds = set(_ordered_unique_kinds(requirement, "preferred"))
    prohibited_kinds = _ordered_unique_kinds(requirement, "prohibited")
    prohibited_kinds_set = set(prohibited_kinds)

    eligible = [
        advertisement
        for advertisement in advertisements
        if is_eligible is None or is_eligible(advertisement["executor_id"])
    ]

    satisfied_by_executor = {
        advertisement["executor_id"]: _satisfied_kinds(advertisement)
        for advertisement in eligible
    }
    union_satisfied: set[str] = set()
    for kinds in satisfied_by_executor.values():
        union_satisfied |= kinds

    # Satisfied-kind sets for advertisements that don't trip a prohibited
    # kind. Used to tell "this required kind is only ever offered by an
    # advertisement that's disqualified for prohibited reasons" (already
    # explained by the prohibited entry below) apart from "this required
    # kind is offered, but never by the same advertisement that covers the
    # other required kinds" (a gap that needs its own explanation).
    prohibited_clean_satisfied = [
        kinds
        for kinds in satisfied_by_executor.values()
        if not (kinds & prohibited_kinds_set)
    ]

    scored: list[tuple[tuple, MatchCandidate]] = []
    for advertisement in eligible:
        satisfied = satisfied_by_executor[advertisement["executor_id"]]
        if satisfied & prohibited_kinds_set:
            continue
        if not set(required_kinds) <= satisfied:
            continue
        relevant_kinds = set(required_kinds) | preferred_kinds
        matched_kinds = frozenset(satisfied & relevant_kinds) if relevant_kinds else satisfied
        candidate = MatchCandidate(
            executor_id=advertisement["executor_id"],
            capability_id=_capability_id(advertisement, matched_kinds),
            satisfied_kinds=satisfied,
        )
        preferred_score = len(satisfied & preferred_kinds)
        cost = _cost_hint(advertisement, matched_kinds)
        scored.append(((-preferred_score, cost, advertisement["executor_id"]), candidate))

    scored.sort(key=lambda pair: pair[0])
    ranked = [candidate for _, candidate in scored]

    if ranked:
        return MatchResult(selected=ranked[0], ranked=ranked, unsatisfied=[])

    unsatisfied: list[UnsatisfiedPromise] = []
    for kind in required_kinds:
        if kind not in union_satisfied:
            unsatisfied.append(
                UnsatisfiedPromise(
                    kind=kind,
                    constraint="required",
                    reason=f"no eligible advertisement satisfies '{kind}'",
                )
            )
        elif any(kind in kinds for kinds in prohibited_clean_satisfied):
            unsatisfied.append(
                UnsatisfiedPromise(
                    kind=kind,
                    constraint="required",
                    reason=(
                        f"no single eligible advertisement satisfies '{kind}' "
                        "together with every other required kind"
                    ),
                )
            )
        else:
            unsatisfied.append(
                UnsatisfiedPromise(
                    kind=kind,
                    constraint="required",
                    reason=(
                        f"every eligible advertisement satisfying '{kind}' "
                        "also satisfies a prohibited kind"
                    ),
                )
            )
    for kind in prohibited_kinds:
        if kind in union_satisfied:
            unsatisfied.append(
                UnsatisfiedPromise(
                    kind=kind,
                    constraint="prohibited",
                    reason=f"an eligible advertisement satisfies prohibited kind '{kind}'",
                )
            )
    return MatchResult(selected=None, ranked=[], unsatisfied=unsatisfied)

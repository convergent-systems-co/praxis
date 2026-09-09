"""`praxis executors match --capability ... --explain` implementation.

Every `--capability` flag maps to a `required` promise only; `preferred` and
`prohibited` constraints are out of scope here (spec criterion 7).
"""

from __future__ import annotations

from typing import Iterable

from praxis_cli.fields import capability_kinds
from praxis_executors import matching, policy
from praxis_executors.interface import Executor, ExecutorError

_SPEC_VERSION = "1.0.0"


def build_requirement(capabilities: list[str]) -> dict:
    return {
        "spec_version": _SPEC_VERSION,
        "requirements": [
            {
                "promise": {"spec_version": _SPEC_VERSION, "kind": kind},
                "constraint": "required",
            }
            for kind in capabilities
        ],
    }


def _format_reason(entry: matching.UnsatisfiedPromise) -> str:
    reason = entry.reason
    if entry.policy_excluded:
        reason += " (policy_excluded)"
    return reason


def _print_unsatisfied(unsatisfied: list[matching.UnsatisfiedPromise]) -> None:
    for entry in unsatisfied:
        print(_format_reason(entry))


def _required_kinds(capabilities: list[str]) -> list[str]:
    """The requested kinds, deduplicated in first-seen order.

    Mirrors how `matching.match` collapses the `required` entries
    `build_requirement` produces, so an explanation never names the same
    missing kind twice.
    """
    kinds: list[str] = []
    for kind in capabilities:
        if kind not in kinds:
            kinds.append(kind)
    return kinds


def _candidate_reason(advertisement: dict, required_kinds: list[str], *, eligible: bool) -> str:
    """Why this one candidate was not ranked, stated about the candidate.

    `matching.match`'s `unsatisfied` reasons describe the requirement across
    the whole advertisement set, so echoing one next to a per-candidate
    `eligible=yes` reads as a contradiction.
    """
    if not eligible:
        return "excluded by auth-transport policy (policy_excluded)"
    satisfied = set(capability_kinds(advertisement))
    missing = [kind for kind in required_kinds if kind not in satisfied]
    if missing:
        return f"does not satisfy required kind(s): {', '.join(missing)}"
    return "not ranked for this requirement"


def run_match(adapters: Iterable[Executor], *, capabilities: list[str], explain: bool) -> int:
    advertisements: list[dict] = []
    for executor in adapters:
        try:
            advertisements.append(executor.capabilities())
        except ExecutorError:
            continue

    requirement = build_requirement(capabilities)
    is_eligible = policy.as_eligibility_callable(policy.AuthTransportPolicy(), advertisements)
    full_result = matching.match(requirement, advertisements, is_eligible=is_eligible)

    if full_result.selected is not None:
        print(full_result.selected.executor_id)
    else:
        print("no executor selected")
        _print_unsatisfied(full_result.unsatisfied)

    if explain:
        rank_by_id = {
            candidate.executor_id: rank
            for rank, candidate in enumerate(full_result.ranked, start=1)
        }
        required_kinds = _required_kinds(capabilities)
        for advertisement in advertisements:
            executor_id = advertisement["executor_id"]
            if executor_id in rank_by_id:
                print(f"{executor_id}: eligible=yes score={rank_by_id[executor_id]}")
                continue
            eligible = is_eligible(executor_id)
            reason = _candidate_reason(advertisement, required_kinds, eligible=eligible)
            print(f"{executor_id}: eligible={'yes' if eligible else 'no'} reason={reason}")

    return 0

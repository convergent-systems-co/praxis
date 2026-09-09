"""`praxis executors match --capability ... --explain` implementation.

Every `--capability` flag maps to a `required` promise only; `preferred` and
`prohibited` constraints are out of scope here (spec criterion 7).
"""

from __future__ import annotations

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


def _print_unsatisfied(unsatisfied: list[matching.UnsatisfiedPromise]) -> None:
    for entry in unsatisfied:
        line = entry.reason
        if entry.policy_excluded:
            line += " (policy_excluded)"
        print(line)


def run_match(adapters: list[Executor], *, capabilities: list[str], explain: bool) -> int:
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
        for advertisement in advertisements:
            executor_id = advertisement["executor_id"]
            if executor_id in rank_by_id:
                print(f"{executor_id}: eligible=yes score={rank_by_id[executor_id]}")
                continue
            single_result = matching.match(requirement, [advertisement], is_eligible=is_eligible)
            eligible = "yes" if is_eligible(executor_id) else "no"
            reasons = []
            for entry in single_result.unsatisfied:
                reason = entry.reason
                if entry.policy_excluded:
                    reason += " (policy_excluded)"
                reasons.append(reason)
            print(f"{executor_id}: eligible={eligible} reason={'; '.join(reasons)}")

    return 0

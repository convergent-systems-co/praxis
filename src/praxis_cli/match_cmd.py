"""`praxis executors match --capability ... --explain` implementation.

Every `--capability` flag maps to a `required` promise only; `preferred` and
`prohibited` constraints are out of scope here (spec criterion 7).
"""

from __future__ import annotations

from typing import Callable, Mapping

from praxis_executors import matching, policy
from praxis_executors.interface import Executor, ExecutorError

_SPEC_VERSION = "1.0.0"

# Spelled once: both the aggregate no-selection report and the per-candidate
# `--explain` line mark a policy exclusion with the same token.
_POLICY_EXCLUDED_SUFFIX = " (policy_excluded)"


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
        reason += _POLICY_EXCLUDED_SUFFIX
    return reason


def _print_unsatisfied(unsatisfied: list[matching.UnsatisfiedPromise]) -> None:
    for entry in unsatisfied:
        print(_format_reason(entry))


def _candidate_reason(
    requirement: dict,
    advertisement: dict,
    is_eligible: Callable[[str], bool],
) -> str:
    """Why this one candidate was not ranked, stated about the candidate.

    Re-runs the real matcher over this advertisement alone rather than
    restating its eligibility or kind-coverage rules here, so a candidate is
    only called policy-excluded when `matching.match` says so. Only the
    wording is candidate-scoped: `match`'s own reasons speak about the whole
    advertisement set, which reads as a contradiction beside a single row.
    """
    if not advertisement["capabilities"]:
        # `AuthTransportPolicy` rejects an empty advertisement outright, so
        # there is no transport to have excluded and no kind it fell short of.
        # Naming the required kind here would leave `eligible=no` unexplained.
        return "advertises no capabilities"
    result = matching.match(requirement, [advertisement], is_eligible=is_eligible)
    if result.unsatisfied:
        kinds = ", ".join(entry.kind for entry in result.unsatisfied)
        if any(entry.policy_excluded for entry in result.unsatisfied):
            return (
                f"policy excludes this candidate for required kind(s): "
                f"{kinds}{_POLICY_EXCLUDED_SUFFIX}"
            )
        return f"does not satisfy required kind(s): {kinds}"
    # Nothing unsatisfied and still unranked means there was no required kind
    # to report against (no `--capability` was given) -- an eligible candidate
    # would have been ranked, so the policy verdict is all that is left.
    return f"excluded by policy{_POLICY_EXCLUDED_SUFFIX}"


def run_match(
    adapters: Mapping[str, Executor], *, capabilities: list[str], explain: bool
) -> int:
    # Kept paired: the mapping key is the id `discover` and `status` print, so
    # it is the one this command prints too, while `matching.match` and the
    # policy only ever know a candidate by the advertisement's own
    # `executor_id`. Every lookup below goes through the latter, every printed
    # name through the former.
    gathered: list[tuple[str, dict]] = []
    for name, executor in adapters.items():
        try:
            gathered.append((name, executor.capabilities()))
        except ExecutorError:
            continue

    advertisements = [advertisement for _, advertisement in gathered]
    # Reversed so the first adapter wins if two advertise the same id.
    name_by_advertised_id = {
        advertisement["executor_id"]: name for name, advertisement in reversed(gathered)
    }

    requirement = build_requirement(capabilities)
    is_eligible = policy.as_eligibility_callable(policy.AuthTransportPolicy(), advertisements)
    full_result = matching.match(requirement, advertisements, is_eligible=is_eligible)

    if full_result.selected is not None:
        selected_id = full_result.selected.executor_id
        print(name_by_advertised_id.get(selected_id, selected_id))
    else:
        print("no executor selected")
        _print_unsatisfied(full_result.unsatisfied)

    if explain:
        rank_by_id = {
            candidate.executor_id: rank
            for rank, candidate in enumerate(full_result.ranked, start=1)
        }
        for name, advertisement in gathered:
            executor_id = advertisement["executor_id"]
            if executor_id in rank_by_id:
                print(f"{name}: eligible=yes score={rank_by_id[executor_id]}")
                continue
            reason = _candidate_reason(requirement, advertisement, is_eligible)
            eligible = "yes" if is_eligible(executor_id) else "no"
            print(f"{name}: eligible={eligible} reason={reason}")

    return 0

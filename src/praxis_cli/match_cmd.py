"""`praxis executors match --capability ... --explain` implementation.

Every `--capability` flag maps to a `required` promise only; `preferred` and
`prohibited` constraints are out of scope here (spec criterion 7).
"""

from __future__ import annotations

from typing import Callable, Mapping

from praxis_cli import fields
from praxis_executors import matching, policy
from praxis_executors.interface import Executor

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


def _candidate_verdict(
    requirement: dict,
    advertisement: dict,
    is_eligible: Callable[[str], bool],
) -> tuple[bool, str]:
    """This one candidate's eligibility, and why it was not ranked.

    Re-runs the real matcher over this advertisement alone rather than
    restating its eligibility or kind-coverage rules here, so a candidate is
    only called policy-excluded when `matching.match` says so. Only the
    wording is candidate-scoped: `match`'s own reasons speak about the whole
    advertisement set, which reads as a contradiction beside a single row.
    """
    eligible = is_eligible(advertisement["executor_id"])
    if not advertisement["capabilities"]:
        # `AuthTransportPolicy` rejects an empty advertisement outright, so
        # there is no transport to have excluded and no kind it fell short of.
        # Naming the required kind here would leave `eligible=no` unexplained.
        return eligible, "advertises no capabilities"

    result = matching.match(requirement, [advertisement], is_eligible=is_eligible)
    # When nothing ranks, `match` reports every required kind, covered ones
    # included -- it is explaining why the set as a whole produced no
    # selection, and a kind this candidate covers still went unsatisfied once
    # the other required kinds went missing. Only the kinds this
    # advertisement genuinely does not carry belong in a candidate-scoped
    # line, so the covered ones are dropped here.
    advertised_kinds = set(fields.capability_kinds(advertisement))
    excluded_kinds = [entry.kind for entry in result.unsatisfied if entry.policy_excluded]
    unmet_kinds = [
        entry.kind
        for entry in result.unsatisfied
        if not entry.policy_excluded and entry.kind not in advertised_kinds
    ]

    if eligible:
        # An eligible candidate is unranked only for kinds it does not cover:
        # `match` marks nothing `policy_excluded` when the single advertisement
        # it ran over was itself eligible, and a candidate with no required kind
        # left to miss would have been ranked.
        return eligible, f"does not satisfy required kind(s): {', '.join(unmet_kinds)}"

    # The policy verdict is what makes `eligible=no` true, so it is always
    # stated. `match` can only mark a kind `policy_excluded` when this
    # candidate advertised it, so a required kind the candidate never
    # advertised comes back unmarked -- the two are reported side by side
    # rather than one silently standing in for the other.
    if excluded_kinds:
        reason = (
            f"policy excludes this candidate for required kind(s): {', '.join(excluded_kinds)}"
        )
    else:
        reason = "excluded by policy"
    if unmet_kinds:
        reason += f"; does not satisfy required kind(s): {', '.join(unmet_kinds)}"
    return eligible, reason + _POLICY_EXCLUDED_SUFFIX


def run_match(
    adapters: Mapping[str, Executor], *, capabilities: list[str], explain: bool
) -> int:
    # Kept paired: the mapping key is the id `discover` and `status` print, so
    # it is the one this command prints too, while `matching.match` and the
    # policy only ever know a candidate by the advertisement's own
    # `executor_id`. Every lookup below goes through the latter, every printed
    # name through the former.
    gathered: list[tuple[str, dict]] = []
    # An adapter that could not be asked is not a candidate `match` can rank,
    # but dropping it silently leaves a user unable to tell it was considered
    # at all -- `--explain` reports it below, the way `discover` and `status`
    # both report the same failure. Caught on the same `fields.PROBE_FAILED`
    # those two degrade a row on, so an adapter that costs `status` one line
    # cannot cost `match` the whole command.
    unreadable: dict[str, str] = {}
    for name, executor in adapters.items():
        try:
            gathered.append((name, executor.capabilities()))
        except fields.PROBE_FAILED as exc:
            unreadable[name] = str(exc)

    advertisements = [advertisement for _, advertisement in gathered]
    # Last adapter wins if two advertise the same id, because that is how both
    # `as_eligibility_callable` and `match` resolve the duplicate: each builds
    # a dict keyed by advertised id over the same list, so the advertisement
    # actually judged and ranked is the last one. Naming the first adapter here
    # would print a name whose advertisement was never the one considered.
    name_by_advertised_id = {
        advertisement["executor_id"]: name for name, advertisement in gathered
    }

    requirement = build_requirement(capabilities)
    is_eligible = policy.as_eligibility_callable(policy.AuthTransportPolicy(), advertisements)
    full_result = matching.match(requirement, advertisements, is_eligible=is_eligible)

    if full_result.selected is not None:
        selected_id = full_result.selected.executor_id
        # Labelled, because the no-selection outcome below says what it is in
        # words: a bare id on stdout reads as a selection only to someone who
        # already knows what this command prints.
        print(f"selected: {name_by_advertised_id.get(selected_id, selected_id)}")
    else:
        print("no executor selected")
        _print_unsatisfied(full_result.unsatisfied)

    if explain:
        rank_by_id = {
            candidate.executor_id: rank
            for rank, candidate in enumerate(full_result.ranked, start=1)
        }
        advertisement_by_name = dict(gathered)
        # Walked in the mapping's own order so every adapter the CLI was given
        # gets a line, in the order `discover` and `status` list them.
        for name in adapters:
            if name in unreadable:
                # Not `eligible=no`: the policy never got an advertisement to
                # judge, so this candidate's eligibility is undetermined rather
                # than decided against.
                print(
                    f"{name}: eligible=unknown "
                    f"reason=advertisement unavailable ({unreadable[name]})"
                )
                continue
            advertisement = advertisement_by_name[name]
            executor_id = advertisement["executor_id"]
            if executor_id in rank_by_id:
                print(f"{name}: eligible=yes score={rank_by_id[executor_id]}")
                continue
            eligible, reason = _candidate_verdict(requirement, advertisement, is_eligible)
            print(f"{name}: eligible={'yes' if eligible else 'no'} reason={reason}")

    return 0

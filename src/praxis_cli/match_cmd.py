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


def _validated_kinds(advertisement: dict) -> list[str]:
    """The kinds this advertisement satisfies, once every key read below exists.

    `capability_kinds` reads the ones `capability-advertisement.schema.json`
    requires; `executor_id` is read here because `name_by_advertised_id` and
    `matching.match` both subscript it unguarded. Both are read under the
    caller's `MalformedAdvertisement` guard, so an advertisement that answers
    without answering conformingly costs its own candidate -- as it costs
    `discover` and `status` a row -- instead of reaching them as a raw
    `KeyError`. Every failure this raises is that one class, and nothing
    wider, so a defect in the derivation code itself still surfaces. Kinds
    first, because a body that is not a mapping at all fails there, with
    `MalformedAdvertisement` to say so.

    The kinds come back rather than being derived a second time per candidate:
    the list `--explain` reports a candidate's shortfall against has to be the
    one the candidate was admitted on.
    """
    kinds = fields.capability_kinds(advertisement)
    if "executor_id" not in advertisement:
        raise fields.malformed_advertisement("executor_id")
    return kinds


def _candidate_verdict(
    requirement: dict,
    advertisement: dict,
    advertised_kinds: list[str],
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
    covered = set(advertised_kinds)
    excluded_kinds = [entry.kind for entry in result.unsatisfied if entry.policy_excluded]
    unmet_kinds = [
        entry.kind
        for entry in result.unsatisfied
        if not entry.policy_excluded and entry.kind not in covered
    ]

    if eligible:
        if not unmet_kinds:
            # Not reachable from `run_match`: an eligible candidate that misses
            # no required kind ranks on its own, and the one case where the
            # full run drops such a candidate -- a duplicate advertised id,
            # which `match` and the policy both resolve last-wins -- is
            # answered before this function is asked. A sentence rather than a
            # kind list with nothing in it, for a caller that reaches it anyway.
            return eligible, "unranked, and a re-run over this candidate alone gives no reason why"
        # Otherwise an eligible candidate is unranked only for kinds it does not
        # cover: `match` marks nothing `policy_excluded` when the single
        # advertisement it ran over was itself eligible.
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
    # Derived once, in the gather loop, and read again by `_candidate_verdict`
    # below: the kinds a candidate is judged to have fallen short of are the
    # ones it was admitted on.
    kinds_by_name: dict[str, list[str]] = {}
    # An adapter that could not be asked is not a candidate `match` can rank,
    # but dropping it silently leaves a user unable to tell it was considered
    # at all -- `--explain` reports it below, the way `discover` and `status`
    # both report the same failure. Caught on the same `fields.PROBE_FAILED`
    # those two degrade a row on, so an adapter that costs `status` one line
    # cannot cost `match` the whole command.
    unreadable: dict[str, str] = {}
    for name, executor in adapters.items():
        # Two guards, not one, exactly as `fields.advertisement_cells` splits
        # them for the other two commands: `PROBE_FAILED` covers the call, which
        # is an adapter that could not be asked, and `MalformedAdvertisement`
        # covers this command's own reading of what came back, and nothing
        # wider. One guard over both blamed the adapter for a defect in that
        # reading -- and cost `match` more than it costs a row-based command,
        # since the candidate simply vanished and the run reported no selection.
        try:
            advertisement = executor.capabilities()
        except fields.PROBE_FAILED as exc:
            fields.note_probe_failure(executor, fields.CAPABILITIES_PROBE, exc)
            unreadable[name] = str(exc)
            continue
        try:
            kinds = _validated_kinds(advertisement)
        except fields.MalformedAdvertisement as exc:
            # Recorded under the response's name, not the call's: the call
            # returned, and a record naming it sends a reader looking for a
            # raise inside a method that never raised.
            fields.note_probe_failure(executor, fields.CAPABILITIES_RESPONSE, exc)
            unreadable[name] = str(exc)
            continue
        gathered.append((name, advertisement))
        kinds_by_name[name] = kinds

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
            if name_by_advertised_id[executor_id] != name:
                # A duplicate advertised id is resolved last-wins by `match` and
                # by the policy alike, both keying a candidate by that id, so
                # this adapter's own advertisement -- and its own auth transport
                # -- was read by neither. `eligible=unknown` for the same reason
                # an unreadable advertisement gets it: the only verdict on file
                # answers for the other adapter's advertisement, and reporting
                # it here would credit this one with an eligibility the policy
                # never judged, or with the other's rank.
                print(
                    f"{name}: eligible=unknown reason=another adapter advertises "
                    f"the same executor id ({executor_id}); that advertisement "
                    f"was the one judged"
                )
                continue
            if executor_id in rank_by_id:
                print(f"{name}: eligible=yes score={rank_by_id[executor_id]}")
                continue
            eligible, reason = _candidate_verdict(
                requirement, advertisement, kinds_by_name[name], is_eligible
            )
            print(f"{name}: eligible={'yes' if eligible else 'no'} reason={reason}")

    # Known limitation, recorded in docs/develop/plans/b2-issue45.md: 0 for a
    # run that selected nothing too, so a caller has to read stdout to tell the
    # two apart. The plan specifies this exit code and the spec names none, so
    # changing it is its own decision rather than this command's to make.
    return 0

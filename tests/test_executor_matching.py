"""Tests for the capability matching algorithm in praxis_executors.matching.

Dict fixtures follow schemas/v1/requirement.schema.json and
schemas/v1/capability-advertisement.schema.json.

This suite is the permanent regression coverage for the matching algorithm,
including the case where a required kind's only qualifying advertisement is
disqualified because it also satisfies a prohibited kind (see
`test_prohibited_kind_disqualifies_only_candidate_and_explains_required_gap`
below).
"""

from __future__ import annotations

from praxis_executors.matching import match

_SPEC_VERSION = "1.0.0"


def _requirement(*, required=(), preferred=(), prohibited=()) -> dict:
    entries = []
    for kind in required:
        entries.append(
            {"promise": {"spec_version": _SPEC_VERSION, "kind": kind}, "constraint": "required"}
        )
    for kind in preferred:
        entries.append(
            {"promise": {"spec_version": _SPEC_VERSION, "kind": kind}, "constraint": "preferred"}
        )
    for kind in prohibited:
        entries.append(
            {"promise": {"spec_version": _SPEC_VERSION, "kind": kind}, "constraint": "prohibited"}
        )
    return {"spec_version": _SPEC_VERSION, "requirements": entries}


def _advertisement(
    executor_id: str, kinds, *, cost: float | None = None, capability_id: str | None = None
) -> dict:
    satisfies = []
    for kind in kinds:
        entry = {"kind": kind}
        if cost is not None:
            entry["parameters"] = {"cost": cost}
        satisfies.append(entry)
    capability: dict = {"spec_version": _SPEC_VERSION, "satisfies": satisfies}
    if capability_id is not None:
        capability["id"] = capability_id
    return {
        "spec_version": _SPEC_VERSION,
        "executor_id": executor_id,
        "capabilities": [capability],
    }


def _multi_capability_advertisement(executor_id: str, capabilities: list[dict]) -> dict:
    """Build an advertisement with more than one Capability entry.

    Each item in `capabilities` is `{"id": ..., "kinds": [...], "cost": ...}`
    (`id`/`cost` optional) -- unlike `_advertisement()`, which only ever
    builds a single-capability advertisement.
    """
    built = []
    for spec in capabilities:
        satisfies = []
        for kind in spec["kinds"]:
            entry = {"kind": kind}
            if "cost" in spec:
                entry["parameters"] = {"cost": spec["cost"]}
            satisfies.append(entry)
        capability: dict = {"spec_version": _SPEC_VERSION, "satisfies": satisfies}
        if "id" in spec:
            capability["id"] = spec["id"]
        built.append(capability)
    return {
        "spec_version": _SPEC_VERSION,
        "executor_id": executor_id,
        "capabilities": built,
    }


def test_capability_id_is_scoped_to_the_capability_satisfying_the_matched_kind():
    requirement = _requirement(required=["kind-a"])
    advertisement = _multi_capability_advertisement(
        "executor-1",
        [
            {"id": "cap-unrelated", "kinds": ["kind-x"]},
            {"id": "cap-match", "kinds": ["kind-a"]},
        ],
    )

    result = match(requirement, [advertisement])

    assert result.selected is not None
    assert result.selected.capability_id == "cap-match"


def test_cost_hint_is_scoped_to_the_capability_satisfying_the_matched_kind():
    requirement = _requirement(required=["kind-a"])
    # executor-1's kind-x capability has a low cost (1), but kind-x is not the
    # matched kind -- only its kind-a capability's cost (9) may be used. If
    # the cost hint were not scoped to the matched capability, the unrelated
    # cost=1 would make executor-1 look cheaper than executor-2 (cost=5) and
    # win the tie-break incorrectly.
    misleadingly_cheap = _multi_capability_advertisement(
        "executor-1",
        [
            {"id": "cap-unrelated", "kinds": ["kind-x"], "cost": 1},
            {"id": "cap-match", "kinds": ["kind-a"], "cost": 9},
        ],
    )
    actually_cheaper = _advertisement("executor-2", ["kind-a"], cost=5)

    result = match(requirement, [misleadingly_cheap, actually_cheaper])

    assert result.selected is not None
    assert result.selected.executor_id == "executor-2"


def test_required_kind_satisfied_by_exactly_one_advertisement_selects_it():
    requirement = _requirement(required=["kind-a"])
    ads = [
        _advertisement("executor-1", ["kind-a"]),
        _advertisement("executor-2", ["kind-b"]),
    ]

    result = match(requirement, ads)

    assert result.selected is not None
    assert result.selected.executor_id == "executor-1"


def test_required_kind_satisfied_by_no_advertisement_yields_no_selection_and_unsatisfied_entry():
    requirement = _requirement(required=["kind-a"])
    ads = [_advertisement("executor-1", ["kind-b"])]

    result = match(requirement, ads)

    assert result.selected is None
    assert any(
        u.kind == "kind-a" and u.constraint == "required" for u in result.unsatisfied
    )


def test_prohibited_kind_disqualifies_only_candidate_and_explains_required_gap():
    requirement = _requirement(required=["kind-a"], prohibited=["kind-x"])
    ads = [_advertisement("executor-1", ["kind-a", "kind-x"])]

    result = match(requirement, ads)

    assert result.selected is None
    assert any(
        u.kind == "kind-x" and u.constraint == "prohibited" for u in result.unsatisfied
    )


def test_preferred_kind_does_not_gate_but_ranks_above_a_candidate_lacking_it():
    requirement = _requirement(required=["kind-a"], preferred=["kind-p"])

    only_without_preferred = match(
        requirement, [_advertisement("executor-1", ["kind-a"])]
    )
    assert only_without_preferred.selected is not None
    assert only_without_preferred.selected.executor_id == "executor-1"

    both_candidates = match(
        requirement,
        [
            _advertisement("executor-1", ["kind-a"]),
            _advertisement("executor-2", ["kind-a", "kind-p"]),
        ],
    )
    assert both_candidates.selected is not None
    assert both_candidates.selected.executor_id == "executor-2"


def test_deterministic_selection_among_equivalent_candidates_picks_lexicographically_lower_id():
    requirement = _requirement(required=["kind-a"])
    ad_a = _advertisement("executor-a", ["kind-a"])
    ad_b = _advertisement("executor-b", ["kind-a"])

    first = match(requirement, [ad_b, ad_a])
    second = match(requirement, [ad_a, ad_b])

    assert first.selected is not None and second.selected is not None
    assert first.selected.executor_id == "executor-a"
    assert second.selected.executor_id == "executor-a"


def test_deterministic_rejection_produces_equal_unsatisfied_lists_across_calls():
    requirement = _requirement(required=["kind-a"])
    ads = [_advertisement("executor-1", ["kind-b"])]

    first = match(requirement, ads)
    second = match(requirement, ads)

    assert first.selected is None
    assert second.selected is None
    assert first.unsatisfied == second.unsatisfied


def test_is_eligible_excluding_only_candidate_matches_explanation_of_nonexistence():
    requirement = _requirement(required=["kind-a"])
    ad = _advertisement("executor-1", ["kind-a"])

    absent = match(requirement, [])
    excluded = match(requirement, [ad], is_eligible=lambda executor_id: False)

    assert absent.selected is excluded.selected is None
    assert absent.unsatisfied == excluded.unsatisfied


def test_selected_candidate_carries_the_matched_capability_id():
    requirement = _requirement(required=["kind-a"])
    ads = [_advertisement("executor-1", ["kind-a"], capability_id="cap-executor-1")]

    result = match(requirement, ads)

    assert result.selected is not None
    assert result.selected.capability_id == "cap-executor-1"


def test_cost_parameter_tie_breaks_equally_preferred_candidates_to_lower_cost():
    requirement = _requirement(required=["kind-a"])
    low_cost = _advertisement("executor-z", ["kind-a"], cost=1)
    high_cost = _advertisement("executor-a", ["kind-a"], cost=5)

    result = match(requirement, [low_cost, high_cost])

    assert result.selected is not None
    assert result.selected.executor_id == "executor-z"


def test_two_required_kinds_split_across_non_overlapping_advertisements_explains_the_gap():
    # Each required kind is satisfied by some advertisement, but no single
    # advertisement satisfies both together -- this must not be confused
    # with "no advertisement satisfies this kind at all" or "disqualified by
    # a prohibited kind"; it needs its own "together with every other
    # required kind" explanation for each required kind.
    requirement = _requirement(required=["kind-a", "kind-b"])
    ads = [
        _advertisement("executor-1", ["kind-a"]),
        _advertisement("executor-2", ["kind-b"]),
    ]

    result = match(requirement, ads)

    assert result.selected is None
    by_kind = {u.kind: u for u in result.unsatisfied}
    assert set(by_kind) == {"kind-a", "kind-b"}
    for kind in ("kind-a", "kind-b"):
        assert by_kind[kind].constraint == "required"
        assert "together with every other required kind" in by_kind[kind].reason


def test_required_kind_gap_is_distinguished_from_prohibited_disqualification():
    # kind-a is satisfied both by a clean advertisement (missing kind-b) and
    # by a prohibited-tainted one that would otherwise cover both required
    # kinds. The gap explanation for kind-a must come from the clean
    # advertisement's missing kind-b, not be swallowed by the prohibited
    # disqualification of the other advertisement.
    requirement = _requirement(required=["kind-a", "kind-b"], prohibited=["kind-x"])
    ads = [
        _advertisement("executor-tainted", ["kind-a", "kind-b", "kind-x"]),
        _advertisement("executor-clean", ["kind-a"]),
    ]

    result = match(requirement, ads)

    assert result.selected is None
    by_kind = {u.kind: u for u in result.unsatisfied if u.constraint == "required"}
    assert "together with every other required kind" in by_kind["kind-a"].reason
    assert "also satisfies a prohibited kind" in by_kind["kind-b"].reason


def test_selected_candidate_satisfied_kinds_includes_every_kind_the_advertisement_satisfies():
    # satisfied_kinds is the full set an advertisement satisfies, not just
    # the kinds relevant to the requirement -- distinct from capability_id's
    # narrower scoping to the matched capability.
    requirement = _requirement(required=["kind-a"])
    ads = [_advertisement("executor-1", ["kind-a", "kind-c"])]

    result = match(requirement, ads)

    assert result.selected is not None
    assert result.selected.satisfied_kinds == frozenset({"kind-a", "kind-c"})

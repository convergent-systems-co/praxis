"""Tests for the capability matching algorithm in praxis_executors.matching.

Dict fixtures follow schemas/v1/requirement.schema.json and
schemas/v1/capability-advertisement.schema.json.

This suite is the permanent regression coverage for logic already implemented
in T4 (see tasks/T4/tdd-writer.result.json for that task's own RED proof,
taken against a fresh module that did not yet exist). T4's adversarial-tester
round flagged that the "required kind's only advertisement is disqualified by
a prohibited kind" fix (`test_prohibited_kind_disqualifies_only_candidate_and_
explains_required_gap` below) had no permanent test yet; this file is where
that coverage lives going forward.
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


def _advertisement(executor_id: str, kinds, *, cost: float | None = None) -> dict:
    satisfies = []
    for kind in kinds:
        entry = {"kind": kind}
        if cost is not None:
            entry["parameters"] = {"cost": cost}
        satisfies.append(entry)
    return {
        "spec_version": _SPEC_VERSION,
        "executor_id": executor_id,
        "capabilities": [{"spec_version": _SPEC_VERSION, "satisfies": satisfies}],
    }


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


def test_cost_parameter_tie_breaks_equally_preferred_candidates_to_lower_cost():
    requirement = _requirement(required=["kind-a"])
    low_cost = _advertisement("executor-z", ["kind-a"], cost=1)
    high_cost = _advertisement("executor-a", ["kind-a"], cost=5)

    result = match(requirement, [low_cost, high_cost])

    assert result.selected is not None
    assert result.selected.executor_id == "executor-z"

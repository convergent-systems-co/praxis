"""Tests for `praxis executors match --capability ... --explain`
(`build_requirement` / `run_match`) in `praxis_cli.match_cmd`.

Advertisement fixtures follow schemas/v1/capability-advertisement.schema.json.
`auth_transport` values exercise the default `AuthTransportPolicy` that
`run_match` wires in the same way `registry.py` does (recognized/safe
"local" is eligible; "metered_api" is unsafe-by-default and produces
`policy_excluded=True` when it's the only advertisement of a required kind).

`--explain`'s per-candidate lines are asserted whole rather than by
substring: a reason is only useful if it says the right thing about the
right candidate, and `"yes" in line` passes on output that no longer does.
"""

from __future__ import annotations

from conftest import _FakeExecutor, _json_decode_error

from praxis_cli.match_cmd import _format_reason, build_requirement, run_match
from praxis_executors.interface import Executor, ExecutorError
from praxis_executors.matching import UnsatisfiedPromise

_SPEC_VERSION = "1.0.0"


def _capability(kind: str, auth_transport: str) -> dict:
    return {
        "spec_version": _SPEC_VERSION,
        "auth_transport": auth_transport,
        "satisfies": [{"kind": kind}],
    }


def _candidate(executor_id: str, kind: str, auth_transport: str) -> _FakeExecutor:
    return _FakeExecutor(executor_id, capabilities=[_capability(kind, auth_transport)])


def _malformed(executor_id: str = "executor-broken") -> _FakeExecutor:
    """A transport layer answering 200 with a body that is not JSON, so
    `.capabilities()` raises a `ValueError` rather than an `ExecutorError`."""
    return _FakeExecutor(executor_id, capabilities_error=_json_decode_error())


def _adapters() -> dict[str, Executor]:
    """The same `{executor_id: instance}` mapping shape `discover`/`status` take.

    * `executor-good` satisfies the requested kind over a policy-eligible
      transport -> selected.
    * `executor-excluded` satisfies it too, but only over an
      unsafe-by-default transport -> `policy_excluded`.
    * `executor-other` is policy-eligible but satisfies a different kind.
    """
    return {
        "executor-good": _candidate("executor-good", "kind-a", "local"),
        "executor-excluded": _candidate("executor-excluded", "kind-a", "metered_api"),
        "executor-other": _candidate("executor-other", "kind-b", "local"),
    }


# _format_reason() -- shared by the no-selection path (_print_unsatisfied)
# and the --explain per-candidate loop; a repair for the code-review finding
# that the two call sites duplicated this "reason + optional
# (policy_excluded) suffix" logic independently.


def test_format_reason_without_policy_exclusion():
    entry = UnsatisfiedPromise(kind="kind-a", constraint="required", reason="no candidate")

    assert _format_reason(entry) == "no candidate"


def test_format_reason_appends_policy_excluded_suffix():
    entry = UnsatisfiedPromise(
        kind="kind-a", constraint="required", reason="excluded", policy_excluded=True
    )

    assert _format_reason(entry) == "excluded (policy_excluded)"


def test_build_requirement_maps_each_capability_to_a_required_promise():
    requirement = build_requirement(["kind-a", "kind-b"])

    assert requirement == {
        "spec_version": "1.0.0",
        "requirements": [
            {
                "promise": {"spec_version": "1.0.0", "kind": "kind-a"},
                "constraint": "required",
            },
            {
                "promise": {"spec_version": "1.0.0", "kind": "kind-b"},
                "constraint": "required",
            },
        ],
    }


def test_non_explain_path_prints_only_the_selection(capsys):
    exit_code = run_match(_adapters(), capabilities=["kind-a"], explain=False)

    captured = capsys.readouterr()
    assert exit_code == 0
    assert captured.out.splitlines() == ["selected: executor-good"]


def test_the_selection_line_is_labelled_the_way_the_no_selection_line_is(capsys):
    # A bare executor id on stdout says what was selected only to a reader who
    # already knows what the command prints; the no-selection outcome has said
    # so in words all along. Both outcomes name themselves.
    run_match(_adapters(), capabilities=["kind-a"], explain=False)
    selected = capsys.readouterr().out.splitlines()[0]

    run_match(
        {"executor-other": _candidate("executor-other", "kind-b", "local")},
        capabilities=["kind-a"],
        explain=False,
    )
    none_selected = capsys.readouterr().out.splitlines()[0]

    assert selected == "selected: executor-good"
    assert "selected" in none_selected


def test_run_match_json_decode_error_from_capabilities_is_dropped_not_raised(capsys):
    adapters = {**_adapters(), "executor-broken": _malformed()}

    exit_code = run_match(adapters, capabilities=["kind-a"], explain=False)

    captured = capsys.readouterr()
    assert exit_code == 0
    assert captured.out.splitlines() == ["selected: executor-good"]


def test_explain_path_distinguishes_selected_policy_excluded_and_unmatched_candidates(capsys):
    exit_code = run_match(_adapters(), capabilities=["kind-a"], explain=True)

    captured = capsys.readouterr()
    assert exit_code == 0
    assert captured.out.splitlines() == [
        "selected: executor-good",
        "executor-good: eligible=yes score=1",
        "executor-excluded: eligible=no reason=policy excludes this candidate for "
        "required kind(s): kind-a (policy_excluded)",
        "executor-other: eligible=yes reason=does not satisfy required kind(s): kind-a",
    ]


def test_explain_names_the_policy_for_a_candidate_that_is_also_kind_short(capsys):
    # `executor-both-bad` is ineligible *and* advertises none of the required
    # kinds. Running `match` over it alone cannot mark the required kind
    # `policy_excluded`, because the candidate never advertised that kind --
    # so a kind-coverage reason on its own would leave `eligible=no`
    # unexplained. Both facts belong on the line.
    adapters = {
        "executor-good": _candidate("executor-good", "kind-a", "local"),
        "executor-both-bad": _candidate("executor-both-bad", "kind-b", "metered_api"),
    }

    run_match(adapters, capabilities=["kind-a"], explain=True)

    lines = {line.split(":", 1)[0]: line for line in capsys.readouterr().out.splitlines()}
    assert lines["executor-both-bad"] == (
        "executor-both-bad: eligible=no reason=excluded by policy; "
        "does not satisfy required kind(s): kind-a (policy_excluded)"
    )


def test_explain_reports_both_an_excluded_kind_and_a_missing_one(capsys):
    # Two required kinds, one of which this candidate advertises over an
    # unsafe transport and one it does not advertise at all: the two reasons
    # are stated side by side, neither swallowing the other.
    mixed = _candidate("executor-mixed", "kind-a", "metered_api")

    run_match({"executor-mixed": mixed}, capabilities=["kind-a", "kind-b"], explain=True)

    lines = {line.split(":", 1)[0]: line for line in capsys.readouterr().out.splitlines()}
    assert lines["executor-mixed"] == (
        "executor-mixed: eligible=no reason=policy excludes this candidate for "
        "required kind(s): kind-a; does not satisfy required kind(s): kind-b "
        "(policy_excluded)"
    )


def test_explain_names_only_the_required_kinds_an_eligible_candidate_actually_misses(capsys):
    # When nothing ranks, `matching.match` emits one `UnsatisfiedPromise` per
    # required kind -- including kinds this candidate does cover, separated
    # only by reason wording. Repeating the kind list verbatim therefore tells
    # each candidate it fails a kind it advertises.
    adapters = {
        "executor-coder": _FakeExecutor(
            "executor-coder",
            capabilities=[_capability("coding", "local"), _capability("reasoning", "local")],
        ),
        "executor-runner": _candidate("executor-runner", "code-execution", "local"),
    }

    run_match(adapters, capabilities=["coding", "code-execution"], explain=True)

    lines = {line.split(":", 1)[0]: line for line in capsys.readouterr().out.splitlines()}
    assert lines["executor-coder"] == (
        "executor-coder: eligible=yes reason=does not satisfy required kind(s): code-execution"
    )
    assert lines["executor-runner"] == (
        "executor-runner: eligible=yes reason=does not satisfy required kind(s): coding"
    )


def test_explain_reason_for_an_eligible_candidate_is_about_that_candidate(capsys):
    # `matching.match`'s own reasons speak about the whole advertisement set
    # ("no eligible advertisement satisfies ..."), which contradicts
    # `eligible=yes` on the same line.
    run_match(_adapters(), capabilities=["kind-a"], explain=True)

    lines = {line.split(":", 1)[0]: line for line in capsys.readouterr().out.splitlines()}
    assert "no eligible advertisement satisfies" not in lines["executor-other"]


def test_explain_does_not_blame_the_policy_for_an_advertisement_with_no_capabilities(capsys):
    # `AuthTransportPolicy.is_eligible` returns False for an empty
    # `capabilities` list, but there is no auth transport to have excluded --
    # the candidate simply advertises nothing. Naming the required kind would
    # blame kind coverage for a verdict the empty advertisement caused, so the
    # reason says what is actually true of this candidate.
    adapters = {"executor-good": _candidate("executor-good", "kind-a", "local"),
                "executor-empty": _FakeExecutor("executor-empty")}

    run_match(adapters, capabilities=["kind-a"], explain=True)

    lines = {line.split(":", 1)[0]: line for line in capsys.readouterr().out.splitlines()}
    assert lines["executor-empty"] == (
        "executor-empty: eligible=no reason=advertises no capabilities"
    )


def test_explain_accounts_for_an_adapter_whose_advertisement_could_not_be_read(capsys):
    # An adapter that could not be asked is not a candidate `match` can rank,
    # but dropping it silently leaves a user unable to tell it was considered.
    # `eligible=unknown`, not `eligible=no`: the policy never got an
    # advertisement to judge.
    adapters = {
        "executor-good": _candidate("executor-good", "kind-a", "local"),
        "executor-bad": _FakeExecutor(
            "executor-bad", capabilities_error=ExecutorError("service unreachable")
        ),
    }

    run_match(adapters, capabilities=["kind-a"], explain=True)

    lines = {line.split(":", 1)[0]: line for line in capsys.readouterr().out.splitlines()}
    assert lines["executor-bad"] == (
        "executor-bad: eligible=unknown reason=advertisement unavailable "
        "(service unreachable)"
    )


def test_explain_names_a_ranked_candidate_by_its_mapping_key(capsys):
    # `discover` and `status` name a row by the `build_adapters()` mapping key;
    # `match` must not print the same adapter under a second id.
    adapters = {"executor-registered": _candidate("something-else", "kind-a", "local")}

    run_match(adapters, capabilities=["kind-a"], explain=True)

    assert capsys.readouterr().out.splitlines() == [
        "selected: executor-registered",
        "executor-registered: eligible=yes score=1",
    ]


def test_explain_names_an_unranked_candidate_by_its_mapping_key(capsys):
    adapters = {"executor-registered": _candidate("something-else", "kind-a", "metered_api")}

    run_match(adapters, capabilities=["kind-a"], explain=True)

    assert capsys.readouterr().out.splitlines()[-1] == (
        "executor-registered: eligible=no reason=policy excludes this candidate for "
        "required kind(s): kind-a (policy_excluded)"
    )


def test_no_selection_prints_unsatisfied_reasons_unconditionally(capsys):
    adapters = {"executor-other": _candidate("executor-other", "kind-b", "local")}

    exit_code = run_match(adapters, capabilities=["kind-a"], explain=False)

    captured = capsys.readouterr()
    assert exit_code == 0
    assert captured.out.splitlines() == [
        "no executor selected",
        "no eligible advertisement satisfies 'kind-a'",
    ]


def test_empty_capabilities_ranks_every_eligible_candidate(capsys):
    exit_code = run_match(_adapters(), capabilities=[], explain=True)

    captured = capsys.readouterr()
    assert exit_code == 0
    # With no required kind, `match` has nothing to report unsatisfied, so the
    # only thing left to say about an unranked candidate is the policy verdict.
    assert captured.out.splitlines() == [
        "selected: executor-good",
        "executor-good: eligible=yes score=1",
        "executor-excluded: eligible=no reason=excluded by policy (policy_excluded)",
        "executor-other: eligible=yes score=2",
    ]

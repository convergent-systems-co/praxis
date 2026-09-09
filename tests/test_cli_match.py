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

import json

from praxis_cli.match_cmd import _format_reason, build_requirement, run_match
from praxis_executors.interface import Executor, ExecutorAvailability
from praxis_executors.matching import UnsatisfiedPromise

_SPEC_VERSION = "1.0.0"


class _FixedAdvertisementExecutor(Executor):
    """A minimal fake Executor whose `.capabilities()` returns a fixed dict."""

    def __init__(self, advertisement: dict) -> None:
        self._advertisement = advertisement

    def capabilities(self) -> dict:
        return self._advertisement

    def health(self) -> ExecutorAvailability:
        raise NotImplementedError

    def launch(self, request):
        raise NotImplementedError

    def status(self, handle):
        raise NotImplementedError

    def cancel(self, handle):
        raise NotImplementedError

    def result(self, handle):
        raise NotImplementedError


class _MalformedResponseExecutor(Executor):
    """A fake Executor whose `.capabilities()` raises `json.JSONDecodeError`
    (a `ValueError` subclass, not an `ExecutorError`) -- reproducing a
    non-Ollama server answering 200 with a non-JSON body."""

    def capabilities(self) -> dict:
        json.loads("not json")
        raise AssertionError("unreachable")

    def health(self) -> ExecutorAvailability:
        raise NotImplementedError

    def launch(self, request):
        raise NotImplementedError

    def status(self, handle):
        raise NotImplementedError

    def cancel(self, handle):
        raise NotImplementedError

    def result(self, handle):
        raise NotImplementedError


def _advertisement(executor_id: str, kind: str, auth_transport: str) -> dict:
    return {
        "spec_version": _SPEC_VERSION,
        "executor_id": executor_id,
        "capabilities": [
            {
                "spec_version": _SPEC_VERSION,
                "auth_transport": auth_transport,
                "satisfies": [{"kind": kind}],
            }
        ],
    }


def _candidate(executor_id: str, kind: str, auth_transport: str) -> _FixedAdvertisementExecutor:
    return _FixedAdvertisementExecutor(_advertisement(executor_id, kind, auth_transport))


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
    assert captured.out.splitlines() == ["executor-good"]


def test_run_match_json_decode_error_from_capabilities_is_dropped_not_raised(capsys):
    adapters = {**_adapters(), "executor-broken": _MalformedResponseExecutor()}

    exit_code = run_match(adapters, capabilities=["kind-a"], explain=False)

    captured = capsys.readouterr()
    assert exit_code == 0
    assert captured.out.splitlines() == ["executor-good"]


def test_explain_path_distinguishes_selected_policy_excluded_and_unmatched_candidates(capsys):
    exit_code = run_match(_adapters(), capabilities=["kind-a"], explain=True)

    captured = capsys.readouterr()
    assert exit_code == 0
    assert captured.out.splitlines() == [
        "executor-good",
        "executor-good: eligible=yes score=1",
        "executor-excluded: eligible=no reason=policy excludes this candidate for "
        "required kind(s): kind-a (policy_excluded)",
        "executor-other: eligible=yes reason=does not satisfy required kind(s): kind-a",
    ]


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
    empty = _FixedAdvertisementExecutor(
        {"spec_version": _SPEC_VERSION, "executor_id": "executor-empty", "capabilities": []}
    )
    adapters = {"executor-good": _candidate("executor-good", "kind-a", "local"),
                "executor-empty": empty}

    run_match(adapters, capabilities=["kind-a"], explain=True)

    lines = {line.split(":", 1)[0]: line for line in capsys.readouterr().out.splitlines()}
    assert lines["executor-empty"] == (
        "executor-empty: eligible=no reason=advertises no capabilities"
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
        "executor-good",
        "executor-good: eligible=yes score=1",
        "executor-excluded: eligible=no reason=excluded by policy (policy_excluded)",
        "executor-other: eligible=yes score=2",
    ]

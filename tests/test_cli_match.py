"""Tests for `praxis executors match --capability ... --explain`
(`build_requirement` / `run_match`) in `praxis_cli.match_cmd`.

Advertisement fixtures follow schemas/v1/capability-advertisement.schema.json.
`auth_transport` values exercise the default `AuthTransportPolicy` that
`run_match` wires in the same way `registry.py` does (recognized/safe
"local" is eligible; "metered_api" is unsafe-by-default and produces
`policy_excluded=True` when it's the only advertisement of a required kind).
"""

from __future__ import annotations

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


def _adapters() -> list[Executor]:
    # Satisfies the requested kind and has a policy-eligible auth_transport
    # -> selected.
    selected = _FixedAdvertisementExecutor(_advertisement("executor-good", "kind-a", "local"))
    # Satisfies the requested kind but its only auth_transport is
    # unsafe-by-default -> excluded by AuthTransportPolicy (policy_excluded).
    policy_excluded = _FixedAdvertisementExecutor(
        _advertisement("executor-excluded", "kind-a", "metered_api")
    )
    # Policy-eligible, but doesn't satisfy the requested kind at all.
    wrong_kind = _FixedAdvertisementExecutor(_advertisement("executor-other", "kind-b", "local"))
    return [selected, policy_excluded, wrong_kind]


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
    assert "executor-good" in captured.out
    assert "executor-excluded" not in captured.out
    assert "executor-other" not in captured.out


def test_explain_path_distinguishes_selected_policy_excluded_and_unmatched_candidates(capsys):
    exit_code = run_match(_adapters(), capabilities=["kind-a"], explain=True)

    captured = capsys.readouterr()
    assert exit_code == 0
    lines = {line.split(":", 1)[0].strip(): line for line in captured.out.splitlines()}

    good_line = lines["executor-good"]
    assert "yes" in good_line
    assert "1" in good_line

    excluded_line = lines["executor-excluded"]
    assert "no" in excluded_line
    assert "policy_excluded" in excluded_line

    other_line = lines["executor-other"]
    assert "yes" in other_line
    assert "policy_excluded" not in other_line


def test_no_selection_prints_unsatisfied_reasons_unconditionally(capsys):
    adapters = [_FixedAdvertisementExecutor(_advertisement("executor-other", "kind-b", "local"))]

    exit_code = run_match(adapters, capabilities=["kind-a"], explain=False)

    captured = capsys.readouterr()
    assert exit_code == 0
    assert "no executor selected" in captured.out
    assert "kind-a" in captured.out


def test_empty_capabilities_does_not_crash():
    exit_code = run_match(_adapters(), capabilities=[], explain=True)

    assert exit_code == 0

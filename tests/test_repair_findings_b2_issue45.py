"""Reproduces the b2-issue45 repair findings, one test per finding.

* status/discover rows carried a fifth `error` key. Spec criterion 6 names
  "exactly the four named columns" for the status table and describes
  `--json` as "a list of the same four fields per executor", so the extra
  key is a shape the spec does not sanction. The failure reason moves back
  into the `capabilities` cell the spec itself specifies for it
  ("capabilities: unavailable (<reason>)", criterion 5).
* A `.health()` call that raised was reported as `"degraded"` -- a verdict
  the probe never returned, indistinguishable from a genuinely degraded
  adapter once the `error` key is gone.
* `match --explain` dropped an adapter whose `.capabilities()` raised
  without printing anything, so a user could not tell it had been skipped.
"""

from __future__ import annotations

import json

from praxis_cli.discover_cmd import build_discover_rows
from praxis_cli.match_cmd import run_match
from praxis_cli.status_cmd import build_status_rows, print_status_json
from praxis_executors.interface import Executor, ExecutorAvailability, ExecutorError

_SPEC_VERSION = "1.0.0"

_STATUS_KEYS = {"executor_id", "auth_transport", "status", "capabilities"}
_DISCOVER_KEYS = {
    "executor_id",
    "installed",
    "version",
    "authenticated",
    "auth_transport",
    "capabilities",
}


class _StubExecutor(Executor):
    """Advertises one capability, or raises `ExecutorError` when given none."""

    def __init__(self, executor_id: str, kind: str | None) -> None:
        self._executor_id = executor_id
        self._kind = kind

    def capabilities(self) -> dict:
        if self._kind is None:
            raise ExecutorError("service unreachable")
        return {
            "spec_version": _SPEC_VERSION,
            "executor_id": self._executor_id,
            "capabilities": [
                {
                    "spec_version": _SPEC_VERSION,
                    "auth_transport": "local",
                    "satisfies": [{"kind": self._kind}],
                }
            ],
        }

    def health(self) -> ExecutorAvailability:
        if self._kind is None:
            return ExecutorAvailability.UNAVAILABLE
        return ExecutorAvailability.AVAILABLE

    def launch(self, request):
        raise NotImplementedError

    def status(self, handle):
        raise NotImplementedError

    def cancel(self, handle):
        raise NotImplementedError

    def result(self, handle):
        raise NotImplementedError


class _UnprobableExecutor(_StubExecutor):
    """Both probes fail: `.health()` raises rather than returning a verdict."""

    def __init__(self, executor_id: str) -> None:
        super().__init__(executor_id, None)

    def health(self) -> ExecutorAvailability:
        raise ExecutorError("health probe failed")


def _adapters() -> dict[str, Executor]:
    return {
        "executor-good": _StubExecutor("executor-good", "coding"),
        "executor-bad": _StubExecutor("executor-bad", None),
    }


# Spec criterion 6: exactly the four named columns, on every row.


def test_status_rows_carry_exactly_the_four_spec_named_fields():
    for row in build_status_rows(_adapters()):
        assert set(row) == _STATUS_KEYS


def test_status_json_rows_carry_exactly_the_four_spec_named_fields(capsys):
    print_status_json(build_status_rows(_adapters()))

    for row in json.loads(capsys.readouterr().out):
        assert set(row) == _STATUS_KEYS


def test_a_failed_status_probe_states_its_reason_in_the_capabilities_cell():
    rows = build_status_rows({"executor-bad": _StubExecutor("executor-bad", None)})

    assert rows[0]["capabilities"] == "unavailable (service unreachable)"


# Spec criterion 5's row shape, plus the `auth_transport` column criterion 5
# names alongside the capability kinds -- and nothing else.


def test_discover_rows_carry_no_error_key():
    for row in build_discover_rows(_adapters()):
        assert set(row) == _DISCOVER_KEYS


def test_a_failed_discover_probe_states_its_reason_in_the_capabilities_cell():
    rows = build_discover_rows({"executor-bad": _StubExecutor("executor-bad", None)})

    assert rows[0]["capabilities"] == "unavailable (service unreachable)"


# A health probe that raised returned no verdict to report.


def test_a_health_probe_that_raised_is_not_reported_as_degraded():
    rows = build_status_rows({"executor-broken": _UnprobableExecutor("executor-broken")})

    assert rows[0]["status"] == "unknown"


# `match --explain` accounts for every adapter it was given.


def test_explain_accounts_for_an_adapter_whose_advertisement_could_not_be_read(capsys):
    run_match(_adapters(), capabilities=["coding"], explain=True)

    lines = {line.split(":", 1)[0]: line for line in capsys.readouterr().out.splitlines()}
    assert lines["executor-bad"] == (
        "executor-bad: eligible=unknown reason=advertisement unavailable "
        "(service unreachable)"
    )

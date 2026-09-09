"""Behavioural regression tests for the `praxis executors` CLI surface.

Each test below pins one behaviour that a review round found broken. They are
grouped by the surface they exercise rather than by the review that filed
them:

* `praxis_cli.fields` must tolerate an adapter class it does not recognise
  and an advertisement that leaves an optional key out, because
  `adapters.build_adapters()` grows a new entry every time a new adapter
  lands and `capability.schema.json` requires only `spec_version` and
  `satisfies`.
* `praxis executors` / `--json` must name every executor by its real id and
  emit one stable type per column, whether or not its advertisement could be
  read.
* `praxis executors discover` must report the auth transport alongside the
  capability kinds, and say `unavailable` for a probe that failed.
* `praxis executors match --explain` must name every candidate by the same id
  the other commands use, and give a per-candidate reason that explains that
  candidate's own eligibility verdict.
* `praxis` argument parsing must reject `--json` where it would be ignored.

Behaviour these tests do not re-pin lives in the per-module suites:
`test_cli_fields.py`, `test_cli_status.py`, `test_cli_discover.py`,
`test_cli_match.py` and `test_praxis_cli_executors.py`.

Nothing here touches a real `claude` binary or a real Ollama socket.
"""

from __future__ import annotations

import json

import pytest

from praxis_cli import fields
from praxis_cli.discover_cmd import build_discover_rows, print_discover_rows
from praxis_cli.main import _build_parser, main
from praxis_cli.match_cmd import run_match
from praxis_cli.status_cmd import build_status_rows
from praxis_executors.interface import Executor, ExecutorAvailability, ExecutorError

_SPEC_VERSION = "1.0.0"


class _StubExecutor(Executor):
    """An `Executor` implemented straight off the ABC.

    Deliberately none of the four concrete adapter classes `fields.py`
    dispatches on -- that is what makes it a stand-in for the fifth adapter
    the CLI has to survive.
    """

    def __init__(self, advertisement: dict | None, health: ExecutorAvailability) -> None:
        self._advertisement = advertisement
        self._health = health

    def capabilities(self) -> dict:
        if self._advertisement is None:
            raise ExecutorError("service unreachable")
        return self._advertisement

    def health(self) -> ExecutorAvailability:
        return self._health

    def launch(self, request):
        raise NotImplementedError

    def status(self, handle):
        raise NotImplementedError

    def cancel(self, handle):
        raise NotImplementedError

    def result(self, handle):
        raise NotImplementedError


def _advertisement(executor_id: str) -> dict:
    return {
        "spec_version": _SPEC_VERSION,
        "executor_id": executor_id,
        "capabilities": [
            {
                "spec_version": _SPEC_VERSION,
                "auth_transport": "local",
                "satisfies": [{"kind": "coding"}],
            },
            {
                "spec_version": _SPEC_VERSION,
                "auth_transport": "subscription_cli",
                "satisfies": [{"kind": "reasoning"}],
            },
        ],
    }


def _good() -> _StubExecutor:
    return _StubExecutor(_advertisement("executor-good"), ExecutorAvailability.AVAILABLE)


def _bad() -> _StubExecutor:
    return _StubExecutor(None, ExecutorAvailability.UNAVAILABLE)


def _mapping() -> dict[str, Executor]:
    return {"executor-good": _good(), "executor-bad": _bad()}


# praxis_cli.fields -- an unrecognised adapter class must not abort a command


def test_discover_survives_an_adapter_class_fields_does_not_recognise():
    rows = build_discover_rows(_mapping())

    assert [row["executor_id"] for row in rows] == ["executor-good", "executor-bad"]


def test_status_survives_an_adapter_class_fields_does_not_recognise():
    rows = build_status_rows(_mapping())

    assert [row["executor_id"] for row in rows] == ["executor-good", "executor-bad"]


# praxis_cli.fields -- an advertisement may leave `auth_transport` out


def _partial_transport_advertisement(executor_id: str) -> dict:
    """Schema-valid without an `auth_transport` on every capability.

    `capability.schema.json` requires only `spec_version` and `satisfies`, so
    a conforming adapter may advertise a capability that names no transport.
    """
    return {
        "spec_version": _SPEC_VERSION,
        "executor_id": executor_id,
        "capabilities": [
            {"spec_version": _SPEC_VERSION, "satisfies": [{"kind": "coding"}]},
            {
                "spec_version": _SPEC_VERSION,
                "auth_transport": "local",
                "satisfies": [{"kind": "reasoning"}],
            },
        ],
    }


def _transportless() -> _StubExecutor:
    return _StubExecutor(
        _partial_transport_advertisement("executor-transportless"),
        ExecutorAvailability.AVAILABLE,
    )


def test_auth_transports_skips_a_capability_that_names_no_transport():
    transports = fields.auth_transports(_partial_transport_advertisement("executor-x"))

    assert transports == ["local"]


def test_auth_transports_is_empty_when_no_capability_names_a_transport():
    advertisement = {
        "spec_version": _SPEC_VERSION,
        "executor_id": "executor-x",
        "capabilities": [{"spec_version": _SPEC_VERSION, "satisfies": [{"kind": "coding"}]}],
    }

    assert fields.auth_transports(advertisement) == []


def test_status_survives_an_advertisement_that_names_no_auth_transport():
    rows = build_status_rows({"executor-transportless": _transportless()})

    assert rows[0]["auth_transport"] == "local"


def test_discover_survives_an_advertisement_that_names_no_auth_transport():
    rows = build_discover_rows({"executor-transportless": _transportless()})

    assert rows[0]["auth_transport"] == "local"


# praxis executors -- real ids and one stable type per column


def test_status_rows_report_the_real_executor_id_even_when_the_probe_fails():
    rows = build_status_rows(_mapping())

    failed = rows[1]
    assert failed["executor_id"] == "executor-bad"


def test_status_row_id_does_not_shift_when_adapter_order_changes():
    reordered = {"executor-bad": _bad(), "executor-good": _good()}

    ids = {row["executor_id"] for row in build_status_rows(reordered)}
    assert ids == {"executor-good", "executor-bad"}


def test_status_json_columns_keep_one_type_across_healthy_and_failed_rows(capsys):
    rows = build_status_rows(_mapping())
    print(json.dumps(rows))

    parsed = json.loads(capsys.readouterr().out)
    for row in parsed:
        assert isinstance(row["capabilities"], list)
        assert isinstance(row["auth_transport"], str)
    assert parsed[0]["error"] is None
    assert parsed[1]["error"] == "service unreachable"
    assert parsed[1]["capabilities"] == []
    assert parsed[1]["auth_transport"] == ""


# praxis executors discover -- auth transport alongside the capability kinds


def test_discover_rows_report_the_auth_transport():
    rows = build_discover_rows(_mapping())

    assert rows[0]["auth_transport"] == "local,subscription_cli"
    # A failed probe leaves the column its own empty string rather than
    # borrowing the status vocabulary, and says why in `error` instead.
    assert rows[1]["auth_transport"] == ""
    assert rows[1]["error"] == "service unreachable"


def test_discover_row_id_comes_from_the_adapter_mapping_not_the_advertisement():
    # The mapping key is the id the CLI knows an executor by, and it is
    # available whether or not `.capabilities()` returns.
    mismatched = _StubExecutor(_advertisement("something-else"), ExecutorAvailability.AVAILABLE)
    adapters = {"executor-registered": mismatched}

    assert build_discover_rows(adapters)[0]["executor_id"] == "executor-registered"


def test_discover_says_capabilities_are_unavailable_with_the_reason(capsys):
    # Spec criterion 5 prescribes this wording for a row whose probe failed.
    print_discover_rows(build_discover_rows({"executor-bad": _bad()}))

    assert "  capabilities: unavailable (service unreachable)" in capsys.readouterr().out


def test_discover_says_nothing_about_availability_on_a_healthy_row(capsys):
    print_discover_rows(build_discover_rows({"executor-good": _good()}))

    out = capsys.readouterr().out
    assert "  capabilities: coding,reasoning" in out
    assert "unavailable" not in out


# praxis executors match --explain -- candidate-scoped reasons


def _match_advertisement(executor_id: str, kind: str, auth_transport: str) -> dict:
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


def _candidate(executor_id: str, kind: str, auth_transport: str) -> _StubExecutor:
    return _StubExecutor(
        _match_advertisement(executor_id, kind, auth_transport),
        ExecutorAvailability.AVAILABLE,
    )


def test_explain_gives_an_eligible_candidate_a_reason_about_itself(capsys):
    adapters = {
        "executor-good": _candidate("executor-good", "kind-a", "local"),
        "executor-other": _candidate("executor-other", "kind-b", "local"),
    }

    run_match(adapters, capabilities=["kind-a"], explain=True)

    lines = {line.split(":", 1)[0]: line for line in capsys.readouterr().out.splitlines()}
    other = lines["executor-other"]
    assert "eligible=yes" in other
    # The requirement-level string contradicts "eligible=yes" on the same line.
    assert "no eligible advertisement satisfies" not in other
    assert "kind-a" in other


def test_explain_names_the_policy_as_the_reason_for_an_ineligible_candidate(capsys):
    adapters = {
        "executor-good": _candidate("executor-good", "kind-a", "local"),
        "executor-excluded": _candidate("executor-excluded", "kind-a", "metered_api"),
    }

    run_match(adapters, capabilities=["kind-a"], explain=True)

    lines = {line.split(":", 1)[0]: line for line in capsys.readouterr().out.splitlines()}
    excluded = lines["executor-excluded"]
    assert "eligible=no" in excluded
    assert "policy_excluded" in excluded


def test_match_names_a_candidate_by_its_mapping_key_not_the_advertisement(capsys):
    # `discover` and `status` name a row by the `build_adapters()` mapping
    # key; `match` must not print the same adapter under a second id.
    adapters = {"executor-registered": _candidate("something-else", "kind-a", "local")}

    run_match(adapters, capabilities=["kind-a"], explain=True)

    assert capsys.readouterr().out.splitlines() == [
        "executor-registered",
        "executor-registered: eligible=yes score=1",
    ]


def test_match_names_an_unranked_candidate_by_its_mapping_key(capsys):
    adapters = {
        "executor-registered": _candidate("something-else", "kind-a", "metered_api"),
    }

    run_match(adapters, capabilities=["kind-a"], explain=True)

    lines = capsys.readouterr().out.splitlines()
    assert lines[-1].startswith("executor-registered: eligible=no")


def test_explain_reason_for_an_empty_advertisement_does_not_blame_kind_coverage(capsys):
    # `AuthTransportPolicy.is_eligible` is False for an empty `capabilities`
    # list, so `eligible=no` is about the advertisement being empty -- naming
    # the required kind instead leaves that verdict unexplained.
    empty = _StubExecutor(
        {"spec_version": _SPEC_VERSION, "executor_id": "executor-empty", "capabilities": []},
        ExecutorAvailability.AVAILABLE,
    )
    adapters = {
        "executor-good": _candidate("executor-good", "kind-a", "local"),
        "executor-empty": empty,
    }

    run_match(adapters, capabilities=["kind-a"], explain=True)

    lines = {line.split(":", 1)[0]: line for line in capsys.readouterr().out.splitlines()}
    assert lines["executor-empty"] == (
        "executor-empty: eligible=no reason=advertises no capabilities"
    )


# praxis argument parsing


def test_json_is_rejected_where_a_nested_subcommand_would_ignore_it(capsys):
    with pytest.raises(SystemExit):
        main(["executors", "--json", "discover"])

    assert "--json" in capsys.readouterr().err


def test_top_level_parser_stores_no_unread_subcommand_destination():
    args = _build_parser().parse_args(["executors"])

    assert not hasattr(args, "command")

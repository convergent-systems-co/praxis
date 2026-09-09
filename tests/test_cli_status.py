"""Tests for `praxis executors` / `praxis executors --json`
(`build_status_rows`, `print_status_table`, `print_status_json`, `run_status`)
in `praxis_cli.status_cmd`.

Two properties the row shape has to hold, both exercised below:

* Every executor is named by the id it is registered under in the
  `{executor_id: instance}` mapping `praxis_cli.adapters.build_adapters()`
  returns, so an id never depends on adapter order or on whether the
  adapter's advertisement could be read.
* Every column keeps one type across healthy and failed rows -- `capabilities`
  is always a list, `auth_transport` always a string -- with the failure
  reason in its own `error` key, so a `--json` consumer never type-switches.

Uses lightweight fake `Executor` subclasses (implementing the ABC directly,
not the real adapters), matching the pattern used for `match_cmd` tests.
"""

from __future__ import annotations

import json

from praxis_cli.status_cmd import (
    build_status_rows,
    print_status_json,
    print_status_table,
    run_status,
)
from praxis_executors.interface import Executor, ExecutorAvailability, ExecutorError

_SPEC_VERSION = "1.0.0"


class _AvailableExecutor(Executor):
    """A fake Executor whose `.capabilities()` succeeds with two capability
    entries carrying different `auth_transport` values, to exercise the
    comma-join in `auth_transports()`."""

    def __init__(self, executor_id: str) -> None:
        self._executor_id = executor_id

    def capabilities(self) -> dict:
        return {
            "spec_version": _SPEC_VERSION,
            "executor_id": self._executor_id,
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

    def health(self) -> ExecutorAvailability:
        return ExecutorAvailability.AVAILABLE

    def launch(self, request):
        raise NotImplementedError

    def status(self, handle):
        raise NotImplementedError

    def cancel(self, handle):
        raise NotImplementedError

    def result(self, handle):
        raise NotImplementedError


class _TransportlessExecutor(Executor):
    """A fake Executor advertising one capability that names no transport.

    `capability.schema.json` requires only `spec_version` and `satisfies`, so
    this advertisement is conforming and must not take its row down.
    """

    def capabilities(self) -> dict:
        return {
            "spec_version": _SPEC_VERSION,
            "executor_id": "executor-transportless",
            "capabilities": [
                {"spec_version": _SPEC_VERSION, "satisfies": [{"kind": "coding"}]},
                {
                    "spec_version": _SPEC_VERSION,
                    "auth_transport": "local",
                    "satisfies": [{"kind": "reasoning"}],
                },
            ],
        }

    def health(self) -> ExecutorAvailability:
        return ExecutorAvailability.AVAILABLE

    def launch(self, request):
        raise NotImplementedError

    def status(self, handle):
        raise NotImplementedError

    def cancel(self, handle):
        raise NotImplementedError

    def result(self, handle):
        raise NotImplementedError


class _UnavailableExecutor(Executor):
    """A fake Executor whose `.capabilities()` raises `ExecutorError`."""

    def capabilities(self) -> dict:
        raise ExecutorError("service unreachable")

    def health(self) -> ExecutorAvailability:
        return ExecutorAvailability.UNAVAILABLE

    def launch(self, request):
        raise NotImplementedError

    def status(self, handle):
        raise NotImplementedError

    def cancel(self, handle):
        raise NotImplementedError

    def result(self, handle):
        raise NotImplementedError


class _MalformedResponseExecutor(Executor):
    """A fake Executor whose transport layer raises `json.JSONDecodeError`
    (a `ValueError` subclass, not an `ExecutorError`) from both `.health()`
    and `.capabilities()` -- reproducing a non-Ollama server answering 200
    with a non-JSON body on the adapter's configured port."""

    def health(self) -> ExecutorAvailability:
        json.loads("not json")
        raise AssertionError("unreachable")

    def capabilities(self) -> dict:
        json.loads("not json")
        raise AssertionError("unreachable")

    def launch(self, request):
        raise NotImplementedError

    def status(self, handle):
        raise NotImplementedError

    def cancel(self, handle):
        raise NotImplementedError

    def result(self, handle):
        raise NotImplementedError


def _adapters() -> dict[str, Executor]:
    return {
        "executor-good": _AvailableExecutor("executor-good"),
        "executor-bad": _UnavailableExecutor(),
    }


# build_status_rows()


def test_build_status_rows_succeeding_executor():
    rows = build_status_rows({"executor-good": _AvailableExecutor("executor-good")})

    assert rows == [
        {
            "executor_id": "executor-good",
            "auth_transport": "local,subscription_cli",
            "status": "available",
            "capabilities": ["coding", "reasoning"],
            "error": None,
        }
    ]


def test_build_status_rows_failing_executor_keeps_its_id_and_carries_the_reason_in_error():
    rows = build_status_rows({"executor-bad": _UnavailableExecutor()})

    assert rows == [
        {
            "executor_id": "executor-bad",
            "auth_transport": "",
            "status": "unavailable",
            "capabilities": [],
            "error": "service unreachable",
        }
    ]


def test_build_status_rows_survives_a_capability_that_names_no_auth_transport():
    rows = build_status_rows({"executor-transportless": _TransportlessExecutor()})

    assert rows[0]["auth_transport"] == "local"
    assert rows[0]["capabilities"] == ["coding", "reasoning"]


def test_build_status_rows_ids_do_not_shift_with_adapter_order():
    forward = build_status_rows(_adapters())
    reversed_mapping = dict(reversed(list(_adapters().items())))

    backward = build_status_rows(reversed_mapping)

    assert {row["executor_id"] for row in forward} == {row["executor_id"] for row in backward}
    assert [row["executor_id"] for row in backward] == ["executor-bad", "executor-good"]


def test_build_status_rows_preserves_adapter_order():
    rows = build_status_rows(_adapters())

    assert [row["executor_id"] for row in rows] == ["executor-good", "executor-bad"]


def test_build_status_rows_health_raising_json_decode_error_degrades_its_row_instead_of_crashing():
    rows = build_status_rows(
        {"executor-good": _AvailableExecutor("executor-good"), "executor-broken": _MalformedResponseExecutor()}
    )

    assert rows[1]["executor_id"] == "executor-broken"
    assert rows[1]["status"] == "degraded"
    assert rows[1]["auth_transport"] == ""
    assert rows[1]["capabilities"] == []
    assert "Expecting value" in rows[1]["error"]
    # The other row is unaffected.
    assert rows[0]["status"] == "available"


# print_status_table()


def _header_offsets(header: str) -> tuple[list[str], list[int]]:
    names = header.split()
    return names, [header.index(name) for name in names]


def _cells(line: str, offsets: list[int]) -> list[str]:
    bounds = [*offsets, len(line) + 1]
    return [line[bounds[i] : bounds[i + 1]].rstrip() for i in range(len(offsets))]


def test_print_status_table_prints_a_header_then_one_line_per_row(capsys):
    rows = build_status_rows(_adapters())

    print_status_table(rows)

    captured = capsys.readouterr()
    lines = captured.out.splitlines()
    assert len(lines) == 3
    names, offsets = _header_offsets(lines[0])
    assert names == [
        "EXECUTOR_ID",
        "AUTH_TRANSPORT",
        "STATUS",
        "CAPABILITIES",
        "ERROR",
    ]
    assert _cells(lines[1], offsets) == [
        "executor-good",
        "local,subscription_cli",
        "available",
        "coding,reasoning",
        "",
    ]
    # The failing row's column boundaries survive even though its last cell is
    # a free-text sentence containing spaces.
    assert _cells(lines[2], offsets) == [
        "executor-bad",
        "",
        "unavailable",
        "",
        "service unreachable",
    ]


def test_print_status_table_omits_the_error_column_when_every_probe_succeeded(capsys):
    print_status_table(build_status_rows({"executor-good": _AvailableExecutor("executor-good")}))

    names, _ = _header_offsets(capsys.readouterr().out.splitlines()[0])
    assert names == ["EXECUTOR_ID", "AUTH_TRANSPORT", "STATUS", "CAPABILITIES"]


# print_status_json()


def test_print_status_json_is_one_line_valid_json_round_trip(capsys):
    rows = build_status_rows(_adapters())

    print_status_json(rows)

    captured = capsys.readouterr()
    lines = captured.out.splitlines()
    assert len(lines) == 1
    assert json.loads(lines[0]) == rows


def test_print_status_json_keeps_one_type_per_column_across_rows(capsys):
    print_status_json(build_status_rows(_adapters()))

    parsed = json.loads(capsys.readouterr().out)
    for row in parsed:
        assert isinstance(row["capabilities"], list)
        assert isinstance(row["auth_transport"], str)
        assert isinstance(row["status"], str)


# run_status()


def test_run_status_table_path_returns_zero_and_prints_table(capsys):
    exit_code = run_status(_adapters(), as_json=False)

    captured = capsys.readouterr()
    assert exit_code == 0
    assert "executor-good" in captured.out
    assert len(captured.out.splitlines()) == 3


def test_run_status_json_path_returns_zero_and_prints_one_json_line(capsys):
    exit_code = run_status(_adapters(), as_json=True)

    captured = capsys.readouterr()
    assert exit_code == 0
    lines = captured.out.splitlines()
    assert len(lines) == 1
    parsed = json.loads(lines[0])
    assert parsed[0]["executor_id"] == "executor-good"

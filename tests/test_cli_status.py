"""Tests for `praxis executors` / `praxis executors --json`
(`build_status_rows`, `print_status_table`, `print_status_json`, `run_status`)
in `praxis_cli.status_cmd`.

Uses lightweight fake `Executor` subclasses (implementing the ABC directly,
not the real adapters), matching the pattern used for `match_cmd` tests.

Design decision this test encodes and that the developer/tech lead should
confirm (see this task's NEEDS_CONTEXT history): `Executor`'s public ABC has
no accessor for identity outside the dict `.capabilities()` returns, so when
that call itself raises `ExecutorError` there is no other public,
in-footprint source for `executor_id` -- reaching into the private
`_executor_id` attribute and touching `praxis_executors/` are both
unavailable. This suite asserts the fallback is `f"{type(executor).__name__}#{index}"`
(the adapter's position in the input list): it needs no adapter changes, no
private access, still lets a reader distinguish which row failed (unlike
reusing the `"unavailable (...)"` string, which would make every failing
row's id identical), and stays unique even when two failing adapters share a
class (code-review repair for a latent collision in the bare class-name
fallback). T3 (`discover_cmd.py`) has the same gap and follows the same
convention for consistency.
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


def _adapters() -> list[Executor]:
    return [_AvailableExecutor("executor-good"), _UnavailableExecutor()]


# build_status_rows()


def test_build_status_rows_succeeding_executor():
    rows = build_status_rows([_AvailableExecutor("executor-good")])

    assert rows == [
        {
            "executor_id": "executor-good",
            "auth_transport": "local, subscription_cli",
            "status": "available",
            "capabilities": ["coding", "reasoning"],
        }
    ]


def test_build_status_rows_failing_executor_reports_unavailable_and_uses_type_name_as_id():
    executor = _UnavailableExecutor()

    rows = build_status_rows([executor])

    assert rows == [
        {
            "executor_id": f"{type(executor).__name__}#0",
            "auth_transport": "unavailable (service unreachable)",
            "status": "unavailable",
            "capabilities": "unavailable (service unreachable)",
        }
    ]


def test_build_status_rows_two_failing_executors_of_same_class_get_distinct_ids():
    rows = build_status_rows([_UnavailableExecutor(), _UnavailableExecutor()])

    ids = [row["executor_id"] for row in rows]
    assert len(set(ids)) == len(ids), f"executor_id collided across same-class failures: {ids}"


def test_build_status_rows_preserves_adapter_order():
    rows = build_status_rows(_adapters())

    assert [row["executor_id"] for row in rows] == ["executor-good", "_UnavailableExecutor#1"]


# print_status_table()


def test_print_status_table_includes_all_four_columns_per_row(capsys):
    rows = build_status_rows(_adapters())

    print_status_table(rows)

    captured = capsys.readouterr()
    assert "executor-good" in captured.out
    assert "available" in captured.out
    assert "local, subscription_cli" in captured.out
    assert "_UnavailableExecutor" in captured.out
    assert "unavailable (service unreachable)" in captured.out


# print_status_json()


def test_print_status_json_is_one_line_valid_json_round_trip(capsys):
    rows = build_status_rows(_adapters())

    print_status_json(rows)

    captured = capsys.readouterr()
    lines = captured.out.splitlines()
    assert len(lines) == 1
    assert json.loads(lines[0]) == rows


# run_status()


def test_run_status_table_path_returns_zero_and_prints_table(capsys):
    exit_code = run_status(_adapters(), as_json=False)

    captured = capsys.readouterr()
    assert exit_code == 0
    assert "executor-good" in captured.out
    assert len(captured.out.splitlines()) > 1


def test_run_status_json_path_returns_zero_and_prints_one_json_line(capsys):
    exit_code = run_status(_adapters(), as_json=True)

    captured = capsys.readouterr()
    assert exit_code == 0
    lines = captured.out.splitlines()
    assert len(lines) == 1
    parsed = json.loads(lines[0])
    assert parsed[0]["executor_id"] == "executor-good"

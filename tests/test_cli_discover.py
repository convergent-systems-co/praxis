"""Tests for `praxis executors discover` (`build_discover_rows`,
`print_discover_rows`, `run_discover`) in `praxis_cli.discover_cmd`.

The command takes the `{executor_id: instance}` mapping
`praxis_cli.adapters.build_adapters()` returns, so every row is named by the
id the CLI knows the adapter as -- including a row whose `.capabilities()`
call failed and left no advertisement to read an id from.

The fakes below implement the `Executor` ABC directly rather than subclassing
a real adapter: `praxis_cli.fields` degrades to `"n/a"` for a class it does
not recognise instead of raising, which is what keeps one unknown adapter
from taking the whole report down. Nothing here starts a `claude` subprocess
or opens an Ollama socket.
"""

from __future__ import annotations

from praxis_cli.discover_cmd import build_discover_rows, print_discover_rows, run_discover
from praxis_executors.interface import Executor, ExecutorAvailability, ExecutorError

_SPEC_VERSION = "1.0.0"

_CAPS = [
    {
        "spec_version": _SPEC_VERSION,
        "id": "cap-primary",
        "satisfies": [{"kind": "coding"}, {"kind": "reasoning"}],
        "auth_transport": "local",
    }
]


class _StubExecutor(Executor):
    """Returns a fixed advertisement, or raises `ExecutorError` when given none."""

    def __init__(self, executor_id: str, capabilities: list[dict] | None) -> None:
        self._executor_id = executor_id
        self._capabilities = capabilities

    def capabilities(self) -> dict:
        if self._capabilities is None:
            raise ExecutorError("capability probe failed")
        return {
            "spec_version": _SPEC_VERSION,
            "executor_id": self._executor_id,
            "capabilities": self._capabilities,
        }

    def health(self) -> ExecutorAvailability:
        if self._capabilities is None:
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


def _failing(executor_id: str = "executor-fake-bad") -> _StubExecutor:
    return _StubExecutor(executor_id, None)


def _succeeding(executor_id: str = "executor-fake-good") -> _StubExecutor:
    return _StubExecutor(executor_id, _CAPS)


# build_discover_rows()


def test_build_discover_rows_succeeding_executor():
    rows = build_discover_rows({"executor-fake-good": _succeeding()})

    assert rows == [
        {
            "executor_id": "executor-fake-good",
            "installed": "n/a",
            "version": "unknown",
            "authenticated": "n/a",
            "auth_transport": "local",
            "capabilities": ["coding", "reasoning"],
        }
    ]


def test_build_discover_rows_failing_executor_reports_unavailable_and_continues():
    adapters = {
        "executor-fake-bad": _failing(),
        "executor-fake-good": _succeeding(),
    }

    rows = build_discover_rows(adapters)

    assert rows == [
        {
            "executor_id": "executor-fake-bad",
            "installed": "n/a",
            "version": "unknown",
            "authenticated": "n/a",
            "auth_transport": "unavailable",
            "capabilities": "unavailable (capability probe failed)",
        },
        {
            "executor_id": "executor-fake-good",
            "installed": "n/a",
            "version": "unknown",
            "authenticated": "n/a",
            "auth_transport": "local",
            "capabilities": ["coding", "reasoning"],
        },
    ]


def test_build_discover_rows_names_every_failing_executor_by_its_registered_id():
    adapters = {
        "executor-fake-bad-1": _failing("executor-fake-bad-1"),
        "executor-fake-bad-2": _failing("executor-fake-bad-2"),
    }

    rows = build_discover_rows(adapters)

    assert [row["executor_id"] for row in rows] == [
        "executor-fake-bad-1",
        "executor-fake-bad-2",
    ]


# print_discover_rows()


def test_print_discover_rows_includes_content_per_row(capsys):
    rows = build_discover_rows(
        {"executor-fake-bad": _failing(), "executor-fake-good": _succeeding()}
    )

    print_discover_rows(rows)

    captured = capsys.readouterr()
    assert "executor-fake-bad" in captured.out
    assert "unavailable (capability probe failed)" in captured.out
    assert "executor-fake-good" in captured.out
    assert "coding" in captured.out
    assert "auth_transport: local" in captured.out


# run_discover() -- must degrade gracefully rather than abort on a failing row.


def test_run_discover_completes_and_returns_zero_despite_a_failing_adapter(capsys):
    adapters = {
        "executor-fake-bad": _failing(),
        "executor-fake-good": _succeeding(),
    }

    exit_code = run_discover(adapters)

    captured = capsys.readouterr()
    assert exit_code == 0
    assert "executor-fake-bad" in captured.out
    assert "executor-fake-good" in captured.out

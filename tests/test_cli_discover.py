"""Tests for `praxis executors discover` (`build_discover_rows`,
`print_discover_rows`, `run_discover`) in `praxis_cli.discover_cmd`.

Design decision this test encodes and that the developer/tech lead should
confirm (see this task's NEEDS_CONTEXT history): `installed_field` and
`authenticated_field` in `praxis_cli.fields` dispatch by closed `isinstance`
against the four concrete adapter classes and raise `TypeError` for anything
else (fields.py:18-25). A fake that only implements the `Executor` ABC
directly is not one of those four, so it cannot flow through
`build_discover_rows` (which calls `installed_field`/`authenticated_field`
for every row, not just the ones whose `.capabilities()` raises) without
either changing `fields.py` (owned by T2, outside this task's footprint) or
using a fake that `fields.py` already recognizes. `FakeCapabilityExecutor`
(`praxis_executors.adapters.fake`) is one of the four recognized types --
grouped with `SubprocessExecutor` in the `"n/a (built-in)"` branch -- and,
unlike `ClaudeCliExecutor`/`OllamaExecutor`/`SubprocessExecutor`, it never
touches a real external environment (`claude`, `ollama`, a real subprocess).
Subclassing it here for the isolated `build_discover_rows` cases satisfies
both "not the real adapters" and fields.py's existing dispatch without
touching fields.py. The separate `run_discover` test still exercises the
real adapters via `praxis_cli.adapters.build_adapters()`, per the brief.

Second design decision, reused rather than re-asked: like T4
(`status_cmd.py`), the only public source of `executor_id` is the dict
`.capabilities()` returns, so when that call itself raises `ExecutorError`
there is no advertisement to read an id from. T4's tests
(`tests/test_cli_status.py`) settled on `f"{type(executor).__name__}#{index}"`
(the adapter's position in the input list, not just the bare class name) as
the fallback -- the index keeps two failing adapters of the same class from
colliding on the same `executor_id` (code-review repair) -- and named T3 as
the sibling that should follow the same convention for consistency; this
suite does that.
"""

from __future__ import annotations

from praxis_cli.discover_cmd import build_discover_rows, print_discover_rows, run_discover
from praxis_executors.adapters.fake import FakeCapabilityExecutor
from praxis_executors.interface import ExecutorError

_SPEC_VERSION = "1.0.0"

_CAPS = [
    {
        "spec_version": _SPEC_VERSION,
        "id": "cap-primary",
        "satisfies": [{"kind": "coding"}, {"kind": "reasoning"}],
        "auth_transport": "local",
    }
]


class _RaisingFakeExecutor(FakeCapabilityExecutor):
    """A fake, recognized-by-`fields.py` executor whose `.capabilities()`
    raises `ExecutorError`."""

    def __init__(self, executor_id: str) -> None:
        super().__init__(executor_id=executor_id, capabilities=[], script={})

    def capabilities(self) -> dict:
        raise ExecutorError("capability probe failed")


def _succeeding_fake(executor_id: str = "executor-fake-good") -> FakeCapabilityExecutor:
    return FakeCapabilityExecutor(executor_id=executor_id, capabilities=_CAPS, script={})


# build_discover_rows()


def test_build_discover_rows_succeeding_executor():
    rows = build_discover_rows([_succeeding_fake()])

    assert rows == [
        {
            "executor_id": "executor-fake-good",
            "installed": "n/a (built-in)",
            "version": "unknown",
            "authenticated": "n/a",
            "capabilities": ["coding", "reasoning"],
        }
    ]


def test_build_discover_rows_failing_executor_reports_unavailable_and_continues():
    failing = _RaisingFakeExecutor("executor-fake-bad")
    succeeding = _succeeding_fake("executor-fake-good")

    rows = build_discover_rows([failing, succeeding])

    assert rows == [
        {
            "executor_id": f"{type(failing).__name__}#0",
            "installed": "n/a (built-in)",
            "version": "unknown",
            "authenticated": "n/a",
            "capabilities": "unavailable (capability probe failed)",
        },
        {
            "executor_id": "executor-fake-good",
            "installed": "n/a (built-in)",
            "version": "unknown",
            "authenticated": "n/a",
            "capabilities": ["coding", "reasoning"],
        },
    ]


def test_build_discover_rows_two_failing_executors_of_same_class_get_distinct_ids():
    first = _RaisingFakeExecutor("executor-fake-bad-1")
    second = _RaisingFakeExecutor("executor-fake-bad-2")

    rows = build_discover_rows([first, second])

    ids = [row["executor_id"] for row in rows]
    assert len(set(ids)) == len(ids), f"executor_id collided across same-class failures: {ids}"


# print_discover_rows()


def test_print_discover_rows_includes_content_per_row(capsys):
    failing = _RaisingFakeExecutor("executor-fake-bad")
    rows = build_discover_rows([failing, _succeeding_fake("executor-fake-good")])

    print_discover_rows(rows)

    captured = capsys.readouterr()
    assert type(failing).__name__ in captured.out
    assert "unavailable (capability probe failed)" in captured.out
    assert "executor-fake-good" in captured.out
    assert "coding" in captured.out


# run_discover() -- real adapters, must degrade gracefully regardless of
# whether `claude`/`ollama` are actually installed in the test environment.


def test_run_discover_completes_and_returns_zero_with_real_adapters(capsys):
    from praxis_cli.adapters import build_adapters

    exit_code = run_discover(build_adapters())

    captured = capsys.readouterr()
    assert exit_code == 0
    assert captured.out

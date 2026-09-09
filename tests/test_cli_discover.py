"""Tests for `praxis executors discover` (`build_discover_rows`,
`print_discover_rows`, `run_discover`) in `praxis_cli.discover_cmd`.

The command takes the `{executor_id: instance}` mapping
`praxis_cli.adapters.build_adapters()` returns, so every row is named by the
id the CLI knows the adapter as -- including a row whose `.capabilities()`
call failed and left no advertisement to read an id from.

Rows carry the same failure contract `status_cmd` does: a failed probe states
its reason in the `capabilities` cell, in spec criterion 5's own wording
(`unavailable (<reason>)`), and marks `auth_transport` unavailable rather than
empty -- there is no separate `error` column.

The fakes below implement the `Executor` ABC directly rather than subclassing
a real adapter: `praxis_cli.fields` degrades to `"n/a"` for a class it does
not recognise instead of raising, which is what keeps one unknown adapter
from taking the whole report down. The two probe-count tests are the
exception -- `installed_field` dispatches on the concrete adapter class, so
they need a real `OllamaExecutor` -- and monkeypatch both of its probes.
Nothing here starts a `claude` subprocess or opens an Ollama socket.
"""

from __future__ import annotations

import json

from praxis_cli.discover_cmd import build_discover_rows, print_discover_rows, run_discover
from praxis_executors.adapters.ollama import OllamaExecutor
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


class _MalformedResponseExecutor(Executor):
    """A fake Executor whose `.capabilities()` raises `json.JSONDecodeError`
    (a `ValueError` subclass, not an `ExecutorError`) -- reproducing a
    non-Ollama server answering 200 with a non-JSON body."""

    def capabilities(self) -> dict:
        json.loads("not json")
        raise AssertionError("unreachable")

    def health(self) -> ExecutorAvailability:
        return ExecutorAvailability.DEGRADED

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


def test_build_discover_rows_json_decode_error_from_capabilities_degrades_its_row_instead_of_crashing():
    rows = build_discover_rows(
        {"executor-fake-good": _succeeding(), "executor-broken": _MalformedResponseExecutor()}
    )

    broken = rows[1]
    assert broken["executor_id"] == "executor-broken"
    assert broken["auth_transport"] == "unavailable"
    assert "Expecting value" in broken["capabilities"]


def test_build_discover_rows_carry_the_same_columns_across_healthy_and_failed_rows():
    # The same guarantee `status_cmd` makes: a consumer reads one fixed set of
    # keys whether or not the probe happened to succeed.
    rows = build_discover_rows(
        {"executor-fake-bad": _failing(), "executor-fake-good": _succeeding()}
    )

    assert [set(row) for row in rows] == [set(rows[0])] * 2
    assert "error" not in rows[0]


def test_build_discover_rows_joins_auth_transports_the_way_status_does():
    multi = _StubExecutor(
        "executor-fake-multi",
        [
            {
                "spec_version": _SPEC_VERSION,
                "satisfies": [{"kind": "coding"}],
                "auth_transport": "local",
            },
            {
                "spec_version": _SPEC_VERSION,
                "satisfies": [{"kind": "reasoning"}],
                "auth_transport": "subscription_cli",
            },
        ],
    )

    rows = build_discover_rows({"executor-fake-multi": multi})

    assert rows[0]["auth_transport"] == "local,subscription_cli"


def test_build_discover_rows_survives_a_capability_that_names_no_auth_transport():
    # `capability.schema.json` requires only `spec_version` and `satisfies`, so
    # a conforming adapter may advertise a capability naming no transport. That
    # capability contributes nothing rather than taking the row down.
    partial = _StubExecutor(
        "executor-transportless",
        [
            {"spec_version": _SPEC_VERSION, "satisfies": [{"kind": "coding"}]},
            {
                "spec_version": _SPEC_VERSION,
                "satisfies": [{"kind": "reasoning"}],
                "auth_transport": "local",
            },
        ],
    )

    rows = build_discover_rows({"executor-transportless": partial})

    assert rows[0]["auth_transport"] == "local"
    assert rows[0]["capabilities"] == ["coding", "reasoning"]


def test_build_discover_rows_names_a_row_by_its_mapping_key_not_the_advertisement():
    # The mapping key is the id the CLI knows an executor by, and it is
    # available whether or not `.capabilities()` returns.
    mismatched = _StubExecutor("something-else", _CAPS)

    rows = build_discover_rows({"executor-registered": mismatched})

    assert rows[0]["executor_id"] == "executor-registered"


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


def test_build_discover_rows_does_not_reprobe_a_service_that_already_advertised(monkeypatch):
    # `OllamaExecutor.health()` and `.capabilities()` both GET `/api/tags`, each
    # at the adapter's own timeout. An advertisement that came back is already
    # proof the service answered, so the row must not pay for that round trip
    # a second time to fill in `installed`.
    executor = OllamaExecutor(executor_id="executor-ollama-1")
    monkeypatch.setattr(
        executor,
        "capabilities",
        lambda: {
            "spec_version": _SPEC_VERSION,
            "executor_id": "executor-ollama-1",
            "capabilities": _CAPS,
        },
    )

    def _unexpected_probe():
        raise AssertionError("health() re-probed a service that already advertised")

    monkeypatch.setattr(executor, "health", _unexpected_probe)

    rows = build_discover_rows({"executor-ollama-1": executor})

    assert rows[0]["installed"] == "yes"
    assert rows[0]["capabilities"] == ["coding", "reasoning"]


def test_build_discover_rows_still_asks_health_when_the_advertisement_probe_failed(monkeypatch):
    # A failed advertisement probe does not say whether the service is down
    # ("no") or up but empty ("yes"), so `health()` is still the only answer.
    executor = OllamaExecutor(executor_id="executor-ollama-1")

    def _raise():
        raise ExecutorError("ollama service unreachable")

    monkeypatch.setattr(executor, "capabilities", _raise)
    monkeypatch.setattr(executor, "health", lambda: ExecutorAvailability.DEGRADED)

    rows = build_discover_rows({"executor-ollama-1": executor})

    assert rows[0]["installed"] == "yes"
    assert rows[0]["capabilities"] == "unavailable (ollama service unreachable)"


# print_discover_rows()


def test_print_discover_rows_includes_content_per_row(capsys):
    rows = build_discover_rows(
        {"executor-fake-bad": _failing(), "executor-fake-good": _succeeding()}
    )

    print_discover_rows(rows)

    captured = capsys.readouterr()
    assert "executor-fake-bad" in captured.out
    assert "capability probe failed" in captured.out
    assert "executor-fake-good" in captured.out
    assert "coding" in captured.out
    assert "auth_transport: local" in captured.out


def test_print_discover_rows_renders_capabilities_as_text_not_python_syntax(capsys):
    print_discover_rows(build_discover_rows({"executor-fake-good": _succeeding()}))

    captured = capsys.readouterr()
    assert "  capabilities: coding,reasoning" in captured.out
    assert "[" not in captured.out
    assert "'" not in captured.out


def test_print_discover_rows_states_a_failed_probe_reason_exactly_once(capsys):
    print_discover_rows(build_discover_rows({"executor-fake-bad": _failing()}))
    failed = capsys.readouterr().out

    print_discover_rows(build_discover_rows({"executor-fake-good": _succeeding()}))
    healthy = capsys.readouterr().out

    # Spec criterion 5's wording, and the only place the block says it: an
    # `error:` line underneath would repeat the same sentence verbatim.
    assert "  capabilities: unavailable (capability probe failed)" in failed
    assert failed.count("capability probe failed") == 1
    assert "error" not in failed
    assert "error" not in healthy
    assert "unavailable" not in healthy


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

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

`conftest._FakeExecutor` implements the `Executor` ABC directly rather than
subclassing a real adapter: `praxis_cli.fields` degrades to `"n/a"` for a
class it does not recognise instead of raising, which is what keeps one
unknown adapter from taking the whole report down. The two probe-count tests
are the exception -- `installed_field` dispatches on the concrete adapter
class, so they need a real `OllamaExecutor` -- and monkeypatch both of its
probes. Nothing here starts a `claude` subprocess or opens an Ollama socket.
"""

from __future__ import annotations

import logging

from conftest import (
    _MALFORMED_ADVERTISEMENTS,
    _FakeExecutor,
    _json_decode_error,
    _undecodable_output_error,
)

from praxis_cli.discover_cmd import build_discover_rows, print_discover_rows, run_discover
from praxis_executors.adapters.claude_cli import ClaudeCliExecutor
from praxis_executors.adapters.ollama import OllamaExecutor
from praxis_executors.interface import ExecutorAvailability, ExecutorError

_SPEC_VERSION = "1.0.0"

_CAPS = [
    {
        "spec_version": _SPEC_VERSION,
        "id": "cap-primary",
        "satisfies": [{"kind": "coding"}, {"kind": "reasoning"}],
        "auth_transport": "local",
    }
]


def _failing(executor_id: str = "executor-fake-bad") -> _FakeExecutor:
    return _FakeExecutor(
        executor_id,
        capabilities_error=ExecutorError("capability probe failed"),
        health=ExecutorAvailability.UNAVAILABLE,
    )


def _succeeding(executor_id: str = "executor-fake-good") -> _FakeExecutor:
    return _FakeExecutor(
        executor_id, capabilities=_CAPS, health=ExecutorAvailability.AVAILABLE
    )


def _malformed(executor_id: str = "executor-broken") -> _FakeExecutor:
    """A transport layer answering 200 with a body that is not JSON."""
    return _FakeExecutor(
        executor_id,
        capabilities_error=_json_decode_error(),
        health=ExecutorAvailability.DEGRADED,
    )


def _non_object_json(executor_id: str = "executor-nonobject") -> _FakeExecutor:
    """A transport layer answering 200 with valid JSON that is not an object.

    `OllamaExecutor` decodes the body and calls `.get()` on it without checking
    its type, so a JSON array comes back out of both probes as an
    `AttributeError` -- neither the adapter's own `ExecutorError` nor a
    `ValueError`, and just as much a one-row failure as either.
    """
    error = AttributeError("'list' object has no attribute 'get'")
    return _FakeExecutor(executor_id, capabilities_error=error, health_error=error)


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
        {"executor-fake-good": _succeeding(), "executor-broken": _malformed()}
    )

    broken = rows[1]
    assert broken["executor_id"] == "executor-broken"
    assert broken["auth_transport"] == "unavailable"
    assert "Expecting value" in broken["capabilities"]


def test_build_discover_rows_attribute_error_from_capabilities_degrades_its_row_instead_of_crashing():
    rows = build_discover_rows(
        {"executor-nonobject": _non_object_json(), "executor-fake-good": _succeeding()}
    )

    assert rows[0]["executor_id"] == "executor-nonobject"
    assert rows[0]["auth_transport"] == "unavailable"
    assert "object has no attribute" in rows[0]["capabilities"]
    # The rest of the report still prints.
    assert rows[1]["capabilities"] == ["coding", "reasoning"]


def test_build_discover_rows_carry_the_same_columns_across_healthy_and_failed_rows():
    # The same guarantee `status_cmd` makes: a consumer reads one fixed set of
    # keys whether or not the probe happened to succeed.
    rows = build_discover_rows(
        {"executor-fake-bad": _failing(), "executor-fake-good": _succeeding()}
    )

    assert [set(row) for row in rows] == [set(rows[0])] * 2
    assert "error" not in rows[0]


def test_build_discover_rows_joins_auth_transports_the_way_status_does():
    multi = _FakeExecutor(
        "executor-fake-multi",
        capabilities=[
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
    partial = _FakeExecutor(
        "executor-transportless",
        capabilities=[
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
    mismatched = _FakeExecutor("something-else", capabilities=_CAPS)

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


def test_build_discover_rows_survives_a_claude_health_probe_that_raised(monkeypatch):
    # `authenticated` is the one cell derived from `ClaudeCliExecutor.health()`,
    # whose `--version` subprocess leaks a `UnicodeDecodeError` for a binary
    # that writes output which is not valid UTF-8. That is one adapter failing
    # one probe, so it degrades one cell -- the other rows still print.
    executor = ClaudeCliExecutor(executor_id="executor-claude-cli-1")
    monkeypatch.setattr("praxis_cli.fields.shutil.which", lambda name: "/usr/local/bin/claude")

    def _raise():
        raise _undecodable_output_error()

    monkeypatch.setattr(executor, "health", _raise)

    rows = build_discover_rows(
        {"executor-claude-cli-1": executor, "executor-fake-good": _succeeding()}
    )

    assert rows[0]["installed"] == "yes"
    assert rows[0]["authenticated"] == "unknown"
    assert rows[1]["capabilities"] == ["coding", "reasoning"]


def test_build_discover_rows_degrades_a_row_whose_returned_advertisement_is_malformed(monkeypatch):
    # A probe that returns is not a probe that answered conformingly. Reading
    # the advertisement outside the guard let a missing required key reach the
    # command as a `KeyError` and take every other row down with it.
    for advertisement in _MALFORMED_ADVERTISEMENTS:
        executor = _succeeding("executor-malformed")
        monkeypatch.setattr(executor, "capabilities", lambda ad=advertisement: ad)

        rows = build_discover_rows(
            {"executor-malformed": executor, "executor-fake-good": _succeeding()}
        )

        assert rows[0]["auth_transport"] == "unavailable"
        assert rows[0]["capabilities"].startswith("unavailable (")
        assert rows[1]["capabilities"] == ["coding", "reasoning"]


# probe-failure logging -- the same record `status` and `match` make


def test_a_capabilities_probe_outside_the_adapter_vocabulary_is_logged_with_its_type(caplog):
    # `AttributeError` from an adapter is far more likely a fault in the adapter
    # than an outage in the service it speaks to, and the row reads
    # `unavailable (...)` either way -- so `discover` records the type, as
    # `status` and `match` do for the same probe.
    with caplog.at_level(logging.WARNING, logger="praxis_cli.fields"):
        rows = build_discover_rows({"executor-nonobject": _non_object_json()})

    assert rows[0]["capabilities"].startswith("unavailable (")
    assert "AttributeError" in caplog.text


def test_an_adapters_own_executor_error_is_not_logged_as_a_fault(caplog):
    with caplog.at_level(logging.WARNING, logger="praxis_cli.fields"):
        build_discover_rows({"executor-fake-bad": _failing()})

    assert caplog.records == []


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

"""Tests for `praxis executors` / `praxis executors --json`
(`build_status_rows`, `print_status_table`, `print_status_json`, `run_status`)
in `praxis_cli.status_cmd`.

Two properties the row shape has to hold, both exercised below:

* Every executor is named by the id it is registered under in the
  `{executor_id: instance}` mapping `praxis_cli.adapters.build_adapters()`
  returns, so an id never depends on adapter order or on whether the
  adapter's advertisement could be read.
* Every row carries exactly the four columns spec criterion 6 names, healthy
  or failed, and a failed probe states its reason in the `capabilities` cell
  rather than in a fifth column the spec does not describe.

Uses `conftest._FakeExecutor` (implementing the ABC directly, not the real
adapters), the same stand-in the `discover_cmd` and `match_cmd` suites take.
The two probe-count tests are the exception -- whether an advertisement can
stand in for a health probe depends on the concrete adapter class, so they
need a real `OllamaExecutor` -- and monkeypatch both of its probes. Nothing
here opens an Ollama socket.
"""

from __future__ import annotations

import json
import logging

import jsonschema
import pytest
from conftest import _MALFORMED_ADVERTISEMENTS, _FakeExecutor, _json_decode_error

from praxis_cli.status_cmd import (
    STATUS_ROW_SCHEMA,
    build_status_rows,
    print_status_json,
    print_status_table,
    run_status,
)
from praxis_executors.adapters.ollama import OllamaExecutor
from praxis_executors.interface import Executor, ExecutorAvailability, ExecutorError

_SPEC_VERSION = "1.0.0"


def _available(executor_id: str = "executor-good") -> _FakeExecutor:
    """Two capability entries carrying different `auth_transport` values, to
    exercise the comma-join in `auth_transports()`."""
    return _FakeExecutor(
        executor_id,
        capabilities=[
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
        health=ExecutorAvailability.AVAILABLE,
    )


def _transportless() -> _FakeExecutor:
    """One capability that names no transport.

    `capability.schema.json` requires only `spec_version` and `satisfies`, so
    this advertisement is conforming and must not take its row down.
    """
    return _FakeExecutor(
        "executor-transportless",
        capabilities=[
            {"spec_version": _SPEC_VERSION, "satisfies": [{"kind": "coding"}]},
            {
                "spec_version": _SPEC_VERSION,
                "auth_transport": "local",
                "satisfies": [{"kind": "reasoning"}],
            },
        ],
        health=ExecutorAvailability.AVAILABLE,
    )


def _unavailable(executor_id: str = "executor-bad") -> _FakeExecutor:
    return _FakeExecutor(
        executor_id,
        capabilities_error=ExecutorError("service unreachable"),
        health=ExecutorAvailability.UNAVAILABLE,
    )


def _malformed(executor_id: str = "executor-broken") -> _FakeExecutor:
    """Both probes raise `json.JSONDecodeError` -- a non-Ollama server
    answering 200 with a non-JSON body on the adapter's configured port."""
    error = _json_decode_error()
    return _FakeExecutor(executor_id, capabilities_error=error, health_error=error)


def _non_object_json(executor_id: str = "executor-nonobject") -> _FakeExecutor:
    """Both probes raise `AttributeError` -- a server answering 200 with valid
    JSON that is not an object.

    `OllamaExecutor` decodes the body and calls `.get()` on it unchecked, in
    both `capabilities()` and `health()`, so a JSON array leaks out as an
    `AttributeError`: neither the adapter's own `ExecutorError` nor a
    `ValueError`, and just as much a one-row failure as either.
    """
    error = AttributeError("'list' object has no attribute 'get'")
    return _FakeExecutor(executor_id, capabilities_error=error, health_error=error)


def _adapters() -> dict[str, Executor]:
    return {
        "executor-good": _available(),
        "executor-bad": _unavailable(),
    }


# build_status_rows()


def test_build_status_rows_succeeding_executor():
    rows = build_status_rows({"executor-good": _available()})

    assert rows == [
        {
            "executor_id": "executor-good",
            "auth_transport": "local,subscription_cli",
            "status": "available",
            "capabilities": ["coding", "reasoning"],
        }
    ]


def test_build_status_rows_failing_executor_keeps_its_id_and_states_the_reason():
    rows = build_status_rows({"executor-bad": _unavailable()})

    assert rows == [
        {
            "executor_id": "executor-bad",
            "auth_transport": "unavailable",
            "status": "unavailable",
            "capabilities": "unavailable (service unreachable)",
        }
    ]


def test_build_status_rows_survives_a_capability_that_names_no_auth_transport():
    rows = build_status_rows({"executor-transportless": _transportless()})

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
        {"executor-good": _available(), "executor-broken": _malformed()}
    )

    assert rows[1]["executor_id"] == "executor-broken"
    # A probe that raised returned no availability, so no availability is
    # claimed for it -- `degraded` is a verdict, not a stand-in for one.
    assert rows[1]["status"] == "unknown"
    assert rows[1]["auth_transport"] == "unavailable"
    assert "Expecting value" in rows[1]["capabilities"]
    # The other row is unaffected.
    assert rows[0]["status"] == "available"


def test_build_status_rows_both_probes_raising_attribute_error_degrade_its_row_instead_of_crashing():
    rows = build_status_rows(
        {"executor-good": _available(), "executor-nonobject": _non_object_json()}
    )

    assert rows[1]["executor_id"] == "executor-nonobject"
    assert rows[1]["status"] == "unknown"
    assert rows[1]["auth_transport"] == "unavailable"
    assert "object has no attribute" in rows[1]["capabilities"]
    # The other row is unaffected.
    assert rows[0]["status"] == "available"


def test_build_status_rows_does_not_reprobe_a_service_that_already_advertised(monkeypatch):
    # `OllamaExecutor.health()` and `.capabilities()` both GET `/api/tags`, each
    # at the adapter's own timeout, and `.capabilities()` only returns at all
    # once that endpoint has answered with at least one model -- exactly the
    # condition `health()` would re-request to call the service available. The
    # row must not pay for that round trip a second time, the same way
    # `build_discover_rows` already does not.
    executor = OllamaExecutor(executor_id="executor-ollama-1")
    monkeypatch.setattr(
        executor,
        "capabilities",
        lambda: {
            "spec_version": _SPEC_VERSION,
            "executor_id": "executor-ollama-1",
            "capabilities": [
                {
                    "spec_version": _SPEC_VERSION,
                    "satisfies": [{"kind": "coding"}, {"kind": "reasoning"}],
                    "auth_transport": "local",
                }
            ],
        },
    )

    def _unexpected_probe():
        raise AssertionError("health() re-probed a service that already advertised")

    monkeypatch.setattr(executor, "health", _unexpected_probe)

    rows = build_status_rows({"executor-ollama-1": executor})

    assert rows[0]["status"] == "available"
    assert rows[0]["auth_transport"] == "local"
    assert rows[0]["capabilities"] == ["coding", "reasoning"]


def test_build_status_rows_still_asks_health_when_the_advertisement_probe_failed(monkeypatch):
    # A failed advertisement probe is not a verdict about the service: reachable
    # but erroring, and reachable but empty, both fail it and both are
    # `degraded`. `health()` is still the only thing that can say which.
    executor = OllamaExecutor(executor_id="executor-ollama-1")

    def _raise():
        raise ExecutorError("ollama service reachable but reported zero installed models")

    monkeypatch.setattr(executor, "capabilities", _raise)
    monkeypatch.setattr(executor, "health", lambda: ExecutorAvailability.DEGRADED)

    rows = build_status_rows({"executor-ollama-1": executor})

    assert rows[0]["status"] == "degraded"
    assert rows[0]["auth_transport"] == "unavailable"
    assert rows[0]["capabilities"].startswith("unavailable (")


def test_build_status_rows_asks_health_for_an_adapter_whose_advertisement_proves_nothing():
    # `ClaudeCliExecutor.capabilities()` is a static dict that answers without
    # probing anything, so a returned advertisement says nothing about whether
    # the backing CLI is there. Only an adapter whose advertisement is itself a
    # successful round trip may skip the health probe.
    executor = _FakeExecutor(
        "executor-static",
        capabilities=[
            {
                "spec_version": _SPEC_VERSION,
                "auth_transport": "subscription_cli",
                "satisfies": [{"kind": "coding"}],
            }
        ],
        health=ExecutorAvailability.UNAVAILABLE,
    )

    rows = build_status_rows({"executor-static": executor})

    assert rows[0]["status"] == "unavailable"
    assert rows[0]["capabilities"] == ["coding"]


def test_build_status_rows_degrades_a_row_whose_returned_advertisement_is_malformed(monkeypatch):
    # A probe that returns is not a probe that answered conformingly. Reading the
    # advertisement outside the guard let a missing required key reach the
    # command as a `KeyError` and take every other row down with it.
    for advertisement in _MALFORMED_ADVERTISEMENTS:
        executor = _FakeExecutor("executor-malformed", health=ExecutorAvailability.AVAILABLE)
        monkeypatch.setattr(executor, "capabilities", lambda ad=advertisement: ad)

        rows = build_status_rows(
            {"executor-malformed": executor, "executor-good": _available()}
        )

        assert rows[0]["auth_transport"] == "unavailable"
        assert rows[0]["capabilities"].startswith("unavailable (")
        assert rows[1]["capabilities"] == ["coding", "reasoning"]


def test_build_status_rows_degrades_a_row_whose_capability_entry_is_not_an_object(monkeypatch):
    # An advertisement can be non-conforming in a way no missing key describes:
    # `capabilities` holding something that is not a capability. That is still
    # the adapter answering without answering, so it costs its own row.
    executor = _FakeExecutor("executor-shape", health=ExecutorAvailability.AVAILABLE)
    monkeypatch.setattr(
        executor,
        "capabilities",
        lambda: {
            "spec_version": _SPEC_VERSION,
            "executor_id": "executor-shape",
            "capabilities": ["not-an-object"],
        },
    )

    rows = build_status_rows({"executor-shape": executor, "executor-good": _available()})

    assert rows[0]["auth_transport"] == "unavailable"
    assert rows[0]["capabilities"].startswith("unavailable (")
    assert rows[1]["capabilities"] == ["coding", "reasoning"]


def test_build_status_rows_lets_a_defect_in_the_clis_own_derivation_surface(monkeypatch):
    # The probe guard exists for an adapter that could not be asked. It used to
    # wrap this module's own reading of the advertisement too, so a bug in that
    # reading came out as `unavailable (...)` -- reported against the adapter,
    # and logged as more likely a fault in it. A defect here is this module's,
    # and it has to be visible as one.
    def _cli_side_defect(_advertisement):
        raise TypeError("sequence item 0: expected str instance, int found")

    monkeypatch.setattr("praxis_cli.status_cmd.auth_transports", _cli_side_defect)

    with pytest.raises(TypeError):
        build_status_rows({"executor-good": _available()})


def test_a_malformed_advertisement_is_logged_as_a_fault_not_an_outage(monkeypatch, caplog):
    # An advertisement missing a required key is the adapter's own doing, and the
    # row reads `unavailable (...)` exactly as a real outage does.
    executor = _FakeExecutor("executor-malformed", health=ExecutorAvailability.AVAILABLE)
    monkeypatch.setattr(executor, "capabilities", lambda: _MALFORMED_ADVERTISEMENTS[0])

    with caplog.at_level(logging.WARNING, logger="praxis_cli.fields"):
        build_status_rows({"executor-malformed": executor})

    assert "MalformedAdvertisement" in caplog.text


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
    ]
    assert _cells(lines[1], offsets) == [
        "executor-good",
        "local,subscription_cli",
        "available",
        "coding,reasoning",
    ]
    # The failing row's column boundaries survive even though its last cell is
    # a free-text sentence containing spaces.
    assert _cells(lines[2], offsets) == [
        "executor-bad",
        "unavailable",
        "unavailable",
        "unavailable (service unreachable)",
    ]


def test_print_status_table_prints_the_same_four_columns_when_every_probe_succeeded(capsys):
    print_status_table(build_status_rows({"executor-good": _available()}))

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


def test_print_status_json_carries_only_the_four_spec_named_fields(capsys):
    print_status_json(build_status_rows(_adapters()))

    parsed = json.loads(capsys.readouterr().out)
    for row in parsed:
        assert list(row) == ["executor_id", "auth_transport", "status", "capabilities"]
        assert isinstance(row["auth_transport"], str)
        assert isinstance(row["status"], str)


def test_print_status_json_reports_a_row_that_does_not_match_the_schema(capsys, caplog):
    # `STATUS_ROW_SCHEMA` is the contract a `--json` consumer reads, so the
    # command that emits the rows is what checks itself against it -- a row shape
    # that widens without the schema saying so is reported here rather than
    # discovered by a consumer downstream.
    row = build_status_rows({"executor-good": _available()})[0]

    with caplog.at_level(logging.WARNING, logger="praxis_cli.status_cmd"):
        print_status_json([{**row, "status": "not-an-availability"}])

    assert "STATUS_ROW_SCHEMA" in caplog.text
    assert "executor-good" in caplog.text
    # Reported, not withheld: a report that prints with a warning beats no report
    # at all, which is the degradation every probe failure here already takes.
    assert json.loads(capsys.readouterr().out)[0]["status"] == "not-an-availability"


def test_print_status_json_says_nothing_about_rows_that_match_the_schema(caplog):
    with caplog.at_level(logging.WARNING, logger="praxis_cli.status_cmd"):
        print_status_json(
            build_status_rows(
                {
                    "executor-good": _available(),
                    "executor-bad": _unavailable(),
                    "executor-broken": _malformed(),
                    "executor-transportless": _transportless(),
                }
            )
        )

    # Only this module's records: the malformed adapter's own probe failure is
    # `fields`' to report, and it is reported whether or not the row conforms.
    assert [record for record in caplog.records if record.name == "praxis_cli.status_cmd"] == []


# STATUS_ROW_SCHEMA


def test_status_row_schema_accepts_a_row_from_an_adapter_that_answered():
    jsonschema.Draft202012Validator(STATUS_ROW_SCHEMA).validate(
        build_status_rows({"executor-good": _available()})[0]
    )


def test_status_row_schema_accepts_a_row_from_an_adapter_that_could_not_be_asked():
    jsonschema.Draft202012Validator(STATUS_ROW_SCHEMA).validate(
        build_status_rows({"executor-bad": _unavailable()})[0]
    )


def test_status_row_schema_declares_capabilities_as_a_union_rather_than_a_bare_list():
    # The point of the schema: a consumer reads that `capabilities` is either a
    # list of kinds or a reason string, instead of discovering the string by
    # iterating it one character at a time. Both branches have to be declared,
    # so neither is a surprise -- and nothing else is accepted in that slot.
    validator = jsonschema.Draft202012Validator(STATUS_ROW_SCHEMA)
    row = build_status_rows({"executor-good": _available()})[0]

    assert validator.is_valid({**row, "capabilities": ["coding"]})
    assert validator.is_valid({**row, "capabilities": "unavailable (service unreachable)"})
    assert not validator.is_valid({**row, "capabilities": 7})
    assert not validator.is_valid({**row, "capabilities": {"coding": True}})


def test_status_row_schema_declares_the_auth_transport_slot_that_also_carries_unavailable():
    validator = jsonschema.Draft202012Validator(STATUS_ROW_SCHEMA)
    row = build_status_rows({"executor-good": _available()})[0]

    assert validator.is_valid({**row, "auth_transport": "local"})
    assert validator.is_valid({**row, "auth_transport": "unavailable"})
    assert not validator.is_valid({**row, "auth_transport": ["local"]})


def test_status_row_schema_covers_exactly_the_emitted_keys():
    validator = jsonschema.Draft202012Validator(STATUS_ROW_SCHEMA)

    for row in build_status_rows(_adapters()):
        validator.validate(row)
        assert not validator.is_valid({**row, "error": "a fifth key the spec does not name"})
        for column in row:
            assert not validator.is_valid({k: v for k, v in row.items() if k != column})


def test_every_row_the_json_path_prints_validates_against_the_schema(capsys):
    print_status_json(
        build_status_rows(
            {
                "executor-good": _available(),
                "executor-bad": _unavailable(),
                "executor-broken": _malformed(),
                "executor-transportless": _transportless(),
            }
        )
    )

    validator = jsonschema.Draft202012Validator(STATUS_ROW_SCHEMA)
    for row in json.loads(capsys.readouterr().out):
        validator.validate(row)


# probe-failure logging


def test_a_capabilities_probe_outside_the_adapter_vocabulary_is_logged_with_its_type(caplog):
    # `AttributeError` from an adapter is far more likely a fault in the
    # adapter than an outage in the service it speaks to, and the row alone
    # cannot say which -- it reads `unavailable (...)` either way.
    with caplog.at_level(logging.WARNING, logger="praxis_cli.fields"):
        rows = build_status_rows({"executor-nonobject": _non_object_json()})

    assert rows[0]["capabilities"].startswith("unavailable (")
    assert "AttributeError" in caplog.text


def test_an_adapters_own_executor_error_is_not_logged_as_a_fault(caplog):
    with caplog.at_level(logging.WARNING, logger="praxis_cli.fields"):
        build_status_rows({"executor-bad": _unavailable()})

    assert caplog.records == []


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

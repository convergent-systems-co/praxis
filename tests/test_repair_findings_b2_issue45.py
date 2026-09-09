"""Behavioural regression tests for the `praxis executors` CLI surface.

Each test below pins one behaviour that a review round found broken. They are
grouped by the surface they exercise rather than by the review that filed
them:

* `praxis_cli.fields` must tolerate an adapter class it does not recognise,
  because `adapters.build_adapters()` grows a new entry every time a new
  adapter lands.
* `praxis executors` / `--json` must name every executor by its real id and
  emit one stable type per column, whether or not its advertisement could be
  read.
* `praxis executors discover` must report the auth transport alongside the
  capability kinds.
* `praxis executors match --explain` must give a per-candidate reason, not
  echo the requirement-level one.
* `praxis` argument parsing must reject `--json` where it would be ignored.

Nothing here touches a real `claude` binary or a real Ollama socket.
"""

from __future__ import annotations

import inspect
import json
import os
import subprocess
import sys

import pytest

from praxis_cli import fields
from praxis_cli.discover_cmd import build_discover_rows
from praxis_cli.main import _build_parser, main
from praxis_cli.match_cmd import run_match
from praxis_cli.status_cmd import build_status_rows, print_status_table
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


def test_installed_field_returns_a_neutral_value_for_an_unrecognised_adapter():
    assert fields.installed_field(_good()) == "n/a"


def test_discover_survives_an_adapter_class_fields_does_not_recognise():
    rows = build_discover_rows(_mapping())

    assert [row["executor_id"] for row in rows] == ["executor-good", "executor-bad"]


def test_status_survives_an_adapter_class_fields_does_not_recognise():
    rows = build_status_rows(_mapping())

    assert [row["executor_id"] for row in rows] == ["executor-good", "executor-bad"]


def test_version_field_marks_its_unused_parameter_as_unused():
    # No adapter exposes a queryable version, so `version_field` reads nothing
    # from its argument; the name records that rather than looking like an
    # oversight. It stays in the signature so every field function in this
    # module is callable the same way.
    (name,) = inspect.signature(fields.version_field).parameters
    assert name.startswith("_")


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


def _header_offsets(header: str) -> tuple[list[str], list[int]]:
    names = header.split()
    return names, [header.index(name) for name in names]


def _cells(line: str, offsets: list[int]) -> list[str]:
    bounds = [*offsets, len(line) + 1]
    return [line[bounds[i] : bounds[i + 1]].rstrip() for i in range(len(offsets))]


def test_status_table_prints_a_header_and_column_aligned_rows(capsys):
    print_status_table(build_status_rows(_mapping()))

    lines = capsys.readouterr().out.splitlines()
    names, offsets = _header_offsets(lines[0])
    assert names == ["EXECUTOR_ID", "AUTH_TRANSPORT", "STATUS", "CAPABILITIES", "ERROR"]
    assert _cells(lines[1], offsets) == [
        "executor-good",
        "local,subscription_cli",
        "available",
        "coding,reasoning",
        "",
    ]
    # The failing row's boundaries survive even though its last cell is a
    # free-text sentence containing spaces.
    assert _cells(lines[2], offsets) == [
        "executor-bad",
        "",
        "unavailable",
        "",
        "service unreachable",
    ]


def test_status_table_omits_the_error_column_when_every_probe_succeeded(capsys):
    print_status_table(build_status_rows({"executor-good": _good()}))

    names, _ = _header_offsets(capsys.readouterr().out.splitlines()[0])
    assert names == ["EXECUTOR_ID", "AUTH_TRANSPORT", "STATUS", "CAPABILITIES"]


# praxis executors discover -- auth transport alongside the capability kinds


def test_discover_rows_report_the_auth_transport():
    rows = build_discover_rows(_mapping())

    assert rows[0]["auth_transport"] == "local, subscription_cli"
    assert rows[1]["auth_transport"] == "unavailable"


def test_discover_row_id_comes_from_the_adapter_mapping_not_the_advertisement():
    # The mapping key is the id the CLI knows an executor by, and it is
    # available whether or not `.capabilities()` returns.
    mismatched = _StubExecutor(_advertisement("something-else"), ExecutorAvailability.AVAILABLE)
    adapters = {"executor-registered": mismatched}

    assert build_discover_rows(adapters)[0]["executor_id"] == "executor-registered"


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
    adapters = [
        _candidate("executor-good", "kind-a", "local"),
        _candidate("executor-other", "kind-b", "local"),
    ]

    run_match(adapters, capabilities=["kind-a"], explain=True)

    lines = {line.split(":", 1)[0]: line for line in capsys.readouterr().out.splitlines()}
    other = lines["executor-other"]
    assert "eligible=yes" in other
    # The requirement-level string contradicts "eligible=yes" on the same line.
    assert "no eligible advertisement satisfies" not in other
    assert "kind-a" in other


def test_explain_names_the_policy_as_the_reason_for_an_ineligible_candidate(capsys):
    adapters = [
        _candidate("executor-good", "kind-a", "local"),
        _candidate("executor-excluded", "kind-a", "metered_api"),
    ]

    run_match(adapters, capabilities=["kind-a"], explain=True)

    lines = {line.split(":", 1)[0]: line for line in capsys.readouterr().out.splitlines()}
    excluded = lines["executor-excluded"]
    assert "eligible=no" in excluded
    assert "policy_excluded" in excluded


# praxis argument parsing


def test_json_is_rejected_where_a_nested_subcommand_would_ignore_it(capsys):
    with pytest.raises(SystemExit):
        main(["executors", "--json", "discover"])

    assert "--json" in capsys.readouterr().err


def test_top_level_parser_stores_no_unread_subcommand_destination():
    args = _build_parser().parse_args(["executors"])

    assert not hasattr(args, "command")


# Lazy import of the adapter modules on the version-only path


_PROBE = """
import json, os, sys, types

watched = [
    "praxis_cli.discover_cmd",
    "praxis_cli.status_cmd",
    "praxis_cli.match_cmd",
    "praxis_cli.fields",
    "praxis_executors.adapters.claude_cli",
    "praxis_executors.adapters.ollama",
]

if os.environ.get("PRAXIS_STUB_ADAPTERS"):
    # Keeps the probe hermetic: main() must still import praxis_cli.adapters
    # lazily, but the stub never touches a real CLI or socket.
    stub = types.ModuleType("praxis_cli.adapters")
    stub.build_adapters = lambda: {}
    sys.modules["praxis_cli.adapters"] = stub
else:
    watched.append("praxis_cli.adapters")

import praxis_cli.main as m

before = [name for name in watched if name in sys.modules]
code = m.main(sys.argv[1:] or None)
after = [name for name in watched if name in sys.modules]
sys.stderr.write(json.dumps({"before": before, "after": after, "code": code}))
"""


def _probe(*argv: str, stub_adapters: bool = False) -> dict:
    environment = dict(os.environ)
    if stub_adapters:
        environment["PRAXIS_STUB_ADAPTERS"] = "1"
    completed = subprocess.run(
        [sys.executable, "-c", _PROBE, *argv],
        capture_output=True,
        text=True,
        check=True,
        env=environment,
    )
    return {**json.loads(completed.stderr), "stdout": completed.stdout}


def test_version_path_never_imports_the_adapter_modules():
    result = _probe("--version")

    assert result["before"] == []
    assert result["after"] == []
    assert result["code"] == 0
    assert result["stdout"].strip() != ""


def test_executors_path_imports_the_command_modules_on_demand():
    result = _probe("executors", "--json", stub_adapters=True)

    assert result["before"] == []
    assert "praxis_cli.status_cmd" in result["after"]
    assert result["code"] == 0
    assert json.loads(result["stdout"]) == []

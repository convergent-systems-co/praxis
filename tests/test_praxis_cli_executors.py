"""End-to-end tests for `main()`'s `executors` dispatch.

`praxis_cli.adapters.build_adapters` is stubbed out in every routing test
here. The real mapping holds a `claude` CLI adapter and an Ollama adapter, and
building a status or discover row calls `.health()`/`.capabilities()` on both
-- which would start a real `claude --version` subprocess and open a real
socket to 127.0.0.1:11434 on every run of the standard suite. What those tests
are actually about is routing: which command module `main()` hands the built
adapters to, and what it does with `--json`. The adapter behaviour behind each
route is covered per-module in test_cli_discover.py / test_cli_status.py /
test_cli_match.py.

The one exception is
`test_the_four_real_adapters_degrade_rather_than_crash_run_discover`, which
drives the real four adapter classes through `run_discover` with the
environment patched empty -- the end-to-end proof that a missing backing CLI
or service degrades its own row instead of taking the command down.
"""

from __future__ import annotations

import json
import os
import re
import shutil
import subprocess
import sys
import urllib.error
import urllib.request

import pytest

from praxis_cli import adapters as adapters_module
from praxis_cli.adapters import build_adapters as real_build_adapters
from praxis_cli.discover_cmd import run_discover
from praxis_cli.main import _build_parser, main
from praxis_executors.interface import Executor, ExecutorAvailability, ExecutorError

_SPEC_VERSION = "1.0.0"
_STATUS_KEYS = {"executor_id", "auth_transport", "status", "capabilities"}


class _StubExecutor(Executor):
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


@pytest.fixture(autouse=True)
def stub_adapters(monkeypatch):
    """Four adapters, one of them unreachable -- the same mix the real
    mapping has, without touching the environment."""

    def build_adapters() -> dict[str, Executor]:
        return {
            "executor-a": _StubExecutor("executor-a", "coding"),
            "executor-b": _StubExecutor("executor-b", "reasoning"),
            "executor-c": _StubExecutor("executor-c", "coding"),
            "executor-d": _StubExecutor("executor-d", None),
        }

    monkeypatch.setattr(adapters_module, "build_adapters", build_adapters)


def test_discover_returns_zero_and_prints_all_four_executors(capsys):
    exit_code = main(["executors", "discover"])

    captured = capsys.readouterr()
    assert exit_code == 0
    # Each block opens with an unindented `<executor_id>:` line; an indented
    # line is one of that block's fields.
    header_lines = [
        line for line in captured.out.splitlines() if line.endswith(":") and line[:1] != " "
    ]
    assert header_lines == ["executor-a:", "executor-b:", "executor-c:", "executor-d:"]


def test_bare_executors_prints_a_header_and_one_line_per_executor(capsys):
    exit_code = main(["executors"])

    captured = capsys.readouterr()
    assert exit_code == 0
    lines = captured.out.splitlines()
    assert lines[0].startswith("EXECUTOR_ID")
    assert len(lines) == 5


def test_executors_json_prints_four_status_objects(capsys):
    exit_code = main(["executors", "--json"])

    captured = capsys.readouterr()
    assert exit_code == 0
    rows = json.loads(captured.out)
    assert isinstance(rows, list)
    assert len(rows) == 4
    for row in rows:
        assert set(row) == _STATUS_KEYS


def test_json_before_a_nested_subcommand_is_rejected_rather_than_ignored(capsys):
    with pytest.raises(SystemExit):
        main(["executors", "--json", "discover"])

    assert "--json" in capsys.readouterr().err


def test_top_level_parser_stores_no_unread_subcommand_destination():
    # The outer subparsers group carries no `dest`, so `executors` never lands
    # a second attribute nothing downstream reads.
    args = _build_parser().parse_args(["executors"])

    assert not hasattr(args, "command")


def test_match_with_capability_and_explain_does_not_crash(capsys):
    exit_code = main(["executors", "match", "--capability", "coding", "--explain"])

    captured = capsys.readouterr()
    assert exit_code == 0
    # "eligible=" only ever comes from match_cmd's --explain output, so this
    # fails if main() mis-routes "executors match" to the status branch.
    assert "eligible=" in captured.out


def test_the_four_real_adapters_degrade_rather_than_crash_run_discover(monkeypatch, capsys):
    """The one test here that drives the real adapter classes, not stubs.

    `real_build_adapters` is bound at import, so the autouse stub above does
    not reach it. The environment is made deterministically empty rather than
    left to chance -- no `claude` on PATH, nothing answering on Ollama's port
    -- which is exactly the condition "degrade gracefully" is about, and keeps
    the test from starting a subprocess or opening a real socket.
    """
    monkeypatch.setattr(shutil, "which", lambda name: None)

    def _refuse(*args, **kwargs):
        raise urllib.error.URLError("connection refused")

    monkeypatch.setattr(urllib.request, "urlopen", _refuse)

    exit_code = run_discover(real_build_adapters())

    captured = capsys.readouterr()
    assert exit_code == 0
    header_lines = [
        line for line in captured.out.splitlines() if line.endswith(":") and line[:1] != " "
    ]
    assert header_lines == [
        "executor-subprocess-1:",
        "executor-fake-1:",
        "executor-claude-cli-1:",
        "executor-ollama-1:",
    ]
    # The absent backing service degrades its own row and nothing else: the
    # two built-in adapters still report their real capability kinds.
    assert "  capabilities: unavailable (" in captured.out
    assert "  capabilities: code-execution" in captured.out
    assert "  capabilities: text-generation" in captured.out


def test_no_subcommand_still_prints_version(capsys):
    exit_code = main(["--version"])

    captured = capsys.readouterr()
    assert exit_code == 0
    assert re.match(r"\d+\.\d+\.\d+", captured.out.strip())


# The version-only path must not pay for the executor machinery. Checked in a
# subprocess because the modules are already imported in this one.


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

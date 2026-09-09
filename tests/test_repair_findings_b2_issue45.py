"""Reproduces the two repair findings for bundle b2-issue45.

1. `discover_cmd.py` and `status_cmd.py` each spelled the fallback
   executor-id string `f"{type(executor).__name__}#{index}"` verbatim, so the
   two commands' display ids could drift apart independently. Both must route
   through one shared helper in `praxis_cli.fields` -- the module that already
   owns every other field both commands render.

2. `praxis_cli.main` imported `praxis_cli.adapters` (and transitively all four
   `praxis_executors.adapters` modules) at module load, even though the legacy
   `--version`/no-subcommand path never uses them.
"""

from __future__ import annotations

import json
import subprocess
import sys

from praxis_cli import discover_cmd, fields, status_cmd
from praxis_executors.adapters.fake import FakeCapabilityExecutor
from praxis_executors.interface import ExecutorError


class _RaisingFakeExecutor(FakeCapabilityExecutor):
    """Recognized by `fields.py`, but its advertisement cannot be read -- so
    both commands must fall back to a synthetic executor id."""

    def __init__(self) -> None:
        super().__init__(executor_id="executor-fake-bad", capabilities=[], script={})

    def capabilities(self) -> dict:
        raise ExecutorError("capability probe failed")


# Finding 1 -- duplicated fallback executor-id string


def test_fallback_executor_id_is_single_sourced_across_both_commands():
    assert discover_cmd.fallback_executor_id is fields.fallback_executor_id
    assert status_cmd.fallback_executor_id is fields.fallback_executor_id


def test_both_commands_render_the_shared_fallback_executor_id():
    executor = _RaisingFakeExecutor()
    expected = fields.fallback_executor_id(executor, 0)

    assert discover_cmd.build_discover_rows([executor])[0]["executor_id"] == expected
    assert status_cmd.build_status_rows([executor])[0]["executor_id"] == expected


# Finding 2 -- eager adapter import on the legacy path

_PROBE = """
import json, sys
import praxis_cli.main as m

lazy = (
    "praxis_cli.adapters",
    "praxis_cli.discover_cmd",
    "praxis_cli.status_cmd",
    "praxis_cli.match_cmd",
    "praxis_cli.fields",
    "praxis_executors.adapters.claude_cli",
    "praxis_executors.adapters.ollama",
)
after_import = [name for name in lazy if name in sys.modules]
code = m.main(sys.argv[1:] or None)
after_call = [name for name in lazy if name in sys.modules]
sys.stderr.write(json.dumps({"import": after_import, "call": after_call, "code": code}))
"""


def _probe(*argv: str) -> dict:
    completed = subprocess.run(
        [sys.executable, "-c", _PROBE, *argv],
        capture_output=True,
        text=True,
        check=True,
    )
    return {**json.loads(completed.stderr), "stdout": completed.stdout}


def test_legacy_version_path_never_imports_the_adapter_modules():
    result = _probe("--version")

    assert result["import"] == []
    assert result["call"] == []
    assert result["code"] == 0
    assert result["stdout"].strip() != ""


def test_executors_path_still_imports_the_adapter_modules_on_demand():
    result = _probe("executors", "--json")

    assert result["import"] == []
    assert "praxis_cli.adapters" in result["call"]
    assert result["code"] == 0
    assert isinstance(json.loads(result["stdout"]), list)

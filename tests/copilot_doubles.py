"""Subprocess doubles for `CopilotCliExecutor`, shared by the copilot test modules.

They live here rather than in conftest.py so that importing the copilot adapter
stays the concern of the modules that test it, instead of something every test
session in the repo pays for -- the same reason `tests/codex_doubles.py` gives.

`check_real_copilot_cli_version_probe_and_health` is the real-CLI smoke test's
body, here so another module can drive it for skip-behaviour tests without
importing a test module and looking a test up by name.
"""

from __future__ import annotations

import shutil
from unittest.mock import MagicMock, patch

import pytest

from praxis_executors.adapters.copilot_cli import CopilotCliExecutor
from praxis_executors.interface import (
    ExecutionHandle,
    ExecutionRequest,
    ExecutionResult,
    ExecutorAvailability,
)

COPILOT_MODULE = "praxis_executors.adapters.copilot_cli"

_SPEC_VERSION = "1.0.0"


def copilot_mock_process(
    returncode: int | None, stdout: str = "", stderr: str = ""
) -> MagicMock:
    process = MagicMock()
    process.poll.return_value = returncode
    process.communicate.return_value = (stdout, stderr)
    process.returncode = returncode
    return process


def copilot_launched(
    process: MagicMock,
    parameters: dict | None = None,
    executor_id: str = "executor-copilot-cli-1",
) -> tuple[CopilotCliExecutor, ExecutionHandle]:
    """(executor, handle) for a launch whose `Popen` is `process`."""
    with (
        patch(f"{COPILOT_MODULE}.shutil.which", return_value="/usr/bin/copilot"),
        patch(f"{COPILOT_MODULE}.subprocess.Popen", return_value=process),
    ):
        executor = CopilotCliExecutor(executor_id=executor_id)
        request = ExecutionRequest(
            promise={"spec_version": _SPEC_VERSION, "kind": "coding"},
            parameters={"prompt": "hello"} if parameters is None else parameters,
        )
        return executor, executor.launch(request)


def copilot_result_of_a_run(
    stdout: str, stderr: str, returncode: int = 0
) -> ExecutionResult:
    """The `ExecutionResult` of a scripted run that printed `stdout`/`stderr`."""
    executor, handle = copilot_launched(
        copilot_mock_process(returncode, stdout, stderr)
    )
    return executor.result(handle)


def check_real_copilot_cli_version_probe_and_health() -> None:
    """The real-CLI smoke test's body: the version probe answers, health() maps it.

    Strictly non-destructive: the only real process this spawns is the
    executable's own `--version`, which starts no agent run and spends no
    subscription credit.

    Asserting membership in the whole `ExecutorAvailability` enum would pass
    even if the real probe raised `OSError` and fell through to None, so it
    could only ever fail by raising. These assertions instead pin that the real
    version probe answers, and that health() maps a present, self-identifying
    executable whose authentication is undeterminable to DEGRADED -- the
    outcome the adapter's module header records for copilot 1.0.83, which
    exposes no read-only auth-status probe.

    A build that answers no version probe is the installation's real answer,
    not a defect this test can pin, so it skips.
    """
    cli_path = shutil.which("copilot")
    executor = CopilotCliExecutor(executor_id="executor-copilot-cli-smoke")

    if executor._probe_version(cli_path) is None:
        pytest.skip(
            "the real `copilot --version` probe did not answer, so there is "
            "nothing version-neutral left to pin"
        )
    assert executor.health() == ExecutorAvailability.DEGRADED

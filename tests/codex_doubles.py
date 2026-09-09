"""Subprocess doubles for `CodexCliExecutor`, shared by the codex test modules.

test_codex_cli.py and test_repair_findings_b1_issue41.py both drive the adapter
through a scripted `Popen`, and each once kept its own copy. They live here
rather than in conftest.py so that importing the codex adapter stays the
concern of the two modules that test it, instead of something every test
session in the repo pays for.

`check_real_codex_cli_auth_probe_and_health` is the real-CLI smoke test's body,
here so the repair-findings module can drive it for its own skip-behaviour
tests without importing another test module and looking a test up by name.
"""

from __future__ import annotations

import shutil
from unittest.mock import MagicMock, patch

import pytest

from praxis_executors.adapters.codex_cli import CodexCliExecutor
from praxis_executors.interface import (
    ExecutionRequest,
    ExecutionResult,
    ExecutorAvailability,
)

CODEX_MODULE = "praxis_executors.adapters.codex_cli"

_SPEC_VERSION = "1.0.0"


def codex_mock_process(
    returncode: int | None, stdout: str = "", stderr: str = ""
) -> MagicMock:
    process = MagicMock()
    process.poll.return_value = returncode
    process.communicate.return_value = (stdout, stderr)
    process.returncode = returncode
    return process


def codex_launched(
    process: MagicMock,
    parameters: dict | None = None,
    executor_id: str = "executor-codex-cli-1",
) -> tuple[CodexCliExecutor, object]:
    """(executor, handle) for a launch whose `Popen` is `process`."""
    with (
        patch(f"{CODEX_MODULE}.shutil.which", return_value="/usr/bin/codex"),
        patch(f"{CODEX_MODULE}.subprocess.Popen", return_value=process),
    ):
        executor = CodexCliExecutor(executor_id=executor_id)
        request = ExecutionRequest(
            promise={"spec_version": _SPEC_VERSION, "kind": "coding"},
            parameters={"prompt": "hello"} if parameters is None else parameters,
        )
        return executor, executor.launch(request)


def codex_result_of_a_run(
    stdout: str, stderr: str, returncode: int = 0
) -> ExecutionResult:
    """The `ExecutionResult` of a scripted run that printed `stdout`/`stderr`."""
    executor, handle = codex_launched(codex_mock_process(returncode, stdout, stderr))
    return executor.result(handle)


def check_real_codex_cli_auth_probe_and_health() -> None:
    """The real-CLI smoke test's body: the probe answers, and health() maps it.

    Asserting membership in the whole `ExecutorAvailability` enum would pass even
    if both real probes raised `OSError` and the auth detection fell through to
    None, so it could only ever fail by raising. These assertions instead pin
    that the real `codex login status` probe answers -- never the None
    fall-through -- and that health() maps that answer the way the mocked tests
    say it should. Both hold whether or not this machine's CLI is logged in.

    A codex build without the `login status` subcommand, or one that renames
    "Logged in using ChatGPT", answers with neither recognized phrase: that is
    the installation's real answer, not a failure to pin without coupling the
    suite to one CLI version's English wording.
    """
    executor = CodexCliExecutor(executor_id="executor-codex-cli-smoke")

    authenticated = executor._detect_authenticated(shutil.which("codex"))

    if authenticated is None:
        pytest.skip(
            "the real `codex login status` probe did not answer (unrecognized "
            "output on this codex version), so there is nothing version-neutral "
            "left to pin"
        )
    if authenticated and executor._probe_version(shutil.which("codex")) is None:
        # health() caps a silent executable at DEGRADED however well the login
        # went, so on such a machine the AVAILABLE assertion below would fail on
        # a machine condition rather than on a defect. Both probes spawn the same
        # binary, so this is close to unreachable -- but the spec asked this test
        # to skip, not fail, whenever conditions are not met.
        pytest.skip(
            "the real `codex --version` probe did not answer, so health() caps "
            "at DEGRADED and there is no AVAILABLE outcome to pin"
        )
    assert executor.health() == (
        ExecutorAvailability.AVAILABLE if authenticated else ExecutorAvailability.UNAVAILABLE
    )

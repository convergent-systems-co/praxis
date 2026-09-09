"""Tests for ClaudeCliExecutor, the Claude subscription-CLI Executor adapter.

Mocks at the subprocess boundary (`shutil.which`, `subprocess.Popen`,
`subprocess.run`) so no real `claude` process is invoked, except for the
optional skipif-guarded smoke test at the bottom of this file.
"""

from __future__ import annotations

import re
import shutil
from unittest.mock import MagicMock, patch

import pytest

import praxis_executors.adapters.claude_cli as claude_cli
from praxis_contracts.schema_paths import SCHEMA_DIR as SCHEMAS_DIR
from praxis_contracts.validator import validate_document
from praxis_executors.adapters.claude_cli import ClaudeCliExecutor
from praxis_executors.interface import (
    ExecutionHandle,
    ExecutionRequest,
    ExecutorAvailability,
    ExecutorError,
    ExecutorStatus,
)

FAKE_SECRET = "sk-ant-api03-FAKESECRETFAKESECRETFAKE"


def _executor() -> ClaudeCliExecutor:
    return ClaudeCliExecutor(executor_id="executor-claude-cli-1")


def _mock_process(returncode: int, stdout: str = "", stderr: str = "") -> MagicMock:
    process = MagicMock()
    process.poll.return_value = returncode
    process.communicate.return_value = (stdout, stderr)
    process.returncode = returncode
    return process


# module docstring


def test_module_docstring_cites_current_schema_location():
    docstring = claude_cli.__doc__

    assert re.search(r"(?<!praxis_contracts/)schemas/v1/capability", docstring) is None
    assert "src/praxis_contracts/schemas/v1/capability-advertisement.schema.json" in docstring
    assert "src/praxis_contracts/schemas/v1/capability.schema.json" in docstring


# capabilities()


def test_capabilities_advertisement_validates_against_schema():
    executor = _executor()

    advertisement = executor.capabilities()

    validate_document(advertisement, SCHEMAS_DIR / "capability-advertisement.schema.json")


def test_capabilities_satisfies_all_four_kinds_with_subscription_cli_auth_transport():
    executor = _executor()

    advertisement = executor.capabilities()

    (capability,) = advertisement["capabilities"]
    assert {entry["kind"] for entry in capability["satisfies"]} == {
        "coding",
        "reasoning",
        "tools",
        "filesystem",
    }
    assert capability["auth_transport"] == "subscription_cli"


def test_capabilities_does_not_touch_shutil_or_subprocess():
    with (
        patch("praxis_executors.adapters.claude_cli.shutil.which") as mock_which,
        patch("praxis_executors.adapters.claude_cli.subprocess.Popen") as mock_popen,
        patch("praxis_executors.adapters.claude_cli.subprocess.run") as mock_run,
    ):
        executor = _executor()
        executor.capabilities()

    mock_which.assert_not_called()
    mock_popen.assert_not_called()
    mock_run.assert_not_called()


# health()


def test_health_unavailable_when_cli_absent():
    with patch("praxis_executors.adapters.claude_cli.shutil.which", return_value=None):
        executor = _executor()
        assert executor.health() == ExecutorAvailability.UNAVAILABLE


def test_health_available_when_cli_present_and_authenticated():
    with (
        patch("praxis_executors.adapters.claude_cli.shutil.which", return_value="/usr/bin/claude"),
        patch("praxis_executors.adapters.claude_cli.subprocess.run"),
        patch.object(ClaudeCliExecutor, "_detect_authenticated", return_value=True),
    ):
        executor = _executor()
        assert executor.health() == ExecutorAvailability.AVAILABLE


def test_health_unavailable_when_cli_present_and_not_authenticated():
    # "Never fall back" guarantee: an unauthenticated CLI must resolve
    # straight to UNAVAILABLE -- there is no other branch health() can take.
    with (
        patch("praxis_executors.adapters.claude_cli.shutil.which", return_value="/usr/bin/claude"),
        patch("praxis_executors.adapters.claude_cli.subprocess.run"),
        patch.object(ClaudeCliExecutor, "_detect_authenticated", return_value=False),
    ):
        executor = _executor()
        assert executor.health() == ExecutorAvailability.UNAVAILABLE


def test_health_degraded_when_authentication_unknown():
    with (
        patch("praxis_executors.adapters.claude_cli.shutil.which", return_value="/usr/bin/claude"),
        patch("praxis_executors.adapters.claude_cli.subprocess.run"),
        patch.object(ClaudeCliExecutor, "_detect_authenticated", return_value=None),
    ):
        executor = _executor()
        assert executor.health() == ExecutorAvailability.DEGRADED


def test_detect_authenticated_unmocked_returns_none_by_default():
    # Proves the "no guess" default is actually wired in, not just mockable:
    # with only shutil.which/subprocess.run patched (not _detect_authenticated
    # itself), the real implementation must still report unknown.
    with (
        patch("praxis_executors.adapters.claude_cli.shutil.which", return_value="/usr/bin/claude"),
        patch("praxis_executors.adapters.claude_cli.subprocess.run"),
    ):
        executor = _executor()
        assert executor._detect_authenticated("/usr/bin/claude") is None


def test_health_invokes_claude_version_via_subprocess_run():
    with (
        patch("praxis_executors.adapters.claude_cli.shutil.which", return_value="/usr/bin/claude"),
        patch("praxis_executors.adapters.claude_cli.subprocess.run") as mock_run,
    ):
        executor = _executor()
        executor.health()

    assert mock_run.called
    argv = mock_run.call_args.args[0]
    assert argv[-1] == "--version"


# launch()


def test_launch_without_prompt_parameter_raises_executor_error():
    executor = _executor()
    request = ExecutionRequest(
        promise={"spec_version": "1.0.0", "kind": "coding"},
        parameters={},
    )

    with pytest.raises(ExecutorError):
        executor.launch(request)


def test_launch_raises_and_skips_popen_when_cli_absent():
    with (
        patch("praxis_executors.adapters.claude_cli.shutil.which", return_value=None),
        patch("praxis_executors.adapters.claude_cli.subprocess.Popen") as mock_popen,
    ):
        executor = _executor()
        request = ExecutionRequest(
            promise={"spec_version": "1.0.0", "kind": "coding"},
            parameters={"prompt": "hello"},
        )

        with pytest.raises(ExecutorError):
            executor.launch(request)

        mock_popen.assert_not_called()


def test_scripted_successful_run_reaches_succeeded_with_true_evidence():
    process = _mock_process(returncode=0, stdout="ok", stderr="")
    with (
        patch("praxis_executors.adapters.claude_cli.shutil.which", return_value="/usr/bin/claude"),
        patch("praxis_executors.adapters.claude_cli.subprocess.Popen", return_value=process),
    ):
        executor = _executor()
        request = ExecutionRequest(
            promise={"spec_version": "1.0.0", "kind": "coding"},
            parameters={"prompt": "hello"},
        )
        handle = executor.launch(request)

        assert executor.status(handle) == ExecutorStatus.SUCCEEDED
        assert executor.result(handle).evidence == {"process-exit-status": True}


def test_scripted_nonzero_exit_run_reaches_failed_with_false_evidence():
    process = _mock_process(returncode=1, stdout="", stderr="boom")
    with (
        patch("praxis_executors.adapters.claude_cli.shutil.which", return_value="/usr/bin/claude"),
        patch("praxis_executors.adapters.claude_cli.subprocess.Popen", return_value=process),
    ):
        executor = _executor()
        request = ExecutionRequest(
            promise={"spec_version": "1.0.0", "kind": "coding"},
            parameters={"prompt": "hello"},
        )
        handle = executor.launch(request)

        assert executor.status(handle) == ExecutorStatus.FAILED
        assert executor.result(handle).evidence == {"process-exit-status": False}


def test_cancel_on_running_process_terminates_and_reaches_cancelled():
    process = MagicMock()
    process.poll.return_value = None  # still running
    process.communicate.return_value = ("", "")
    with (
        patch("praxis_executors.adapters.claude_cli.shutil.which", return_value="/usr/bin/claude"),
        patch("praxis_executors.adapters.claude_cli.subprocess.Popen", return_value=process),
    ):
        executor = _executor()
        request = ExecutionRequest(
            promise={"spec_version": "1.0.0", "kind": "coding"},
            parameters={"prompt": "hello"},
        )
        handle = executor.launch(request)

        executor.cancel(handle)

        process.terminate.assert_called_once()
        # Simulate the OS finishing the terminated process for status polling.
        process.poll.return_value = -15
        process.returncode = -15
        assert executor.status(handle) == ExecutorStatus.CANCELLED


def test_status_result_cancel_each_raise_executor_error_for_unknown_handle():
    executor = _executor()
    unknown_handle = ExecutionHandle(handle_id="no-such-handle")

    with pytest.raises(ExecutorError):
        executor.status(unknown_handle)

    with pytest.raises(ExecutorError):
        executor.result(unknown_handle)

    with pytest.raises(ExecutorError):
        executor.cancel(unknown_handle)


# Credential safety (Clarified AC 7)


def test_result_redacts_credential_shaped_secret_from_evidence_and_payload():
    process = _mock_process(
        returncode=0,
        stdout=f"...{FAKE_SECRET}...",
        stderr=f"...{FAKE_SECRET}...",
    )
    with (
        patch("praxis_executors.adapters.claude_cli.shutil.which", return_value="/usr/bin/claude"),
        patch("praxis_executors.adapters.claude_cli.subprocess.Popen", return_value=process),
    ):
        executor = _executor()
        request = ExecutionRequest(
            promise={"spec_version": "1.0.0", "kind": "coding"},
            parameters={"prompt": "hello"},
        )
        handle = executor.launch(request)
        result = executor.result(handle)

    assert FAKE_SECRET not in str(result.evidence)
    assert FAKE_SECRET not in result.payload["stdout"]
    assert FAKE_SECRET not in result.payload["stderr"]


def test_launch_failure_redacts_credential_shaped_secret_from_error_message():
    with (
        patch("praxis_executors.adapters.claude_cli.shutil.which", return_value="/usr/bin/claude"),
        patch(
            "praxis_executors.adapters.claude_cli.subprocess.Popen",
            side_effect=OSError(f"launch failed: {FAKE_SECRET}"),
        ),
    ):
        executor = _executor()
        request = ExecutionRequest(
            promise={"spec_version": "1.0.0", "kind": "coding"},
            parameters={"prompt": "hello"},
        )

        with pytest.raises(ExecutorError) as exc_info:
            executor.launch(request)

    assert FAKE_SECRET not in str(exc_info.value)


# Optional real-CLI smoke test


@pytest.mark.skipif(shutil.which("claude") is None, reason="claude CLI not installed")
def test_smoke_real_cli_health_returns_a_valid_availability():
    executor = ClaudeCliExecutor(executor_id="executor-claude-cli-smoke")

    assert executor.health() in {
        ExecutorAvailability.AVAILABLE,
        ExecutorAvailability.DEGRADED,
        ExecutorAvailability.UNAVAILABLE,
    }

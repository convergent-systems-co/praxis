"""Tests for SubprocessExecutor, the first real (non-fake) Executor adapter.

Launches actual OS subprocesses via `sys.executable -c ...` so behavior is
proven against a real process lifecycle rather than a scripted double.
"""

from __future__ import annotations

import sys
import time
from pathlib import Path

import pytest

from praxis_contracts.validator import validate_document
from praxis_executors.adapters.subprocess_executor import SubprocessExecutor
from praxis_executors.interface import (
    ExecutionRequest,
    ExecutorError,
    ExecutorStatus,
)

REPO_ROOT = Path(__file__).resolve().parent.parent
SCHEMAS_DIR = REPO_ROOT / "schemas" / "v1"


def _executor() -> SubprocessExecutor:
    return SubprocessExecutor(executor_id="executor-subprocess-1", satisfies_kinds=["code-execution"])


def _wait_for_terminal(executor: SubprocessExecutor, handle, timeout: float = 5.0) -> ExecutorStatus:
    deadline = time.monotonic() + timeout
    status = executor.status(handle)
    while status not in (
        ExecutorStatus.SUCCEEDED,
        ExecutorStatus.FAILED,
        ExecutorStatus.CANCELLED,
    ):
        if time.monotonic() > deadline:
            raise AssertionError(f"execution did not reach a terminal state within {timeout}s")
        time.sleep(0.05)
        status = executor.status(handle)
    return status


def test_successful_subprocess_reaches_succeeded_with_true_evidence():
    executor = _executor()
    request = ExecutionRequest(
        promise={"spec_version": "1.0.0", "kind": "code-execution"},
        parameters={"command": [sys.executable, "-c", "print('ok')"]},
    )

    handle = executor.launch(request)

    assert _wait_for_terminal(executor, handle) == ExecutorStatus.SUCCEEDED
    assert executor.result(handle).evidence == {"process-exit-status": True}


def test_nonzero_exit_subprocess_reaches_failed_with_false_evidence():
    executor = _executor()
    request = ExecutionRequest(
        promise={"spec_version": "1.0.0", "kind": "code-execution"},
        parameters={"command": [sys.executable, "-c", "import sys; sys.exit(1)"]},
    )

    handle = executor.launch(request)

    assert _wait_for_terminal(executor, handle) == ExecutorStatus.FAILED
    assert executor.result(handle).evidence == {"process-exit-status": False}


def test_launch_without_command_parameter_raises_executor_error():
    executor = _executor()
    request = ExecutionRequest(
        promise={"spec_version": "1.0.0", "kind": "code-execution"},
        parameters={},
    )

    with pytest.raises(ExecutorError):
        executor.launch(request)


def test_cancel_on_long_running_process_reaches_terminal_state_promptly():
    executor = _executor()
    request = ExecutionRequest(
        promise={"spec_version": "1.0.0", "kind": "code-execution"},
        parameters={"command": [sys.executable, "-c", "import time; time.sleep(30)"]},
    )
    handle = executor.launch(request)

    executor.cancel(handle)

    status = _wait_for_terminal(executor, handle, timeout=5.0)
    assert status in (ExecutorStatus.FAILED, ExecutorStatus.CANCELLED)


def test_capabilities_advertisement_validates_against_schema():
    executor = _executor()

    advertisement = executor.capabilities()

    validate_document(advertisement, SCHEMAS_DIR / "capability-advertisement.schema.json")

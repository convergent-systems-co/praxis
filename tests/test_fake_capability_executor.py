"""Tests for FakeCapabilityExecutor, the deterministic fixed-script Executor
adapter used to exercise the Executor interface without a real backend.

`test_capabilities_advertisement_validates_against_schema` is the one place
this bundle's tests round-trip an adapter's `capabilities()` output through
praxis_contracts's validator, proving the capability advertisement format is
actually schema-conformant rather than merely shaped-by-convention.
"""

from __future__ import annotations

from pathlib import Path

import pytest

from praxis_contracts.validator import validate_document
from praxis_executors.adapters.fake import FakeCapabilityExecutor
from praxis_executors.interface import (
    ExecutionHandle,
    ExecutionRequest,
    ExecutionResult,
    ExecutorError,
    ExecutorStatus,
)

REPO_ROOT = Path(__file__).resolve().parent.parent
SCHEMAS_DIR = REPO_ROOT / "schemas" / "v1"

_CAPABILITIES = [
    {
        "spec_version": "1.0.0",
        "id": "cap-primary",
        "satisfies": [{"kind": "text-generation"}],
    }
]


def _executor(script: dict[str, ExecutionResult]) -> FakeCapabilityExecutor:
    return FakeCapabilityExecutor(
        executor_id="executor-fake-1", capabilities=_CAPABILITIES, script=script
    )


def test_capabilities_advertisement_validates_against_schema():
    executor = _executor(script={})

    advertisement = executor.capabilities()

    validate_document(advertisement, SCHEMAS_DIR / "capability-advertisement.schema.json")


def test_launch_status_result_for_scripted_succeeded_outcome_returns_exact_result():
    scripted_result = ExecutionResult(
        status=ExecutorStatus.SUCCEEDED,
        evidence={"test-pass": True},
        payload={"output": "done"},
    )
    executor = _executor(script={"do-thing": scripted_result})
    request = ExecutionRequest(
        promise={"spec_version": "1.0.0", "kind": "do-thing"},
        parameters={"request_key": "do-thing"},
    )

    handle = executor.launch(request)

    assert executor.status(handle) == ExecutorStatus.SUCCEEDED
    assert executor.result(handle) == scripted_result


def test_launch_raises_executor_error_for_request_key_with_no_script_entry():
    executor = _executor(script={"known-key": ExecutionResult(status=ExecutorStatus.SUCCEEDED)})
    request = ExecutionRequest(
        promise={"spec_version": "1.0.0", "kind": "unscripted-key"},
        parameters={},
    )

    with pytest.raises(ExecutorError):
        executor.launch(request)


def test_status_result_cancel_each_raise_executor_error_for_unknown_handle():
    executor = _executor(script={})
    unknown_handle = ExecutionHandle(handle_id="no-such-handle")

    with pytest.raises(ExecutorError):
        executor.status(unknown_handle)

    with pytest.raises(ExecutorError):
        executor.result(unknown_handle)

    with pytest.raises(ExecutorError):
        executor.cancel(unknown_handle)


def test_cancel_is_a_no_op_for_a_known_handle():
    executor = _executor(script={"do-thing": ExecutionResult(status=ExecutorStatus.SUCCEEDED)})
    request = ExecutionRequest(
        promise={"spec_version": "1.0.0", "kind": "do-thing"},
        parameters={"request_key": "do-thing"},
    )
    handle = executor.launch(request)

    assert executor.cancel(handle) is None
    # cancel() must not disturb the scripted terminal outcome.
    assert executor.status(handle) == ExecutorStatus.SUCCEEDED
    assert executor.result(handle) == ExecutionResult(status=ExecutorStatus.SUCCEEDED)


def test_two_launches_with_same_script_return_identical_results():
    scripted_result = ExecutionResult(
        status=ExecutorStatus.SUCCEEDED,
        evidence={"test-pass": True},
        payload={"output": "done"},
    )
    executor = _executor(script={"do-thing": scripted_result})
    request = ExecutionRequest(
        promise={"spec_version": "1.0.0", "kind": "do-thing"},
        parameters={"request_key": "do-thing"},
    )

    first_handle = executor.launch(request)
    second_handle = executor.launch(request)

    assert executor.result(first_handle) == executor.result(second_handle) == scripted_result

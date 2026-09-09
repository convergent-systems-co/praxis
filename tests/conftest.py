"""Shared test fixtures for the praxis_runtime test suite.

`_linear_graph()` is the minimal two-node graph used by test_transitions.py,
test_fail_closed_cases.py, test_checkpoint_resume.py, and
test_repair_findings_b3_issue4.py -- kept here once so those suites import
the same helper instead of each defining their own copy.

`_PassthroughGrader` is the deterministic (or, via its constructor args,
model/human) stand-in grader used by test_evidence_gates.py,
test_transitions.py, test_checkpoint_resume.py, and
test_repair_findings_b3_issue4.py wherever a test wants the grader's verdict
to track whatever each record itself claims, rather than exercising the
grading algorithm -- kept here once for the same reason as `_linear_graph`.

The `_codex_*` helpers are the `CodexCliExecutor` subprocess doubles shared by
test_codex_cli.py and test_repair_findings_b1_issue41.py, which previously kept
a copy each. `_check_real_codex_cli_auth_probe_and_health` is the real-CLI
smoke test's body, here so the repair-findings module can drive it without
importing another module's test function by name.
"""

from __future__ import annotations

import shutil
from unittest.mock import MagicMock, patch

import pytest

from praxis_evidence.types import GradeResult, ProofRecord
from praxis_executors.adapters.codex_cli import CodexCliExecutor
from praxis_executors.interface import (
    ExecutionRequest,
    ExecutionResult,
    ExecutorAvailability,
)
from praxis_runtime.graph import Edge, Graph, Node

_SPEC_VERSION = "1.0.0"


def pytest_configure(config) -> None:
    config.addinivalue_line(
        "markers",
        "slow: load-sensitive or long-running; deselect with -m 'not slow' "
        "where the machine cannot give the test its timing headroom",
    )


def _linear_graph() -> Graph:
    return Graph(
        spec_version=_SPEC_VERSION,
        nodes={
            "n1": Node(id="n1", kind="task"),
            "n2": Node(id="n2", kind="task"),
        },
        edges=[Edge(source="n1", target="n2", kind="sequential")],
        entry_node="n1",
        terminal_nodes={"n2"},
    )


_CODEX_MODULE = "praxis_executors.adapters.codex_cli"


def _codex_mock_process(
    returncode: int | None, stdout: str = "", stderr: str = ""
) -> MagicMock:
    process = MagicMock()
    process.poll.return_value = returncode
    process.communicate.return_value = (stdout, stderr)
    process.returncode = returncode
    return process


def _codex_launched(
    process: MagicMock,
    parameters: dict | None = None,
    executor_id: str = "executor-codex-cli-1",
) -> tuple[CodexCliExecutor, object]:
    """(executor, handle) for a launch whose `Popen` is `process`."""
    with (
        patch(f"{_CODEX_MODULE}.shutil.which", return_value="/usr/bin/codex"),
        patch(f"{_CODEX_MODULE}.subprocess.Popen", return_value=process),
    ):
        executor = CodexCliExecutor(executor_id=executor_id)
        request = ExecutionRequest(
            promise={"spec_version": _SPEC_VERSION, "kind": "coding"},
            parameters={"prompt": "hello"} if parameters is None else parameters,
        )
        return executor, executor.launch(request)


def _codex_result_of_a_run(
    stdout: str, stderr: str, returncode: int = 0
) -> ExecutionResult:
    """The `ExecutionResult` of a scripted run that printed `stdout`/`stderr`."""
    executor, handle = _codex_launched(_codex_mock_process(returncode, stdout, stderr))
    return executor.result(handle)


def _check_real_codex_cli_auth_probe_and_health() -> None:
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


class _PassthroughGrader:
    """Mirrors the record's own submitted status/confidence -- used where the
    test wants the grader's verdict to track whatever each record claims."""

    def __init__(self, grader_kind: str = "deterministic", advisory: bool = False) -> None:
        self._grader_kind = grader_kind
        self._advisory = advisory

    def grade(self, record: ProofRecord) -> GradeResult:
        return GradeResult(
            proof_type=record.proof_type,
            status=record.status,
            confidence=record.confidence,
            grader_kind=self._grader_kind,
            advisory=self._advisory,
        )

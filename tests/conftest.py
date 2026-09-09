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

`_FakeExecutor` and `_json_decode_error()` are the `Executor`-ABC stand-in
used by test_cli_discover.py, test_cli_match.py, test_cli_status.py and
test_praxis_cli_executors.py -- kept here for the same reason again, since
the ABC obliges every one of those suites to spell out four abstract methods
none of them ever calls.

`_undecodable_output_error()` is the third such helper, shared by
test_cli_fields.py and test_cli_discover.py so the one probe failure a real
`claude` binary can raise is constructed the same way in both, and
`_MALFORMED_ADVERTISEMENTS` is the fourth, shared by test_cli_discover.py and
test_cli_status.py.
"""

from __future__ import annotations

import json

from praxis_evidence.types import GradeResult, ProofRecord
from praxis_executors.interface import Executor, ExecutorAvailability
from praxis_runtime.graph import Edge, Graph, Node

_SPEC_VERSION = "1.0.0"

# Advertisements returned by a probe that did not raise, each missing one key
# capability-advertisement.schema.json or capability.schema.json requires: the
# `capabilities` list, a capability's `satisfies` list, and a `satisfies`
# entry's `kind`. No shipped adapter emits one; a fifth adapter, or a stub,
# can, and `discover` and `status` both have to survive it -- so the three
# shapes are spelled here once for both suites.
_MALFORMED_ADVERTISEMENTS = (
    {"spec_version": _SPEC_VERSION, "executor_id": "executor-malformed"},
    {
        "spec_version": _SPEC_VERSION,
        "executor_id": "executor-malformed",
        "capabilities": [{"spec_version": _SPEC_VERSION, "auth_transport": "local"}],
    },
    {
        "spec_version": _SPEC_VERSION,
        "executor_id": "executor-malformed",
        "capabilities": [{"spec_version": _SPEC_VERSION, "satisfies": [{}]}],
    },
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


class _FakeExecutor(Executor):
    """One parameterizable stand-in for the `Executor` ABC, for the CLI suites.

    Implements the ABC directly rather than subclassing a real adapter:
    `praxis_cli.fields` degrades to `"n/a"` for a class it does not recognise
    instead of raising, which is what keeps one unknown adapter from taking a
    whole report down, and every CLI suite relies on that.

    Each probe is given as data rather than overridden per test:

    * `capabilities` -- the advertisement's `capabilities` list; defaults to
      empty, which is itself a conforming advertisement.
    * `capabilities_error` -- raised by `.capabilities()` instead, standing in
      for a backing CLI or service that could not be asked.
    * `health` -- the availability `.health()` returns. Left unset, `.health()`
      raises `NotImplementedError`, so a suite that never probes health does
      not have to name a verdict it does not mean.
    * `health_error` -- raised by `.health()` instead, standing in for a probe
      that failed rather than returning a verdict.

    `launch`/`status`/`cancel`/`result` exist only because the ABC requires
    them; no CLI test calls them.
    """

    def __init__(
        self,
        executor_id: str = "executor-fake",
        *,
        capabilities: list[dict] | None = None,
        capabilities_error: BaseException | None = None,
        health: ExecutorAvailability | None = None,
        health_error: BaseException | None = None,
    ) -> None:
        self._executor_id = executor_id
        self._capabilities = capabilities or []
        self._capabilities_error = capabilities_error
        self._health = health
        self._health_error = health_error

    def capabilities(self) -> dict:
        if self._capabilities_error is not None:
            raise self._capabilities_error
        return {
            "spec_version": _SPEC_VERSION,
            "executor_id": self._executor_id,
            "capabilities": self._capabilities,
        }

    def health(self) -> ExecutorAvailability:
        if self._health_error is not None:
            raise self._health_error
        if self._health is None:
            raise NotImplementedError
        return self._health

    def launch(self, request):
        raise NotImplementedError

    def status(self, handle):
        raise NotImplementedError

    def cancel(self, handle):
        raise NotImplementedError

    def result(self, handle):
        raise NotImplementedError


def _json_decode_error() -> ValueError:
    """A real `json.JSONDecodeError` -- a `ValueError`, not an `ExecutorError`.

    What an adapter's transport layer surfaces when a non-Ollama server
    answers 200 on the configured port with a body that is not JSON. Built by
    actually decoding rather than constructed by hand, so it carries the same
    message a real decode failure would.
    """
    try:
        json.loads("not json")
    except ValueError as exc:
        return exc
    raise AssertionError("json.loads accepted a non-JSON body")


def _undecodable_output_error() -> UnicodeDecodeError:
    """A real `UnicodeDecodeError` -- also a `ValueError`, not an `ExecutorError`.

    What `subprocess.run(..., text=True)` raises when the CLI it ran wrote
    bytes that are not valid UTF-8, which is how a `claude` binary on PATH can
    fail `ClaudeCliExecutor.health()` mid-probe: `_probe_version` catches only
    `(OSError, subprocess.TimeoutExpired)`, so the decode failure leaves the
    adapter. Built by actually decoding, for the same reason as above.
    """
    try:
        b"claude \xff\xfe".decode("utf-8")
    except UnicodeDecodeError as exc:
        return exc
    raise AssertionError("utf-8 accepted a non-utf-8 version banner")

"""Executor registry: register adapters, select one for a requirement, and
drive a selected adapter's launch/poll/result lifecycle.

This module has no dependency on praxis_runtime. `ExecutionResult.evidence`
is a flat claim dict, not the `list[dict]` of proof-record documents
`TransitionEngine.apply(node_id, event_type, evidence=...)` requires; a
caller with run/graph/node context must convert it first -- wiring that call
into `TransitionEngine.apply` is the caller's responsibility, not the
registry's. `evidence_to_proof_records` below is that reusable conversion.
`ExecutorRegistry.execute_with_proof_records` is its production caller: the
registry already knows which `executor_id` `select()` chose while executing
a request, so it performs the conversion itself instead of leaving every
caller to re-derive the winning `executor_id` out of band. The
executor-to-runtime orchestration seam lives in
`src/praxis_orchestration/escalation.py` and is documented in
`docs/orchestration.md`; the registry itself remains independent of
`praxis_runtime`.
"""

from __future__ import annotations

import time
from dataclasses import dataclass
from typing import Callable

from praxis_evidence.proof import build_proof_record
from praxis_evidence.types import proof_record_to_document

from . import matching, policy
from .interface import Executor, ExecutionRequest, ExecutionResult, ExecutorAvailability, ExecutorStatus
from .telemetry import (
    ExecutionTelemetry,
    build_execution_candidate_config,
    build_execution_evaluation_record,
    build_execution_event_document,
)

_TERMINAL_STATUSES = frozenset(
    {ExecutorStatus.SUCCEEDED, ExecutorStatus.FAILED, ExecutorStatus.CANCELLED}
)


@dataclass(frozen=True)
class ExecutionOutcome:
    """All non-persistent records produced for one measured execution."""

    executor_id: str
    result: ExecutionResult
    proof_records: list[dict]
    telemetry: ExecutionTelemetry
    evaluation_record: object
    event_document: dict


class RegistryError(Exception):
    """Raised for registry-level failures: identity conflicts or no selection."""


class ExecutorRegistry:
    """Tracks registered Executors and mediates selection and execution."""

    def __init__(self) -> None:
        self._executors: dict[str, Executor] = {}

    def register(self, executor_id: str, executor: Executor) -> None:
        if executor_id in self._executors:
            raise RegistryError(f"executor_id '{executor_id}' is already registered")
        self._executors[executor_id] = executor

    def unregister(self, executor_id: str) -> None:
        self._executors.pop(executor_id, None)

    def advertisements(self, *, healthy_only: bool = True) -> list[dict]:
        result = []
        for executor_id, executor in self._executors.items():
            try:
                health = executor.health()
            except Exception:
                continue
            if healthy_only and health is not ExecutorAvailability.AVAILABLE:
                continue
            result.append(executor.capabilities())
        return result

    def select(
        self,
        requirement: dict,
        *,
        is_eligible: Callable[[str], bool] | None = None,
    ) -> matching.MatchResult:
        advertisements = self.advertisements()
        if is_eligible is None:
            is_eligible = policy.as_eligibility_callable(
                policy.AuthTransportPolicy(), advertisements
            )
        return matching.match(requirement, advertisements, is_eligible=is_eligible)

    def execute(
        self,
        requirement: dict,
        request: ExecutionRequest,
        *,
        is_eligible: Callable[[str], bool] | None = None,
        poll: Callable[[], None] | None = None,
    ) -> ExecutionResult:
        _, result, _wall_seconds = self._execute_selected(
            requirement, request, is_eligible=is_eligible, poll=poll
        )
        return result

    def execute_with_proof_records(
        self,
        requirement: dict,
        request: ExecutionRequest,
        *,
        run_id: str,
        graph_version: str,
        node_id: str,
        grader_kind: str = "deterministic",
        is_eligible: Callable[[str], bool] | None = None,
        poll: Callable[[], None] | None = None,
    ) -> tuple[ExecutionResult, list[dict]]:
        """Like `execute`, but also converts the result's flat `evidence`
        claim dict into the `list[dict]` of proof-record documents
        `TransitionEngine.apply(..., evidence=...)` requires, using the
        `executor_id` this call's own `select()` actually chose -- a caller
        no longer has to re-derive that out of band.
        """
        executor_id, result, _wall_seconds = self._execute_selected(
            requirement, request, is_eligible=is_eligible, poll=poll
        )
        records = evidence_to_proof_records(
            result.evidence,
            run_id=run_id,
            graph_version=graph_version,
            node_id=node_id,
            executor_id=executor_id,
            grader_kind=grader_kind,
        )
        return result, records

    def execute_with_telemetry(
        self,
        requirement: dict,
        request: ExecutionRequest,
        *,
        run_id: str,
        graph_version: str,
        node_id: str,
        node_kind: str,
        seq: int,
        grader_kind: str = "deterministic",
        repairs: int | None = None,
        verification_passed: bool | None = None,
        failure_class: str | None = None,
        is_eligible: Callable[[str], bool] | None = None,
        poll: Callable[[], None] | None = None,
    ) -> ExecutionOutcome:
        """Execute once and return proof, evaluation, telemetry, and event records.

        `repairs`, `verification_passed`, and `failure_class` are caller-owned
        observations (from the policy ledger, evidence gate, and failure
        classifier respectively). This registry imports none of those policy
        modules and persists none of the returned documents.
        """
        executor_id, result, wall_seconds = self._execute_selected(
            requirement, request, is_eligible=is_eligible, poll=poll
        )
        proof_records = evidence_to_proof_records(
            result.evidence,
            run_id=run_id,
            graph_version=graph_version,
            node_id=node_id,
            executor_id=executor_id,
            grader_kind=grader_kind,
        )
        model = self._advertised_model(executor_id)
        telemetry = ExecutionTelemetry(
            executor_id=executor_id,
            node_id=node_id,
            node_kind=node_kind,
            status=result.status.value,
            wall_seconds=wall_seconds,
            model=model,
            repairs=repairs,
            verification_passed=verification_passed,
            failure_class=failure_class,
        )
        evaluation_record = build_execution_evaluation_record(telemetry)
        event_document = build_execution_event_document(
            telemetry, run_id=run_id, seq=seq
        )
        return ExecutionOutcome(
            executor_id=executor_id,
            result=result,
            proof_records=proof_records,
            telemetry=telemetry,
            evaluation_record=evaluation_record,
            event_document=event_document,
        )

    def _advertised_model(self, executor_id: str) -> str | None:
        """Read an optional model from the selected executor's advertisement."""
        advertisement = self._executors[executor_id].capabilities()
        for capability in advertisement.get("capabilities", []):
            for satisfied in capability.get("satisfies", []):
                parameters = satisfied.get("parameters", {})
                model = parameters.get("model")
                if isinstance(model, str):
                    return model
        return None

    def _execute_selected(
        self,
        requirement: dict,
        request: ExecutionRequest,
        *,
        is_eligible: Callable[[str], bool] | None = None,
        poll: Callable[[], None] | None = None,
    ) -> tuple[str, ExecutionResult, float]:
        result = self.select(requirement, is_eligible=is_eligible)
        if result.selected is None:
            raise RegistryError(
                f"no executor selected for requirement: {result.unsatisfied!r}"
            )

        executor_id = result.selected.executor_id
        executor = self._executors[executor_id]
        started_at = time.monotonic()
        handle = executor.launch(request)

        while True:
            current_status = executor.status(handle)
            if current_status in _TERMINAL_STATUSES:
                break
            if poll is not None:
                poll()

        # Stop at terminal status, before result() may perform a slow read or
        # decode. A monotonic clock measures elapsed duration without wall-clock
        # adjustments making the telemetry negative or inflated.
        wall_seconds = time.monotonic() - started_at
        return executor_id, executor.result(handle), wall_seconds


def evidence_to_proof_records(
    evidence: dict,
    *,
    run_id: str,
    graph_version: str,
    node_id: str,
    executor_id: str,
    grader_kind: str = "deterministic",
) -> list[dict]:
    """Convert an `ExecutionResult.evidence` flat `{proof_type: claim}` dict
    into the `list[dict]` of proof-record documents
    `TransitionEngine.apply(..., evidence=...)` requires.

    This is the run/graph/node-aware conversion `ExecutionResult`'s
    docstring describes: one proof record per claimed `proof_type`, graded
    "pass" for a truthy claim and "fail" otherwise.
    """
    return [
        proof_record_to_document(
            build_proof_record(
                run_id=run_id,
                graph_version=graph_version,
                node_id=node_id,
                proof_type=proof_type,
                executor_id=executor_id,
                grader_kind=grader_kind,
                status="pass" if claim else "fail",
            )
        )
        for proof_type, claim in evidence.items()
    ]

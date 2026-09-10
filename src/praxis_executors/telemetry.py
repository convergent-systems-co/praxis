"""Per-execution telemetry: measurements, candidate identity, record, event.

This module may import `praxis_evidence`, `praxis_eval`, `praxis_learning`
and `praxis_contracts`; it must never import `praxis_runtime`. Every
run/graph/node context it needs (`run_id`, `node_id`, `node_kind`, `seq`)
arrives as a plain value, so adapters and their telemetry stay independent
of the runtime engine -- the same rule `interface.py` and `registry.py`
already follow. Nothing here persists or dispatches anything: each function
returns a value and leaves storage and delivery to its caller.

The discipline the module exists to enforce is honest observation. An
observation the executed adapter did not actually make is expressed by the
measurement (or the payload key) being *absent* -- never by `value: 0`,
`value: null`, a `unit` of `"unknown"`, or a derived estimate. The same rule
governs `executor_configuration`, whose dict feeds a content-addressed
`candidate_id`, so an executor that genuinely has no model concept must not
collide with one whose model simply went unreported.
"""

from __future__ import annotations

import uuid
from dataclasses import dataclass

from praxis_contracts.schema_paths import schema_path
from praxis_contracts.validator import validate_document
from praxis_eval.candidates import build_candidate_config
from praxis_eval.measurements import build_evaluation_record
from praxis_eval.types import CandidateConfig, EvaluationRecord, Measurement

from .interface import ExecutorStatus

_SPEC_VERSION = "1.0.0"

# schemas/v1/event.schema.json requires exactly spec_version, seq, run_id,
# node_id, event_type, event_id and payload, and forbids any other key.
EVENT_SCHEMA_PATH = schema_path("event.schema.json")


@dataclass(frozen=True)
class ExecutionTelemetry:
    """What one execution was observed to be, as plain values.

    `status` is an `ExecutorStatus.value` string. Every optional field is
    `None` when the observation was not made, and stays out of the derived
    measurements and payloads entirely.
    """

    executor_id: str
    node_id: str
    node_kind: str
    status: str
    wall_seconds: float
    model: str | None = None
    repairs: int | None = None
    verification_passed: bool | None = None
    human_interrupts: int | None = None
    failure_class: str | None = None
    resource_measurements: tuple[Measurement, ...] = ()


def _succeeded(telemetry: ExecutionTelemetry) -> bool:
    return telemetry.status == ExecutorStatus.SUCCEEDED.value


def telemetry_measurements(telemetry: ExecutionTelemetry) -> list[Measurement]:
    """Measure what was observed, and only what was observed.

    `wall_seconds` and `success` are always available, which is what keeps
    `measurements` non-empty for the sparsest execution (the schema requires
    `minItems: 1`). Resource measurements are appended verbatim -- this
    module never synthesizes a cost or a token count.
    """
    measurements = [
        Measurement(metric="wall_seconds", value=telemetry.wall_seconds, unit="s"),
        Measurement(metric="success", value=1.0 if _succeeded(telemetry) else 0.0),
    ]
    if telemetry.repairs is not None:
        measurements.append(Measurement(metric="repairs", value=float(telemetry.repairs)))
    if telemetry.verification_passed is not None:
        measurements.append(
            Measurement(
                metric="verification_passed",
                value=1.0 if telemetry.verification_passed else 0.0,
            )
        )
    if telemetry.human_interrupts is not None:
        measurements.append(
            Measurement(metric="human_interrupts", value=float(telemetry.human_interrupts))
        )
    measurements.extend(telemetry.resource_measurements)
    return measurements


def executor_configuration(telemetry: ExecutionTelemetry) -> dict:
    """The configuration dict that identifies which executor ran the work.

    `model` is present only when the adapter really exposed one; the key is
    omitted otherwise, never set to `None` or `"unknown"`, because this dict
    is hashed into `candidate_id`. A model string never goes into
    `executor_id`, `evaluator_id` or `candidate_id`.
    """
    configuration: dict = {"executor_id": telemetry.executor_id}
    if telemetry.model is not None:
        configuration["model"] = telemetry.model
    return configuration


def build_execution_candidate_config(telemetry: ExecutionTelemetry) -> CandidateConfig:
    """Build the content-addressed candidate for this execution's routing."""
    return build_candidate_config(executor_configuration(telemetry), target="routing")


def build_execution_evaluation_record(
    telemetry: ExecutionTelemetry,
    *,
    candidate_id: str | None = None,
    evaluator_id: str = "praxis_executors.telemetry",
) -> EvaluationRecord:
    """Record this execution's measurements against its routing candidate."""
    if candidate_id is None:
        candidate_id = build_execution_candidate_config(telemetry).candidate_id
    return build_evaluation_record(
        candidate_id=candidate_id,
        # docs/eval.md's citation convention: workload_id cites an exact
        # external identifier verbatim, never a paraphrase. A live execution
        # has no corpus file, so the exact identifier is the graph node_id --
        # used unmodified, with no prefix or synthesized string.
        workload_id=telemetry.node_id,
        measurements=telemetry_measurements(telemetry),
        evaluator_id=evaluator_id,
    )


def _event_payload(telemetry: ExecutionTelemetry, *, succeeded: bool) -> dict:
    payload: dict = {
        "node_kind": telemetry.node_kind,
        "executor_id": telemetry.executor_id,
        "wall_seconds": telemetry.wall_seconds,
    }
    if telemetry.model is not None:
        payload["model"] = telemetry.model
    if telemetry.repairs is not None:
        payload["repairs"] = telemetry.repairs
    if telemetry.verification_passed is not None:
        payload["verification_passed"] = telemetry.verification_passed
    if telemetry.human_interrupts is not None:
        payload["human_interrupts"] = telemetry.human_interrupts
    for measurement in telemetry.resource_measurements:
        payload[measurement.metric] = measurement.value
    if not succeeded and telemetry.failure_class is not None:
        # praxis_learning.extraction groups recurrent failures by this key,
        # and praxis_policy.failure_classification.FailureClass is its
        # vocabulary.
        payload["failure_class"] = telemetry.failure_class
    return payload


def build_execution_event_document(
    telemetry: ExecutionTelemetry,
    *,
    run_id: str,
    seq: int,
    event_id: str | None = None,
) -> dict:
    """Emit this execution as a `"complete"`/`"fail"` event document.

    Those are the two real transition event types
    `praxis_learning.extraction` already classifies, so the outcome feeds
    the recurrent-failure and successful-recovery patterns with no change to
    `praxis_learning`. A `"measurement"` record is never emitted: that
    pattern needs an `improvement_pct` a single execution cannot observe,
    and synthesizing one would be a fabrication.
    """
    succeeded = _succeeded(telemetry)
    document = {
        "spec_version": _SPEC_VERSION,
        "seq": seq,
        "run_id": run_id,
        "node_id": telemetry.node_id,
        "event_type": "complete" if succeeded else "fail",
        "event_id": event_id or uuid.uuid4().hex,
        "payload": _event_payload(telemetry, succeeded=succeeded),
    }
    validate_document(document, EVENT_SCHEMA_PATH)
    return document

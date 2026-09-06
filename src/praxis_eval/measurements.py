"""Evaluation-record construction and validation.

`workload_id` citation convention: it must cite an exact external
workload/scenario identifier -- e.g. a `benchmark/corpus/*.md` filename --
verbatim, never a paraphrase. This mirrors the citation discipline
`benchmark/baseline/acceptance-thresholds.md` already established. The
schema only requires `workload_id` to be a string; this rule is a documented
convention (see also `docs/eval.md`), not something enforced at runtime.
"""

from __future__ import annotations

import uuid
from datetime import datetime, timezone

from praxis_contracts.validator import validate_document
from praxis_eval.types import (
    EVALUATION_RECORD_SCHEMA_PATH,
    EvaluationRecord,
    Measurement,
    evaluation_record_to_document,
)

_SPEC_VERSION = "1.0.0"


def validate_evaluation_record(document: dict) -> None:
    """Fail-closed: raises ContractValidationError unchanged on any violation."""
    validate_document(document, EVALUATION_RECORD_SCHEMA_PATH)


def _normalize_measurements(
    measurements: dict[str, float] | list[tuple[str, float]] | list[Measurement],
) -> tuple[Measurement, ...]:
    if isinstance(measurements, dict):
        normalized = tuple(
            Measurement(metric=metric, value=value) for metric, value in measurements.items()
        )
    else:
        normalized = tuple(
            item if isinstance(item, Measurement) else Measurement(metric=item[0], value=item[1])
            for item in measurements
        )
    if not normalized:
        raise ValueError("measurements must not be empty")
    return normalized


def build_evaluation_record(
    *,
    candidate_id: str,
    workload_id: str,
    measurements: dict[str, float] | list[tuple[str, float]] | list[Measurement],
    baseline_candidate_id: str | None = None,
    evaluator_id: str | None = None,
    produced_at: str | None = None,
    evaluation_id: str | None = None,
) -> EvaluationRecord:
    record = EvaluationRecord(
        spec_version=_SPEC_VERSION,
        evaluation_id=evaluation_id or uuid.uuid4().hex,
        candidate_id=candidate_id,
        workload_id=workload_id,
        measurements=_normalize_measurements(measurements),
        produced_at=produced_at or datetime.now(timezone.utc).isoformat(),
        baseline_candidate_id=baseline_candidate_id,
        evaluator_id=evaluator_id,
    )
    validate_evaluation_record(evaluation_record_to_document(record))
    return record

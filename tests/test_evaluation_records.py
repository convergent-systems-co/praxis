"""Evaluation-record construction and validation.

Covers the three accepted `measurements` input shapes normalizing to
equivalent `Measurement` tuples, the fail-closed empty-measurements rule,
and the workload_id citation rule being a documented convention (checked
only for type: string by the schema) rather than a runtime non-empty check.
"""

from __future__ import annotations

import pytest

from praxis_contracts.validator import ContractValidationError
from praxis_eval.measurements import build_evaluation_record, validate_evaluation_record
from praxis_eval.types import EvaluationRecord, Measurement, evaluation_record_to_document

_WORKLOAD_ID = "benchmark/corpus/example.md"


def test_build_evaluation_record_round_trips_for_each_measurement_input_shape():
    dict_measurements = {"wall_seconds": 1.5, "cost": 0.2}
    tuple_measurements = [("wall_seconds", 1.5), ("cost", 0.2)]
    measurement_objects = [
        Measurement(metric="wall_seconds", value=1.5),
        Measurement(metric="cost", value=0.2),
    ]
    expected_measurements = (
        Measurement(metric="wall_seconds", value=1.5),
        Measurement(metric="cost", value=0.2),
    )

    for measurements_input in (dict_measurements, tuple_measurements, measurement_objects):
        record = build_evaluation_record(
            candidate_id="c1",
            workload_id=_WORKLOAD_ID,
            measurements=measurements_input,
        )

        assert isinstance(record, EvaluationRecord)
        assert record.measurements == expected_measurements
        assert record.spec_version == "1.0.0"
        assert record.evaluation_id
        assert record.produced_at

        # Should not raise: build_evaluation_record already validated, but
        # re-validating the emitted document proves the shape really is
        # schema-conformant.
        validate_evaluation_record(evaluation_record_to_document(record))


@pytest.mark.parametrize("empty_measurements", [{}, [], ()])
def test_build_evaluation_record_raises_on_empty_measurements(empty_measurements):
    with pytest.raises(ValueError):
        build_evaluation_record(
            candidate_id="c1",
            workload_id=_WORKLOAD_ID,
            measurements=empty_measurements,
        )


def test_build_evaluation_record_does_not_enforce_workload_id_citation_at_runtime():
    # workload_id="" is schema-valid (the schema only requires type: string);
    # the "cite an exact external identifier, never a paraphrase" rule is a
    # documented convention (docs/eval.md), not a runtime check.
    record = build_evaluation_record(
        candidate_id="c1",
        workload_id="",
        measurements={"cost": 1.0},
    )

    assert record.workload_id == ""
    validate_evaluation_record(evaluation_record_to_document(record))


def test_validate_evaluation_record_raises_on_non_string_workload_id():
    document = {
        "spec_version": "1.0.0",
        "evaluation_id": "eval-1",
        "candidate_id": "c1",
        "workload_id": 12345,
        "measurements": [{"metric": "cost", "value": 1.0}],
        "produced_at": "2026-01-01T00:00:00+00:00",
    }

    with pytest.raises(ContractValidationError):
        validate_evaluation_record(document)


def test_build_evaluation_record_accepts_arbitrary_open_metric_and_evaluator_id():
    record = build_evaluation_record(
        candidate_id="c1",
        workload_id=_WORKLOAD_ID,
        measurements={"totally-made-up-metric": 3.14},
        evaluator_id="totally-made-up-evaluator-id",
    )

    assert record.measurements[0].metric == "totally-made-up-metric"
    assert record.evaluator_id == "totally-made-up-evaluator-id"
    validate_evaluation_record(evaluation_record_to_document(record))

"""Tests for `praxis_executors.telemetry`, the per-execution telemetry builders.

These cover the honest-observation discipline the module exists to enforce: an
observation the adapter could not actually make is *omitted*, never filled in
with `0`, `None` or an `"unknown"` unit. The same rule applies to the
configuration dict that feeds the content-addressed `candidate_id`, so a
model-exposing executor and a model-silent one must not collide.

Extraction/classification behaviour is deliberately not asserted here; it
belongs to `tests/test_executor_telemetry_learning.py`.
"""

from __future__ import annotations

import pytest

from praxis_contracts.schema_paths import schema_path
from praxis_contracts.validator import validate_document
from praxis_eval.types import Measurement, evaluation_record_to_document
from praxis_executors.interface import ExecutorStatus
from praxis_executors.telemetry import (
    ExecutionTelemetry,
    build_execution_candidate_config,
    build_execution_evaluation_record,
    build_execution_event_document,
    executor_configuration,
    telemetry_measurements,
)

EVENT_SCHEMA = schema_path("event.schema.json")
EVALUATION_RECORD_SCHEMA = schema_path("evaluation-record.schema.json")


def _telemetry(**overrides) -> ExecutionTelemetry:
    fields = {
        "executor_id": "executor-1",
        "node_id": "n1",
        "node_kind": "implement",
        "status": ExecutorStatus.SUCCEEDED.value,
        "wall_seconds": 12.5,
    }
    fields.update(overrides)
    return ExecutionTelemetry(**fields)


def _metrics(measurements) -> dict[str, float]:
    return {m.metric: m.value for m in measurements}


# --- telemetry_measurements: always-present observations ---------------------


def test_wall_seconds_and_success_are_always_measured():
    measurements = telemetry_measurements(_telemetry())

    by_metric = {m.metric: m for m in measurements}
    assert by_metric["wall_seconds"].value == 12.5
    assert by_metric["wall_seconds"].unit == "s"
    assert by_metric["success"].value == 1.0


def test_success_is_zero_for_a_non_succeeded_status():
    measurements = telemetry_measurements(_telemetry(status=ExecutorStatus.FAILED.value))

    assert _metrics(measurements)["success"] == 0.0


# --- telemetry_measurements: omission of unobserved fields -------------------


def test_unobserved_optional_fields_are_omitted_entirely():
    measurements = telemetry_measurements(_telemetry())

    metrics = _metrics(measurements)
    assert "repairs" not in metrics
    assert "verification_passed" not in metrics
    assert "human_interrupts" not in metrics


@pytest.mark.parametrize(
    ("field", "value", "metric", "expected"),
    [
        ("repairs", 0, "repairs", 0.0),
        ("repairs", 3, "repairs", 3.0),
        ("verification_passed", True, "verification_passed", 1.0),
        ("verification_passed", False, "verification_passed", 0.0),
        ("human_interrupts", 0, "human_interrupts", 0.0),
        ("human_interrupts", 2, "human_interrupts", 2.0),
    ],
)
def test_an_observed_optional_field_is_measured(field, value, metric, expected):
    measurements = telemetry_measurements(_telemetry(**{field: value}))

    assert _metrics(measurements)[metric] == expected


def test_no_measurement_carries_an_unknown_unit_or_null_value():
    measurements = telemetry_measurements(
        _telemetry(repairs=1, verification_passed=False, human_interrupts=1)
    )

    for measurement in measurements:
        assert measurement.value is not None
        assert measurement.unit != "unknown"


def test_resource_measurements_are_appended_verbatim():
    tokens = Measurement(metric="tokens", value=1200.0, unit="tokens")
    measurements = telemetry_measurements(_telemetry(resource_measurements=(tokens,)))

    assert tokens in measurements


def test_no_cost_measurement_is_synthesized_without_a_resource_observation():
    measurements = telemetry_measurements(_telemetry())

    assert "cost" not in _metrics(measurements)


# --- executor_configuration: the content-addressed identity ------------------


def test_configuration_omits_the_model_key_when_no_model_is_exposed():
    configuration = executor_configuration(_telemetry())

    assert configuration == {"executor_id": "executor-1"}
    assert "model" not in configuration


def test_configuration_carries_the_model_key_when_a_model_is_exposed():
    configuration = executor_configuration(_telemetry(model="some-model-v2"))

    assert configuration["model"] == "some-model-v2"
    assert configuration["executor_id"] == "executor-1"


def test_model_exposing_and_model_silent_configurations_hash_differently():
    with_model = build_execution_candidate_config(_telemetry(model="some-model-v2"))
    without_model = build_execution_candidate_config(_telemetry())

    assert with_model.candidate_id != without_model.candidate_id


def test_the_same_configuration_always_hashes_to_the_same_candidate_id():
    first = build_execution_candidate_config(_telemetry(model="some-model-v2"))
    second = build_execution_candidate_config(_telemetry(model="some-model-v2", wall_seconds=99.0))

    assert first.candidate_id == second.candidate_id


def test_candidate_config_targets_routing_and_carries_the_configuration():
    config = build_execution_candidate_config(_telemetry(model="some-model-v2"))

    assert config.target == "routing"
    assert config.configuration == {"executor_id": "executor-1", "model": "some-model-v2"}


def test_no_model_string_leaks_into_candidate_id_or_evaluator_id():
    telemetry = _telemetry(model="some-model-v2")
    record = build_execution_evaluation_record(telemetry)

    assert "some-model-v2" not in record.candidate_id
    assert "some-model-v2" not in (record.evaluator_id or "")


# --- build_execution_evaluation_record ---------------------------------------


def test_workload_id_is_the_node_id_verbatim():
    record = build_execution_evaluation_record(_telemetry(node_id="graph/node-7"))

    assert record.workload_id == "graph/node-7"


def test_evaluation_record_defaults_its_candidate_id_to_the_execution_candidate():
    telemetry = _telemetry(model="some-model-v2")
    record = build_execution_evaluation_record(telemetry)

    assert record.candidate_id == build_execution_candidate_config(telemetry).candidate_id


def test_an_explicit_candidate_id_overrides_the_default():
    record = build_execution_evaluation_record(_telemetry(), candidate_id="candidate-x")

    assert record.candidate_id == "candidate-x"


def test_the_sparsest_execution_still_produces_a_schema_valid_record():
    # A cancelled run: no verification result, no repairs, no resource usage.
    record = build_execution_evaluation_record(
        _telemetry(status=ExecutorStatus.CANCELLED.value, wall_seconds=0.4)
    )

    document = evaluation_record_to_document(record)
    validate_document(document, EVALUATION_RECORD_SCHEMA)
    assert len(document["measurements"]) >= 1
    assert {m["metric"] for m in document["measurements"]} == {"wall_seconds", "success"}


# --- build_execution_event_document ------------------------------------------


def test_a_succeeded_execution_produces_a_valid_complete_event():
    document = build_execution_event_document(_telemetry(), run_id="run-1", seq=7)

    validate_document(document, EVENT_SCHEMA)
    assert document["event_type"] == "complete"
    assert document["run_id"] == "run-1"
    assert document["seq"] == 7
    assert document["node_id"] == "n1"
    assert document["event_id"]


@pytest.mark.parametrize(
    "status",
    [ExecutorStatus.FAILED.value, ExecutorStatus.CANCELLED.value],
)
def test_a_non_succeeded_execution_produces_a_valid_fail_event(status):
    document = build_execution_event_document(
        _telemetry(status=status, failure_class="timeout"), run_id="run-1", seq=2
    )

    validate_document(document, EVENT_SCHEMA)
    assert document["event_type"] == "fail"
    assert document["payload"]["failure_class"] == "timeout"


def test_an_explicit_event_id_is_used_and_defaults_are_unique():
    given = build_execution_event_document(
        _telemetry(), run_id="run-1", seq=1, event_id="event-9"
    )
    first = build_execution_event_document(_telemetry(), run_id="run-1", seq=1)
    second = build_execution_event_document(_telemetry(), run_id="run-1", seq=1)

    assert given["event_id"] == "event-9"
    assert first["event_id"] != second["event_id"]


def test_the_event_payload_carries_the_observed_telemetry():
    document = build_execution_event_document(
        _telemetry(
            model="some-model-v2",
            repairs=2,
            verification_passed=True,
            human_interrupts=1,
            resource_measurements=(Measurement(metric="tokens", value=1200.0, unit="tokens"),),
        ),
        run_id="run-1",
        seq=3,
    )

    payload = document["payload"]
    assert payload["node_kind"] == "implement"
    assert payload["executor_id"] == "executor-1"
    assert payload["wall_seconds"] == 12.5
    assert payload["model"] == "some-model-v2"
    assert payload["repairs"] == 2
    assert payload["verification_passed"] is True
    assert payload["human_interrupts"] == 1
    assert payload["tokens"] == 1200.0


def test_the_event_payload_omits_every_unobserved_key():
    document = build_execution_event_document(_telemetry(), run_id="run-1", seq=3)

    payload = document["payload"]
    for key in ("model", "repairs", "verification_passed", "human_interrupts", "cost"):
        assert key not in payload


def test_a_complete_event_carries_no_failure_class():
    document = build_execution_event_document(
        _telemetry(failure_class="timeout"), run_id="run-1", seq=3
    )

    assert document["event_type"] == "complete"
    assert "failure_class" not in document["payload"]


def test_the_event_never_synthesizes_a_measurement_record_or_improvement():
    document = build_execution_event_document(_telemetry(), run_id="run-1", seq=3)

    assert document["event_type"] != "measurement"
    assert "improvement_pct" not in document["payload"]

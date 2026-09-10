"""Human answers are explicit evidence, not inferred executor outcomes."""

from praxis_contracts.schema_paths import schema_path
from praxis_contracts.validator import validate_document
from praxis_executors.human_decision import (
    HumanDecision,
    build_human_proof_record,
    human_decision_telemetry,
    human_denial_events,
)


def test_human_verdict_produces_human_graded_proof_and_telemetry():
    decision = HumanDecision("review-1", "pass", "human-reviewer", escalations=2)
    record = build_human_proof_record(
        decision, run_id="run-1", graph_version="1.0.0", proof_type="peer-attestation"
    )
    validate_document(record, schema_path("proof-record.schema.json"))
    assert record["grader_kind"] == "human"
    assert record["status"] == "pass"
    assert record["executor_id"] == "human-reviewer"
    telemetry = human_decision_telemetry(decision, node_kind="review", wall_seconds=2.0)
    assert telemetry.human_interrupts == 2
    assert human_denial_events() == ["accept", "fail"]


def test_invalid_or_missing_human_verdict_never_produces_a_record():
    try:
        build_human_proof_record(
            HumanDecision("review-1", "", "human-reviewer"),
            run_id="run-1", graph_version="1.0.0", proof_type="peer-attestation",
        )
    except ValueError:
        pass
    else:
        raise AssertionError("missing human verdict must not create evidence")

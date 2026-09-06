"""Shared data shapes for the candidate/evaluation/promotion subsystem.

CandidateConfig mirrors candidate-config.schema.json, EvaluationRecord mirrors
evaluation-record.schema.json, PromotionPolicy mirrors
promotion-policy.schema.json, and PromotionRecord mirrors
promotion-record.schema.json. The *_to_document/*_from_document functions
convert between each dataclass and the plain-dict document shape that
praxis_contracts.validator.validate_document validates, following the
convention in praxis_evidence/types.py.

MetricComparison and PromotionGateResult have no backing schema file -- they
are never validated against or built from an external document.
"""

from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path

SCHEMA_DIR = Path(__file__).resolve().parent.parent.parent / "schemas" / "v1"
CANDIDATE_CONFIG_SCHEMA_PATH = SCHEMA_DIR / "candidate-config.schema.json"
EVALUATION_RECORD_SCHEMA_PATH = SCHEMA_DIR / "evaluation-record.schema.json"
PROMOTION_POLICY_SCHEMA_PATH = SCHEMA_DIR / "promotion-policy.schema.json"
PROMOTION_RECORD_SCHEMA_PATH = SCHEMA_DIR / "promotion-record.schema.json"


@dataclass(frozen=True)
class CandidateConfig:
    spec_version: str
    candidate_id: str
    configuration: dict
    created_at: str
    parent_candidate_id: str | None = None
    target: str | None = None
    description: str | None = None


@dataclass(frozen=True)
class Measurement:
    metric: str
    value: float
    unit: str | None = None


@dataclass(frozen=True)
class EvaluationRecord:
    spec_version: str
    evaluation_id: str
    candidate_id: str
    workload_id: str
    measurements: tuple[Measurement, ...]
    produced_at: str
    baseline_candidate_id: str | None = None
    evaluator_id: str | None = None


@dataclass(frozen=True)
class MetricThreshold:
    metric: str
    constraint: str
    direction: str
    max_regression_pct: float | None = None


@dataclass(frozen=True)
class PromotionPolicy:
    spec_version: str
    thresholds: tuple[MetricThreshold, ...]
    name: str | None = None
    authority_requirement: dict | None = None


@dataclass(frozen=True)
class MetricComparison:
    metric: str
    constraint: str
    candidate_value: float | None
    baseline_value: float | None
    status: str
    reason: str | None = None


@dataclass(frozen=True)
class PromotionGateResult:
    candidate_id: str
    satisfied: bool
    reasons: tuple[str, ...]
    evaluated: tuple[str, ...]


@dataclass(frozen=True)
class PromotionRecord:
    spec_version: str
    record_id: str
    seq: int
    action: str
    candidate_id: str
    decision: str
    produced_at: str
    previous_candidate_id: str | None = None
    reasons: tuple[str, ...] = ()
    evaluation_ids: tuple[str, ...] = ()
    authority_outcome: str | None = None


def candidate_config_to_document(config: CandidateConfig) -> dict:
    document: dict = {
        "spec_version": config.spec_version,
        "candidate_id": config.candidate_id,
        "configuration": dict(config.configuration),
        "created_at": config.created_at,
    }
    if config.parent_candidate_id is not None:
        document["parent_candidate_id"] = config.parent_candidate_id
    if config.target is not None:
        document["target"] = config.target
    if config.description is not None:
        document["description"] = config.description
    return document


def candidate_config_from_document(doc: dict) -> CandidateConfig:
    return CandidateConfig(
        spec_version=doc["spec_version"],
        candidate_id=doc["candidate_id"],
        configuration=dict(doc["configuration"]),
        created_at=doc["created_at"],
        parent_candidate_id=doc.get("parent_candidate_id"),
        target=doc.get("target"),
        description=doc.get("description"),
    )


def _measurement_to_document(measurement: Measurement) -> dict:
    document: dict = {"metric": measurement.metric, "value": measurement.value}
    if measurement.unit is not None:
        document["unit"] = measurement.unit
    return document


def _measurement_from_document(doc: dict) -> Measurement:
    return Measurement(metric=doc["metric"], value=doc["value"], unit=doc.get("unit"))


def evaluation_record_to_document(record: EvaluationRecord) -> dict:
    document: dict = {
        "spec_version": record.spec_version,
        "evaluation_id": record.evaluation_id,
        "candidate_id": record.candidate_id,
        "workload_id": record.workload_id,
        "measurements": [_measurement_to_document(m) for m in record.measurements],
        "produced_at": record.produced_at,
    }
    if record.baseline_candidate_id is not None:
        document["baseline_candidate_id"] = record.baseline_candidate_id
    if record.evaluator_id is not None:
        document["evaluator_id"] = record.evaluator_id
    return document


def evaluation_record_from_document(doc: dict) -> EvaluationRecord:
    return EvaluationRecord(
        spec_version=doc["spec_version"],
        evaluation_id=doc["evaluation_id"],
        candidate_id=doc["candidate_id"],
        workload_id=doc["workload_id"],
        measurements=tuple(
            _measurement_from_document(m) for m in doc["measurements"]
        ),
        produced_at=doc["produced_at"],
        baseline_candidate_id=doc.get("baseline_candidate_id"),
        evaluator_id=doc.get("evaluator_id"),
    )


def _metric_threshold_to_document(threshold: MetricThreshold) -> dict:
    document: dict = {
        "metric": threshold.metric,
        "constraint": threshold.constraint,
        "direction": threshold.direction,
    }
    if threshold.max_regression_pct is not None:
        document["max_regression_pct"] = threshold.max_regression_pct
    return document


def _metric_threshold_from_document(doc: dict) -> MetricThreshold:
    return MetricThreshold(
        metric=doc["metric"],
        constraint=doc["constraint"],
        direction=doc["direction"],
        max_regression_pct=doc.get("max_regression_pct"),
    )


def promotion_policy_to_document(policy: PromotionPolicy) -> dict:
    document: dict = {
        "spec_version": policy.spec_version,
        "thresholds": [_metric_threshold_to_document(t) for t in policy.thresholds],
    }
    if policy.name is not None:
        document["name"] = policy.name
    if policy.authority_requirement is not None:
        document["authority_requirement"] = dict(policy.authority_requirement)
    return document


def promotion_policy_from_document(doc: dict) -> PromotionPolicy:
    return PromotionPolicy(
        spec_version=doc["spec_version"],
        thresholds=tuple(
            _metric_threshold_from_document(t) for t in doc["thresholds"]
        ),
        name=doc.get("name"),
        authority_requirement=doc.get("authority_requirement"),
    )


def promotion_record_to_document(record: PromotionRecord) -> dict:
    document: dict = {
        "spec_version": record.spec_version,
        "record_id": record.record_id,
        "seq": record.seq,
        "action": record.action,
        "candidate_id": record.candidate_id,
        "decision": record.decision,
        "produced_at": record.produced_at,
    }
    if record.previous_candidate_id is not None:
        document["previous_candidate_id"] = record.previous_candidate_id
    if record.reasons:
        document["reasons"] = list(record.reasons)
    if record.evaluation_ids:
        document["evaluation_ids"] = list(record.evaluation_ids)
    if record.authority_outcome is not None:
        document["authority_outcome"] = record.authority_outcome
    return document


def promotion_record_from_document(doc: dict) -> PromotionRecord:
    return PromotionRecord(
        spec_version=doc["spec_version"],
        record_id=doc["record_id"],
        seq=doc["seq"],
        action=doc["action"],
        candidate_id=doc["candidate_id"],
        decision=doc["decision"],
        produced_at=doc["produced_at"],
        previous_candidate_id=doc.get("previous_candidate_id"),
        reasons=tuple(doc.get("reasons", ())),
        evaluation_ids=tuple(doc.get("evaluation_ids", ())),
        authority_outcome=doc.get("authority_outcome"),
    )

"""Shared data shapes for the evidence/proof subsystem.

ProofRecord and GateResult mirror proof-record.schema.json and
gate-result.schema.json respectively; the *_to_document/_from_document
functions convert between the dataclasses and the plain-dict document shape
that praxis_contracts.validator.validate_document validates, following the
convention in praxis_runtime/state.py.
"""

from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path

SCHEMA_DIR = Path(__file__).resolve().parent.parent.parent / "schemas" / "v1"
PROOF_RECORD_SCHEMA_PATH = SCHEMA_DIR / "proof-record.schema.json"
GATE_RESULT_SCHEMA_PATH = SCHEMA_DIR / "gate-result.schema.json"


@dataclass(frozen=True)
class ProofRecord:
    spec_version: str
    proof_id: str
    run_id: str
    graph_version: str
    node_id: str
    proof_type: str
    executor_id: str
    grader_kind: str
    status: str
    confidence: float | None = None
    artifacts: tuple[dict, ...] = ()
    inputs: tuple[str, ...] = ()
    produced_at: str | None = None


@dataclass(frozen=True)
class GradeResult:
    proof_type: str
    status: str
    confidence: float | None
    grader_kind: str
    advisory: bool
    reason: str | None = None


@dataclass(frozen=True)
class GateResult:
    node_id: str
    satisfied: bool
    reasons: tuple[str, ...]
    evaluated: tuple[str, ...]


def proof_record_to_document(record: ProofRecord) -> dict:
    document: dict = {
        "spec_version": record.spec_version,
        "proof_id": record.proof_id,
        "run_id": record.run_id,
        "graph_version": record.graph_version,
        "node_id": record.node_id,
        "proof_type": record.proof_type,
        "executor_id": record.executor_id,
        "grader_kind": record.grader_kind,
        "status": record.status,
    }
    if record.confidence is not None:
        document["confidence"] = record.confidence
    if record.artifacts:
        document["artifacts"] = [dict(artifact) for artifact in record.artifacts]
    if record.inputs:
        document["inputs"] = list(record.inputs)
    if record.produced_at is not None:
        document["produced_at"] = record.produced_at
    return document


def proof_record_from_document(doc: dict) -> ProofRecord:
    return ProofRecord(
        spec_version=doc["spec_version"],
        proof_id=doc["proof_id"],
        run_id=doc["run_id"],
        graph_version=doc["graph_version"],
        node_id=doc["node_id"],
        proof_type=doc["proof_type"],
        executor_id=doc["executor_id"],
        grader_kind=doc["grader_kind"],
        status=doc["status"],
        confidence=doc.get("confidence"),
        artifacts=tuple(doc.get("artifacts", ())),
        inputs=tuple(doc.get("inputs", ())),
        produced_at=doc.get("produced_at"),
    )

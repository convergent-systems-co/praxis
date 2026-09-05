"""Proof record construction and validation.

No secrets by default: proof-record.schema.json is closed
(additionalProperties: false at both the document and artifact level) and
artifacts carry only references (uri/digest/media_type), never inline
content. An executor cannot smuggle secret material into a proof record
through an extra field, and there is no content payload for scanning logic
to inspect in the first place — so this module intentionally contains none.
"""

from __future__ import annotations

import uuid
from datetime import datetime, timezone

from praxis_contracts.validator import validate_document
from praxis_evidence.types import (
    PROOF_RECORD_SCHEMA_PATH,
    ProofRecord,
    proof_record_to_document,
)

_SPEC_VERSION = "1.0.0"


def validate_proof_record(document: dict) -> None:
    """Fail-closed: raises ContractValidationError unchanged on any violation."""
    validate_document(document, PROOF_RECORD_SCHEMA_PATH)


def build_proof_record(
    *,
    run_id: str,
    graph_version: str,
    node_id: str,
    proof_type: str,
    executor_id: str,
    grader_kind: str,
    status: str,
    confidence: float | None = None,
    artifacts: list[dict] | None = None,
    inputs: list[str] | None = None,
    produced_at: str | None = None,
) -> ProofRecord:
    record = ProofRecord(
        spec_version=_SPEC_VERSION,
        proof_id=uuid.uuid4().hex,
        run_id=run_id,
        graph_version=graph_version,
        node_id=node_id,
        proof_type=proof_type,
        executor_id=executor_id,
        grader_kind=grader_kind,
        status=status,
        confidence=confidence,
        artifacts=tuple(artifacts or ()),
        inputs=tuple(inputs or ()),
        produced_at=produced_at or datetime.now(timezone.utc).isoformat(),
    )
    validate_proof_record(proof_record_to_document(record))
    return record

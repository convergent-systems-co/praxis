"""Rollback to the previously accepted configuration.

rollback() deliberately does **not** re-run evaluate_candidate's gate/authority
checks -- it is a safety-restoration mechanism for a candidate that was
already ACCEPTED once before, not a new promotion, so re-gating it here would
defeat its purpose as a fallback when something has already gone wrong.
"""

from __future__ import annotations

import uuid
from datetime import datetime, timezone

from praxis_eval.candidates import CandidateRegistry
from praxis_eval.ledger import PromotionLedger
from praxis_eval.types import PromotionRecord

_ACCEPTED = "accepted"


class RollbackError(Exception):
    """Raised when there is no previous accepted configuration to restore."""


def rollback(ledger: PromotionLedger, registry: CandidateRegistry, *, reason: str) -> PromotionRecord:
    records = ledger.read_all()
    accepted = [r for r in records if r.decision == _ACCEPTED]

    if len(accepted) < 2:
        raise RollbackError(
            "no previous accepted configuration to restore: fewer than two "
            "accepted records in the ledger"
        )

    target_candidate_id = accepted[-2].candidate_id

    if registry.get(target_candidate_id) is None:
        raise RollbackError(
            f"cannot roll back to candidate_id {target_candidate_id!r}: not found in registry"
        )

    record = PromotionRecord(
        spec_version="1.0.0",
        record_id=uuid.uuid4().hex,
        seq=0,
        action="rollback",
        candidate_id=target_candidate_id,
        previous_candidate_id=accepted[-1].candidate_id,
        decision=_ACCEPTED,
        reasons=(reason,),
        evaluation_ids=(),
        authority_outcome=None,
        produced_at=datetime.now(timezone.utc).isoformat(),
    )

    return ledger.append(record)

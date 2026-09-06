"""Promotion orchestration: gate + authority + append-only accept/reject decision.

`evaluate_candidate` composes T5's `comparison.compare_measurements`, T7's
`gates.evaluate_promotion_gate`, and `praxis_policy.authority.evaluate_authority`
into a single `PromotionDecision`. The health/regression gate is authoritative
over whether authority is even consulted: an unsatisfied gate short-circuits
straight to `REJECTED` without ever constructing an authority decision -- no
reason to ask a human to approve a candidate that already failed its
objective checks.

`promote()` is the structural enforcement of "a candidate cannot become
active without recorded evaluation evidence": it raises `PromotionError`
fail-closed for anything other than an `ACCEPTED` decision, for an
`ACCEPTED` decision missing `evaluation_ids`, and for a `candidate_id` never
registered in the `CandidateRegistry`.

A `HUMAN_REQUIRED` or `REJECTED` `PromotionDecision` is never passed to
`promote()` by this module itself -- the caller (a future orchestrator, out
of this bundle's scope) is responsible for routing a `HUMAN_REQUIRED`
decision to an actual human approval step and only calling `promote()` again
with a decision that has since become `ACCEPTED`.
"""

from __future__ import annotations

import enum
import uuid
from dataclasses import dataclass
from datetime import datetime, timezone
from typing import TYPE_CHECKING

import praxis_policy.authority
from praxis_eval import comparison, gates
from praxis_eval.candidates import CandidateRegistry
from praxis_eval.ledger import PromotionLedger
from praxis_eval.types import Measurement, PromotionGateResult, PromotionPolicy, PromotionRecord

if TYPE_CHECKING:
    import praxis_policy.profiles

_SPEC_VERSION = "1.0.0"


class PromotionOutcome(enum.Enum):
    ACCEPTED = "accepted"
    REJECTED = "rejected"
    HUMAN_REQUIRED = "human_required"


@dataclass(frozen=True)
class PromotionDecision:
    outcome: PromotionOutcome
    candidate_id: str
    gate_result: PromotionGateResult
    authority_outcome: str | None
    reasons: tuple[str, ...]


class PromotionError(Exception):
    """Raised when promote() cannot fail-closed-safely promote a decision."""


def evaluate_candidate(
    *,
    candidate_id: str,
    candidate_measurements: tuple[Measurement, ...],
    baseline_measurements: tuple[Measurement, ...] | None,
    policy: PromotionPolicy,
    profile: "praxis_policy.profiles.PolicyProfile",
    granted_scopes: frozenset[str] = frozenset(),
) -> PromotionDecision:
    comparisons = comparison.compare_measurements(
        candidate_measurements, baseline_measurements, policy
    )
    gate_result = gates.evaluate_promotion_gate(candidate_id, comparisons)

    if not gate_result.satisfied:
        return PromotionDecision(
            outcome=PromotionOutcome.REJECTED,
            candidate_id=candidate_id,
            gate_result=gate_result,
            authority_outcome=None,
            reasons=gate_result.reasons,
        )

    authority_decision = praxis_policy.authority.evaluate_authority(
        policy.authority_requirement, profile, granted_scopes=granted_scopes
    )

    outcome_map = {
        praxis_policy.authority.AuthorityOutcome.AUTO_APPROVED: PromotionOutcome.ACCEPTED,
        praxis_policy.authority.AuthorityOutcome.HUMAN_REQUIRED: PromotionOutcome.HUMAN_REQUIRED,
        praxis_policy.authority.AuthorityOutcome.DENIED: PromotionOutcome.REJECTED,
    }
    outcome = outcome_map[authority_decision.outcome]

    reasons = gate_result.reasons
    if authority_decision.outcome is praxis_policy.authority.AuthorityOutcome.HUMAN_REQUIRED:
        scopes = ", ".join(sorted(authority_decision.unresolved_scopes))
        reasons = gate_result.reasons + (f"authority scope(s) require human approval: {scopes}",)
    elif authority_decision.outcome is praxis_policy.authority.AuthorityOutcome.DENIED:
        scopes = ", ".join(sorted(authority_decision.denied_scopes))
        reasons = gate_result.reasons + (f"authority scope(s) denied: {scopes}",)

    return PromotionDecision(
        outcome=outcome,
        candidate_id=candidate_id,
        gate_result=gate_result,
        authority_outcome=authority_decision.outcome.value,
        reasons=reasons,
    )


def promote(
    ledger: PromotionLedger,
    registry: CandidateRegistry,
    decision: PromotionDecision,
    *,
    evaluation_ids: list[str],
) -> PromotionRecord:
    if decision.outcome is not PromotionOutcome.ACCEPTED:
        raise PromotionError(
            f"cannot promote candidate_id {decision.candidate_id!r}: "
            f"decision outcome is {decision.outcome.value!r}, not accepted"
        )

    if not evaluation_ids:
        raise PromotionError(
            f"cannot promote candidate_id {decision.candidate_id!r}: "
            "no evaluation_ids cited as evidence"
        )

    if registry.get(decision.candidate_id) is None:
        raise PromotionError(
            f"cannot promote candidate_id {decision.candidate_id!r}: not found in registry"
        )

    previous = ledger.active_candidate_id()

    record = PromotionRecord(
        spec_version=_SPEC_VERSION,
        record_id=uuid.uuid4().hex,
        seq=0,  # placeholder: ledger.append() assigns the real seq
        action="promote",
        candidate_id=decision.candidate_id,
        previous_candidate_id=previous,
        decision="accepted",
        reasons=decision.reasons,
        evaluation_ids=tuple(evaluation_ids),
        authority_outcome=decision.authority_outcome,
        produced_at=datetime.now(timezone.utc).isoformat(),
    )

    return ledger.append(record)

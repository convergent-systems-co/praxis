"""Tests for rollback() -- restoring the previously accepted candidate.

rollback() walks the ledger's accepted records and targets accepted[-2] (the
accepted record immediately before the current one), so it is a proper
stack-like walk-back rather than "always the first candidate ever accepted."
It raises RollbackError fail-closed when there is no previous accepted
configuration to restore (fewer than two accepted records), or when the
target candidate_id can no longer be resolved via the registry.
"""

from __future__ import annotations

from pathlib import Path

import pytest

from praxis_eval.candidates import CandidateRegistry, build_candidate_config
from praxis_eval.ledger import PromotionLedger
from praxis_eval.rollback import RollbackError, rollback


def _promote(ledger: PromotionLedger, *, record_id: str, candidate_id: str) -> None:
    from praxis_eval.types import PromotionRecord

    ledger.append(
        PromotionRecord(
            spec_version="1.0.0",
            record_id=record_id,
            seq=0,
            action="promote",
            candidate_id=candidate_id,
            decision="accepted",
            produced_at="2026-09-06T00:00:00Z",
        )
    )


def test_rollback_raises_with_only_one_accepted_record_ever(tmp_path: Path) -> None:
    ledger = PromotionLedger(tmp_path / "ledger")
    registry = CandidateRegistry(tmp_path / "registry")
    candidate_a = build_candidate_config({"alpha": 1})
    registry.register(candidate_a)
    _promote(ledger, record_id="rec-1", candidate_id=candidate_a.candidate_id)

    with pytest.raises(RollbackError):
        rollback(ledger, registry, reason="operator requested rollback")


def test_rollback_raises_with_no_accepted_records(tmp_path: Path) -> None:
    ledger = PromotionLedger(tmp_path / "ledger")
    registry = CandidateRegistry(tmp_path / "registry")

    with pytest.raises(RollbackError):
        rollback(ledger, registry, reason="operator requested rollback")


def test_rollback_restores_previous_candidate_and_updates_active_id(
    tmp_path: Path,
) -> None:
    ledger = PromotionLedger(tmp_path / "ledger")
    registry = CandidateRegistry(tmp_path / "registry")
    candidate_a = build_candidate_config({"alpha": 1})
    candidate_b = build_candidate_config({"alpha": 2})
    registry.register(candidate_a)
    registry.register(candidate_b)
    _promote(ledger, record_id="rec-1", candidate_id=candidate_a.candidate_id)
    _promote(ledger, record_id="rec-2", candidate_id=candidate_b.candidate_id)

    result = rollback(ledger, registry, reason="candidate B regressed")

    assert result.action == "rollback"
    assert result.candidate_id == candidate_a.candidate_id
    assert result.previous_candidate_id == candidate_b.candidate_id
    assert result.decision == "accepted"
    assert result.reasons == ("candidate B regressed",)
    assert ledger.active_candidate_id() == candidate_a.candidate_id


def test_rollback_after_rollback_walks_back_to_the_prior_accepted_record(
    tmp_path: Path,
) -> None:
    ledger = PromotionLedger(tmp_path / "ledger")
    registry = CandidateRegistry(tmp_path / "registry")
    candidate_a = build_candidate_config({"alpha": 1})
    candidate_b = build_candidate_config({"alpha": 2})
    registry.register(candidate_a)
    registry.register(candidate_b)
    _promote(ledger, record_id="rec-1", candidate_id=candidate_a.candidate_id)
    _promote(ledger, record_id="rec-2", candidate_id=candidate_b.candidate_id)
    rollback(ledger, registry, reason="candidate B regressed")

    second_result = rollback(ledger, registry, reason="undo the rollback too")

    assert second_result.candidate_id == candidate_b.candidate_id
    assert second_result.previous_candidate_id == candidate_a.candidate_id
    assert ledger.active_candidate_id() == candidate_b.candidate_id


def test_rollback_raises_when_target_candidate_is_not_in_registry(
    tmp_path: Path,
) -> None:
    ledger = PromotionLedger(tmp_path / "ledger")
    registry = CandidateRegistry(tmp_path / "registry")
    candidate_b = build_candidate_config({"alpha": 2})
    registry.register(candidate_b)
    _promote(ledger, record_id="rec-1", candidate_id="unregistered-candidate")
    _promote(ledger, record_id="rec-2", candidate_id=candidate_b.candidate_id)

    with pytest.raises(RollbackError):
        rollback(ledger, registry, reason="operator requested rollback")

    assert ledger.active_candidate_id() == candidate_b.candidate_id

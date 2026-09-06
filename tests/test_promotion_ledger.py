"""Tests for the append-only promotion/rollback ledger.

PromotionLedger persists PromotionRecords as JSONL, assigning `seq` itself
(never trusting a caller-supplied value) and rejecting a duplicate
`record_id` outright, so a caller retry after a crash can never
double-append a promotion or rollback. `active_candidate_id` replays the
ledger and returns the `candidate_id` of the last *accepted* record --
covering both "promote" and "rollback" actions, since both mutate what is
active -- while a "rejected" record must never be mistaken for a new active
candidate.
"""

from __future__ import annotations

from pathlib import Path

import pytest

from praxis_eval.ledger import PromotionLedger, PromotionLedgerError
from praxis_eval.types import PromotionRecord


def _make_record(
    *,
    record_id: str,
    candidate_id: str,
    action: str = "promote",
    decision: str = "accepted",
    seq: int = 0,
    previous_candidate_id: str | None = None,
) -> PromotionRecord:
    return PromotionRecord(
        spec_version="1.0.0",
        record_id=record_id,
        seq=seq,
        action=action,
        candidate_id=candidate_id,
        decision=decision,
        produced_at="2026-09-06T00:00:00Z",
        previous_candidate_id=previous_candidate_id,
    )


def test_sequential_appends_get_increasing_seq(tmp_path: Path) -> None:
    ledger = PromotionLedger(tmp_path)

    first = ledger.append(_make_record(record_id="rec-1", candidate_id="cand-a"))
    second = ledger.append(_make_record(record_id="rec-2", candidate_id="cand-b"))
    third = ledger.append(_make_record(record_id="rec-3", candidate_id="cand-c"))

    assert [first.seq, second.seq, third.seq] == [0, 1, 2]


def test_reopening_reconstructs_all_previously_appended_records(tmp_path: Path) -> None:
    ledger = PromotionLedger(tmp_path)
    ledger.append(_make_record(record_id="rec-1", candidate_id="cand-a"))
    ledger.append(_make_record(record_id="rec-2", candidate_id="cand-b"))

    reopened = PromotionLedger(tmp_path)
    records = reopened.read_all()

    assert [record.record_id for record in records] == ["rec-1", "rec-2"]
    assert [record.seq for record in records] == [0, 1]


def test_duplicate_record_id_raises_and_does_not_corrupt_file(tmp_path: Path) -> None:
    ledger = PromotionLedger(tmp_path)
    ledger.append(_make_record(record_id="rec-1", candidate_id="cand-a"))

    with pytest.raises(PromotionLedgerError):
        ledger.append(_make_record(record_id="rec-1", candidate_id="cand-b"))

    records = ledger.read_all()
    assert [record.record_id for record in records] == ["rec-1"]
    assert records[0].candidate_id == "cand-a"

    reopened = PromotionLedger(tmp_path)
    reopened_records = reopened.read_all()
    assert [record.record_id for record in reopened_records] == ["rec-1"]
    assert reopened_records[0].candidate_id == "cand-a"


def test_active_candidate_id_returns_none_on_empty_ledger(tmp_path: Path) -> None:
    ledger = PromotionLedger(tmp_path)

    assert ledger.active_candidate_id() is None


def test_active_candidate_id_returns_sole_record_candidate_after_one_accepted_promote(
    tmp_path: Path,
) -> None:
    ledger = PromotionLedger(tmp_path)
    ledger.append(
        _make_record(
            record_id="rec-1",
            candidate_id="cand-a",
            action="promote",
            decision="accepted",
        )
    )

    assert ledger.active_candidate_id() == "cand-a"


def test_active_candidate_id_reflects_later_accepted_rollback_superseding_earlier_promote(
    tmp_path: Path,
) -> None:
    ledger = PromotionLedger(tmp_path)
    ledger.append(
        _make_record(
            record_id="rec-1",
            candidate_id="cand-a",
            action="promote",
            decision="accepted",
        )
    )
    ledger.append(
        _make_record(
            record_id="rec-2",
            candidate_id="cand-baseline",
            action="rollback",
            decision="accepted",
            previous_candidate_id="cand-a",
        )
    )

    assert ledger.active_candidate_id() == "cand-baseline"


def test_rejected_decision_after_accepted_does_not_change_active_candidate_id(
    tmp_path: Path,
) -> None:
    ledger = PromotionLedger(tmp_path)
    ledger.append(
        _make_record(
            record_id="rec-1",
            candidate_id="cand-a",
            action="promote",
            decision="accepted",
        )
    )
    ledger.append(
        _make_record(
            record_id="rec-2",
            candidate_id="cand-b",
            action="promote",
            decision="rejected",
        )
    )

    assert ledger.active_candidate_id() == "cand-a"

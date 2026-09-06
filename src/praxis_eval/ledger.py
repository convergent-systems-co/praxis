"""Append-only promotion/rollback ledger.

PromotionLedger persists PromotionRecords as JSONL (one JSON object per
line), assigning `seq` itself (ignoring any caller-supplied value) and
rejecting a duplicate `record_id` outright via PromotionLedgerError, so a
caller retry after a crash can never double-append a promotion or rollback.
Every append flushes and `os.fsync`s so a crash immediately after `append()`
returns is guaranteed durable. Re-opening a PromotionLedger over the same
directory replays the file to reconstruct `seq` and the seen `record_id`s,
so a restarted process can resume purely from persisted records.
`active_candidate_id()` replays the ledger and returns the `candidate_id` of
the last record with `decision == "accepted"` -- covering both "promote" and
"rollback" actions, since both mutate what is active -- while a "rejected"
(or "human_required") record must never be mistaken for a new active
candidate.

This mirrors praxis_runtime.events.EventLog's concurrency/atomicity
guarantees exactly (same flock-on-sidecar-lock-file, re-derive-on-append,
fsync-before-return mechanics), but is not a subclass or reuse of it: the
document shape differs and this module does not import praxis_runtime.
`append()` holds an exclusive `flock` on a sidecar lock file and recomputes
`seq`/duplicate-`record_id` state from the on-disk log while holding it, so
two `PromotionLedger` instances (same process or different processes)
opened concurrently on the same directory serialize their appends instead
of racing on a `seq` cached at construction time. Callers that construct
scratch/short-lived PromotionLedgers should `close()` them (or use the
context-manager protocol) to release the underlying file handle.
"""

from __future__ import annotations

import dataclasses
import fcntl
import json
import os
from pathlib import Path

from praxis_contracts.validator import ContractValidationError, validate_document

from praxis_eval.types import (
    PROMOTION_RECORD_SCHEMA_PATH,
    PromotionRecord,
    promotion_record_from_document,
    promotion_record_to_document,
)

LOG_FILENAME = "promotions.jsonl"

_ACCEPTED = "accepted"


class PromotionLedgerError(Exception):
    """Raised when a promotion record fails validation or duplicates an existing record_id."""


class PromotionLedger:
    def __init__(self, directory: Path) -> None:
        self._directory = Path(directory)
        self._directory.mkdir(parents=True, exist_ok=True)
        self._path = self._directory / LOG_FILENAME
        self._lock_path = self._directory / (LOG_FILENAME + ".lock")

        self._lock_handle = open(self._lock_path, "a", encoding="utf-8")
        fcntl.flock(self._lock_handle, fcntl.LOCK_SH)
        try:
            self._records: list[PromotionRecord] = list(self._read_from_disk())
            self._seen_record_ids = {record.record_id for record in self._records}
            self._next_seq = len(self._records)
        finally:
            fcntl.flock(self._lock_handle, fcntl.LOCK_UN)

        self._handle = open(self._path, "a", encoding="utf-8")

    def _read_from_disk(self) -> list[PromotionRecord]:
        if not self._path.exists():
            return []
        records = []
        with open(self._path, "r", encoding="utf-8") as handle:
            for line in handle:
                line = line.strip()
                if not line:
                    continue
                try:
                    document = json.loads(line)
                except json.JSONDecodeError as exc:
                    raise PromotionLedgerError(f"malformed promotion ledger line: {exc}") from exc
                try:
                    validate_document(document, PROMOTION_RECORD_SCHEMA_PATH)
                except ContractValidationError as exc:
                    raise PromotionLedgerError(str(exc)) from exc
                records.append(promotion_record_from_document(document))
        return records

    def append(self, record: PromotionRecord) -> PromotionRecord:
        fcntl.flock(self._lock_handle, fcntl.LOCK_EX)
        try:
            # Re-derive authoritative state from disk while holding the lock:
            # another PromotionLedger instance (this process or another one)
            # may have appended since this instance's construction or last
            # append, and a stale in-memory seq/record_id cache would let two
            # concurrent instances assign the same seq or miss each other's
            # record_ids.
            self._records = list(self._read_from_disk())
            self._seen_record_ids = {existing.record_id for existing in self._records}
            self._next_seq = len(self._records)

            if record.record_id in self._seen_record_ids:
                raise PromotionLedgerError(f"duplicate record_id: {record.record_id!r}")

            stored = dataclasses.replace(record, seq=self._next_seq)

            document = promotion_record_to_document(stored)
            try:
                validate_document(document, PROMOTION_RECORD_SCHEMA_PATH)
            except ContractValidationError as exc:
                raise PromotionLedgerError(str(exc)) from exc

            self._handle.write(json.dumps(document) + "\n")
            self._handle.flush()
            os.fsync(self._handle.fileno())

            self._records.append(stored)
            self._seen_record_ids.add(stored.record_id)
            self._next_seq += 1
        finally:
            fcntl.flock(self._lock_handle, fcntl.LOCK_UN)

        return stored

    def read_all(self) -> list[PromotionRecord]:
        fcntl.flock(self._lock_handle, fcntl.LOCK_SH)
        try:
            # Re-derive from disk while holding the lock: another
            # PromotionLedger instance (this process or another one) may
            # have appended since this instance's construction or last
            # append/read, and a stale in-memory cache would hide those
            # records from a long-lived instance that never appends itself.
            self._records = list(self._read_from_disk())
            self._seen_record_ids = {record.record_id for record in self._records}
            self._next_seq = len(self._records)
        finally:
            fcntl.flock(self._lock_handle, fcntl.LOCK_UN)

        return list(self._records)

    def active_candidate_id(self) -> str | None:
        for record in reversed(self.read_all()):
            if record.decision == _ACCEPTED:
                return record.candidate_id
        return None

    def close(self) -> None:
        if not self._handle.closed:
            self._handle.close()
        if not self._lock_handle.closed:
            self._lock_handle.close()

    def __enter__(self) -> "PromotionLedger":
        return self

    def __exit__(self, *exc_info: object) -> None:
        self.close()

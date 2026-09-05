"""Append-only event log.

EventLog persists Events as JSONL (one JSON object per line), assigning
`seq` itself (ignoring any caller-supplied value) and rejecting a duplicate
`event_id` outright via EventLogError, so a caller retry after a crash can
never double-apply an event. Every append flushes and `os.fsync`s so a crash
immediately after `append()` returns is guaranteed durable. Re-opening an
EventLog over the same directory replays the file to reconstruct `seq` and
the seen `event_id`s, so a restarted process can resume purely from
persisted events. Each stored document is passed through
praxis_runtime.migrations.migrate_document before being parsed, so an event
written by an older schema minor version is upgraded in place on read.
Callers that construct scratch/short-lived EventLogs should `close()` them
(or use the context-manager protocol) to release the underlying file handle.
`append()` holds an exclusive `flock` on a sidecar lock file and recomputes
`seq`/duplicate-`event_id` state from the on-disk log while holding it, so
two `EventLog` instances (same process or different processes) opened
concurrently on the same directory serialize their appends instead of
racing on a `seq` cached at construction time.
"""

from __future__ import annotations

import dataclasses
import fcntl
import json
import os
from dataclasses import dataclass
from pathlib import Path

from praxis_contracts.validator import ContractValidationError, validate_document
from praxis_runtime.migrations import migrate_document

SCHEMA_PATH = Path(__file__).resolve().parent.parent.parent / "schemas" / "v1" / "event.schema.json"

LOG_FILENAME = "events.jsonl"
_KIND = "event"


class EventLogError(Exception):
    """Raised when an event fails validation or duplicates an existing event_id."""


@dataclass(frozen=True)
class Event:
    spec_version: str
    seq: int
    run_id: str
    node_id: str
    event_type: str
    payload: dict
    event_id: str


class EventLog:
    def __init__(self, directory: Path) -> None:
        self._directory = Path(directory)
        self._directory.mkdir(parents=True, exist_ok=True)
        self._path = self._directory / LOG_FILENAME
        self._lock_path = self._directory / (LOG_FILENAME + ".lock")

        self._lock_handle = open(self._lock_path, "a", encoding="utf-8")
        fcntl.flock(self._lock_handle, fcntl.LOCK_SH)
        try:
            self._events: list[Event] = list(self._read_from_disk())
            self._seen_event_ids = {event.event_id for event in self._events}
            self._next_seq = len(self._events)
        finally:
            fcntl.flock(self._lock_handle, fcntl.LOCK_UN)

        self._handle = open(self._path, "a", encoding="utf-8")

    def _read_from_disk(self) -> list[Event]:
        if not self._path.exists():
            return []
        events = []
        with open(self._path, "r", encoding="utf-8") as handle:
            for line in handle:
                line = line.strip()
                if not line:
                    continue
                try:
                    document = json.loads(line)
                except json.JSONDecodeError as exc:
                    raise EventLogError(f"malformed event log line: {exc}") from exc
                try:
                    document = migrate_document(document, _KIND)
                except ContractValidationError as exc:
                    raise EventLogError(str(exc)) from exc
                events.append(Event(**document))
        return events

    def append(self, event: Event) -> Event:
        fcntl.flock(self._lock_handle, fcntl.LOCK_EX)
        try:
            # Re-derive authoritative state from disk while holding the lock:
            # another EventLog instance (this process or another one) may have
            # appended since this instance's construction or last append, and
            # a stale in-memory seq/event_id cache would let two concurrent
            # instances assign the same seq or miss each other's event_ids.
            self._events = list(self._read_from_disk())
            self._seen_event_ids = {existing.event_id for existing in self._events}
            self._next_seq = len(self._events)

            if event.event_id in self._seen_event_ids:
                raise EventLogError(f"duplicate event_id: {event.event_id!r}")

            stored = dataclasses.replace(event, seq=self._next_seq)

            instance = dataclasses.asdict(stored)
            try:
                validate_document(instance, SCHEMA_PATH)
            except ContractValidationError as exc:
                raise EventLogError(str(exc)) from exc

            self._handle.write(json.dumps(instance) + "\n")
            self._handle.flush()
            os.fsync(self._handle.fileno())

            self._events.append(stored)
            self._seen_event_ids.add(stored.event_id)
            self._next_seq += 1
        finally:
            fcntl.flock(self._lock_handle, fcntl.LOCK_UN)

        return stored

    def read_all(self) -> list[Event]:
        fcntl.flock(self._lock_handle, fcntl.LOCK_SH)
        try:
            # Re-derive from disk while holding the lock: another EventLog
            # instance (this process or another one) may have appended since
            # this instance's construction or last append/read, and a stale
            # in-memory cache would hide those events from a long-lived
            # instance that never appends itself.
            self._events = list(self._read_from_disk())
            self._seen_event_ids = {event.event_id for event in self._events}
            self._next_seq = len(self._events)
        finally:
            fcntl.flock(self._lock_handle, fcntl.LOCK_UN)

        return list(self._events)

    def close(self) -> None:
        if not self._handle.closed:
            self._handle.close()
        if not self._lock_handle.closed:
            self._lock_handle.close()

    def __enter__(self) -> "EventLog":
        return self

    def __exit__(self, *exc_info: object) -> None:
        self.close()

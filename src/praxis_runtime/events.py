"""Append-only event log.

EventLog persists Events as JSONL (one JSON object per line), assigning
`seq` itself (ignoring any caller-supplied value) and rejecting a duplicate
`event_id` outright via EventLogError, so a caller retry after a crash can
never double-apply an event. Every append flushes and `os.fsync`s so a crash
immediately after `append()` returns is guaranteed durable. Re-opening an
EventLog over the same directory replays the file to reconstruct `seq` and
the seen `event_id`s, so a restarted process can resume purely from
persisted events.
"""

from __future__ import annotations

import dataclasses
import json
import os
from dataclasses import dataclass
from pathlib import Path

from praxis_contracts.validator import ContractValidationError, validate_document

SCHEMA_PATH = Path(__file__).resolve().parent.parent.parent / "schemas" / "v1" / "event.schema.json"

LOG_FILENAME = "events.jsonl"


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

        self._events: list[Event] = list(self._read_from_disk())
        self._seen_event_ids = {event.event_id for event in self._events}
        self._next_seq = len(self._events)

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
                events.append(Event(**json.loads(line)))
        return events

    def append(self, event: Event) -> Event:
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

        return stored

    def read_all(self) -> list[Event]:
        return list(self._events)

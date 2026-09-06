"""Append-only, project-scoped observation log.

ObservationLog persists Observations as JSONL (one JSON object per line),
rejecting a duplicate `observation_id` outright via ObservationLogError, so a
caller retry after a crash can never double-append an observation. Unlike
praxis_eval.ledger.PromotionLedger, there is no `seq` to assign -- Observation
has no `seq` field, so dedupe is purely on `observation_id`. Every append
flushes and `os.fsync`s so a crash immediately after `append()` returns is
guaranteed durable. Re-opening an ObservationLog over the same directory
replays the file to reconstruct the seen `observation_id`s, so a restarted
process can resume purely from persisted records. `read_for_project()` is the
read-side enforcement that a project's observations are queried scoped, by
default, complementing the heuristic layer's scope enforcement.

This mirrors praxis_eval.ledger.PromotionLedger's durability mechanics
exactly (same flock-on-sidecar-lock-file, re-derive-on-append,
fsync-before-return mechanics), but is not a subclass or reuse of it: the
document shape differs and this module does not import praxis_eval.ledger.
`append()` holds an exclusive `flock` on a sidecar lock file and recomputes
the seen-`observation_id` state from the on-disk log while holding it, so two
`ObservationLog` instances (same process or different processes) opened
concurrently on the same directory serialize their appends instead of racing.
Callers that construct scratch/short-lived ObservationLogs should `close()`
them (or use the context-manager protocol) to release the underlying file
handle.
"""

from __future__ import annotations

import fcntl
import json
import os
from pathlib import Path

from praxis_contracts.validator import ContractValidationError, validate_document

from praxis_learning.types import (
    OBSERVATION_SCHEMA_PATH,
    Observation,
    observation_from_document,
    observation_to_document,
)

LOG_FILENAME = "observations.jsonl"


class ObservationLogError(Exception):
    """Raised when an observation fails validation or duplicates an existing observation_id."""


class ObservationLog:
    def __init__(self, directory: Path) -> None:
        self._directory = Path(directory)
        self._directory.mkdir(parents=True, exist_ok=True)
        self._path = self._directory / LOG_FILENAME
        self._lock_path = self._directory / (LOG_FILENAME + ".lock")

        self._lock_handle = open(self._lock_path, "a", encoding="utf-8")
        fcntl.flock(self._lock_handle, fcntl.LOCK_SH)
        try:
            self._observations: list[Observation] = list(self._read_from_disk())
            self._seen_observation_ids = {
                observation.observation_id for observation in self._observations
            }
        finally:
            fcntl.flock(self._lock_handle, fcntl.LOCK_UN)

        self._handle = open(self._path, "a", encoding="utf-8")

    def _read_from_disk(self) -> list[Observation]:
        if not self._path.exists():
            return []
        observations = []
        with open(self._path, "r", encoding="utf-8") as handle:
            for line in handle:
                line = line.strip()
                if not line:
                    continue
                try:
                    document = json.loads(line)
                except json.JSONDecodeError as exc:
                    raise ObservationLogError(f"malformed observation log line: {exc}") from exc
                try:
                    validate_document(document, OBSERVATION_SCHEMA_PATH)
                except ContractValidationError as exc:
                    raise ObservationLogError(str(exc)) from exc
                observations.append(observation_from_document(document))
        return observations

    def append(self, observation: Observation) -> Observation:
        fcntl.flock(self._lock_handle, fcntl.LOCK_EX)
        try:
            # Re-derive authoritative state from disk while holding the lock:
            # another ObservationLog instance (this process or another one)
            # may have appended since this instance's construction or last
            # append, and a stale in-memory observation_id cache would let
            # two concurrent instances miss each other's observation_ids.
            self._observations = list(self._read_from_disk())
            self._seen_observation_ids = {
                existing.observation_id for existing in self._observations
            }

            if observation.observation_id in self._seen_observation_ids:
                raise ObservationLogError(
                    f"duplicate observation_id: {observation.observation_id!r}"
                )

            document = observation_to_document(observation)
            try:
                validate_document(document, OBSERVATION_SCHEMA_PATH)
            except ContractValidationError as exc:
                raise ObservationLogError(str(exc)) from exc

            self._handle.write(json.dumps(document) + "\n")
            self._handle.flush()
            os.fsync(self._handle.fileno())

            self._observations.append(observation)
            self._seen_observation_ids.add(observation.observation_id)
        finally:
            fcntl.flock(self._lock_handle, fcntl.LOCK_UN)

        return observation

    def read_all(self) -> list[Observation]:
        fcntl.flock(self._lock_handle, fcntl.LOCK_SH)
        try:
            # Re-derive from disk while holding the lock: another
            # ObservationLog instance (this process or another one) may have
            # appended since this instance's construction or last
            # append/read, and a stale in-memory cache would hide those
            # records from a long-lived instance that never appends itself.
            self._observations = list(self._read_from_disk())
            self._seen_observation_ids = {
                observation.observation_id for observation in self._observations
            }
        finally:
            fcntl.flock(self._lock_handle, fcntl.LOCK_UN)

        return list(self._observations)

    def read_for_project(self, project_id: str) -> list[Observation]:
        return [
            observation
            for observation in self.read_all()
            if observation.project_id == project_id
        ]

    def close(self) -> None:
        if not self._handle.closed:
            self._handle.close()
        if not self._lock_handle.closed:
            self._lock_handle.close()

    def __enter__(self) -> "ObservationLog":
        return self

    def __exit__(self, *exc_info: object) -> None:
        self.close()

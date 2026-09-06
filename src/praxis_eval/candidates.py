"""Candidate registry: content-addressed immutable identity + durable storage.

compute_candidate_id derives a candidate_id purely from `configuration` and
`parent_candidate_id`, so identical content (regardless of dict key
insertion order) always yields the same id, and lineage (the parent) is part
of identity. CandidateRegistry stores one validated CandidateConfig document
per candidate_id, written atomically (mirroring
praxis_runtime.state.RunStateStore.save's .tmp + os.replace pattern) so a
crash mid-write never leaves a torn candidate file.
"""

from __future__ import annotations

import hashlib
import json
import os
from datetime import datetime, timezone
from pathlib import Path

from praxis_contracts.validator import validate_document
from praxis_eval.types import (
    CANDIDATE_CONFIG_SCHEMA_PATH,
    CandidateConfig,
    candidate_config_from_document,
    candidate_config_to_document,
)


def compute_candidate_id(configuration: dict, *, parent_candidate_id: str | None = None) -> str:
    encoded = json.dumps(configuration, sort_keys=True, separators=(",", ":"))
    payload = f"{parent_candidate_id or ''}\n{encoded}"
    return hashlib.sha256(payload.encode("utf-8")).hexdigest()


def build_candidate_config(
    configuration: dict,
    *,
    parent_candidate_id: str | None = None,
    target: str | None = None,
    description: str | None = None,
    created_at: str | None = None,
) -> CandidateConfig:
    candidate_id = compute_candidate_id(configuration, parent_candidate_id=parent_candidate_id)
    config = CandidateConfig(
        spec_version="1.0.0",
        candidate_id=candidate_id,
        configuration=configuration,
        created_at=created_at or datetime.now(timezone.utc).isoformat(),
        parent_candidate_id=parent_candidate_id,
        target=target,
        description=description,
    )
    validate_document(candidate_config_to_document(config), CANDIDATE_CONFIG_SCHEMA_PATH)
    return config


class CandidateRegistryError(Exception):
    """Raised when a candidate is registered with content that conflicts with an existing id."""


class CandidateRegistry:
    def __init__(self, path: Path) -> None:
        self._path = Path(path)

    def _file_path(self, candidate_id: str) -> Path:
        return self._path / f"{candidate_id}.json"

    def register(self, candidate: CandidateConfig) -> CandidateConfig:
        existing = self.get(candidate.candidate_id)
        if existing is None:
            document = candidate_config_to_document(candidate)
            validate_document(document, CANDIDATE_CONFIG_SCHEMA_PATH)
            file_path = self._file_path(candidate.candidate_id)
            self._path.mkdir(parents=True, exist_ok=True)
            tmp_path = file_path.with_name(file_path.name + ".tmp")
            tmp_path.write_text(json.dumps(document, indent=2))
            os.replace(tmp_path, file_path)
            return candidate

        if existing.configuration != candidate.configuration:
            raise CandidateRegistryError(
                f"candidate_id {candidate.candidate_id!r} already registered with "
                "different configuration"
            )
        return existing

    def get(self, candidate_id: str) -> CandidateConfig | None:
        file_path = self._file_path(candidate_id)
        if not file_path.is_file():
            return None
        document = json.loads(file_path.read_text())
        return candidate_config_from_document(document)

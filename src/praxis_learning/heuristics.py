"""Heuristic registry: content-addressed identity + clustering/deduplication.

compute_heuristic_id derives a heuristic_id purely from `project_id`,
`pattern`, and `trigger` (deliberately excluding evidence/confidence/status),
so two observations that share those three fields but differ in
observation_id always cluster onto the same heuristic_id. HeuristicRegistry
stores one validated HeuristicCandidate document per heuristic_id, written
atomically (mirroring praxis_eval.candidates.CandidateRegistry's .tmp +
os.replace pattern), and HeuristicRegistry.save overwrites in place rather
than rejecting on mismatch, because a HeuristicCandidate is expected to
mutate its evidence/confidence/status fields under the same id as new
observations arrive.
"""

from __future__ import annotations

import hashlib
import json
import os
from datetime import datetime, timezone
from pathlib import Path

from praxis_contracts.validator import ContractValidationError, validate_document
from praxis_learning.types import (
    HEURISTIC_CANDIDATE_SCHEMA_PATH,
    HeuristicCandidate,
    Observation,
    heuristic_candidate_from_document,
    heuristic_candidate_to_document,
)


def compute_heuristic_id(project_id: str, pattern: str, trigger: dict) -> str:
    encoded = json.dumps(
        {"project_id": project_id, "pattern": pattern, "trigger": trigger},
        sort_keys=True,
        separators=(",", ":"),
    )
    return hashlib.sha256(encoded.encode("utf-8")).hexdigest()


def cluster_key(pattern: str, trigger: dict) -> str:
    """Same operation as the (pattern, trigger) portion of compute_heuristic_id.

    Callers that need to cluster candidates without a project_id (or that
    just want to confirm two observations would collide) can use this
    directly; compute_heuristic_id remains the single source of truth for
    the full heuristic_id.
    """
    encoded = json.dumps(
        {"pattern": pattern, "trigger": trigger},
        sort_keys=True,
        separators=(",", ":"),
    )
    return hashlib.sha256(encoded.encode("utf-8")).hexdigest()


def build_heuristic_candidate_from_observation(
    observation: Observation,
    *,
    proposed_configuration: dict,
    expected_outcome: str,
    description: str | None = None,
    created_at: str | None = None,
) -> HeuristicCandidate:
    timestamp = created_at or datetime.now(timezone.utc).isoformat()
    heuristic_id = compute_heuristic_id(
        observation.project_id, observation.pattern, observation.trigger
    )
    candidate = HeuristicCandidate(
        spec_version=observation.spec_version,
        heuristic_id=heuristic_id,
        project_id=observation.project_id,
        scope="project",
        pattern=observation.pattern,
        trigger=observation.trigger,
        expected_outcome=expected_outcome,
        proposed_configuration=proposed_configuration,
        status="candidate",
        confidence=0.5,
        evidence_ids=(observation.observation_id,),
        contradiction_ids=(),
        created_at=timestamp,
        updated_at=timestamp,
        description=description,
    )
    validate_document(heuristic_candidate_to_document(candidate), HEURISTIC_CANDIDATE_SCHEMA_PATH)
    return candidate


class HeuristicRegistryError(Exception):
    """Raised for heuristic registry failures."""


class HeuristicRegistry:
    def __init__(self, path: Path) -> None:
        self._path = Path(path)

    def _file_path(self, heuristic_id: str) -> Path:
        return self._path / f"{heuristic_id}.json"

    def save(self, heuristic: HeuristicCandidate) -> HeuristicCandidate:
        document = heuristic_candidate_to_document(heuristic)
        validate_document(document, HEURISTIC_CANDIDATE_SCHEMA_PATH)
        file_path = self._file_path(heuristic.heuristic_id)
        self._path.mkdir(parents=True, exist_ok=True)
        tmp_path = file_path.with_name(file_path.name + ".tmp")
        tmp_path.write_text(json.dumps(document, indent=2))
        os.replace(tmp_path, file_path)
        return heuristic

    def _load_validated(self, file_path: Path) -> dict:
        document = json.loads(file_path.read_text())
        try:
            validate_document(document, HEURISTIC_CANDIDATE_SCHEMA_PATH)
        except ContractValidationError as exc:
            raise HeuristicRegistryError(
                f"stored heuristic document {file_path.name!r} failed schema "
                f"validation: {exc.reason}"
            ) from exc
        return document

    def get(self, heuristic_id: str) -> HeuristicCandidate | None:
        file_path = self._file_path(heuristic_id)
        if not file_path.is_file():
            return None
        document = self._load_validated(file_path)
        return heuristic_candidate_from_document(document)

    def list_for_project(self, project_id: str) -> list[HeuristicCandidate]:
        if not self._path.is_dir():
            return []
        candidates = []
        for file_path in sorted(self._path.glob("*.json")):
            document = self._load_validated(file_path)
            if document.get("project_id") == project_id:
                candidates.append(heuristic_candidate_from_document(document))
        return candidates

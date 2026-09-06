"""Ingestion pipeline: extraction -> observation log -> heuristic clustering/update.

ingest_telemetry wires the already-tested extraction/observations/heuristics/
confidence modules together: every extracted observation is durably appended
to the observation log first (regardless of what happens to its heuristic),
then folded into a new or existing heuristic. A heuristic already past the
candidate stage (confidence.ConfidenceError) does not silently absorb new
evidence -- the observation is still logged, but the heuristic update is
skipped.
"""

from __future__ import annotations

from praxis_learning import confidence, extraction, heuristics
from praxis_learning.heuristics import HeuristicRegistry
from praxis_learning.observations import ObservationLog
from praxis_learning.types import HeuristicCandidate


def ingest_telemetry(
    telemetry_records: list[dict],
    *,
    project_id: str,
    observation_log: ObservationLog,
    heuristic_registry: HeuristicRegistry,
    default_proposed_configuration: dict | None = None,
) -> list[HeuristicCandidate]:
    observations = extraction.extract_observations(telemetry_records, project_id=project_id)

    touched: list[HeuristicCandidate] = []
    for observation in observations:
        observation_log.append(observation)

        heuristic_id = heuristics.compute_heuristic_id(
            observation.project_id, observation.pattern, observation.trigger
        )
        existing = heuristic_registry.get(heuristic_id)

        if existing is None:
            candidate = heuristics.build_heuristic_candidate_from_observation(
                observation,
                proposed_configuration=default_proposed_configuration or {},
                expected_outcome=observation.observed_outcome,
            )
            heuristic_registry.save(candidate)
            touched.append(candidate)
        else:
            try:
                updated = confidence.apply_observation(existing, observation)
            except confidence.ConfidenceError:
                continue
            heuristic_registry.save(updated)
            touched.append(updated)

    return touched

"""Evidence/proof projection and stale-proof detection for the dashboard.

`stored_evidence_for` is a public, read-only re-implementation of
src/praxis_runtime/transitions.py::TransitionEngine._stored_evidence (that
method is private to TransitionEngine, so it cannot be imported across
packages): reverse-scan `events` for the most recent event whose `node_id`
matches and whose `payload` carries an `"evidence"` key, returning
`payload["evidence"] or []`.

`build_evidence_view` reads `node.metadata["evidence_requirement"]` (shape:
schemas/v1/evidence-requirement.schema.json -- the requirement's proof-type
list lives under the `"evidence"` key) and calls the same read-only
`praxis_evidence.gates.evaluate_gate` used by
`TransitionEngine._check_evidence` (src/praxis_runtime/transitions.py) to
grade whatever proof has been stored so far, without mutating anything. An
empty stored-evidence list means "not yet attempted" (`satisfied=None`), not
a failing evaluation.
"""

from __future__ import annotations

from dataclasses import dataclass

import praxis_evidence.gates
import praxis_evidence.graders
import praxis_runtime.events
import praxis_runtime.graph


@dataclass(frozen=True)
class EvidenceView:
    node_id: str
    required_proof_types: tuple[str, ...]
    satisfied: bool | None
    reasons: tuple[str, ...]
    stale_warning: str | None


def stored_evidence_for(node_id: str, events: "list[praxis_runtime.events.Event]") -> list[dict]:
    """Mirrors TransitionEngine._stored_evidence (src/praxis_runtime/transitions.py)."""
    for event in reversed(events):
        if event.node_id == node_id and "evidence" in event.payload:
            return event.payload["evidence"] or []
    return []


def _stale_warning(records: list[dict], graph_version: str) -> str | None:
    for record in records:
        record_version = record.get("graph_version")
        proof_type = record.get("proof_type", "<unknown>")
        if record_version is not None and record_version != graph_version:
            return (
                f"proof for {proof_type!r} recorded against graph_version "
                f"{record_version}, current graph is {graph_version}"
            )
    return None


def build_evidence_view(
    node: "praxis_runtime.graph.Node",
    events: "list[praxis_runtime.events.Event]",
    graph: "praxis_runtime.graph.Graph",
    *,
    grader_registry: "praxis_evidence.graders.GraderRegistry | None" = None,
) -> EvidenceView:
    requirement = node.metadata.get("evidence_requirement")
    if not requirement:
        return EvidenceView(
            node_id=node.id,
            required_proof_types=(),
            satisfied=None,
            reasons=(),
            stale_warning=None,
        )

    records = stored_evidence_for(node.id, events)
    required_proof_types = tuple(
        item["proof_type"]
        for item in requirement.get("evidence", [])
        if isinstance(item, dict) and "proof_type" in item
    )

    if not records:
        return EvidenceView(
            node_id=node.id,
            required_proof_types=required_proof_types,
            satisfied=None,
            reasons=(),
            stale_warning=None,
        )

    result = praxis_evidence.gates.evaluate_gate(
        requirement,
        records,
        node_id=node.id,
        graph_version=graph.spec_version,
        registry=grader_registry or praxis_evidence.graders.default_registry(),
    )

    return EvidenceView(
        node_id=node.id,
        required_proof_types=required_proof_types,
        satisfied=result.satisfied,
        reasons=result.reasons,
        stale_warning=_stale_warning(records, graph.spec_version),
    )

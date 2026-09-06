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
a failing evaluation. `required_proof_types` only names `"required"`-
constraint items (per evidence-requirement.schema.json's `constraint` enum
of `"required"`/`"preferred"`/`"prohibited"`) -- a `"preferred"` item is
optional and a `"prohibited"` item's presence is the thing being guarded
against, so neither belongs in a field an operator reads as "what's
required".

For a node reached via one or more join edges, this module mirrors
`TransitionEngine._check_evidence`'s join-node aggregation
(src/praxis_runtime/transitions.py): each incoming join edge's source's own
gate result is re-derived fresh from that source's stored evidence (never
trusting that the source already reached `TERMINAL_SUCCESS`), then combined
with this node's own result via `praxis_evidence.aggregate.aggregate_gate_results`,
so a join node's `EvidenceView` reflects the same additional gating its real
terminal transition is subject to.
"""

from __future__ import annotations

from dataclasses import dataclass

import praxis_evidence.gates
import praxis_evidence.graders
import praxis_runtime.events
import praxis_runtime.graph
from praxis_evidence.aggregate import aggregate_gate_results
from praxis_evidence.types import GateResult


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


def _source_gate_result(
    source_node_id: str,
    graph: "praxis_runtime.graph.Graph",
    events: "list[praxis_runtime.events.Event]",
    registry: "praxis_evidence.graders.GraderRegistry",
) -> GateResult:
    """Mirrors TransitionEngine._source_gate_result (src/praxis_runtime/transitions.py):
    re-derives a join edge's upstream source's own gate result fresh from its
    stored evidence and the current registry, rather than trusting that the
    source already reached TERMINAL_SUCCESS."""
    source_node = graph.nodes.get(source_node_id)
    requirement = source_node.metadata.get("evidence_requirement") if source_node else None
    if not requirement:
        return GateResult(node_id=source_node_id, satisfied=True, reasons=(), evaluated=())

    return praxis_evidence.gates.evaluate_gate(
        requirement,
        stored_evidence_for(source_node_id, events),
        node_id=source_node_id,
        graph_version=graph.spec_version,
        registry=registry,
    )


def build_evidence_view(
    node: "praxis_runtime.graph.Node",
    events: "list[praxis_runtime.events.Event]",
    graph: "praxis_runtime.graph.Graph",
    *,
    grader_registry: "praxis_evidence.graders.GraderRegistry | None" = None,
) -> EvidenceView:
    requirement = node.metadata.get("evidence_requirement")
    # Mirrors TransitionEngine._check_evidence's own join-source lookup
    # (src/praxis_runtime/transitions.py): every incoming "join"-kind edge's
    # source is an upstream branch whose own gate result also gates this
    # node's real terminal transition.
    join_sources = [
        edge.source for edge in graph.edges if edge.target == node.id and edge.kind == "join"
    ]

    if not requirement and not join_sources:
        return EvidenceView(
            node_id=node.id,
            required_proof_types=(),
            satisfied=None,
            reasons=(),
            stale_warning=None,
        )

    registry = grader_registry or praxis_evidence.graders.default_registry()

    required_proof_types = tuple(
        item["proof_type"]
        for item in (requirement.get("evidence", []) if requirement else [])
        if isinstance(item, dict) and item.get("constraint") == "required"
    )

    records = stored_evidence_for(node.id, events) if requirement else []

    own_result: GateResult | None = None
    if requirement and records:
        own_result = praxis_evidence.gates.evaluate_gate(
            requirement,
            records,
            node_id=node.id,
            graph_version=graph.spec_version,
            registry=registry,
        )

    if not join_sources:
        if own_result is None:
            return EvidenceView(
                node_id=node.id,
                required_proof_types=required_proof_types,
                satisfied=None,
                reasons=(),
                stale_warning=None,
            )
        return EvidenceView(
            node_id=node.id,
            required_proof_types=required_proof_types,
            satisfied=own_result.satisfied,
            reasons=own_result.reasons,
            stale_warning=_stale_warning(records, graph.spec_version),
        )

    if requirement and own_result is None:
        # Mirrors TransitionEngine._check_evidence's unconditional
        # `evaluate_gate(requirement, evidence or [], ...)` call
        # (src/praxis_runtime/transitions.py): when this node has its own
        # requirement and join_sources exist, the real terminal transition
        # always grades this node's own (possibly empty) stored evidence --
        # it never treats a join node's own missing evidence as merely
        # "not yet attempted" the way the non-join branch above does.
        own_result = praxis_evidence.gates.evaluate_gate(
            requirement,
            records,
            node_id=node.id,
            graph_version=graph.spec_version,
            registry=registry,
        )

    source_results = [
        _source_gate_result(source, graph, events, registry) for source in join_sources
    ]
    upstream = aggregate_gate_results(node.id, source_results)

    if own_result is None:
        satisfied = upstream.satisfied
        reasons = upstream.reasons
    else:
        satisfied = own_result.satisfied and upstream.satisfied
        reasons = (*own_result.reasons, *upstream.reasons)

    return EvidenceView(
        node_id=node.id,
        required_proof_types=required_proof_types,
        satisfied=satisfied,
        reasons=reasons,
        stale_warning=_stale_warning(records, graph.spec_version),
    )

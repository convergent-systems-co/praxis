"""Aggregate per-source gate results for fan-in/join nodes.

Combines one `GateResult` per incoming join-edge source -- each already
evaluated via `evaluate_gate` against that source's own requirement/evidence
-- into a single `GateResult` for the join/fan-in node.
"""

from __future__ import annotations

from praxis_evidence.types import GateResult


def aggregate_gate_results(node_id: str, results: list[GateResult]) -> GateResult:
    """Combine `results` (one per join-edge source) into the join node's `GateResult`."""
    satisfied = all(result.satisfied for result in results)

    reasons: list[str] = []
    for result in results:
        if not result.satisfied:
            reasons.extend(f"{result.node_id}: {reason}" for reason in result.reasons)

    evaluated: list[str] = []
    seen: set[str] = set()
    for result in results:
        for proof_type in result.evaluated:
            if proof_type not in seen:
                seen.add(proof_type)
                evaluated.append(proof_type)

    return GateResult(
        node_id=node_id,
        satisfied=satisfied,
        reasons=tuple(reasons),
        evaluated=tuple(evaluated),
    )

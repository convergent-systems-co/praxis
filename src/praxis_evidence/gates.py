"""Single-node gate evaluation engine.

Evaluates one graph node's evidence requirement (an
evidence-requirement.schema.json-shaped dict) against the raw proof-record
documents collected for that node, and produces a `GateResult`.
"""

from __future__ import annotations

from praxis_contracts.validator import ContractValidationError
from praxis_evidence.graders import GraderRegistry
from praxis_evidence.proof import validate_proof_record
from praxis_evidence.types import GateResult, GradeResult, ProofRecord, proof_record_from_document


def evaluate_gate(
    requirement: dict,
    records: list[dict],
    *,
    node_id: str,
    graph_version: str,
    registry: GraderRegistry,
) -> GateResult:
    """Grade `records` against `requirement` and return the resulting `GateResult`.

    Algorithm:
    - Parse each entry in `records` via `validate_proof_record`; a
      `ContractValidationError` marks that entry malformed -- it is excluded
      from grading and a `"malformed: <index or proof_type>"` reason is
      collected (fail-closed: malformed evidence can never count toward
      satisfying a requirement).
    - Among the remaining valid records, one whose `graph_version` differs
      from this call's `graph_version` is stale -- it is excluded and a
      `"stale: <proof_type>"` reason is collected; it can never satisfy or
      contradict anything.
    - Surviving records are grouped by `proof_type`. For each `proof_type`
      named in the requirement, grading precedence is: a registered
      `"deterministic"` grader is always authoritative; if a `"model"`
      grader is also registered its verdict is graded too but only recorded
      as an advisory reason and can never flip satisfaction; with no
      deterministic grader, a registered `"model"` grader is authoritative;
      with only a `"human"` grader registered, satisfaction requires at
      least one surviving record with `grader_kind="human"` graded
      `status="pass"` -- absence is unsatisfied, never default-approved.
      With no grader registered at all for a required `proof_type`, the
      result is unsatisfied with reason `"no grader registered: <proof_type>"`.
    - If the authoritative grader, applied to 2+ surviving records of the
      same `proof_type`, yields more than one distinct `status`, that is
      contradictory -- a `"contradictory: <proof_type>"` reason is added and
      the `proof_type` is treated as unsatisfied regardless of any
      individual passing record. A single (non-contradictory) authoritative
      grade whose status is anything other than `"pass"` is unsatisfied with
      reason `"failed: <proof_type> (status=<status>)"`.
    - If the requirement item sets `min_confidence`, the `proof_type` is
      unsatisfied with reason `"below min_confidence: <proof_type>"` whenever
      the authoritative grade's confidence is below it, *or absent entirely*
      (fail-closed: a grader that reports no confidence can never satisfy a
      minimum-confidence requirement). When multiple surviving records agree
      on status, the lowest confidence among them is used (fail-closed).
    - A `proof_type` with zero surviving records produces a
      `"missing: <proof_type>"` reason, distinct from malformed/stale --
      but only when no raw record for that `proof_type` was submitted at
      all; a `proof_type` whose only submissions were malformed or stale
      relies on those reasons instead.
    - `constraint` semantics: `"required"` must be satisfied per the above
      or the gate blocks; `"preferred"` is graded for informational reasons
      but never blocks; `"prohibited"` blocks if any authoritative grade for
      that `proof_type` is `status="pass"`.
    - `GateResult.satisfied` is `True` only if every `"required"` item is
      satisfied and no `"prohibited"` item is violated. `evaluated` lists
      every `proof_type` named in the requirement, in order.
    """
    reasons: list[str] = []
    raw_proof_types: set[str] = set()
    for doc in records:
        if isinstance(doc, dict):
            proof_type = doc.get("proof_type")
            if isinstance(proof_type, str):
                raw_proof_types.add(proof_type)

    parsed: list[ProofRecord] = []
    for index, doc in enumerate(records):
        try:
            validate_proof_record(doc)
        except ContractValidationError:
            proof_type = doc.get("proof_type") if isinstance(doc, dict) else None
            label = proof_type if isinstance(proof_type, str) and proof_type else str(index)
            reasons.append(f"malformed: {label}")
            continue

        record = proof_record_from_document(doc)
        if record.graph_version != graph_version:
            reasons.append(f"stale: {record.proof_type}")
            continue
        parsed.append(record)

    by_proof_type: dict[str, list[ProofRecord]] = {}
    for record in parsed:
        by_proof_type.setdefault(record.proof_type, []).append(record)

    satisfied = True
    evaluated: list[str] = []
    for item in requirement["evidence"]:
        proof_type = item["proof_type"]
        constraint = item["constraint"]
        min_confidence = item.get("min_confidence")
        evaluated.append(proof_type)

        group = by_proof_type.get(proof_type, [])
        item_satisfied, pass_exists, item_reasons = _evaluate_item(
            proof_type, group, registry, min_confidence, seen_raw=proof_type in raw_proof_types
        )
        reasons.extend(item_reasons)

        if constraint == "required" and not item_satisfied:
            satisfied = False
        elif constraint == "prohibited" and pass_exists:
            satisfied = False

    return GateResult(
        node_id=node_id,
        satisfied=satisfied,
        reasons=tuple(reasons),
        evaluated=tuple(evaluated),
    )


def _evaluate_item(
    proof_type: str,
    group: list[ProofRecord],
    registry: GraderRegistry,
    min_confidence: float | None,
    *,
    seen_raw: bool,
) -> tuple[bool, bool, list[str]]:
    reasons: list[str] = []

    if not group:
        if not seen_raw:
            reasons.append(f"missing: {proof_type}")
        return False, False, reasons

    kinds = registry.kinds_for(proof_type)
    if not kinds:
        reasons.append(f"no grader registered: {proof_type}")
        return False, False, reasons

    if "deterministic" in kinds:
        grader = registry.get(proof_type, "deterministic")
        candidates = group
    elif "model" in kinds:
        grader = registry.get(proof_type, "model")
        candidates = group
    else:
        grader = registry.get(proof_type, "human")
        candidates = [record for record in group if record.grader_kind == "human"]
        if not candidates:
            reasons.append(f"missing human review: {proof_type}")
            return False, False, reasons

    assert grader is not None
    grades = [grader.grade(record) for record in candidates]
    statuses = {grade.status for grade in grades}

    if len(grades) >= 2 and len(statuses) > 1:
        reasons.append(f"contradictory: {proof_type}")
        satisfied = False
        confidence = None
    else:
        status = statuses.pop() if statuses else None
        satisfied = status == "pass"
        confidence = _min_confidence(grades)
        if not satisfied:
            reasons.append(f"failed: {proof_type} (status={status!r})")

    pass_exists = any(grade.status == "pass" for grade in grades)

    if "deterministic" in kinds and "model" in kinds:
        model_grader = registry.get(proof_type, "model")
        assert model_grader is not None
        model_grades = [model_grader.grade(record) for record in group]
        for model_status in sorted({grade.status for grade in model_grades}):
            reasons.append(
                f"advisory: model grader for {proof_type} returned status={model_status!r}"
            )

    if satisfied and min_confidence is not None:
        if confidence is None or confidence < min_confidence:
            reasons.append(f"below min_confidence: {proof_type}")
            satisfied = False

    return satisfied, pass_exists, reasons


def _min_confidence(grades: list[GradeResult]) -> float | None:
    confidences = [grade.confidence for grade in grades if grade.confidence is not None]
    return min(confidences) if confidences else None

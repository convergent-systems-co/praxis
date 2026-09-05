"""Aggregate per-source gate results for fan-in/join nodes
(praxis_evidence.aggregate.aggregate_gate_results).

A join node has one incoming edge per upstream source, each already graded
independently via evaluate_gate into its own GateResult. Aggregation must
AND the per-source satisfaction, carry forward only the unsatisfied
sources' reasons (prefixed by their own node_id for traceability), and
union the evaluated proof_types across all sources, order-preserving and
de-duplicated.
"""

from __future__ import annotations

from praxis_evidence.aggregate import aggregate_gate_results
from praxis_evidence.types import GateResult


def _result(node_id: str, satisfied: bool, reasons: tuple[str, ...], evaluated: tuple[str, ...]) -> GateResult:
    return GateResult(node_id=node_id, satisfied=satisfied, reasons=reasons, evaluated=evaluated)


def test_all_satisfied_inputs_yield_satisfied():
    results = [
        _result("source-a", True, (), ("proof-a",)),
        _result("source-b", True, (), ("proof-b",)),
    ]

    aggregated = aggregate_gate_results("join-1", results)

    assert aggregated.satisfied is True
    assert aggregated.reasons == ()
    assert aggregated.evaluated == ("proof-a", "proof-b")


def test_one_unsatisfied_among_several_blocks_with_only_its_reasons():
    results = [
        _result("source-a", True, (), ("proof-a",)),
        _result("source-b", False, ("missing: proof-b",), ("proof-b",)),
        _result("source-c", True, (), ("proof-c",)),
    ]

    aggregated = aggregate_gate_results("join-1", results)

    assert aggregated.satisfied is False
    assert aggregated.reasons == ("source-b: missing: proof-b",)


def test_empty_results_list_is_satisfied_with_no_reasons_or_evaluated():
    aggregated = aggregate_gate_results("join-1", [])

    assert aggregated.satisfied is True
    assert aggregated.reasons == ()
    assert aggregated.evaluated == ()


def test_evaluated_is_order_preserving_union_deduplicating_shared_proof_type():
    results = [
        _result("source-a", True, (), ("proof-shared", "proof-a")),
        _result("source-b", True, (), ("proof-b", "proof-shared")),
    ]

    aggregated = aggregate_gate_results("join-1", results)

    assert aggregated.evaluated == ("proof-shared", "proof-a", "proof-b")


def test_aggregated_result_node_id_is_the_join_node():
    results = [_result("source-a", True, (), ())]

    aggregated = aggregate_gate_results("join-1", results)

    assert aggregated.node_id == "join-1"

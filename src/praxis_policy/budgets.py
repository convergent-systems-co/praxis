"""Effective retry/repair budgets: composing a node's optional
`budget_requirement` (validated against
`schemas/v1/budget-requirement.schema.json`) with a policy profile's
defaults, and tracking per-node consumption against that budget.

A node-declared field is a ceiling, never a floor: it can only tighten the
selected profile's default, never loosen it. This mirrors the policy
profile's "cannot lower below a declared minimum" direction but for the
opposite quantity -- budgets only shrink under a stricter constraint, they
never grow past what the node itself was declared safe for.

`BudgetLedger` is a plain in-memory counter, scoped to one `BudgetLedger`
instance -- no file persistence in this bundle. Persisting budget
consumption across a process restart is a follow-up integration seam for a
future overlay/integration layer to reconcile, not required by this
module's own contract.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    import praxis_policy.profiles


@dataclass(frozen=True)
class EffectiveBudget:
    max_retries: int
    max_repairs: int
    max_cost: float | None
    max_time_seconds: float | None


def _tightened(default: int, declared: int | None) -> int:
    if declared is None:
        return default
    return min(default, declared)


def _tightened_optional(default: float | None, declared: float | None) -> float | None:
    if declared is None:
        return default
    if default is None:
        return declared
    return min(default, declared)


def effective_budget(
    profile: "praxis_policy.profiles.PolicyProfile",
    budget_requirement: dict | None,
) -> EffectiveBudget:
    requirement = budget_requirement or {}
    return EffectiveBudget(
        max_retries=_tightened(profile.default_retry_budget, requirement.get("max_retries")),
        max_repairs=_tightened(profile.default_repair_budget, requirement.get("max_repairs")),
        max_cost=_tightened_optional(profile.default_max_cost, requirement.get("max_cost")),
        max_time_seconds=_tightened_optional(
            profile.default_max_time_seconds, requirement.get("max_time_seconds")
        ),
    )


class BudgetLedger:
    """In-memory per-node retry/repair consumption counters."""

    def __init__(self) -> None:
        self._retries: dict[str, int] = {}
        self._repairs: dict[str, int] = {}

    def retries_used(self, node_id: str) -> int:
        return self._retries.get(node_id, 0)

    def repairs_used(self, node_id: str) -> int:
        return self._repairs.get(node_id, 0)

    def record_retry(self, node_id: str) -> int:
        new_count = self.retries_used(node_id) + 1
        self._retries[node_id] = new_count
        return new_count

    def record_repair(self, node_id: str) -> int:
        new_count = self.repairs_used(node_id) + 1
        self._repairs[node_id] = new_count
        return new_count

    def is_retry_exhausted(self, node_id: str, budget: EffectiveBudget) -> bool:
        return self.retries_used(node_id) >= budget.max_retries

    def is_repair_exhausted(self, node_id: str, budget: EffectiveBudget) -> bool:
        return self.repairs_used(node_id) >= budget.max_repairs

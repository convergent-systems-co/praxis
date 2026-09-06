"""`PolicyGate`: ties profiles, authority, budgets, and failure classification
together into `PolicyDecision`s.

This is a decision-only module: its public methods take a plain
`node_metadata: dict` (never a `praxis_runtime.graph.Node`) and it never
imports `praxis_runtime`. This mirrors `praxis_executors.registry`'s
`ExecutorRegistry`, which "has no dependency on `praxis_runtime`; wiring
`ExecutionResult`'s `evidence` through to `TransitionEngine.apply` is the
caller's responsibility, not the registry's" (docs/executors.md, "##
praxis_executors.registry") -- here too, the caller is responsible for
applying the returned `event_type` to a `TransitionEngine` and for tracking
`NodeStatus`.

Per `praxis_runtime.transitions.py::_TRANSITIONS`, "handoff", "fail", and
"block" are all legal directly from `RUNNING`, and "accept" is the only
legal transition out of `HANDOFF` (back to `RUNNING`); there is no direct
`HANDOFF -> TERMINAL_FAILED` edge.
"""

from __future__ import annotations

import enum
from dataclasses import dataclass, field
from typing import TYPE_CHECKING

import praxis_policy.authority as authority
import praxis_policy.budgets as budgets
import praxis_policy.failure_classification as failure_classification
from praxis_policy.authority import AuthorityOutcome
from praxis_policy.failure_classification import FailureClass

if TYPE_CHECKING:
    import praxis_policy.profiles


class PolicyOutcome(enum.Enum):
    AUTHORIZED = "authorized"
    HUMAN_REQUIRED = "human_required"
    DENIED = "denied"
    RETRY_SAME_EXECUTOR = "retry_same_executor"
    RETRY_ALTERNATE_EXECUTOR = "retry_alternate_executor"


@dataclass(frozen=True)
class PolicyDecision:
    outcome: PolicyOutcome
    event_type: str | None
    reason: str
    excluded_executor_ids: frozenset[str] = frozenset()
    detail: dict = field(default_factory=dict)


class PolicyGate:
    def __init__(
        self,
        profile: "praxis_policy.profiles.PolicyProfile",
        ledger: "praxis_policy.budgets.BudgetLedger",
        *,
        granted_authority_scopes: frozenset[str] = frozenset(),
    ) -> None:
        self._profile = profile
        self._ledger = ledger
        self._granted_authority_scopes = granted_authority_scopes

    def authorize_start(self, node_metadata: dict) -> PolicyDecision:
        """Decide whether a node may proceed to launch.

        The caller must apply this decision's `event_type` (if any) before
        the node is asked to run: this method has no view of the run's
        actual `NodeStatus`, so it assumes the node is still `PENDING` or
        about to become `RUNNING`. Both `"handoff"` and `"fail"` are legal
        directly from `RUNNING` per `_TRANSITIONS`.
        """
        decision = authority.evaluate_authority(
            node_metadata.get("authority_requirement"),
            self._profile,
            granted_scopes=self._granted_authority_scopes,
        )

        if decision.outcome is AuthorityOutcome.AUTO_APPROVED:
            return PolicyDecision(
                outcome=PolicyOutcome.AUTHORIZED,
                event_type=None,
                reason="authority requirement satisfied",
            )
        if decision.outcome is AuthorityOutcome.HUMAN_REQUIRED:
            return PolicyDecision(
                outcome=PolicyOutcome.HUMAN_REQUIRED,
                event_type="handoff",
                reason="required authority scope is unresolved",
                detail={"unresolved_scopes": sorted(decision.unresolved_scopes)},
            )
        return PolicyDecision(
            outcome=PolicyOutcome.DENIED,
            event_type="fail",
            reason="prohibited authority scope was granted",
            detail={"denied_scopes": sorted(decision.denied_scopes)},
        )

    def decide_on_failure(
        self,
        node_id: str,
        node_metadata: dict,
        failure_payload: dict | None,
        *,
        previously_tried_executor_ids: frozenset[str] = frozenset(),
    ) -> PolicyDecision:
        """Decide how to respond to a failed execution of a `RUNNING` node.

        The caller applies the returned `event_type` (both `"handoff"` and
        `"block"` are legal directly from `RUNNING` per `_TRANSITIONS`); for
        `"block"`, the caller later applies `"resume"` (the only legal
        transition out of `BLOCKED`) once ready to relaunch, optionally
        against an alternate executor filtered by `excluded_executor_ids`.
        """
        budget = budgets.effective_budget(self._profile, node_metadata.get("budget_requirement"))
        classification = failure_classification.classify_failure(failure_payload)

        if classification is FailureClass.SUBSTANTIVE:
            return PolicyDecision(
                outcome=PolicyOutcome.HUMAN_REQUIRED,
                event_type="handoff",
                reason="substantive failure requires human review",
            )

        if self._ledger.is_retry_exhausted(node_id, budget):
            return PolicyDecision(
                outcome=PolicyOutcome.HUMAN_REQUIRED,
                event_type="handoff",
                reason="retry budget exhausted",
                detail={
                    "retries_used": self._ledger.retries_used(node_id),
                    "max_retries": budget.max_retries,
                },
            )

        self._ledger.record_retry(node_id)

        if (
            self._profile.allow_alternate_executor_retry
            and previously_tried_executor_ids
            and not self._ledger.is_repair_exhausted(node_id, budget)
        ):
            self._ledger.record_repair(node_id)
            return PolicyDecision(
                outcome=PolicyOutcome.RETRY_ALTERNATE_EXECUTOR,
                event_type="block",
                reason="transient failure retried against an alternate executor",
                excluded_executor_ids=previously_tried_executor_ids,
            )

        return PolicyDecision(
            outcome=PolicyOutcome.RETRY_SAME_EXECUTOR,
            event_type="block",
            reason="transient failure retried against the same executor",
        )


def human_denial_event_sequence() -> list[str]:
    """The event sequence modeling a human explicitly rejecting a `HANDOFF` node.

    `_TRANSITIONS` has no direct `HANDOFF -> TERMINAL_FAILED` edge, and this
    bundle does not add one. A human denial is instead modeled as accepting
    the handoff (`"accept"`, the only legal transition out of `HANDOFF`,
    landing back on `RUNNING`) immediately followed by failing the
    now-`RUNNING` node (`"fail"`).
    """
    return ["accept", "fail"]

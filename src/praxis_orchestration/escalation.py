"""The escalation ladder: topology, tier table, and runner.

Topology
--------
An escalation ladder is a chain of attempt nodes ending in a single human
node. Each attempt node has one outgoing `"on-failure"` edge pointing at the
next attempt, and the last attempt points at the human node. `"on-failure"`
edges fire only when their source reaches `TERMINAL_FAILED`
(`praxis_runtime.transitions`), so applying `fail` on an attempt node is what
advances the ladder one rung.

The edge kind is spelled with a hyphen. `TransitionEngine` compares against
the literal `"on-failure"` (`src/praxis_runtime/transitions.py:238,246`), and
the `kind` pattern in `src/praxis_contracts/schemas/v1/graph.schema.json`
(`^[a-z0-9]+(-[a-z0-9]+)*$`) rejects an underscored spelling outright, so
`"on_failure"` would both miss the engine's comparison and fail schema
validation in `load_graph`.

Tier table
----------
Each rung narrows executor eligibility to one authentication transport, from
the cheapest outward: first local executors, then subscription-backed ones,
then whatever the default policy admits. The ladder never widens the default.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Callable, Sequence

from praxis_executors.interface import ExecutionRequest
from praxis_executors.interface import ExecutorStatus
from praxis_executors.policy import (
    AllowListPolicy,
    AuthTransportPolicy,
    DenyListPolicy,
    as_eligibility_callable,
)
from praxis_executors.registry import ExecutorRegistry
from praxis_policy.gate import PolicyGate
from praxis_policy.gate import PolicyOutcome
from praxis_policy.receipts import record_policy_decision
from praxis_runtime.events import EventLog
from praxis_runtime.graph import Edge, Node
from praxis_runtime.transitions import TransitionEngine

# `"local"` and `"subscription_cli"` are both members of
# `_RECOGNIZED_AUTH_TRANSPORTS` in `src/praxis_executors/policy.py`; a name
# outside that set would make every candidate ineligible. `None` means the
# default-constructed `AuthTransportPolicy()`.
ATTEMPT_TIERS: tuple[frozenset[str] | None, ...] = (
    frozenset({"local"}),
    frozenset({"subscription_cli"}),
    None,
)


def tier_transport_policy(tier_index: int) -> AuthTransportPolicy:
    """Return the transport policy for one rung of the ladder.

    A named tier narrows eligibility to exactly that transport; the final tier
    is the default-constructed policy. The ladder never widens the default and
    so never admits `metered_api` or `api_key` — those are unsafe by default in
    `praxis_executors.policy`, and opting in to a paid transport stays with the
    caller.
    """
    tier = ATTEMPT_TIERS[tier_index]
    if tier is None:
        return AuthTransportPolicy()
    return AuthTransportPolicy(allowed_auth_transports=tier)


def build_escalation_ladder(
    *,
    attempt_node_ids: Sequence[str],
    human_node_id: str,
    node_kind: str = "executor-attempt",
    human_node_kind: str = "human-escalation",
) -> tuple[list[Node], list[Edge]]:
    """Build the attempt nodes, the human node, and the `"on-failure"` chain.

    Raises `ValueError` when `attempt_node_ids` is empty, contains duplicates,
    or when `human_node_id` collides with an attempt id.
    """
    attempt_ids = list(attempt_node_ids)
    if not attempt_ids:
        raise ValueError("attempt_node_ids must name at least one attempt")
    if len(set(attempt_ids)) != len(attempt_ids):
        raise ValueError(f"attempt_node_ids contains duplicates: {attempt_ids!r}")
    if human_node_id in attempt_ids:
        raise ValueError(
            f"human_node_id collides with an attempt id: {human_node_id!r}"
        )

    # Every node carries an empty `metadata` dict on purpose: an attempt node
    # must not carry an `evidence_requirement`. `TransitionEngine._check_evidence`
    # runs on `TERMINAL_FAILED` too, so an unsatisfied gate would raise
    # `TransitionError` on `apply(node, "fail")` and deadlock the ladder at the
    # rung that was supposed to advance it.
    nodes = [Node(id=node_id, kind=node_kind, metadata={}) for node_id in attempt_ids]
    nodes.append(Node(id=human_node_id, kind=human_node_kind, metadata={}))

    targets = [*attempt_ids[1:], human_node_id]
    edges = [
        Edge(source=source, target=target, kind="on-failure")
        for source, target in zip(attempt_ids, targets)
    ]
    return nodes, edges


def _tier_eligibility(
    registry: ExecutorRegistry,
    tier: frozenset[str] | None,
    tried_executor_ids: frozenset[str],
) -> Callable[[str], bool]:
    """Return a snapshot-based eligibility predicate for one ladder tier."""
    advertisements = registry.advertisements()
    transport = (
        AuthTransportPolicy()
        if tier is None
        else AuthTransportPolicy(allowed_auth_transports=tier)
    )
    transport_eligibility = as_eligibility_callable(transport, advertisements)
    deny_eligibility = as_eligibility_callable(
        DenyListPolicy(denied_executor_ids=tried_executor_ids), advertisements
    )
    return lambda executor_id: transport_eligibility(executor_id) and deny_eligibility(
        executor_id
    )


def _pinned_eligibility(
    registry: ExecutorRegistry, executor_id: str
) -> Callable[[str], bool]:
    """Return a predicate that permits exactly the executor selected earlier."""
    advertisements = registry.advertisements()
    return as_eligibility_callable(
        AllowListPolicy(allowed_executor_ids=frozenset({executor_id})), advertisements
    )


@dataclass(frozen=True)
class AttemptOutcome:
    """What happened on one rung of the ladder."""

    node_id: str
    attempt_index: int  # 1-based
    executor_id: str | None  # None when the tier had no candidate
    status: str  # "succeeded" | "failed" | "no-candidate"
    reason: str


@dataclass(frozen=True)
class EscalationResult:
    """Where the ladder ended and everything it tried on the way."""

    outcome: str  # "succeeded" | "human_required"
    terminal_node_id: str
    attempts: tuple[AttemptOutcome, ...]
    tried_executor_ids: frozenset[str]


def run_escalation_ladder(
    engine: TransitionEngine,
    registry: ExecutorRegistry,
    gate: PolicyGate,
    event_log: EventLog,
    *,
    run_id: str,
    graph_version: str,
    requirement: dict,
    request: ExecutionRequest,
    attempt_node_ids: Sequence[str],
    human_node_id: str,
    node_metadata: dict | None = None,
    tiers: Sequence[frozenset[str] | None] = ATTEMPT_TIERS,
) -> EscalationResult:
    """Walk the ladder, one attempt node per tier, until it ends.

    For each attempt node the runner applies `start`, selects a candidate under
    that tier's transport policy, executes it, and acts on the outcome:

    | Situation | Transition applied | Ladder effect |
    | --- | --- | --- |
    | Execution succeeds | `complete` with the converted proof records | Ladder ends, `outcome="succeeded"` |
    | No eligible candidate in this tier | `fail` (no gate call, no budget consumed) | Advances to the next tier |
    | Gate returns `RETRY_ALTERNATE_EXECUTOR` | `fail` | Advances to the next tier, this executor added to the denied set |
    | Gate returns `RETRY_SAME_EXECUTOR` | `block`, then `resume` | Re-runs the same node against the same tier, denied set unchanged |
    | Gate returns `HUMAN_REQUIRED`, before the last attempt | `handoff` | Ladder ends in `HANDOFF` on that attempt node |
    | Last attempt fails for any reason other than success | `fail` | The `"on-failure"` edge reaches the human node, where the runner applies `start` then `handoff` |

    A tier advance is applied as `fail` rather than the `"block"` event the
    gate returns, because `"on-failure"` edges fire only on `TERMINAL_FAILED`.
    The gate stays decision-only; choosing the event is the caller's part.
    """
    attempt_ids = list(attempt_node_ids)
    if not attempt_ids:
        raise ValueError("attempt_node_ids must name at least one attempt")
    if len(tiers) < len(attempt_ids):
        raise ValueError("tiers must provide one transport tier per attempt node")

    # The ledger in praxis_policy.budgets is keyed by an opaque node id. Use
    # the first attempt id for every decision so the budget bounds the ladder,
    # rather than giving each rung an independent retry allowance.
    budget_node_id = attempt_ids[0]
    metadata = node_metadata or {}
    tried_executor_ids: set[str] = set()
    outcomes: list[AttemptOutcome] = []

    for attempt_position, node_id in enumerate(attempt_ids):
        tier = tiers[attempt_position]
        engine.apply(node_id, "start")
        selected_executor_id: str | None = None
        final_status = "no-candidate"
        final_reason = "no eligible executor"
        same_executor_retries = 0

        while True:
            if selected_executor_id is None:
                eligibility = _tier_eligibility(
                    registry, tier, frozenset(tried_executor_ids)
                )
            else:
                # A same-executor retry must remain pinned to the executor
                # whose transient failure produced the decision. It is the
                # only intentional exception to the tried-executor deny list.
                eligibility = _pinned_eligibility(registry, selected_executor_id)

            match = registry.select(requirement, is_eligible=eligibility)
            if match.selected is None:
                # MatchResult.selected and MatchResult.unsatisfied are the
                # canonical fields from praxis_executors.matching.MatchResult.
                final_status = "no-candidate"
                final_reason = "; ".join(
                    promise.reason for promise in match.unsatisfied
                ) or "no eligible executor"
                engine.apply(node_id, "fail")
                break

            selected_executor_id = match.selected.executor_id
            tried_executor_ids.add(selected_executor_id)
            result, proof_records = registry.execute_with_proof_records(
                requirement,
                request,
                run_id=run_id,
                graph_version=graph_version,
                node_id=node_id,
                is_eligible=_pinned_eligibility(registry, selected_executor_id),
            )

            if result.status is ExecutorStatus.SUCCEEDED:
                engine.apply(node_id, "complete", evidence=proof_records)
                outcomes.append(
                    AttemptOutcome(
                        node_id=node_id,
                        attempt_index=attempt_position + 1,
                        executor_id=selected_executor_id,
                        status="succeeded",
                        reason="execution succeeded",
                    )
                )
                return EscalationResult(
                    outcome="succeeded",
                    terminal_node_id=node_id,
                    attempts=tuple(outcomes),
                    tried_executor_ids=frozenset(tried_executor_ids),
                )

            final_status = "failed"
            final_reason = result.payload.get("failure_class", result.status.value)
            decision = gate.decide_on_failure(
                budget_node_id,
                metadata,
                result.payload,
                previously_tried_executor_ids=frozenset(tried_executor_ids),
            )
            # Policy receipts are audit events; the actual transition remains
            # the orchestration caller's responsibility.
            record_policy_decision(
                event_log,
                run_id=run_id,
                node_id=node_id,
                decision=decision,
            )

            if decision.outcome is PolicyOutcome.RETRY_ALTERNATE_EXECUTOR:
                # on-failure edges fire only on TERMINAL_FAILED, so `fail`
                # advances the ladder. PolicyGate is decision-only; its
                # returned `block` event is not applied for this branch.
                engine.apply(node_id, "fail", evidence=proof_records)
                break

            if decision.outcome is PolicyOutcome.RETRY_SAME_EXECUTOR:
                engine.apply(node_id, "block")
                engine.apply(node_id, "resume")
                same_executor_retries += 1
                # The real bound is PolicyGate's retry ledger returning
                # HUMAN_REQUIRED once is_retry_exhausted holds. This cap is a
                # defensive guard against a faulty/custom gate implementation.
                if same_executor_retries > 128:
                    decision = None
                    final_reason = "same-executor retry safety limit exhausted"
                    engine.apply(node_id, "handoff")
                    break
                continue

            # HUMAN_REQUIRED (and any future non-retry outcome) is terminal
            # for this rung. Before the final rung, a handoff leaves the
            # failed attempt auditable and stops before creating a successor.
            # On the final rung, fail first so its on-failure edge creates the
            # human cursor; this preserves the ladder's terminal topology.
            if attempt_position == len(attempt_ids) - 1:
                engine.apply(node_id, "fail", evidence=proof_records)
                engine.apply(human_node_id, "start")
                engine.apply(human_node_id, "handoff")
                outcomes.append(
                    AttemptOutcome(
                        node_id=node_id,
                        attempt_index=attempt_position + 1,
                        executor_id=selected_executor_id,
                        status=final_status,
                        reason=decision.reason,
                    )
                )
                return EscalationResult(
                    outcome="human_required",
                    terminal_node_id=human_node_id,
                    attempts=tuple(outcomes),
                    tried_executor_ids=frozenset(tried_executor_ids),
                )

            engine.apply(node_id, "handoff")
            outcomes.append(
                AttemptOutcome(
                    node_id=node_id,
                    attempt_index=attempt_position + 1,
                    executor_id=selected_executor_id,
                    status=final_status,
                    reason=decision.reason,
                )
            )
            return EscalationResult(
                outcome="human_required",
                terminal_node_id=node_id,
                attempts=tuple(outcomes),
                tried_executor_ids=frozenset(tried_executor_ids),
            )

        outcomes.append(
            AttemptOutcome(
                node_id=node_id,
                attempt_index=attempt_position + 1,
                executor_id=selected_executor_id,
                status=final_status,
                reason=final_reason,
            )
        )

        if attempt_position == len(attempt_ids) - 1:
            # The last failed rung creates the human cursor through its
            # on-failure edge. Start and hand off that cursor explicitly.
            engine.apply(human_node_id, "start")
            engine.apply(human_node_id, "handoff")
            return EscalationResult(
                outcome="human_required",
                terminal_node_id=human_node_id,
                attempts=tuple(outcomes),
                tried_executor_ids=frozenset(tried_executor_ids),
            )

    # The loop always returns at the last attempt; this keeps the type checker
    # honest if the control flow is changed later.
    raise RuntimeError("escalation ladder ended without a terminal result")

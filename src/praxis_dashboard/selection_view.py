"""Selection-reasoning projection for the dashboard.

`build_selection_reasoning` answers, per graph node that declares a
requirement, which executor was selected and why every other candidate was
not. It reuses `matching.match`'s own verdicts rather than restating its
eligibility or kind-coverage rules -- the same discipline
`match_cmd._candidate_verdict` records ("so a candidate is only called
policy-excluded when `matching.match` says so"), and for the same reason:
the CLI and the dashboard answer one question, so a rule spelled a second
time here is a rule that can drift from the matcher it claims to report.
The per-candidate reason strings are `match_cmd`'s, produced by calling its
helper rather than re-deriving them.

That helper, `match_cmd._candidate_verdict`, is private, and importing it
here points the dashboard at the CLI. The coupling is deliberate and the
plan (docs/develop/plans/b-issue50.md, T3) directs it: copying the wording
instead would put the same sentence on two surfaces with nothing keeping
them equal, which is the drift AC-B3 exists to prevent. The honest fix is
to promote the helper to a module both surfaces may import (it depends
only on `matching` and `fields`), but `src/praxis_cli/match_cmd.py` is
outside this task's footprint, so that move belongs to a task that owns it.

Requirement dicts are read from `node.metadata["requirement"]` (the key
src/overlays/development/graph.py sets, shaped by
schemas/v1/requirement.schema.json) and are never synthesized; a node
without one contributes no entry. Advertisements are plain dicts shaped by
schemas/v1/capability-advertisement.schema.json.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Callable

from praxis_cli import fields
from praxis_cli.match_cmd import _candidate_verdict
from praxis_executors import matching, policy
from praxis_runtime.graph import Graph


@dataclass(frozen=True)
class UnsatisfiedPromiseView:
    kind: str
    constraint: str
    reason: str
    policy_excluded: bool


@dataclass(frozen=True)
class SelectionCandidateView:
    executor_id: str
    eligible: str  # "yes" | "no" | "unknown"
    # None exactly when the candidate ranked, which needs no explanation.
    # `eligible="yes"` alone does not imply None: an eligible candidate that
    # still failed to rank -- it covers some required kind but not every one
    # of them -- carries `match_cmd`'s reason for that, which is the wording
    # `praxis executors match --explain` prints beside `eligible=yes`
    # (AC-B3). The plan's interface sketch reads "None when eligible ==
    # yes"; matching the CLI's own output takes precedence over the sketch,
    # because a projection that dropped the reason would report less than
    # the surface it claims to mirror.
    reason: str | None
    rank: int | None  # index in MatchResult.ranked, or None when unranked
    policy_excluded: bool


@dataclass(frozen=True)
class SelectionReasoningView:
    node_id: str
    selected_executor_id: str | None
    candidates: tuple[SelectionCandidateView, ...]
    unsatisfied: tuple[UnsatisfiedPromiseView, ...]


def _candidate_view(
    requirement: dict,
    advertisement: dict,
    rank_by_id: dict[str, int],
    judged: dict[str, dict],
    is_eligible: Callable[[str], bool],
) -> SelectionCandidateView:
    """One candidate's row, worded as `src/praxis_cli/match_cmd.py:200-256` words it."""
    executor_id = advertisement["executor_id"]
    if judged[executor_id] is not advertisement:
        # A duplicate advertised id is resolved last-wins by `match` and by the
        # policy alike, so this advertisement was read by neither.
        # `eligible=unknown` for the reason `match_cmd` gives it: the only
        # verdict on file answers for the other advertisement, and reporting it
        # here would credit this one with an eligibility the policy never
        # judged, or with the other's rank.
        return SelectionCandidateView(
            executor_id=executor_id,
            eligible="unknown",
            reason=(
                f"another advertisement carries the same executor id ({executor_id}); "
                "that advertisement was the one judged"
            ),
            rank=None,
            policy_excluded=False,
        )
    if executor_id in rank_by_id:
        return SelectionCandidateView(
            executor_id=executor_id,
            eligible="yes",
            reason=None,
            rank=rank_by_id[executor_id],
            policy_excluded=False,
        )
    eligible, reason = _candidate_verdict(
        requirement, advertisement, fields.capability_kinds(advertisement), is_eligible
    )
    return SelectionCandidateView(
        executor_id=executor_id,
        eligible="yes" if eligible else "no",
        reason=reason,
        rank=None,
        # The same predicate `_candidate_verdict` appends
        # `_POLICY_EXCLUDED_SUFFIX` on: the policy verdict is what makes
        # `eligible=no` true, so the flag and the suffix cannot disagree.
        policy_excluded=not eligible,
    )


def build_selection_reasoning(
    graph: Graph,
    advertisements: list[dict] | tuple[dict, ...] | None,
) -> tuple[SelectionReasoningView, ...]:
    # No advertisements is replay mode: there is no live registry to have
    # judged anything, and an empty list gives the policy nothing to judge
    # either, so neither yields a projection rather than an empty verdict.
    if not advertisements:
        return ()

    advertisements = list(advertisements)
    # The same construction `ExecutorRegistry.select` and `run_match` use, so
    # the dashboard judges candidates against the policy the runtime applies.
    is_eligible = policy.as_eligibility_callable(policy.AuthTransportPolicy(), advertisements)
    # Last wins, because that is how both `as_eligibility_callable` and `match`
    # resolve a duplicate advertised id: each builds a dict keyed by that id
    # over this same list.
    judged = {advertisement["executor_id"]: advertisement for advertisement in advertisements}

    views: list[SelectionReasoningView] = []
    for node in graph.nodes.values():
        requirement = node.metadata.get("requirement")
        if requirement is None:
            continue
        result = matching.match(requirement, advertisements, is_eligible=is_eligible)
        rank_by_id = {
            candidate.executor_id: rank for rank, candidate in enumerate(result.ranked)
        }
        views.append(
            SelectionReasoningView(
                node_id=node.id,
                selected_executor_id=(
                    result.selected.executor_id if result.selected is not None else None
                ),
                candidates=tuple(
                    _candidate_view(
                        requirement, advertisement, rank_by_id, judged, is_eligible
                    )
                    for advertisement in advertisements
                ),
                # `match`'s own reasons, unreworded; the exclusion is marked by
                # the flag here rather than by `match_cmd._POLICY_EXCLUDED_SUFFIX`,
                # which is that surface's way of marking the same thing.
                unsatisfied=tuple(
                    UnsatisfiedPromiseView(
                        kind=entry.kind,
                        constraint=entry.constraint,
                        reason=entry.reason,
                        policy_excluded=entry.policy_excluded,
                    )
                    for entry in result.unsatisfied
                ),
            )
        )
    return tuple(views)

"""Tests for the dashboard's selection-reasoning projection
(`praxis_dashboard.selection_view.build_selection_reasoning`).

The projection reads `matching.match`'s own verdicts rather than restating
its rules, so these tests pin the projected wording to `match_cmd`'s: the
same question must not be answered two different ways on two surfaces
(AC-B3). Advertisement fixtures follow
schemas/v1/capability-advertisement.schema.json and their `auth_transport`
values exercise the default `AuthTransportPolicy` the projection wires in
the same way `ExecutorRegistry.select` and `run_match` do -- "local" is
recognized and safe, "metered_api" is unsafe-by-default and produces
`policy_excluded=True` when it is the only advertisement of a required
kind.

Requirements come from `node.metadata["requirement"]` -- the key
`src/overlays/development/graph.py` sets and requirement.schema.json
shapes -- and are never synthesized here (AC-B2).
"""

from __future__ import annotations

from conftest import _FakeExecutor

from praxis_cli.match_cmd import _candidate_verdict, run_match
from praxis_dashboard.selection_view import (
    SelectionCandidateView,
    UnsatisfiedPromiseView,
    build_selection_reasoning,
)
from praxis_executors import policy
from praxis_runtime.graph import Graph, Node

_SPEC_VERSION = "1.0.0"
_GRAPH_VERSION = "1.0.0"

_GENERATION = "development.code-generation"
_REVIEW = "development.code-review"


def _requirement(*kinds: str, constraint: str = "required") -> dict:
    return {
        "spec_version": _SPEC_VERSION,
        "requirements": [
            {
                "promise": {"spec_version": _SPEC_VERSION, "kind": kind},
                "constraint": constraint,
            }
            for kind in kinds
        ],
    }


def _capability(kind: str, auth_transport: str, *, cost: float | None = None) -> dict:
    entry: dict = {"kind": kind}
    if cost is not None:
        entry["parameters"] = {"cost": cost}
    return {
        "spec_version": _SPEC_VERSION,
        "auth_transport": auth_transport,
        "satisfies": [entry],
    }


def _advertisement(
    executor_id: str, kind: str, auth_transport: str, *, cost: float | None = None
) -> dict:
    return {
        "spec_version": _SPEC_VERSION,
        "executor_id": executor_id,
        "capabilities": [_capability(kind, auth_transport, cost=cost)],
    }


def _graph(nodes: dict[str, Node]) -> Graph:
    entry = next(iter(nodes), "n1")
    return Graph(
        spec_version=_GRAPH_VERSION,
        nodes=nodes,
        edges=[],
        entry_node=entry,
        terminal_nodes={entry},
    )


def _requiring_graph(*kinds: str, node_id: str = "n1") -> Graph:
    return _graph(
        {
            node_id: Node(
                id=node_id,
                kind="implement",
                metadata={"requirement": _requirement(*kinds)},
            )
        }
    )


def test_advertisements_none_yields_empty_projection():
    """Replay mode has no live registry to judge anything against (AC-E2)."""
    assert build_selection_reasoning(_requiring_graph(_GENERATION), None) == ()


def test_empty_advertisement_list_yields_empty_projection():
    assert build_selection_reasoning(_requiring_graph(_GENERATION), []) == ()


def test_graph_with_no_requirement_metadata_yields_empty_projection():
    """AC-B2: no requirement on disk means no projection, not a fabricated one."""
    graph = _graph(
        {
            "n1": Node(id="n1", kind="implement", metadata={}),
            "n2": Node(id="n2", kind="verify", metadata={"label": "verify"}),
        }
    )

    assert build_selection_reasoning(graph, [_advertisement("x-1", _GENERATION, "local")]) == ()


def test_near_miss_metadata_key_is_not_read_as_a_requirement():
    """AC-B2: the key is `requirement`, singular, and nothing else.

    `src/overlays/development/graph.py` sets exactly that key, and the
    projection is forbidden from synthesizing a requirement. A node carrying
    a plausible neighbour -- `requirements`, the plural that names the list
    *inside* a requirement document -- must therefore contribute nothing:
    widening the lookup to accept it would let the dashboard report a
    selection for a node that never declared one.
    """
    graph = _graph(
        {
            "n1": Node(
                id="n1",
                kind="implement",
                metadata={"requirements": _requirement(_GENERATION)},
            ),
            "n2": Node(
                id="n2",
                kind="implement",
                metadata={"node_requirement": _requirement(_GENERATION)},
            ),
        }
    )

    assert build_selection_reasoning(graph, [_advertisement("x-1", _GENERATION, "local")]) == ()


def test_only_nodes_declaring_a_requirement_contribute_entries():
    graph = _graph(
        {
            "n1": Node(id="n1", kind="implement", metadata={}),
            "n2": Node(
                id="n2",
                kind="verify",
                metadata={"requirement": _requirement(_GENERATION)},
            ),
        }
    )

    views = build_selection_reasoning(graph, [_advertisement("x-1", _GENERATION, "local")])

    assert [view.node_id for view in views] == ["n2"]


def test_eligible_candidate_is_selected_and_needs_no_reason():
    views = build_selection_reasoning(
        _requiring_graph(_GENERATION),
        [_advertisement("executor-good", _GENERATION, "local")],
    )

    assert len(views) == 1
    view = views[0]
    assert view.node_id == "n1"
    assert view.selected_executor_id == "executor-good"
    assert view.unsatisfied == ()
    assert len(view.candidates) == 1
    candidate = view.candidates[0]
    assert candidate.executor_id == "executor-good"
    assert candidate.eligible == "yes"
    assert candidate.reason is None
    assert candidate.policy_excluded is False
    # Pinned to the absolute index, not merely "ranked": the plan defines rank
    # as the position in `MatchResult.ranked`, so a projection that enumerated
    # from any other origin would still be reporting an order while reporting
    # the wrong number. (`match_cmd` prints a 1-based `score` for the same
    # position; the dashboard reports the index the plan names.)
    assert candidate.rank == 0


def test_ranked_candidates_carry_ascending_ranks_cheapest_first():
    """Ranking itself is out of scope; the projection must report `match`'s
    order, so the selected candidate outranks the costlier one."""
    views = build_selection_reasoning(
        _requiring_graph(_GENERATION),
        [
            _advertisement("executor-costly", _GENERATION, "local", cost=0.9),
            _advertisement("executor-cheap", _GENERATION, "local", cost=0.1),
        ],
    )

    view = views[0]
    assert view.selected_executor_id == "executor-cheap"
    rank_by_id = {candidate.executor_id: candidate.rank for candidate in view.candidates}
    # The exact pair, not just the ordering: ranks are indices into
    # `MatchResult.ranked`, so both the order and the origin are asserted.
    assert rank_by_id == {"executor-cheap": 0, "executor-costly": 1}


def test_policy_excluded_candidate_is_marked_and_worded_as_match_cmd_words_it():
    """AC-B3: the projected reason is `match_cmd`'s own, suffix included."""
    advertisement = _advertisement("executor-excluded", _GENERATION, "metered_api")

    views = build_selection_reasoning(_requiring_graph(_GENERATION), [advertisement])

    view = views[0]
    assert view.selected_executor_id is None
    assert view.candidates == (
        SelectionCandidateView(
            executor_id="executor-excluded",
            eligible="no",
            reason=(
                "policy excludes this candidate for required kind(s): "
                f"{_GENERATION} (policy_excluded)"
            ),
            rank=None,
            policy_excluded=True,
        ),
    )

    # The same verdict `praxis executors match --explain` prints for this
    # candidate, so the two surfaces cannot drift apart in wording.
    requirement = _requirement(_GENERATION)
    is_eligible = policy.as_eligibility_callable(
        policy.AuthTransportPolicy(), [advertisement]
    )
    _, cli_reason = _candidate_verdict(
        requirement, advertisement, [_GENERATION], is_eligible
    )
    assert view.candidates[0].reason == cli_reason


def test_eligible_but_unranked_candidate_keeps_match_cmds_reason():
    """AC-B3: `eligible="yes"` does not imply no reason.

    A candidate the policy allows can still fail to rank when it covers one
    required kind and not the other, and `praxis executors match --explain`
    prints a reason beside `eligible=yes` in exactly that case. The
    projection reports the same sentence rather than the plan sketch's
    `None`, which would say less than the surface it mirrors.
    """
    generation_only = _advertisement("executor-gen", _GENERATION, "local")
    review_only = _advertisement("executor-rev", _REVIEW, "local")
    graph = _graph(
        {
            "n1": Node(
                id="n1",
                kind="implement",
                metadata={"requirement": _requirement(_GENERATION, _REVIEW)},
            )
        }
    )

    views = build_selection_reasoning(graph, [generation_only, review_only])

    view = views[0]
    assert view.selected_executor_id is None
    by_id = {candidate.executor_id: candidate for candidate in view.candidates}
    candidate = by_id["executor-gen"]
    assert candidate.eligible == "yes"
    assert candidate.rank is None
    assert candidate.policy_excluded is False
    assert candidate.reason == f"does not satisfy required kind(s): {_REVIEW}"

    is_eligible = policy.as_eligibility_callable(
        policy.AuthTransportPolicy(), [generation_only, review_only]
    )
    cli_eligible, cli_reason = _candidate_verdict(
        _requirement(_GENERATION, _REVIEW), generation_only, [_GENERATION], is_eligible
    )
    assert cli_eligible is True
    assert candidate.reason == cli_reason


def test_duplicate_advertised_executor_id_leaves_the_unjudged_one_unknown():
    """AC-B3: `unknown` is for a candidate the policy never got to judge.

    `match` and the policy both key a candidate by advertised id, so a
    duplicate is resolved last-wins and the earlier advertisement -- and its
    own auth transport -- was read by neither. Reporting it as `no` would
    credit it with a verdict passed on the other advertisement, which is why
    `match_cmd` prints `eligible=unknown` for the same collision.

    The reason is asserted whole here; `_DUPLICATE_ID_TAIL` below is what
    ties it to the CLI's own sentence.
    """
    unjudged = _advertisement("executor-dup", _GENERATION, "local")
    judged = _advertisement("executor-dup", _GENERATION, "metered_api")

    views = build_selection_reasoning(_requiring_graph(_GENERATION), [unjudged, judged])

    view = views[0]
    assert view.selected_executor_id is None
    first, second = view.candidates
    assert first == SelectionCandidateView(
        executor_id="executor-dup",
        eligible="unknown",
        reason=(
            "another advertisement carries the same executor id (executor-dup); "
            "that advertisement was the one judged"
        ),
        rank=None,
        policy_excluded=False,
    )
    # The advertisement that was judged answers for the id, and its own
    # transport is what the policy excluded it on.
    assert second.eligible == "no"
    assert second.policy_excluded is True


# The part of the duplicate-id reason both surfaces must word identically.
# The subject differs by design and only there: `match_cmd` is walking a
# mapping of *adapter names*, so it names the other adapter, while the
# projection is given plain advertisement dicts and has no adapter name to
# use -- it names the other advertisement instead. Everything from the
# object onward answers the same question on both surfaces and is asserted
# against the CLI's live output below, so a rewording on either side fails
# here rather than drifting quietly (AC-B3).
_DUPLICATE_ID_TAIL = "the same executor id (executor-dup); that advertisement was the one judged"
_CLI_DUPLICATE_ID_SUBJECT = "another adapter advertises "
_PROJECTION_DUPLICATE_ID_SUBJECT = "another advertisement carries "


def test_duplicate_id_reason_shares_the_clis_wording_apart_from_its_subject(capsys):
    """AC-B3: the projection mirrors `praxis executors match --explain`.

    Unlike the other wording tests, this one cannot go through
    `_candidate_verdict`: the duplicate-id sentence is built inline in
    `run_match`'s `--explain` loop (src/praxis_cli/match_cmd.py:243-247),
    not in a helper the dashboard can call. So the CLI is run for the same
    collision and its printed reason compared piece by piece -- shared tail,
    deliberately different subject. Promoting the sentence to a constant
    both surfaces import would be the better fix, but `match_cmd.py` is
    outside this task's footprint.
    """
    adapters = {
        "adapter-unjudged": _FakeExecutor(
            "executor-dup", capabilities=[_capability(_GENERATION, "local")]
        ),
        "adapter-judged": _FakeExecutor(
            "executor-dup", capabilities=[_capability(_GENERATION, "metered_api")]
        ),
    }

    assert run_match(adapters, capabilities=[_GENERATION], explain=True) == 0

    line = next(
        line
        for line in capsys.readouterr().out.splitlines()
        if line.startswith("adapter-unjudged:")
    )
    cli_reason = line.split("reason=", 1)[1]

    views = build_selection_reasoning(
        _requiring_graph(_GENERATION),
        [
            _advertisement("executor-dup", _GENERATION, "local"),
            _advertisement("executor-dup", _GENERATION, "metered_api"),
        ],
    )
    projected_reason = views[0].candidates[0].reason

    assert cli_reason.endswith(_DUPLICATE_ID_TAIL)
    assert projected_reason.endswith(_DUPLICATE_ID_TAIL)
    assert cli_reason[: -len(_DUPLICATE_ID_TAIL)] == _CLI_DUPLICATE_ID_SUBJECT
    assert projected_reason[: -len(_DUPLICATE_ID_TAIL)] == _PROJECTION_DUPLICATE_ID_SUBJECT


def test_unsatisfied_required_kind_surfaces_unaltered():
    """AC-B1: `kind`, `constraint`, `reason` and `policy_excluded` as-is."""
    views = build_selection_reasoning(
        _requiring_graph(_GENERATION),
        [_advertisement("executor-other", _REVIEW, "local")],
    )

    view = views[0]
    assert view.selected_executor_id is None
    assert view.unsatisfied == (
        UnsatisfiedPromiseView(
            kind=_GENERATION,
            constraint="required",
            reason=f"no eligible advertisement satisfies '{_GENERATION}'",
            policy_excluded=False,
        ),
    )


def test_policy_excluded_unsatisfied_promise_keeps_matchs_own_reason():
    """AC-B1: a policy exclusion is marked, and its reason is not reworded."""
    views = build_selection_reasoning(
        _requiring_graph(_GENERATION),
        [_advertisement("executor-excluded", _GENERATION, "metered_api")],
    )

    assert views[0].unsatisfied == (
        UnsatisfiedPromiseView(
            kind=_GENERATION,
            constraint="required",
            reason=(
                f"'{_GENERATION}' is satisfied by at least one advertisement, "
                "but policy excludes every eligible candidate for it"
            ),
            policy_excluded=True,
        ),
    )

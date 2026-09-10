"""RED-phase proof for T10 (issue #28): `docs/overlays/development.md` must
describe the widened graph shape -- the bundle lane, the recovery lane, the
recovery/retry edges (#32), the
`build_development_graph()` bypass of `load_graph()`'s reachability check --
and list the four new proof types (T4) and four new graders (T5), all without
rewriting the existing core-boundary-rule preamble or `conflict_fn`
wiring section (the gap it once described was closed by issue #27; see
tests/test_repair_findings_b1_issue27.py).

This is a doc-content task with no other test file covering
`docs/overlays/development.md`'s prose (unlike T19's
`docs/parity/state-event-migration.md`, which `test_parity_fixtures.py`
already asserts against). T20's `docs/parity/decision.md` resolved the same
class of question by adding a dedicated doc-content test file
(`tests/test_parity_decision_addendum.py`); this file applies the identical
resolution to T10, per `agents/tdd-writer.md`'s priority that a real test in
a built-in facility beats no test.

Bundle remediation-1's T10/T11 (issue #32) later fixed the recovery/retry
edges' `kind` from `sequential` to `on-failure` and corrected this doc's
"fires unconditionally on TERMINAL_SUCCESS" claim to describe the fixed
`on-failure`/`TERMINAL_FAILED` semantics; the retry-count/budget/exhaustion
gap the paragraph also discloses remains real and undone.
"""

from __future__ import annotations

from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
DEV_DOC = REPO_ROOT / "docs" / "overlays" / "development.md"

CONFLICT_FN_HEADING = "**`conflict_fn` wiring:**"

EXISTING_HEADINGS = [
    "## Manifest",
    "## Graph",
    "## Graders",
    "## Resource provider",
    "## Composition",
    "## See also",
]

BUNDLE_LANE_EDGES = [
    "plan_bundle` -> `task_scheduler",
    "task_scheduler` -> `bundle_verify",
    "bundle_verify` -> `final_review",
    "final_review` -> `documentation_review",
    "documentation_review` -> `create_pr",
]

RECOVERY_LANE_NAMES = ["context_recovery", "blocker_recovery", "awaiting_human"]

NEW_PROOF_TYPES = [
    "development.plan-done",
    "development.bundle-verify-pass",
    "development.doc-review-done",
    "development.pr-created",
]


def _doc_text() -> str:
    return DEV_DOC.read_text()


def _section(heading_marker: str) -> str:
    doc_text = _doc_text()
    idx = doc_text.find(heading_marker)
    assert idx != -1, f"expected {DEV_DOC} to contain a heading matching {heading_marker!r}"
    rest = doc_text[idx:]
    next_idx = rest.find("\n## ", 1)
    return rest if next_idx == -1 else rest[:next_idx]


def test_existing_headings_and_gap_sections_untouched() -> None:
    doc_text = _doc_text()
    for heading in EXISTING_HEADINGS:
        assert heading in doc_text, (
            f"T10 must extend around the existing {heading!r} section, not remove/rename it"
        )
    assert CONFLICT_FN_HEADING in doc_text, (
        "T10 must not rewrite the existing conflict_fn wiring section"
    )
    assert "core-boundary" in doc_text and "rule enforced by" in doc_text, (
        "T10 must not rewrite the existing core-boundary-rule preamble"
    )


def test_graph_section_describes_bundle_lane_topology() -> None:
    section = _section("## Graph")
    for edge_text in BUNDLE_LANE_EDGES:
        assert f"`{edge_text}`" in section, (
            f"## Graph section must describe the bundle-lane edge `{edge_text}`"
        )
    assert "`repair_bundle`" in section, (
        "## Graph section must name `repair_bundle` as the bundle lane's retry node"
    )
    assert "retry" in section.lower(), (
        "## Graph section must describe repair_bundle as a retry branch off "
        "bundle_verify/final_review"
    )


def test_graph_section_describes_recovery_lane_as_topology_only() -> None:
    section = _section("## Graph")
    for name in RECOVERY_LANE_NAMES:
        assert f"`{name}`" in section, (
            f"## Graph section must name the recovery-lane node `{name}`"
        )
    assert "topology-only" in section, (
        "## Graph section must describe the recovery lane as topology-only"
    )
    assert "`repair_bundle` -> `awaiting_human`" in section, (
        "## Graph section must name repair_bundle -> awaiting_human as the recovery "
        "lane's only edge"
    )


def test_graph_section_describes_recovery_retry_edges_as_on_failure() -> None:
    section = _section("## Graph")
    assert "_advance_successors" in section, (
        "## Graph section must name TransitionEngine._advance_successors as the "
        "mechanism these edges rely on"
    )
    assert "on-failure" in section, (
        "## Graph section must describe the three repair/recovery edges as "
        "kind=\"on-failure\", not the stale topology-only/unconditional claim"
    )
    assert "TERMINAL_FAILED" in section, (
        "## Graph section must state these edges fire only on the source node's "
        "genuine TERMINAL_FAILED, not unconditionally on success"
    )
    assert "#32" in section, (
        "## Graph section must cross-reference #32, matching the conflict_fn "
        "wiring paragraph's disclosure register"
    )


def test_graph_section_retains_retry_budget_gap_disclosure() -> None:
    section = _section("## Graph")
    section_lower = section.lower()
    assert "retry" in section_lower and (
        "budget" in section_lower or "exhaustion" in section_lower
    ), (
        "## Graph section must retain the disclosure that these edges still don't "
        "model /develop's actual retry-count/budget/exhaustion semantics -- only the "
        "'fires on the wrong condition' claim is stale, this broader gap remains real"
    )


def test_graph_section_notes_build_development_graph_bypasses_reachability_check() -> None:
    section = _section("## Graph")
    assert "load_graph" in section, (
        "## Graph section must name load_graph() as the reachability check "
        "build_development_graph() bypasses"
    )
    assert "reachability" in section.lower(), (
        "## Graph section must describe the bypassed check as a reachability check"
    )
    assert "hand-construct" in section.lower(), (
        "## Graph section must note build_development_graph() hand-constructs "
        "Graph(...) directly instead of loading it"
    )
    assert "write_tdd" in section, (
        "## Graph section must name write_tdd as the entry_node the new nodes "
        "are unreachable from"
    )


def test_manifest_section_lists_four_new_proof_types() -> None:
    section = _section("## Manifest")
    for proof_type in NEW_PROOF_TYPES:
        assert f"`{proof_type}`" in section, (
            f"## Manifest section must list the new proof type `{proof_type}`"
        )


def test_graders_section_lists_four_new_graders() -> None:
    section = _section("## Graders")
    for proof_type in NEW_PROOF_TYPES:
        assert f"`{proof_type}`" in section, (
            f"## Graders section must list the new grader for `{proof_type}`"
        )

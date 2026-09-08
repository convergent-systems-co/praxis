"""Regression tests for repair-findings.md (bundle b2-issue28).

Each test reproduces one finding before its fix and must pass after it:

1. (Important) `docs/overlays/development.md`'s description of
   `load_graph()` claimed it "performs no reachability check from
   entry_node over the edge set" -- factually wrong: `load_graph()`
   (`src/praxis_runtime/graph.py`) calls `_reachable_from()` and raises
   `GraphValidationError` on any unreachable node, per that module's own
   docstring. The true, relevant fact (`build_development_graph()` never
   calls `load_graph()`, so the check doesn't run for *this* graph) was
   wrapped in an incorrect description of `load_graph()`'s own behavior.
2. (Important) `docs/parity/state-event-migration.md`'s "See also" entry
   for `docs/overlays/development-compat.md` claimed that file covers "the
   recovery-lane routing events", but `development-compat.md` was not
   touched by this bundle and still describes `compat.py` as exposing only
   two pure functions and the `VERIFY_DONE`/`REVIEW_APPROVED` slice -- no
   mention of `legacy_event_to_recovery_node`, `_EVENT_NODE_MAP`, or any
   bundle-lane/recovery event this bundle added to `compat.py`.
3. (Minor) `tests/test_captured_run_report.py`'s `_reachable_node_ids()`
   duplicated the exact BFS/adjacency-map algorithm already implemented as
   `_reachable_from()` in `src/praxis_runtime/graph.py`, instead of reusing
   it.
"""

from __future__ import annotations

from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
DEVELOPMENT_DOC_PATH = REPO_ROOT / "docs" / "overlays" / "development.md"
MIGRATION_DOC_PATH = REPO_ROOT / "docs" / "parity" / "state-event-migration.md"
CAPTURED_RUN_REPORT_TEST_PATH = REPO_ROOT / "tests" / "test_captured_run_report.py"


def test_development_doc_does_not_misdescribe_load_graph_reachability_check():
    text = DEVELOPMENT_DOC_PATH.read_text()

    assert "performs no reachability check" not in text, (
        "docs/overlays/development.md must not claim load_graph() performs no "
        "reachability check -- src/praxis_runtime/graph.py's load_graph() calls "
        "_reachable_from() and raises GraphValidationError on any node "
        "unreachable from entry_node, per that module's own docstring"
    )
    assert "build_development_graph() bypasses" in text or "bypasses `load_graph()`" in text, (
        "docs/overlays/development.md must still make the true, relevant claim: "
        "build_development_graph() hand-constructs a Graph(...) and never calls "
        "load_graph(), so that function's real reachability check never runs "
        "for this graph"
    )


def test_migration_doc_does_not_overclaim_development_compat_doc_coverage():
    text = MIGRATION_DOC_PATH.read_text()
    compat_doc_text = (REPO_ROOT / "docs" / "overlays" / "development-compat.md").read_text()

    assert "cites for category H and the recovery-lane routing events" not in text, (
        "docs/parity/state-event-migration.md's See-also entry for "
        "docs/overlays/development-compat.md must not claim that file covers "
        "the recovery-lane routing events -- development-compat.md was not "
        "touched by this bundle and still only describes compat.py's original "
        "two-function, VERIFY_DONE/REVIEW_APPROVED-only surface"
    )
    assert "legacy_event_to_recovery_node" not in compat_doc_text, (
        "sanity check: this test's premise is that development-compat.md was "
        "not updated for the recovery-lane additions -- if it now documents "
        "legacy_event_to_recovery_node, the See-also claim being tested here "
        "may no longer be an overclaim"
    )


def test_captured_run_report_reuses_graph_reachable_from_helper():
    text = CAPTURED_RUN_REPORT_TEST_PATH.read_text()

    assert "adjacency: dict[str, list[str]] = {}" not in text, (
        "tests/test_captured_run_report.py must not duplicate the "
        "BFS/adjacency-map reachability algorithm already implemented as "
        "_reachable_from() in src/praxis_runtime/graph.py -- it should import "
        "and reuse that helper instead"
    )
    assert "_reachable_from" in text, (
        "tests/test_captured_run_report.py must reuse "
        "praxis_runtime.graph._reachable_from() instead of reimplementing it"
    )

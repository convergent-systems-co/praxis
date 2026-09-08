"""RED-phase proof for T20 (issue #30): `docs/parity/decision.md` must carry
a new, separately-headed addendum documenting the widened-overlay gap
closure, without editing the existing `## T3`/`## T4`/`## T6`/`## T7`
sections.

This is a doc-content task with no other test file covering
`docs/parity/decision.md`'s prose (unlike T19's
`docs/parity/state-event-migration.md`, which `test_parity_fixtures.py`
already asserts against). Per `agents/tdd-writer.md`'s priority that a real
test in a built-in facility beats no test, this file reads the rendered
markdown and asserts on required substrings the same way
`test_parity_fixtures.py`'s `test_state_event_migration_doc_*` tests do.
"""

from __future__ import annotations

from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
DECISION_DOC = REPO_ROOT / "docs" / "parity" / "decision.md"

ADDENDUM_HEADING = "## Addendum (issue #30): widened-overlay gap closure"

R_CATEGORY_CAVEAT = (
    "structurally present, not yet functionally reachable via a real failure "
    "transition, see #32"
)
R_CATEGORY_NAMES = [
    "repair_bundle",
    "context_recovery",
    "blocker_recovery",
    "awaiting_human",
    "CONCERN_TRIAGED",
    "TASK_REPAIR_DONE",
]


def _addendum_section() -> str:
    doc_text = DECISION_DOC.read_text()
    assert ADDENDUM_HEADING in doc_text, (
        f"expected {DECISION_DOC} to contain the heading {ADDENDUM_HEADING!r} -- "
        "T20 appends this addendum after the existing T3/T4/T6/T7 sections"
    )
    start = doc_text.index(ADDENDUM_HEADING)
    return doc_text[start:]


def test_addendum_heading_present_and_existing_sections_untouched() -> None:
    doc_text = DECISION_DOC.read_text()
    assert ADDENDUM_HEADING in doc_text
    for existing_heading in ("## T3 —", "## T4 —", "## T6 —", "## T7 —"):
        assert existing_heading in doc_text, (
            f"T20 must not remove or rename the existing {existing_heading!r} section"
        )


def test_addendum_states_category_b_closed_except_bundle_scheduler() -> None:
    section = _addendum_section()
    assert "category B" in section or "Category B" in section
    assert "`bundle_scheduler`" in section, (
        "addendum must name `bundle_scheduler` as category B's still-open exception"
    )
    assert "closed" in section


def test_addendum_states_r_category_caveat_verbatim_for_every_named_entry() -> None:
    section = _addendum_section()
    assert R_CATEGORY_CAVEAT in section, (
        f"addendum must state the exact caveat phrase {R_CATEGORY_CAVEAT!r} for the "
        "category R node/event names -- not a paraphrase"
    )
    for name in R_CATEGORY_NAMES:
        assert f"`{name}`" in section, (
            f"addendum must name `{name}` among the category R/H entries covered by "
            "the structurally-present-but-not-functionally-reachable caveat"
        )


def test_addendum_states_human_required_remains_fully_open() -> None:
    section = _addendum_section()
    assert "`human_required`" in section, (
        "addendum must name `human_required` as remaining fully open, distinct from "
        "the `awaiting_human` caveat"
    )
    assert "open" in section


def test_addendum_restates_performance_parity_as_open() -> None:
    section = _addendum_section()
    assert "performance parity" in section.lower()
    assert "open" in section.lower()


def test_addendum_cross_references_sibling_docs_without_duplicating_tables() -> None:
    section = _addendum_section()
    assert "state-event-migration.md" in section
    assert "development.md" in section

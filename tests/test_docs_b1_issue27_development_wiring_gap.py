"""RED-phase test for bundle b1-issue27, task T3.

`docs/overlays/development.md`'s "`conflict_fn` wiring gap" paragraph currently
says claims against `development.filesystem` "fall back to" exact-identifier
matching. Once T3 closes the gap, the paragraph must state that
`TransitionEngine._lease_conflict_fn` now selects the glob-aware
`paths_overlap` conflict function for `development.filesystem` (matching the
`.`-segment suffix rule), not the stale fallback claim.
"""

from __future__ import annotations

from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
DEVELOPMENT_DOC_PATH = REPO_ROOT / "docs" / "overlays" / "development.md"


def _wiring_gap_paragraph() -> str:
    text = DEVELOPMENT_DOC_PATH.read_text()
    marker = "**`conflict_fn` wiring:**"
    start = text.index(marker)
    end = text.index("\n## ", start)
    return text[start:end]


def test_wiring_gap_paragraph_no_longer_claims_exact_identifier_fallback():
    paragraph = _wiring_gap_paragraph()

    assert "fall back to" not in paragraph, (
        "docs/overlays/development.md's wiring-gap paragraph still claims "
        "development.filesystem claims fall back to exact-identifier "
        "matching, which contradicts the fixed _lease_conflict_fn"
    )


def test_wiring_gap_paragraph_documents_the_dot_segment_suffix_match():
    paragraph = _wiring_gap_paragraph()

    assert "development.filesystem" in paragraph
    assert "glob-aware" in paragraph
    assert "segment" in paragraph, (
        "docs/overlays/development.md's wiring-gap paragraph must describe "
        "the final `.`-separated segment suffix match now used by "
        "TransitionEngine._lease_conflict_fn to select paths_overlap for "
        "development.filesystem"
    )

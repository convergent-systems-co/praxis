"""Regression tests for repair-findings.md (bundle b1-issue27).

Each test reproduces one finding before its fix and must pass after it:

1. `src/overlays/development/resources.py`'s module docstring still stated
   that `TransitionEngine._lease_conflict_fn` "only recognizes the literal
   resource_type \"filesystem\"" and "exposes no hook ... the gap is
   documented in docs/overlays/development.md instead of worked around
   here" -- this contradicts the fixed `_lease_conflict_fn`
   (`src/praxis_runtime/transitions.py`), which now selects the glob-aware
   `paths_overlap` conflict function for any resource type whose final
   `.`-separated segment is `"filesystem"`, and contradicts
   `docs/overlays/development.md`'s own paragraph, which this same bundle
   already rewrote to describe the fix.
2. `docs/overlays/development.md`'s "`conflict_fn` wiring gap:" heading was
   retained verbatim even though the paragraph beneath it now describes the
   gap as resolved, producing a heading/body mismatch. This also folds in
   the coverage that used to live in the now-removed
   `tests/test_docs_b1_issue27_development_wiring_gap.py` (the paragraph no
   longer claiming a "fall back to exact-identifier" fallback, and
   documenting the `.`-segment suffix match) -- both files were asserting
   facts about the very same doc paragraph via separate, overlapping string
   checks, so that coverage now lives in one place.

Claim checks use `_mentions`, a small case/hyphen/space-tolerant matcher,
rather than single literal phrases: a doc/docstring rewrite that preserves
the same underlying claim (e.g. "glob aware" instead of "glob-aware") should
not break these tests, only a real regression of the claim should.
"""

from __future__ import annotations

import ast
import re
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
DEVELOPMENT_DOC_PATH = REPO_ROOT / "docs" / "overlays" / "development.md"


def _module_docstring(path: Path) -> str:
    source = path.read_text()
    doc = ast.get_docstring(ast.parse(source))
    assert doc, f"{path} has no module docstring"
    return doc


def _wiring_paragraph() -> str:
    text = DEVELOPMENT_DOC_PATH.read_text()
    marker = "**`conflict_fn` wiring"
    start = text.index(marker)
    end = text.index("\n## ", start)
    return text[start:end]


def _mentions(text: str, *patterns: str) -> bool:
    """True if `text` matches any pattern, tolerating case/hyphen/space variants.

    Anchoring regression checks to one literal phrase makes them brittle to
    unrelated rewording that preserves the same claim; matching a small set
    of tolerant patterns instead keeps the check tied to the underlying
    claim rather than its exact wording.
    """
    return any(re.search(pattern, text, re.IGNORECASE) for pattern in patterns)


def test_mentions_helper_tolerates_wording_variants_of_the_same_claim():
    for variant in ("glob-aware", "glob aware", "GLOB-AWARE", "Glob Aware"):
        assert _mentions(variant, r"glob[\s-]*aware")
    assert not _mentions("exact-identifier matching only", r"glob[\s-]*aware")

    for variant in ("fall back to", "falls back to", "fallback to", "Fall Back"):
        assert _mentions(variant, r"falls?[\s-]*back")
    assert not _mentions("gets real glob-aware detection", r"falls?[\s-]*back")


def test_resources_module_docstring_reflects_resolved_conflict_fn_wiring():
    doc = _module_docstring(REPO_ROOT / "src" / "overlays" / "development" / "resources.py")

    assert not _mentions(doc, r"only recognizes the literal resource[_ ]type"), (
        "resources.py's module docstring still claims _lease_conflict_fn "
        "only recognizes the bare literal \"filesystem\" -- it must describe "
        "the fixed suffix-match behavior instead"
    )
    assert not _mentions(doc, r"the gap is documented"), (
        "resources.py's module docstring still frames the conflict_fn "
        "wiring as an unresolved gap -- it must describe the fix that "
        "already lives in core's _lease_conflict_fn"
    )
    assert _mentions(doc, r"glob[\s-]*aware"), (
        "resources.py's module docstring should describe that "
        "development.filesystem now gets glob-aware footprint-conflict "
        "detection through TransitionEngine"
    )


def test_development_md_wiring_section_no_longer_describes_a_gap():
    paragraph = _wiring_paragraph()

    assert not _mentions(paragraph, r"wiring gap"), (
        "docs/overlays/development.md still headlines the conflict_fn "
        "section as a 'wiring gap' even though the paragraph beneath it "
        "now describes the gap as resolved"
    )
    assert "`conflict_fn` wiring" in paragraph, (
        "docs/overlays/development.md should still headline the "
        "conflict_fn wiring section, just without the stale 'gap' framing"
    )
    assert not _mentions(paragraph, r"falls?[\s-]*back"), (
        "docs/overlays/development.md's wiring paragraph still claims "
        "development.filesystem claims fall back to exact-identifier "
        "matching, which contradicts the fixed _lease_conflict_fn"
    )


def test_development_md_wiring_section_documents_the_dot_segment_suffix_match():
    paragraph = _wiring_paragraph()

    assert "development.filesystem" in paragraph
    assert _mentions(paragraph, r"glob[\s-]*aware")
    assert _mentions(paragraph, r"segment"), (
        "docs/overlays/development.md's wiring paragraph must describe "
        "the final `.`-separated segment suffix match now used by "
        "TransitionEngine._lease_conflict_fn to select paths_overlap for "
        "development.filesystem"
    )

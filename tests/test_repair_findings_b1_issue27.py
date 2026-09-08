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
   gap as resolved, producing a heading/body mismatch.
"""

from __future__ import annotations

import ast
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent


def _module_docstring(path: Path) -> str:
    source = path.read_text()
    doc = ast.get_docstring(ast.parse(source))
    assert doc, f"{path} has no module docstring"
    return doc


def test_resources_module_docstring_reflects_resolved_conflict_fn_wiring():
    doc = _module_docstring(REPO_ROOT / "src" / "overlays" / "development" / "resources.py")

    assert "only recognizes the literal resource_type" not in doc, (
        "resources.py's module docstring still claims _lease_conflict_fn "
        "only recognizes the bare literal \"filesystem\" -- it must describe "
        "the fixed suffix-match behavior instead"
    )
    assert "the gap is documented" not in doc, (
        "resources.py's module docstring still frames the conflict_fn "
        "wiring as an unresolved gap -- it must describe the fix that "
        "already lives in core's _lease_conflict_fn"
    )
    assert "glob-aware" in doc, (
        "resources.py's module docstring should describe that "
        "development.filesystem now gets glob-aware footprint-conflict "
        "detection through TransitionEngine"
    )


def test_development_md_conflict_fn_heading_does_not_say_gap():
    text = (REPO_ROOT / "docs" / "overlays" / "development.md").read_text()

    assert "`conflict_fn` wiring gap" not in text, (
        "docs/overlays/development.md still headlines the conflict_fn "
        "section as a 'wiring gap' even though the paragraph beneath it "
        "now describes the gap as resolved"
    )
    assert "`conflict_fn` wiring" in text, (
        "docs/overlays/development.md should still headline the "
        "conflict_fn wiring section, just without the stale 'gap' framing"
    )

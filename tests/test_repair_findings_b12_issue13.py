"""Regression test for repair-findings.md (bundle b12-issue13).

Reproduces the one finding before its fix and must pass after it:

1. (Minor) `docs/eval.md`'s H1 title was immediately followed by the "See
   also" body paragraph with no blank line between them, deviating from
   every other doc file's convention (a blank line after the H1).
"""

from __future__ import annotations

from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
EVAL_DOC_PATH = REPO_ROOT / "docs" / "eval.md"


def test_eval_doc_has_blank_line_after_h1():
    lines = EVAL_DOC_PATH.read_text().splitlines()

    assert lines[0].startswith("# "), "docs/eval.md must start with an H1 title"
    assert lines[1] == "", (
        "docs/eval.md must have a blank line between its H1 title and the "
        "body paragraph, matching every other doc file's convention"
    )

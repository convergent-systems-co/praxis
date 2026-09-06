"""Regression tests for repair-findings.md (bundle b7-issue8).

Each test reproduces one finding before its fix and must pass after it:

1. `src/praxis_policy/budgets.py`'s module docstring cited an external issue
   number ("#12") and an internal task-graph label ("T1"), embedding
   software-development-process vocabulary into shipped source
   documentation -- a repeat of a mistake already blocked and repaired
   elsewhere in this run (see `tests/test_repair_findings_b3_issue4.py`).
2. `tests/test_authority_boundaries.py`'s module docstring cited "the task
   brief" and internal task-graph labels ("T1", "T2"), leaking planning
   vocabulary into test documentation.
3. `tests/test_retry_budgets.py`'s module docstring had the same "task
   brief" / task-graph-label leak as `test_authority_boundaries.py`.
4. `run_tests.sh` was added at the repo root without being named in any
   task's file list and without being referenced by README or docs -- an
   unrequested, undocumented addition.
"""

from __future__ import annotations

import ast
import re
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent

_ISSUE_OR_TASK_LABEL_PATTERN = re.compile(r"#\d+|\bT\d+\b")


def _module_docstring(path: Path) -> str:
    source = path.read_text()
    doc = ast.get_docstring(ast.parse(source))
    assert doc, f"{path} has no module docstring"
    return doc


def test_budgets_docstring_has_no_issue_or_task_references():
    doc = _module_docstring(REPO_ROOT / "src" / "praxis_policy" / "budgets.py")

    match = _ISSUE_OR_TASK_LABEL_PATTERN.search(doc)
    assert match is None, (
        f"budgets.py's module docstring still cites {match.group(0) if match else ''!r} "
        "-- it must describe the follow-up integration seam generically, "
        "without naming an issue number or internal task-graph label"
    )


def test_authority_boundaries_docstring_has_no_process_vocabulary():
    doc = _module_docstring(REPO_ROOT / "tests" / "test_authority_boundaries.py")

    assert "task brief" not in doc.lower(), (
        "test_authority_boundaries.py's module docstring still cites "
        "'the task brief' -- it must describe the duck-typed PolicyProfile "
        "design constraint without referencing this delivery pipeline"
    )
    match = _ISSUE_OR_TASK_LABEL_PATTERN.search(doc)
    assert match is None, (
        f"test_authority_boundaries.py's module docstring still cites "
        f"{match.group(0) if match else ''!r} -- it must describe only the "
        "design constraint, nothing about internal task numbering"
    )


def test_retry_budgets_docstring_has_no_process_vocabulary():
    doc = _module_docstring(REPO_ROOT / "tests" / "test_retry_budgets.py")

    assert "task brief" not in doc.lower(), (
        "test_retry_budgets.py's module docstring still cites 'the task "
        "brief' -- it must describe the duck-typed PolicyProfile design "
        "constraint without referencing this delivery pipeline"
    )
    match = _ISSUE_OR_TASK_LABEL_PATTERN.search(doc)
    assert match is None, (
        f"test_retry_budgets.py's module docstring still cites "
        f"{match.group(0) if match else ''!r} -- it must describe only the "
        "design constraint, nothing about internal task numbering"
    )


def test_no_undocumented_run_tests_script_at_repo_root():
    assert not (REPO_ROOT / "run_tests.sh").exists(), (
        "run_tests.sh was added at the repo root without being named in any "
        "task's file list and without being referenced by README or docs -- "
        "an unrequested, undocumented addition"
    )

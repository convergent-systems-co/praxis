"""Regression tests for the citation repairs in ADR 0001 (bundle b-issue58).

The tests keep the ADR's symbol and title citations meaningful by checking that
the cited source facts still exist, rather than only checking for prose in the
ADR itself.
"""

from __future__ import annotations

import re
from pathlib import Path

from praxis_runtime.transitions import NodeStatus, _TRANSITIONS

REPO_ROOT = Path(__file__).resolve().parent.parent
ADR_PATH = REPO_ROOT / "docs" / "adr" / "0001-capacity-tiering-boundary.md"
POLICY_PATH = REPO_ROOT / "docs" / "policy.md"
BUDGETS_PATH = REPO_ROOT / "src" / "praxis_policy" / "budgets.py"


def test_adr_cites_transitions_symbols_not_line_numbers() -> None:
    """The ADR must cite live transition symbols, not fragile line numbers."""
    assert hasattr(NodeStatus, "HANDOFF"), "NodeStatus.HANDOFF must remain a live enum member"
    assert _TRANSITIONS[NodeStatus.HANDOFF] == {"accept": NodeStatus.RUNNING}, (
        "the cited NodeStatus.HANDOFF transition row must remain the bare pause edge "
        "described by ADR 0001"
    )

    adr_text = " ".join(ADR_PATH.read_text().split())
    assert "_TRANSITIONS" in adr_text, "ADR 0001 must cite the live transition table"
    assert "NodeStatus.HANDOFF" in adr_text, "ADR 0001 must cite the live HANDOFF symbol"
    assert "line 71" not in adr_text, "the stale NodeStatus line citation must stay removed"
    assert "line 89" not in adr_text, "the stale transition-table line citation must stay removed"
    assert not re.search(r"lines? \d+(-\d+)?", adr_text), (
        "ADR 0001 must not regress to raw line-number citations"
    )


def test_adr_attributes_overlay_quote_to_budgets_module() -> None:
    """The overlay-integration quote must be attributed to budgets.py."""
    phrase = "a future overlay/integration layer to reconcile"
    budgets_text = " ".join(BUDGETS_PATH.read_text().split())
    policy_text = POLICY_PATH.read_text()
    assert phrase in budgets_text, "the quoted overlay-integration phrase must remain in budgets.py"
    assert phrase not in policy_text, "policy.md must not become the source of the budgets quote"

    adr_text = " ".join(ADR_PATH.read_text().split())
    assert phrase in adr_text, "ADR 0001 must retain the overlay-integration quote"
    phrase_index = adr_text.index(phrase)
    sentence_start = adr_text.rfind(". ", 0, phrase_index) + 2
    sentence_end = adr_text.find(". ", phrase_index) + 1
    sentence = adr_text[sentence_start:sentence_end]
    assert "budgets.py" in sentence, (
        "the sentence carrying the overlay-integration quote must identify "
        "src/praxis_policy/budgets.py as its source"
    )


def test_adr_cites_policy_bullet_by_title_not_line_range() -> None:
    """The BudgetLedger gap must be cited by its stable policy bullet title."""
    bullet = "**`BudgetLedger`'s in-memory-only persistence is a follow-up integration seam.**"
    policy_text = POLICY_PATH.read_text()
    assert bullet in policy_text, "the cited BudgetLedger policy bullet must remain present"

    adr_text = " ".join(ADR_PATH.read_text().split())
    title = "`BudgetLedger`'s in-memory-only persistence is a follow-up integration seam."
    assert "lines 203-207" not in adr_text, "the stale policy line range must stay removed"
    assert adr_text.count(title) >= 3, (
        "ADR 0001 must cite the BudgetLedger policy bullet at the context, "
        "alternative, and consequences sites"
    )

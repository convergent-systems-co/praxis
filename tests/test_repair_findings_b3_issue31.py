"""Regression test for repair-findings.md (bundle b3-issue31).

1. `docs/adr/0001-capacity-tiering-boundary.md` cited "Performance parity
   remains open" as if it were a titled section heading in
   `docs/parity/decision.md`. No such heading exists -- the quoted text is
   bold prose under the real "## Recommendation for acceptance criterion 6"
   heading. The quoted content itself is accurate; only the section label
   was imprecise.
"""

from __future__ import annotations

import re
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
ADR_PATH = REPO_ROOT / "docs" / "adr" / "0001-capacity-tiering-boundary.md"
DECISION_PATH = REPO_ROOT / "docs" / "parity" / "decision.md"


def test_adr_does_not_cite_performance_parity_phrase_as_a_section_heading():
    decision_headings = re.findall(r"^## (.+)$", DECISION_PATH.read_text(), re.MULTILINE)
    assert "Performance parity remains open" not in decision_headings, (
        "docs/parity/decision.md has no '## Performance parity remains open' "
        "heading -- that phrase is bold text under a differently named heading"
    )
    assert "Recommendation for acceptance criterion 6" in decision_headings, (
        "the real heading that contains the performance-parity content must "
        "still exist in docs/parity/decision.md for this test to be meaningful"
    )

    adr_text = ADR_PATH.read_text()
    assert '"Performance parity remains open" section' not in adr_text, (
        "the ADR must not cite 'Performance parity remains open' as a titled "
        "section of docs/parity/decision.md -- it is bold text under the "
        "'Recommendation for acceptance criterion 6' heading, not a heading itself"
    )
    assert "Recommendation for acceptance criterion 6" in adr_text, (
        "the ADR must cite the real heading name when referencing the "
        "performance-parity content in docs/parity/decision.md"
    )

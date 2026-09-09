"""Regression test for repair-findings.md (bundle b3-issue31).

1. `docs/adr/0001-capacity-tiering-boundary.md` cited "Performance parity
   remains open" as if it were a titled section heading in
   `docs/parity/decision.md`. No such heading exists -- the quoted text is
   bold prose under the real "## Recommendation for acceptance criterion 6"
   heading. The quoted content itself is accurate; only the section label
   was imprecise.
2. The same ADR made the identical mistake for
   `docs/overlays/development-compat.md`'s "Follow-up, out of scope here"
   text, citing it (twice) as a document "section". No such heading exists --
   the phrase is bold prose under the real
   "## Existing `develop` invocation is preserved, not transitioned" heading.
3. The same ADR paraphrased `docs/policy.md`'s "Fail-closed, no domain logic
   in core." bullet as "the fail-closed-design section", in a document whose
   other citations are exact quotes/headings.
"""

from __future__ import annotations

import re
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
ADR_PATH = REPO_ROOT / "docs" / "adr" / "0001-capacity-tiering-boundary.md"
DECISION_PATH = REPO_ROOT / "docs" / "parity" / "decision.md"
COMPAT_PATH = REPO_ROOT / "docs" / "overlays" / "development-compat.md"
POLICY_PATH = REPO_ROOT / "docs" / "policy.md"


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


def test_adr_does_not_cite_follow_up_phrase_as_a_section_heading():
    compat_headings = re.findall(r"^## (.+)$", COMPAT_PATH.read_text(), re.MULTILINE)
    assert "Follow-up, out of scope here" not in compat_headings, (
        "docs/overlays/development-compat.md has no "
        "'## Follow-up, out of scope here' heading -- that phrase is bold "
        "text under a differently named heading"
    )
    assert (
        "Existing `develop` invocation is preserved, not transitioned"
        in compat_headings
    ), (
        "the real heading that contains the follow-up content must still "
        "exist in docs/overlays/development-compat.md for this test to be "
        "meaningful"
    )

    adr_text = ADR_PATH.read_text()
    adr_text_unwrapped = " ".join(adr_text.split())
    assert '"Follow-up, out of scope here" section' not in adr_text_unwrapped, (
        "the ADR must not cite 'Follow-up, out of scope here' as a titled "
        "section of docs/overlays/development-compat.md -- it is bold text "
        "under the 'Existing `develop` invocation is preserved, not "
        "transitioned' heading, not a heading itself"
    )
    assert (
        "Existing `develop` invocation is preserved, not transitioned"
        in adr_text_unwrapped
    ), (
        "the ADR must cite the real heading name when referencing the "
        "follow-up content in docs/overlays/development-compat.md"
    )


def test_adr_quotes_policy_fail_closed_bullet_instead_of_paraphrasing():
    policy_text = POLICY_PATH.read_text()
    assert "**Fail-closed, no domain logic in core.**" in policy_text, (
        "the real bullet title must still exist in docs/policy.md for this "
        "test to be meaningful"
    )

    adr_text_unwrapped = " ".join(ADR_PATH.read_text().split())
    assert "fail-closed-design section" not in adr_text_unwrapped, (
        "the ADR must not paraphrase docs/policy.md's bullet as 'the "
        "fail-closed-design section' -- no such section exists; the real "
        "bullet is titled 'Fail-closed, no domain logic in core.'"
    )
    assert "Fail-closed, no domain logic in core" in adr_text_unwrapped, (
        "the ADR must cite the real bullet title when referencing "
        "docs/policy.md's fail-closed principle"
    )

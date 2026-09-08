"""RED proof for T3 (b4-issue32): docs/runtime.md must document "on-failure" edges.

Asserts the "Fail-closed guarantee" bullet under "## praxis_runtime.transitions" names the
new "on-failure" edge kind (per T1's transitions.py change and #32) and describes its
semantics: fires only on TERMINAL_FAILED, creates each target's cursor unconditionally
(fan-out-style, no join-on-failure equivalent), and never fires on TERMINAL_SUCCESS.
"""

import re
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
RUNTIME_DOC = REPO_ROOT / "docs" / "runtime.md"


def _fail_closed_guarantee_text() -> str:
    text = RUNTIME_DOC.read_text()
    match = re.search(
        r"\*\*Fail-closed guarantee:\*\*(.*?)\*\*Evidence audit trail:\*\*",
        text,
        re.DOTALL,
    )
    assert match is not None, (
        "docs/runtime.md's 'Fail-closed guarantee' bullet (or the 'Evidence audit "
        "trail' bullet that follows it) was not found -- has the section been renamed?"
    )
    return match.group(1)


def _on_failure_sentence() -> str:
    section = _fail_closed_guarantee_text()
    idx = section.find('"on-failure"')
    assert idx != -1, (
        "docs/runtime.md's Fail-closed guarantee bullet must name the \"on-failure\" "
        "edge kind next to the existing fan-out/join sentence"
    )
    return section[idx:]


def test_fail_closed_guarantee_names_on_failure_edge_kind_after_join_sentence():
    section = _fail_closed_guarantee_text()
    join_idx = section.find("join edges only create their")
    assert join_idx != -1, (
        "docs/runtime.md's existing fan-out/join sentence in the Fail-closed "
        "guarantee bullet was not found -- has it been reworded?"
    )
    on_failure_idx = section.find('"on-failure"')
    assert on_failure_idx != -1 and on_failure_idx > join_idx, (
        "docs/runtime.md must name the \"on-failure\" edge kind in a new sentence "
        "placed next to (after) the existing fan-out/join sentence"
    )


def test_on_failure_sentence_states_fires_only_on_terminal_failed():
    sentence = _on_failure_sentence()
    assert "TERMINAL_FAILED" in sentence, (
        "docs/runtime.md's \"on-failure\" sentence must state it fires only when "
        "its source reaches TERMINAL_FAILED"
    )


def test_on_failure_sentence_states_unconditional_fan_out_style_creation():
    sentence = _on_failure_sentence()
    assert "unconditionally" in sentence, (
        "docs/runtime.md's \"on-failure\" sentence must state each target's cursor "
        "is created unconditionally (fan-out-style)"
    )
    assert "join-on-failure" in sentence, (
        "docs/runtime.md's \"on-failure\" sentence must state there is no "
        "join-on-failure equivalent"
    )


def test_on_failure_sentence_states_never_fires_on_terminal_success():
    sentence = _on_failure_sentence()
    assert "TERMINAL_SUCCESS" in sentence and "never" in sentence.lower(), (
        "docs/runtime.md's \"on-failure\" sentence must state it never fires on "
        "TERMINAL_SUCCESS"
    )

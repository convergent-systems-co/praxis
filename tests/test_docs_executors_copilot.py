"""Doc regression tests for the shipped Copilot adapter (bundle b-issue42).

`docs/executors.md` still described Copilot as a hypothetical,
not-yet-implemented backend ("Adding a new backend (e.g. a future Copilot,
OpenCode, or MLX/local adapter ...)" and "None of those remaining hypothetical
adapters (Copilot, OpenCode, MLX) exist yet") even though `CopilotCliExecutor`
(`src/praxis_executors/adapters/copilot_cli.py`) ships in this branch. These
tests pin the doc to the current, correct claim: Copilot is a shipped concrete
adapter, not a hypothetical or future one.

They assert on the doc's text only and deliberately do not import
`praxis_executors.adapters.copilot_cli`, which keeps them independent of the
adapter's own implementation and tests. They are the Copilot counterparts of
the three `test_doc_*` tests in `test_repair_findings_b1_issue41.py`, which pin
the identical three claims for Codex.

Unlike those Codex tests, these assert on the *claim* rather than on one
phrasing of it: the checks run over whole sentences of unwrapped prose taken
from the whole document, so a restatement of the same wrong claim in other
words, one that lands on a continuation line of hard-wrapped prose, or one
placed outside the "Adding a new executor adapter" section still fails. The
recognised wordings are enumerated by `_HYPOTHETICAL`; a phrasing outside that
list is not caught.

That the class and module path this doc cites are real code, rather than a
claim outrunning the branch, is checked by
`tests/test_copilot_cli.py::test_the_shipped_adapter_the_doc_names_actually_exists` --
which is where the adapter import belongs.
"""

from __future__ import annotations

import re
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
EXECUTORS_DOC = REPO_ROOT / "docs" / "executors.md"

# Verbs that make a claim about something being real, shipped or working, so
# that a negation of one of them is a not-yet-real claim.
_REAL_VERB = (
    r"(?:exists?|existed|ships?|shipped|implemented|landed|written|build|built"
    r"|wired|arrived|available|ready|real)"
)

# Wordings that mark a backend as not-yet-real: any of them in the same
# sentence as Copilot restates the claim these tests forbid. The negated-verb
# alternative allows a couple of filler words ("has not yet been implemented",
# "is not currently available") but not a whole clause, so a correct claim that
# merely contains "not" ("is not registered by default") is not mistaken for a
# hypothetical one.
_HYPOTHETICAL = re.compile(
    r"hypothetical"
    r"|\b(?:future|planned|prospective|unimplemented|placeholder|someday)\b"
    r"|\bcoming soon\b"
    r"|\bnot yet\b"
    r"|\bexists? yet\b"
    r"|\b(?:is|are|was|were|do|does|did|will|would|has|have|had)\s+not"
    r"(?:\W+\w+){0,2}\W+" + _REAL_VERB + r"\b",
    re.IGNORECASE,
)


def _doc_text() -> str:
    return EXECUTORS_DOC.read_text()


def _adding_adapter_section() -> str:
    text = _doc_text()
    marker = "## Adding a new executor adapter"
    start = text.index(marker)
    return text[start:]


def _unwrapped(text: str) -> str:
    """Collapse hard-wrapped prose newlines to single spaces for substring checks."""
    return re.sub(r"\s+", " ", text)


def _sentences(text: str) -> list[str]:
    """Unwrapped prose split into sentences.

    The lookahead for a capital, backtick or digit keeps abbreviations
    ("e.g. a future ...") and dotted file names ("codex_cli.py") from
    splitting a sentence; a numbered list item does start a new one.
    """
    return re.split(r"(?<=[.!?])\s+(?=[A-Z`0-9])", _unwrapped(text))


def _sentence_naming(text: str, marker: str) -> str:
    """The sentence of `text` that starts at `marker`."""
    unwrapped = _unwrapped(text)
    return _sentences(unwrapped[unwrapped.index(marker) :])[0]


def test_doc_no_longer_lists_copilot_among_hypothetical_adapters() -> None:
    for sentence in _sentences(_doc_text()):
        if "Copilot" not in sentence:
            continue
        assert not _HYPOTHETICAL.search(sentence), (
            "docs/executors.md must not describe Copilot as a hypothetical, "
            "future or not-yet-implemented adapter now that CopilotCliExecutor "
            f"ships; offending sentence: {sentence!r}"
        )


def test_doc_lists_copilot_cli_executor_among_concrete_adapters() -> None:
    # Deliberately not pinned to the running adapter count or to which
    # adapter the prose happens to describe next: a seventh adapter or a
    # reordering of the list is unrelated to the finding this guards.
    section = _unwrapped(_adding_adapter_section())
    assert "`CopilotCliExecutor`" in section, (
        "docs/executors.md must list CopilotCliExecutor among the concrete "
        "adapters that ship today"
    )
    assert "src/praxis_executors/adapters/copilot_cli.py" in section, (
        "docs/executors.md must cite CopilotCliExecutor's module path"
    )
    copilot_description = section.split("`CopilotCliExecutor`", 1)[1].split(";", 1)[0]
    assert 'auth_transport: "subscription_cli"' in copilot_description, (
        "docs/executors.md must state CopilotCliExecutor's auth_transport"
    )


def test_doc_example_of_future_adapters_no_longer_names_copilot() -> None:
    # The whole sentence, not just its first physical line: the example is
    # hard-wrapped, so a Copilot mention can land on a continuation line.
    sentence = _sentence_naming(_doc_text(), "Adding a new backend")
    assert "Copilot" not in sentence, (
        "docs/executors.md's example of possible future backends must not "
        "still name Copilot now that it is a shipped adapter, not a future one"
    )

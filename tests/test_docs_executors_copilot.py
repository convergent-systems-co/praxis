"""Doc regression tests for the shipped Copilot adapter (bundle b-issue42).

`docs/executors.md` still described Copilot as a hypothetical,
not-yet-implemented backend ("Adding a new backend (e.g. a future Copilot,
OpenCode, or MLX/local adapter ...)" and "None of those remaining hypothetical
adapters (Copilot, OpenCode, MLX) exist yet") even though `CopilotCliExecutor`
(`src/praxis_executors/adapters/copilot_cli.py`) ships in this branch. These
tests pin the doc to the current, correct claims: Copilot is a shipped concrete
adapter, not a hypothetical or future one, and it is not registered by default.

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
recognised wordings are enumerated by `_HYPOTHETICAL` and by the registration
patterns below; a phrasing outside those lists is not caught.
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

# Wordings that assert registration. `ExecutorRegistry` itself is deliberately
# not matched: naming the type is not a claim that anything is registered.
_REGISTERED = re.compile(r"\bregist(?:er|ers|ered|ration)\b", re.IGNORECASE)

# Whatever a claim in this doc can be *about*: a named adapter class, a vendor
# name, or a collective ("None of the six ..."). Used to work out which adapter
# a registration verb is talking about, so that a correct claim about another
# adapter cannot vouch for a wrong one about Copilot in the same sentence.
# `ExecutorRegistry` and friends are excluded: the registry is the object of a
# registration, not the subject whose status is being described.
_SUBJECT = re.compile(
    r"`\w*Executor`"
    r"|\b(?:Copilot|Codex|Claude|Ollama|OpenCode|MLX|Subprocess|Fake)\w*"
    r"|\b(?:none|neither|nothing|no one) of the \w+",
    re.IGNORECASE,
)

# A denial that governs the registration verb it precedes: "is not registered",
# "is never auto-registered", "nor ... is registered". The bounded run of
# filler words is what keeps an unrelated "not" elsewhere in the same clause
# from vouching for the verb.
_DENIAL_BEFORE_VERB = re.compile(
    r"\b(?:not|never|nor|no longer)\b(?:\W+\w+){0,3}\W*$", re.IGNORECASE
)

# A denial carried by the subject itself, at the head of the clause: "None of
# the six is registered ...", "Nothing registers ...".
_COLLECTIVE_DENIAL = re.compile(r"^\W*(?:none|nothing|neither|nobody|no|not)\b", re.IGNORECASE)

# Clause boundaries inside a sentence. A claim is scoped to the clause its
# subject sits in, so a correct claim in a neighbouring clause cannot excuse a
# wrong one about Copilot.
_CLAUSE_BREAK = re.compile(r"[;,:—]")

_ADAPTER_MODULE = re.compile(r"src/praxis_executors/adapters/\w+\.py")
_SHIPPED_COUNT = re.compile(r"\b(\w+) concrete adapters ship today\b", re.IGNORECASE)
_UNREGISTERED_COUNT = re.compile(r"\bNone of the (\w+) is\b", re.IGNORECASE)
_NUMBER_WORDS = {
    "one": 1,
    "two": 2,
    "three": 3,
    "four": 4,
    "five": 5,
    "six": 6,
    "seven": 7,
    "eight": 8,
    "nine": 9,
    "ten": 10,
    "eleven": 11,
    "twelve": 12,
}


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


def _clause_start(sentence: str, index: int) -> int:
    """Offset at which the clause of `sentence` containing `index` begins."""
    breaks = [m.end() for m in _CLAUSE_BREAK.finditer(sentence) if m.end() <= index]
    return breaks[-1] if breaks else 0


def _nearest_subject(sentence: str, index: int) -> re.Match[str] | None:
    """The last subject mention in `sentence` that ends at or before `index`."""
    subjects = [m for m in _SUBJECT.finditer(sentence) if m.end() <= index]
    return subjects[-1] if subjects else None


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


def test_doc_does_not_claim_the_copilot_adapter_is_registered() -> None:
    # Registration is out of scope for this bundle: nothing registers
    # CopilotCliExecutor, so the doc must not say or imply that anything does.
    # Each registration verb is judged against the subject it is actually
    # about, within that subject's clause, so a true statement about another
    # adapter next to it cannot license a false one about Copilot.
    for sentence in _sentences(_doc_text()):
        for verb in _REGISTERED.finditer(sentence):
            subject = _nearest_subject(sentence, verb.start())
            if subject is None:
                # No subject before the verb: only Copilot's own sentences are
                # this test's business, and the whole run-up is the window.
                if "Copilot" not in sentence:
                    continue
                window_start = 0
            elif "copilot" not in subject.group(0).lower():
                continue
            else:
                window_start = _clause_start(sentence, subject.start())
            run_up = sentence[window_start : verb.start()]
            assert _DENIAL_BEFORE_VERB.search(run_up) or _COLLECTIVE_DENIAL.match(run_up), (
                "docs/executors.md must not state or imply that CopilotCliExecutor "
                f"is registered with an ExecutorRegistry; offending sentence: {sentence!r}"
            )


def test_doc_shipped_adapter_count_matches_the_adapters_it_lists() -> None:
    # Internal consistency, not a pinned count: a seventh adapter is fine as
    # long as both spelled counts are updated with it.
    section = _unwrapped(_adding_adapter_section())
    listed = {m.group(0) for m in _ADAPTER_MODULE.finditer(section)}
    shipped = _SHIPPED_COUNT.search(section)
    assert shipped, (
        "docs/executors.md must still say how many concrete adapters ship today"
    )
    assert _NUMBER_WORDS.get(shipped.group(1).lower()) == len(listed), (
        f"docs/executors.md says {shipped.group(1)!r} concrete adapters ship today "
        f"but lists {len(listed)} adapter modules: {sorted(listed)}"
    )
    unregistered = _UNREGISTERED_COUNT.search(section)
    assert unregistered, (
        "docs/executors.md must still deny that the shipped adapters are registered "
        "by default"
    )
    assert _NUMBER_WORDS.get(unregistered.group(1).lower()) == len(listed), (
        f"docs/executors.md denies registration for {unregistered.group(1)!r} adapters "
        f"but lists {len(listed)} adapter modules: {sorted(listed)}"
    )

"""Regression tests for repair-findings.md (bundle b1-issue41).

Finding: `docs/executors.md` still described Codex as a hypothetical,
not-yet-implemented adapter ("None of those hypothetical adapters (Codex,
Copilot, OpenCode, MLX) exist yet. Four concrete adapters ship today: ...")
even though `CodexCliExecutor` (`src/praxis_executors/adapters/codex_cli.py`)
shipped in this same branch. This test pins the doc to the current, correct
claim: Codex is a shipped concrete adapter, not a hypothetical one.

This module is also where the prose `codex_cli.py`'s own comments are pinned
to lives: those assertions exercise no code path, so they belong with the
doc pinning rather than among `tests/test_codex_cli.py`'s behavioural tests.
"""

from __future__ import annotations

import re
from pathlib import Path

from praxis_executors.adapters import codex_cli
from praxis_executors.interface import Executor

REPO_ROOT = Path(__file__).resolve().parent.parent
EXECUTORS_DOC = REPO_ROOT / "docs" / "executors.md"


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


def test_doc_no_longer_lists_codex_among_hypothetical_adapters() -> None:
    section = _adding_adapter_section()
    assert not re.search(r"hypothetical adapters \([^)]*Codex[^)]*\)", section), (
        "docs/executors.md must not describe Codex as a hypothetical, "
        "not-yet-implemented adapter now that CodexCliExecutor ships"
    )


def test_doc_lists_codex_cli_executor_among_concrete_adapters() -> None:
    # Deliberately not pinned to the running adapter count or to which
    # adapter the prose happens to describe next: a sixth adapter or a
    # reordering of the list is unrelated to the finding this guards.
    section = _unwrapped(_adding_adapter_section())
    assert "`CodexCliExecutor`" in section, (
        "docs/executors.md must list CodexCliExecutor among the concrete "
        "adapters that ship today"
    )
    assert "src/praxis_executors/adapters/codex_cli.py" in section, (
        "docs/executors.md must cite CodexCliExecutor's module path"
    )
    codex_description = section.split("`CodexCliExecutor`", 1)[1].split(";", 1)[0]
    assert 'auth_transport: "subscription_cli"' in codex_description, (
        "docs/executors.md must state CodexCliExecutor's auth_transport"
    )


# codex_cli.py's own comments


def _adapter_comments() -> str:
    """Every `#` comment line in codex_cli.py, unwrapped into one string.

    Comments only, so a phrase that also appears in the module docstring or
    in code does not satisfy an assertion about what a comment records.
    """
    lines = [
        line.strip().lstrip("#").strip()
        for line in Path(codex_cli.__file__).read_text().splitlines()
        if line.strip().startswith("#")
    ]
    return re.sub(r"\s+", " ", " ".join(lines))


def test_env_strip_comment_states_what_codex_doctor_actually_reports() -> None:
    # Verified live against the installed binary: running `codex doctor`
    # with and without CODEX_API_KEY leaves "stored auth mode" reported as
    # `chatgpt` in both runs. The only delta is an added "auth env vars
    # present" line, which is what the adapter's comment must record.
    comments = _adapter_comments()

    assert "auth env vars present" in comments, (
        "codex_cli.py's comment must state what `codex doctor` actually "
        "reports when CODEX_API_KEY is set"
    )


# The decision, not two common words: models and modes are not probed, and
# the reason is that the Capability contract is vendor/model-neutral and
# offers no field a discovered model list could travel in. Both matchers stay
# tolerant of rewording -- neither pins a full sentence.
_MODELS_AND_MODES_NOT_PROBED = re.compile(r"models?\s+and\s+modes?[^.]*\bnot\b", re.IGNORECASE)
_NO_CONTRACT_FIELD_FOR_MODELS = re.compile(r"no field[^.;]*\bmodel", re.IGNORECASE)


def test_adapter_records_its_models_and_modes_discovery_decision() -> None:
    # The spec's discovery bullet asks for "supported models/modes where
    # exposed". The auth-probe and env-var investigations each left their
    # outcome in a comment; the third discovery item left none, so there was
    # no record of whether models/modes had been considered at all.
    comments = _adapter_comments()

    assert _MODELS_AND_MODES_NOT_PROBED.search(comments), (
        "codex_cli.py must record that it deliberately does not probe "
        "supported models and modes, as it does for the auth probe and "
        "the stripped env vars"
    )
    assert _NO_CONTRACT_FIELD_FOR_MODELS.search(comments), (
        "codex_cli.py's models/modes note must state the reason the "
        "decision rests on: the contract has no field a discovered model "
        "list could travel in"
    )
    # Deliberately not also pinned to the phrase "vendor/model-neutral": that
    # quotes capability.schema.json's own wording, so the assertion would
    # break on a reword in either file while the decision it guards stayed
    # true. The reason matcher above carries that claim.
    assert "capability.schema.json" in comments, (
        "codex_cli.py's models/modes note must cite the contract that "
        "settles it -- capability.schema.json"
    )


# The decision, not a sentence: the subprocess-lifecycle block shared with
# ClaudeCliExecutor is duplicated on purpose, and the comment has to say so
# and name the sibling it duplicates. Both matchers stay tolerant of
# rewording.
_DUPLICATION_IS_DELIBERATE = re.compile(r"duplicat\w*", re.IGNORECASE)
_SHARING_ALTERNATIVE_WEIGHED = re.compile(r"shared base|factor\w*", re.IGNORECASE)


def test_adapter_records_why_its_subprocess_lifecycle_duplicates_the_sibling() -> None:
    # `_process_for`, `status`, `_terminal_status` and `cancel` are
    # byte-identical to claude_cli.py's. Duplication between two adapters is
    # a defensible call, but an unexplained one reads as an oversight, so
    # the reason belongs in the file next to the duplicated block. The two
    # matchers ask only that the comment names the duplication and the
    # sharing alternative it was weighed against, in any wording or order.
    comments = _adapter_comments()

    assert _DUPLICATION_IS_DELIBERATE.search(comments), (
        "codex_cli.py must record that its subprocess-lifecycle block "
        "duplicates the sibling adapter's, rather than leaving the "
        "duplication unremarked"
    )
    assert _SHARING_ALTERNATIVE_WEIGHED.search(comments), (
        "codex_cli.py's duplication note must say why the block was not "
        "factored into a shared base instead"
    )
    assert "claude_cli.py" in comments, (
        "codex_cli.py's duplication note must name the sibling module it "
        "duplicates"
    )


def test_adapter_exposes_no_public_method_outside_the_executor_abc() -> None:
    # A public method no production code calls is dead wiring: nothing on the
    # Executor ABC, in the registry, in policy or in the docs reads it, and
    # the sibling ClaudeCliExecutor has no equivalent, so only its own tests
    # keep it alive. Holding the adapter's public surface to the ABC's is
    # what stops that recurring.
    abc_surface = {name for name in vars(Executor) if not name.startswith("_")}
    adapter_surface = {
        name for name in vars(codex_cli.CodexCliExecutor) if not name.startswith("_")
    }

    assert adapter_surface <= abc_surface, (
        "CodexCliExecutor exposes public methods that are not on the "
        "Executor ABC and that no production code calls: "
        f"{sorted(adapter_surface - abc_surface)}"
    )


def test_doc_example_of_future_adapters_no_longer_names_codex() -> None:
    text = _doc_text()
    marker = "Adding a new backend"
    idx = text.index(marker)
    sentence_end = text.index("\n", idx)
    sentence = text[idx:sentence_end]
    assert "Codex" not in sentence, (
        "docs/executors.md's example of possible future backends must not "
        "still name Codex now that it is a shipped adapter, not a future one"
    )

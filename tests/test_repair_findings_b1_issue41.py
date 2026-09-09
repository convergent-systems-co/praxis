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


# Assembled from fragments so the pattern cannot match its own source text
# here; it is searched against codex_cli.py only.
_FALSE_AUTH_MODE_CLAIM = re.compile("flips" + r"[^.]*" + "reported auth mode", re.IGNORECASE)


def test_env_strip_comment_states_what_codex_doctor_actually_reports() -> None:
    # Verified live against the installed binary: running `codex doctor`
    # with and without CODEX_API_KEY leaves "stored auth mode" reported as
    # `chatgpt` in both runs. The only delta is an added "auth env vars
    # present" line, so the adapter's comment must not claim the variable
    # flips the reported mode.
    comments = _adapter_comments()

    assert not _FALSE_AUTH_MODE_CLAIM.search(comments), (
        "codex_cli.py must not claim CODEX_API_KEY changes the auth mode "
        "`codex doctor` reports -- it does not"
    )
    assert "auth env vars present" in comments, (
        "codex_cli.py's comment must state what `codex doctor` actually "
        "reports when CODEX_API_KEY is set"
    )


def test_adapter_records_its_models_and_modes_discovery_decision() -> None:
    # The spec's discovery bullet asks for "supported models/modes where
    # exposed". The auth-probe and env-var investigations each left their
    # outcome in a comment; the third discovery item left none, so there was
    # no record of whether models/modes had been considered at all.
    comments = _adapter_comments()

    assert re.search(r"\bmodels?\b", comments, re.IGNORECASE), (
        "codex_cli.py must record what it does about the spec's "
        "models/modes discovery item, as it does for the auth probe and "
        "the stripped env vars"
    )
    assert re.search(r"\bmodes?\b", comments, re.IGNORECASE), (
        "codex_cli.py's models/modes note must cover modes too"
    )
    assert "capability.schema.json" in comments, (
        "codex_cli.py's models/modes note must cite the contract that "
        "settles it -- capability.schema.json's vendor/model-neutral "
        "Capability, which has no field a discovered model list could "
        "travel in"
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

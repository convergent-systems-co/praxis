"""Regression tests for repair-findings.md (bundle b1-issue41).

Finding: `docs/executors.md` still described Codex as a hypothetical,
not-yet-implemented adapter ("None of those hypothetical adapters (Codex,
Copilot, OpenCode, MLX) exist yet. Four concrete adapters ship today: ...")
even though `CodexCliExecutor` (`src/praxis_executors/adapters/codex_cli.py`)
shipped in this same branch. This test pins the doc to the current, correct
claim: Codex is a shipped concrete adapter, not a hypothetical one.

Later findings against the same bundle are pinned here too, each by a test
that exercises the behaviour it is about. Assertions over the prose of
`codex_cli.py`'s own `#` comments used to live here as well; they were
removed as part of this file's own repair round, because they exercised no
code path and failed on a harmless reword. This convention pins published
prose in `docs/`, not implementation comments.
"""

from __future__ import annotations

import re
import subprocess
from pathlib import Path
from unittest.mock import MagicMock, patch

from praxis_executors.adapters import codex_cli
from praxis_executors.adapters.codex_cli import CodexCliExecutor
from praxis_executors.interface import (
    ExecutionRequest,
    Executor,
    ExecutorAvailability,
)

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


# Auth probe: an unrecognized login wording is ambiguous, not a denial


def _login_status(text: str) -> subprocess.CompletedProcess:
    return subprocess.CompletedProcess(
        args=["codex", "login", "status"], returncode=0, stdout="", stderr=text
    )


# A login line the probe cannot classify: it names neither the "using
# ChatGPT" wording the adapter recognizes nor an API key. A codex build that
# rewords its ChatGPT login line reads exactly like this.
_UNRECOGNIZED_LOGIN = "Logged in as a@b.c via ChatGPT subscription\n"


def test_detect_authenticated_is_unknown_for_an_unrecognized_login_wording() -> None:
    # Resolving this to False claims the CLI is unauthenticated, which is a
    # stronger claim than the probe supports. The spec's mapping sends an
    # ambiguous probe result to DEGRADED, i.e. None here.
    with patch(
        "praxis_executors.adapters.codex_cli.subprocess.run",
        return_value=_login_status(_UNRECOGNIZED_LOGIN),
    ):
        executor = CodexCliExecutor(executor_id="executor-codex-cli-repair")

        assert executor._detect_authenticated("/usr/bin/codex") is None


def test_health_is_degraded_for_an_unrecognized_login_wording() -> None:
    with (
        patch("praxis_executors.adapters.codex_cli.shutil.which", return_value="/usr/bin/codex"),
        patch(
            "praxis_executors.adapters.codex_cli.subprocess.run",
            return_value=_login_status(_UNRECOGNIZED_LOGIN),
        ),
    ):
        executor = CodexCliExecutor(executor_id="executor-codex-cli-repair")

        assert executor.health() == ExecutorAvailability.DEGRADED


def test_detect_authenticated_still_denies_an_api_key_login() -> None:
    # The counterpart the ambiguity branch must not swallow: a login the CLI
    # names as an API key is a metered credential, so it stays a denial.
    with patch(
        "praxis_executors.adapters.codex_cli.subprocess.run",
        return_value=_login_status("Logged in using an API key\n"),
    ):
        executor = CodexCliExecutor(executor_id="executor-codex-cli-repair")

        assert executor._detect_authenticated("/usr/bin/codex") is False


# Redaction coverage: token shapes the sk-/JWT/long-Bearer patterns miss


def _result_payload(stdout: str, stderr: str) -> dict:
    process = MagicMock()
    process.poll.return_value = 0
    process.communicate.return_value = (stdout, stderr)
    process.returncode = 0
    with (
        patch("praxis_executors.adapters.codex_cli.shutil.which", return_value="/usr/bin/codex"),
        patch("praxis_executors.adapters.codex_cli.subprocess.Popen", return_value=process),
    ):
        executor = CodexCliExecutor(executor_id="executor-codex-cli-repair")
        request = ExecutionRequest(
            promise={"spec_version": "1.0.0", "kind": "coding"},
            parameters={"prompt": "hello"},
        )
        return executor.result(executor.launch(request)).payload


SHORT_BEARER_TOKEN = "FAKEtok3n99"


def test_result_redacts_a_bearer_token_shorter_than_twenty_characters() -> None:
    # Nothing about a credential stops being a credential below twenty
    # characters; the length floor was only ever there to avoid matching
    # prose.
    header = f"Authorization: Bearer {SHORT_BEARER_TOKEN}"

    payload = _result_payload(f"...{header}...", f"...{header}...")

    assert SHORT_BEARER_TOKEN not in str(payload)
    assert "Bearer" in payload["stdout"]


OPAQUE_TOKEN = "FAKEOPAQUECHATGPTTOKEN0123456789"


def test_result_redacts_an_opaque_token_named_by_its_field() -> None:
    # An opaque ChatGPT token that is neither JWT-shaped nor behind a
    # `Bearer` prefix -- the shape `~/.codex/auth.json` holds it in, and the
    # shape it reaches stdout in when the CLI echoes that file back.
    line = f'{{"access_token": "{OPAQUE_TOKEN}"}}'

    payload = _result_payload(line, line)

    assert OPAQUE_TOKEN not in str(payload)
    assert OPAQUE_TOKEN not in payload["stdout"]
    assert OPAQUE_TOKEN not in payload["stderr"]


def test_redaction_keeps_a_non_secret_value_of_a_credential_named_field() -> None:
    # The field-name matcher must not swallow the diagnostics around it:
    # `codex doctor` reports credential *presence* with the same field
    # names, and a redacted `false` would make that output unreadable.
    assert codex_cli._redact('{"stored API key": false}') == '{"stored API key": false}'


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

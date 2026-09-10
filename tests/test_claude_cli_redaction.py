"""Credential-redaction regressions for the Claude CLI adapter (issues #73/#74).

The patterns are ported from the sibling Codex adapter. These tests use only
fabricated secrets and mock the subprocess boundary; they never inspect a real
Claude credential store.
"""

from __future__ import annotations

import time
from unittest.mock import patch

import pytest

from praxis_executors.adapters.claude_cli import ClaudeCliExecutor, _redact
from praxis_executors.interface import ExecutionRequest


def _result_of_a_run(stdout: str, stderr: str):
    process = type("Process", (), {})()
    process.poll = lambda: 0
    process.communicate = lambda: (stdout, stderr)
    process.returncode = 0
    with (
        patch("praxis_executors.adapters.claude_cli.shutil.which", return_value="/usr/bin/claude"),
        patch("praxis_executors.adapters.claude_cli.subprocess.Popen", return_value=process),
    ):
        executor = ClaudeCliExecutor(executor_id="e")
        handle = executor.launch(
            ExecutionRequest(
                promise={"spec_version": "1.0.0", "kind": "coding"},
                parameters={"prompt": "hello"},
            )
        )
        return executor.result(handle)


@pytest.mark.parametrize(
    ("secret", "transcript"),
    [
        ("sk-ant-1234567890", "anthropic key: sk-ant-1234567890"),
        ("sk-12345678901234567890", "generic key: sk-12345678901234567890"),
        ("eyJheader12345.eyJpayload.signature", "jwt: eyJheader12345.eyJpayload.signature"),
        ("abcdefg1", "Authorization: Bearer abcdefg1"),
        ("abcdefghijklmnopqrstuvwxyz", "api_key=abcdefghijklmnopqrstuvwxyz"),
    ],
)
def test_result_redacts_each_credential_family(secret: str, transcript: str) -> None:
    result = _result_of_a_run(transcript, transcript)

    assert secret not in str(result.payload), "credential-shaped text must not survive in payload"
    assert result.payload["credentials-redacted"] is True
    assert result.evidence == {"process-exit-status": True}


@pytest.mark.parametrize(
    "transcript",
    [
        "api_key_path=/tmp/credential-file",
        "token=123456789",
        "token=~/credential-file",
        "Authorization: Bearer abcdefgh",
    ],
)
def test_redaction_narrowings_preserve_paths_counts_and_short_bearers(transcript: str) -> None:
    assert _redact(transcript) == transcript


def test_redaction_is_bounded_for_large_base64_like_transcripts() -> None:
    transcript = "A" * 200_000
    started = time.perf_counter()
    redacted = _redact(transcript)
    elapsed = time.perf_counter() - started

    assert redacted == transcript
    assert elapsed < 1.0, "credential redaction must remain bounded on long transcripts"


def test_unmodified_result_marks_credentials_redacted_false() -> None:
    result = _result_of_a_run("ordinary output", "ordinary diagnostics")

    assert result.payload["credentials-redacted"] is False

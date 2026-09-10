"""Launch-parameter validation regressions for the Claude CLI adapter (issue #74)."""

from __future__ import annotations

from unittest.mock import patch

import pytest

from praxis_executors.adapters.claude_cli import ClaudeCliExecutor
from praxis_executors.interface import ExecutionRequest, ExecutorError


def _request(**parameters):
    return ExecutionRequest(
        promise={"spec_version": "1.0.0", "kind": "coding"},
        parameters={"prompt": "hello", **parameters},
    )


@pytest.mark.parametrize("prompt", [None, 7, ["hello"], {"text": "hello"}])
def test_non_string_prompt_is_rejected_before_cli_lookup_or_popen(prompt) -> None:
    with (
        patch("praxis_executors.adapters.claude_cli.shutil.which") as mock_which,
        patch("praxis_executors.adapters.claude_cli.subprocess.Popen") as mock_popen,
    ):
        with pytest.raises(ExecutorError, match="prompt.*must be a string"):
            ClaudeCliExecutor(executor_id="e").launch(_request(prompt=prompt))

    mock_which.assert_not_called()
    mock_popen.assert_not_called()


@pytest.mark.parametrize(
    "extra_args",
    ["--verbose", ("--verbose",), ["--verbose", 7]],
)
def test_invalid_extra_args_are_rejected_before_cli_lookup_or_popen(extra_args) -> None:
    with (
        patch("praxis_executors.adapters.claude_cli.shutil.which") as mock_which,
        patch("praxis_executors.adapters.claude_cli.subprocess.Popen") as mock_popen,
    ):
        with pytest.raises(ExecutorError, match="extra_args.*must be a list of strings"):
            ClaudeCliExecutor(executor_id="e").launch(_request(extra_args=extra_args))

    mock_which.assert_not_called()
    mock_popen.assert_not_called()


def test_valid_extra_args_are_forwarded_after_validation() -> None:
    process = type("Process", (), {})()
    process.poll = lambda: 0
    process.communicate = lambda: ("", "")
    process.returncode = 0
    with (
        patch("praxis_executors.adapters.claude_cli.shutil.which", return_value="/usr/bin/claude"),
        patch("praxis_executors.adapters.claude_cli.subprocess.Popen", return_value=process) as mock_popen,
    ):
        ClaudeCliExecutor(executor_id="e").launch(
            _request(extra_args=["--verbose", "--format", "json"])
        )

    assert mock_popen.call_args.args[0] == [
        "/usr/bin/claude",
        "-p",
        "hello",
        "--verbose",
        "--format",
        "json",
    ]

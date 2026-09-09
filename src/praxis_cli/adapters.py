"""Shared adapter construction path for praxis_cli commands.

Builds the fixed set of Executor instances the CLI operates over. Construction
alone must never probe the environment -- only each adapter's `.health()` /
`.capabilities()` does that.
"""

from __future__ import annotations

from praxis_executors.adapters.claude_cli import ClaudeCliExecutor
from praxis_executors.adapters.fake import FakeCapabilityExecutor
from praxis_executors.adapters.ollama import OllamaExecutor
from praxis_executors.adapters.subprocess_executor import SubprocessExecutor
from praxis_executors.interface import Executor

_FAKE_CAPABILITIES = [
    {
        "spec_version": "1.0.0",
        "id": "cap-primary",
        "satisfies": [{"kind": "text-generation"}],
        "auth_transport": "local",
    }
]


def build_adapters() -> list[Executor]:
    return [
        SubprocessExecutor(executor_id="executor-subprocess-1", satisfies_kinds=["code-execution"]),
        FakeCapabilityExecutor(
            executor_id="executor-fake-1", capabilities=_FAKE_CAPABILITIES, script={}
        ),
        ClaudeCliExecutor(executor_id="executor-claude-cli-1"),
        OllamaExecutor(executor_id="executor-ollama-1"),
    ]

"""Shared adapter construction path for praxis_cli commands.

Builds the fixed set of Executor instances the CLI operates over. Construction
alone must never probe the environment -- only each adapter's `.health()` /
`.capabilities()` does that.
"""

from __future__ import annotations

import copy

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

# Keyed by the executor id each adapter is constructed with, so the id is
# spelled once and stays available to a caller even when the adapter's own
# `.capabilities()` -- otherwise the only public source of `executor_id` --
# raises.
_ADAPTER_FACTORIES = {
    "executor-subprocess-1": lambda executor_id: SubprocessExecutor(
        executor_id=executor_id, satisfies_kinds=["code-execution"]
    ),
    # Deep-copied per call: `FakeCapabilityExecutor.__init__` copies the outer
    # list and nothing inside it, so passing the constant itself would hand
    # every instance the process ever builds the same capability dicts. A caller
    # editing a returned advertisement would be editing this module's constant,
    # against the independently-constructed-objects property `build_adapters()`
    # otherwise holds.
    "executor-fake-1": lambda executor_id: FakeCapabilityExecutor(
        executor_id=executor_id, capabilities=copy.deepcopy(_FAKE_CAPABILITIES), script={}
    ),
    "executor-claude-cli-1": lambda executor_id: ClaudeCliExecutor(executor_id=executor_id),
    "executor-ollama-1": lambda executor_id: OllamaExecutor(executor_id=executor_id),
}


def build_adapters() -> dict[str, Executor]:
    """Return a fresh `{executor_id: instance}` mapping, in declaration order."""
    return {
        executor_id: factory(executor_id)
        for executor_id, factory in _ADAPTER_FACTORIES.items()
    }

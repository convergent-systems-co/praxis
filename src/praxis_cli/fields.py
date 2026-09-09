"""Per-adapter field derivation for CLI display (discover/status commands).

Dispatches by `isinstance` against the adapter classes directly; does not
call `adapters.build_adapters()`.
"""

from __future__ import annotations

import shutil

from praxis_executors.adapters.claude_cli import ClaudeCliExecutor
from praxis_executors.adapters.fake import FakeCapabilityExecutor
from praxis_executors.adapters.ollama import OllamaExecutor
from praxis_executors.adapters.subprocess_executor import SubprocessExecutor
from praxis_executors.interface import Executor, ExecutorAvailability


def installed_field(executor: Executor) -> str:
    if isinstance(executor, ClaudeCliExecutor):
        return "yes" if shutil.which("claude") is not None else "no"
    if isinstance(executor, OllamaExecutor):
        return "yes" if executor.health() != ExecutorAvailability.UNAVAILABLE else "no"
    if isinstance(executor, (SubprocessExecutor, FakeCapabilityExecutor)):
        return "n/a (built-in)"
    raise TypeError(f"unrecognized executor type: {type(executor)!r}")


def version_field(executor: Executor) -> str:
    return "unknown"


def authenticated_field(executor: Executor, installed: str) -> str:
    if not isinstance(executor, ClaudeCliExecutor):
        return "n/a"
    if installed == "no":
        return "n/a (not installed)"
    health = executor.health()
    if health == ExecutorAvailability.AVAILABLE:
        return "yes"
    if health == ExecutorAvailability.DEGRADED:
        return "unknown"
    return "no"


def capability_kinds(advertisement: dict) -> list[str]:
    kinds: list[str] = []
    for capability in advertisement["capabilities"]:
        for entry in capability["satisfies"]:
            kind = entry["kind"]
            if kind not in kinds:
                kinds.append(kind)
    return kinds


def auth_transports(advertisement: dict) -> list[str]:
    transports: list[str] = []
    for capability in advertisement["capabilities"]:
        transport = capability["auth_transport"]
        if transport not in transports:
            transports.append(transport)
    return transports

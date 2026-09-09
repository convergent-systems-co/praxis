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
    # Every adapter that lands after this module was written arrives here.
    # "Installed" is not derivable for a class we know nothing about, and a
    # raise would take down the whole command for one unknown row.
    return "n/a"


def version_field(_executor: Executor) -> str:
    """Always `"unknown"`: no adapter exposes a version on its public interface.

    The argument is unread -- kept so every field function in this module is
    callable the same way, and named to say so.
    """
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


def render_cell(value) -> str:
    """One display string for a row value: `None` is empty, a list joins on ",".

    Shared by `discover`'s block report and `status`'s table so the two
    commands never disagree on how the same value looks -- and so a list is
    never printed through `repr` as Python syntax.
    """
    if value is None:
        return ""
    if isinstance(value, list):
        return ",".join(value)
    return str(value)


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

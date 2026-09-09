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


def installed_field(executor: Executor, advertisement: dict | None) -> str:
    """`advertisement` is the caller's one `.capabilities()` result, or `None`.

    A returned advertisement is itself evidence the backing service answered,
    so it stands in for a `.health()` probe rather than prompting a second
    round trip to the same endpoint at the adapter's full timeout.
    """
    if isinstance(executor, ClaudeCliExecutor):
        return "yes" if shutil.which("claude") is not None else "no"
    if isinstance(executor, OllamaExecutor):
        if advertisement is not None:
            # `capabilities()` only returns once `/api/tags` has answered with
            # at least one model -- exactly what `health()` would re-request to
            # decide the same thing.
            return "yes"
        try:
            health = executor.health()
        except ValueError:
            # A malformed response (e.g. `json.JSONDecodeError`, a `ValueError`
            # subclass) is a mid-probe failure, not a verdict -- neither "yes"
            # nor "no" would be honest, so this degrades the same way an
            # unrecognised adapter class does below.
            return "unknown"
        return "yes" if health != ExecutorAvailability.UNAVAILABLE else "no"
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
    """No `ValueError` guard here, unlike `installed_field`'s Ollama branch.

    That branch needs one because `OllamaExecutor.health()` decodes a JSON
    body it does not control. Every path through `ClaudeCliExecutor.health()`
    either returns an availability or is already handled inside the adapter:
    `shutil.which` cannot raise, `_probe_version` catches its own subprocess
    failures, and `_detect_authenticated` returns unconditionally. A guard
    here would only ever catch a stub.
    """
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
    """Every transport named in the advertisement, first-seen order, deduped.

    `capability.schema.json` requires only `spec_version` and `satisfies`, so a
    conforming capability may name no transport at all. Such a capability
    contributes nothing here rather than taking the whole command down for one
    row -- the same defence `installed_field` makes for an unknown adapter
    class, and the same `.get()` `AuthTransportPolicy` already reads it with.
    """
    transports: list[str] = []
    for capability in advertisement["capabilities"]:
        transport = capability.get("auth_transport")
        if transport is not None and transport not in transports:
            transports.append(transport)
    return transports

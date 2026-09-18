"""Executor adapters exposed through the generic Praxis plugin subsystem.

Concrete provider modules are imported lazily by their factories. Importing the
plugin substrate therefore does not make Claude, Ollama, or any future provider
an architectural dependency of the Praxis composition layer.
"""

from __future__ import annotations

import copy
from collections.abc import Callable

from praxis_plugins import PluginActivation, PluginContext, PluginManifest, PluginService

from .interface import Executor

EXECUTOR_SERVICE_PREFIX = "executor/"

_FAKE_CAPABILITIES = [
    {
        "spec_version": "1.0.0",
        "id": "cap-primary",
        "satisfies": [{"kind": "text-generation"}],
        "auth_transport": "local",
    }
]


class ExecutorAdapterPlugin:
    """Expose one Executor factory as a Praxis plugin service."""

    def __init__(
        self,
        *,
        plugin_id: str,
        executor_id: str,
        factory: Callable[[str], Executor],
        version: str = "1.0.0",
    ) -> None:
        self._executor_id = executor_id
        self._factory = factory
        self._service_name = f"{EXECUTOR_SERVICE_PREFIX}{executor_id}"
        self._manifest = PluginManifest(
            plugin_id=plugin_id,
            version=version,
            kind="executor",
            description=f"Executor adapter for {executor_id}",
            provides=(self._service_name,),
        )

    @property
    def manifest(self) -> PluginManifest:
        return self._manifest

    def activate(self, context: PluginContext) -> PluginActivation:
        del context
        return PluginActivation(
            services=(PluginService(self._service_name, self._factory(self._executor_id)),)
        )

    def deactivate(self, context: PluginContext) -> None:
        del context


def _subprocess_executor(executor_id: str) -> Executor:
    from .adapters.subprocess_executor import SubprocessExecutor

    return SubprocessExecutor(executor_id=executor_id, satisfies_kinds=["code-execution"])


def _fake_executor(executor_id: str) -> Executor:
    from .adapters.fake import FakeCapabilityExecutor

    return FakeCapabilityExecutor(
        executor_id=executor_id,
        capabilities=copy.deepcopy(_FAKE_CAPABILITIES),
        script={},
    )


def _claude_cli_executor(executor_id: str) -> Executor:
    from .adapters.claude_cli import ClaudeCliExecutor

    return ClaudeCliExecutor(executor_id=executor_id)


def _ollama_executor(executor_id: str) -> Executor:
    from .adapters.ollama import OllamaExecutor

    return OllamaExecutor(executor_id=executor_id)


def default_executor_plugins() -> tuple[ExecutorAdapterPlugin, ...]:
    """Return the current default CLI executor set as independent plugins.

    This preserves the pre-plugin default set while keeping provider imports out
    of the generic plugin/composition import path. Additional adapters such as
    Codex, Copilot, and MLX remain reusable and can be exposed as opt-in or
    separately packaged plugins without changing the Executor contract.
    """

    return (
        ExecutorAdapterPlugin(
            plugin_id="praxis.executor.subprocess",
            executor_id="executor-subprocess-1",
            factory=_subprocess_executor,
        ),
        ExecutorAdapterPlugin(
            plugin_id="praxis.executor.fake",
            executor_id="executor-fake-1",
            factory=_fake_executor,
        ),
        ExecutorAdapterPlugin(
            plugin_id="praxis.executor.claude-cli",
            executor_id="executor-claude-cli-1",
            factory=_claude_cli_executor,
        ),
        ExecutorAdapterPlugin(
            plugin_id="praxis.executor.ollama",
            executor_id="executor-ollama-1",
            factory=_ollama_executor,
        ),
    )

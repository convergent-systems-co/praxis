"""Executor adapters exposed through the generic Praxis plugin subsystem."""

from __future__ import annotations

import copy
from collections.abc import Callable

from praxis_plugins import PluginActivation, PluginContext, PluginManifest, PluginService

from .adapters.claude_cli import ClaudeCliExecutor
from .adapters.fake import FakeCapabilityExecutor
from .adapters.ollama import OllamaExecutor
from .adapters.subprocess_executor import SubprocessExecutor
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
    """Small adapter that exposes one Executor instance as a Praxis service."""

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


def default_executor_plugins() -> tuple[ExecutorAdapterPlugin, ...]:
    """Return the current default CLI executor set as independent plugins.

    This deliberately preserves the pre-plugin composition exactly. Additional
    adapters can migrate to opt-in plugins without changing default behavior.
    """

    return (
        ExecutorAdapterPlugin(
            plugin_id="praxis.executor.subprocess",
            executor_id="executor-subprocess-1",
            factory=lambda executor_id: SubprocessExecutor(
                executor_id=executor_id, satisfies_kinds=["code-execution"]
            ),
        ),
        ExecutorAdapterPlugin(
            plugin_id="praxis.executor.fake",
            executor_id="executor-fake-1",
            factory=lambda executor_id: FakeCapabilityExecutor(
                executor_id=executor_id,
                capabilities=copy.deepcopy(_FAKE_CAPABILITIES),
                script={},
            ),
        ),
        ExecutorAdapterPlugin(
            plugin_id="praxis.executor.claude-cli",
            executor_id="executor-claude-cli-1",
            factory=lambda executor_id: ClaudeCliExecutor(executor_id=executor_id),
        ),
        ExecutorAdapterPlugin(
            plugin_id="praxis.executor.ollama",
            executor_id="executor-ollama-1",
            factory=lambda executor_id: OllamaExecutor(executor_id=executor_id),
        ),
    )

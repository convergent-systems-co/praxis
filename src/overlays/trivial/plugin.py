"""Trivial example graph exposed through the generic Praxis plugin contract."""

from __future__ import annotations

from praxis_plugins import PluginActivation, PluginContext, PluginManifest, PluginService

from .overlay import build_trivial_graph


class TrivialPlugin:
    @property
    def manifest(self) -> PluginManifest:
        return PluginManifest(
            plugin_id="praxis.example.trivial",
            version="1.0.0",
            kind="example",
            description="Minimal example graph used for fixtures and smoke testing",
            provides=("graph/trivial",),
        )

    def activate(self, context: PluginContext) -> PluginActivation:
        del context
        return PluginActivation(services=(PluginService("graph/trivial", build_trivial_graph),))

    def deactivate(self, context: PluginContext) -> None:
        del context


def plugin() -> TrivialPlugin:
    return TrivialPlugin()

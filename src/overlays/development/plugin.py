"""Development domain exposed as a Praxis plugin."""

from __future__ import annotations

from praxis_plugins import PluginActivation, PluginContext, PluginManifest, PluginService

from .graph import build_development_graph
from .overlay import register_development_overlay


class DevelopmentPlugin:
    @property
    def manifest(self) -> PluginManifest:
        return PluginManifest(
            plugin_id="praxis.domain.development",
            version="1.0.0",
            kind="domain",
            description="Software-delivery domain graph and overlay extensions",
            provides=("graph/development", "overlay/development"),
        )

    def activate(self, context: PluginContext) -> PluginActivation:
        del context
        return PluginActivation(
            services=(
                PluginService("graph/development", build_development_graph),
                PluginService("overlay/development", register_development_overlay),
            )
        )

    def deactivate(self, context: PluginContext) -> None:
        del context


def plugin() -> DevelopmentPlugin:
    return DevelopmentPlugin()

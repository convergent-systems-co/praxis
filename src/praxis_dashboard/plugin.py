"""Dashboard observability surface exposed as a Praxis plugin.

The dashboard remains read-only with respect to authoritative runtime state.
Its primary Praxis 2 visualization is the live node-and-edge graph view.
"""

from __future__ import annotations

from praxis_plugins import PluginActivation, PluginContext, PluginManifest, PluginService

from .server import main as dashboard_main


class DashboardPlugin:
    @property
    def manifest(self) -> PluginManifest:
        return PluginManifest(
            plugin_id="praxis.observability.dashboard",
            version="1.0.0",
            kind="observability",
            description="Read-only live node-and-edge graph observability dashboard",
            provides=("observability/dashboard",),
        )

    def activate(self, context: PluginContext) -> PluginActivation:
        del context
        return PluginActivation(
            services=(PluginService("observability/dashboard", dashboard_main),)
        )

    def deactivate(self, context: PluginContext) -> None:
        del context


def plugin() -> DashboardPlugin:
    return DashboardPlugin()

"""Dashboard observability surface exposed as a Praxis plugin.

The dashboard remains read-only with respect to authoritative runtime state.
Its primary Praxis 2 visualizations are the live node-and-edge graph view,
generalized team/agent view, and time-oriented event log.
"""

from __future__ import annotations

from praxis_plugins import PluginActivation, PluginContext, PluginManifest, PluginService

from .cli import main as dashboard_main


class DashboardPlugin:
    @property
    def manifest(self) -> PluginManifest:
        return PluginManifest(
            plugin_id="praxis.observability.dashboard",
            version="1.0.0",
            kind="observability",
            description="Read-only live graph, team, and timeline observability dashboard",
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

"""Praxis application composition root.

The kernel and plugin subsystem remain unaware of concrete built-ins. This
module is the application boundary that opts into Praxis-owned plugins and
then adds externally installed plugins discovered through entry points.
"""

from __future__ import annotations

from overlays.development.plugin import plugin as development_plugin
from overlays.trivial.plugin import plugin as trivial_plugin
from praxis_dashboard.plugin import plugin as dashboard_plugin
from praxis_executors.plugins import default_executor_plugins
from praxis_plugins import PluginRegistry, discover_entrypoint_plugins


def build_plugin_registry(*, include_external: bool = True) -> PluginRegistry:
    registry = PluginRegistry()
    registry.register_many(default_executor_plugins())
    registry.register(development_plugin())
    registry.register(trivial_plugin())
    registry.register(dashboard_plugin())
    if include_external:
        registry.register_many(discover_entrypoint_plugins())
    registry.activate_all()
    return registry

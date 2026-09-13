"""Praxis plugin subsystem.

The plugin layer is intentionally domain-neutral. It provides lifecycle,
dependency, discovery, and service-registration primitives that domain
packages, executor adapters, UI surfaces, transports, storage providers, and
future subsystems can implement without coupling the Praxis kernel to them.
"""

from .discovery import PluginDiscoveryError, discover_entrypoint_plugins
from .registry import PluginRegistry, PluginRegistryError
from .types import Plugin, PluginActivation, PluginContext, PluginManifest, PluginService

__all__ = [
    "Plugin",
    "PluginActivation",
    "PluginContext",
    "PluginDiscoveryError",
    "PluginManifest",
    "PluginRegistry",
    "PluginRegistryError",
    "PluginService",
    "discover_entrypoint_plugins",
]

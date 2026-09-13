"""Python entry-point discovery for externally installed Praxis plugins."""

from __future__ import annotations

import importlib.metadata

from .types import Plugin

ENTRYPOINT_GROUP = "praxis.plugins"


class PluginDiscoveryError(Exception):
    """Raised when an installed plugin entry point cannot be loaded safely."""


def discover_entrypoint_plugins(*, group: str = ENTRYPOINT_GROUP) -> list[Plugin]:
    """Load plugins advertised through the ``praxis.plugins`` entry-point group.

    An entry point may expose either a plugin instance or a zero-argument
    factory returning one. Discovery is fail-closed: a broken installed plugin
    must not be silently ignored because doing so would make behavior depend on
    accidental import success.
    """

    try:
        discovered = importlib.metadata.entry_points()
        selected = discovered.select(group=group) if hasattr(discovered, "select") else discovered.get(group, [])
    except Exception as exc:
        raise PluginDiscoveryError(f"unable to enumerate {group!r} entry points: {exc}") from exc

    plugins: list[Plugin] = []
    for entrypoint in selected:
        try:
            loaded = entrypoint.load()
            plugin = loaded() if callable(loaded) and not hasattr(loaded, "manifest") else loaded
            manifest = plugin.manifest
            if not manifest.plugin_id:
                raise TypeError("plugin manifest has no plugin_id")
            if not callable(getattr(plugin, "activate", None)):
                raise TypeError("plugin has no activate(context) method")
            if not callable(getattr(plugin, "deactivate", None)):
                raise TypeError("plugin has no deactivate(context) method")
        except Exception as exc:
            raise PluginDiscoveryError(
                f"unable to load Praxis plugin entry point {entrypoint.name!r}: {exc}"
            ) from exc
        plugins.append(plugin)
    return plugins

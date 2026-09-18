"""Shared executor composition path for Praxis CLI commands."""

from __future__ import annotations

from praxis_executors.interface import Executor
from praxis_executors.plugins import EXECUTOR_SERVICE_PREFIX

from .composition import build_plugin_registry


def build_adapters() -> dict[str, Executor]:
    """Return a fresh ``{executor_id: instance}`` mapping from plugin services."""

    plugins = build_plugin_registry()
    adapters: dict[str, Executor] = {}
    for service_name, value in plugins.services(prefix=EXECUTOR_SERVICE_PREFIX).items():
        if not isinstance(value, Executor):
            raise TypeError(
                f"plugin service {service_name!r} uses the executor namespace "
                f"but does not implement Executor"
            )
        executor_id = service_name[len(EXECUTOR_SERVICE_PREFIX) :]
        adapters[executor_id] = value
    return adapters

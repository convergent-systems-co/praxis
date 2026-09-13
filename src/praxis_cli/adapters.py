"""Shared executor composition path for Praxis CLI commands.

Concrete executor construction now flows through the generic Praxis plugin
registry. The current built-in set is preserved exactly, while externally
installed plugins may contribute additional ``executor/`` services through the
``praxis.plugins`` entry-point group.

Construction alone must never probe the environment; only each executor's
``health()`` / ``capabilities()`` methods do that.
"""

from __future__ import annotations

from praxis_executors.interface import Executor
from praxis_executors.plugins import EXECUTOR_SERVICE_PREFIX, default_executor_plugins
from praxis_plugins import PluginRegistry, discover_entrypoint_plugins


def build_adapters() -> dict[str, Executor]:
    """Return a fresh ``{executor_id: instance}`` mapping.

    Built-in executors and third-party executor plugins share the same plugin
    lifecycle. Non-executor plugins may also be discovered and activated when
    they are installed; only services under ``executor/`` are returned here.
    """

    plugins = PluginRegistry()
    plugins.register_many(default_executor_plugins())
    plugins.register_many(discover_entrypoint_plugins())
    plugins.activate_all()

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

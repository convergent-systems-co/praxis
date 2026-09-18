"""Fail-closed plugin registry with dependency-ordered lifecycle management."""

from __future__ import annotations

from collections.abc import Iterable

from .types import Plugin, PluginActivation, PluginContext, PluginService


class PluginRegistryError(Exception):
    """Raised when plugin composition is ambiguous or invalid."""


class PluginRegistry:
    """Owns plugin registration, dependency ordering, activation, and services.

    The registry intentionally does not import any concrete Praxis subsystem.
    The application composition root decides which built-ins to register, and
    external distributions can be discovered independently.
    """

    def __init__(self) -> None:
        self._plugins: dict[str, Plugin] = {}
        self._services: dict[str, object] = {}
        self._service_owners: dict[str, str] = {}
        self._activation_order: list[str] = []

    def register(self, plugin: Plugin) -> None:
        plugin_id = plugin.manifest.plugin_id
        if plugin_id in self._plugins:
            raise PluginRegistryError(f"plugin_id {plugin_id!r} is already registered")
        self._plugins[plugin_id] = plugin

    def register_many(self, plugins: Iterable[Plugin]) -> None:
        for plugin in plugins:
            self.register(plugin)

    def registered(self) -> tuple[Plugin, ...]:
        return tuple(self._plugins.values())

    def service(self, name: str) -> object | None:
        return self._services.get(name)

    def services(self, *, prefix: str | None = None) -> dict[str, object]:
        if prefix is None:
            return dict(self._services)
        return {name: value for name, value in self._services.items() if name.startswith(prefix)}

    def activate_all(self) -> tuple[str, ...]:
        if self._activation_order:
            return tuple(self._activation_order)

        order = self._dependency_order()
        try:
            for plugin_id in order:
                plugin = self._plugins[plugin_id]
                context = PluginContext(self._services)
                activation = plugin.activate(context)
                self._install_services(plugin_id, plugin.manifest.provides, activation)
                self._activation_order.append(plugin_id)
        except Exception:
            self._rollback_activation()
            raise
        return tuple(self._activation_order)

    def deactivate_all(self) -> None:
        errors: list[Exception] = []
        for plugin_id in reversed(self._activation_order):
            plugin = self._plugins[plugin_id]
            try:
                plugin.deactivate(PluginContext(self._services))
            except Exception as exc:  # continue cleanup, then surface first failure
                errors.append(exc)
            self._remove_services_owned_by(plugin_id)
        self._activation_order.clear()
        if errors:
            raise PluginRegistryError(f"plugin deactivation failed: {errors[0]}") from errors[0]

    def _dependency_order(self) -> list[str]:
        temporary: set[str] = set()
        permanent: set[str] = set()
        order: list[str] = []

        def visit(plugin_id: str) -> None:
            if plugin_id in permanent:
                return
            if plugin_id in temporary:
                raise PluginRegistryError(f"plugin dependency cycle includes {plugin_id!r}")
            temporary.add(plugin_id)
            plugin = self._plugins[plugin_id]
            for dependency in plugin.manifest.requires:
                if dependency not in self._plugins:
                    raise PluginRegistryError(
                        f"plugin {plugin_id!r} requires missing plugin {dependency!r}"
                    )
                visit(dependency)
            temporary.remove(plugin_id)
            permanent.add(plugin_id)
            order.append(plugin_id)

        for plugin_id in self._plugins:
            visit(plugin_id)
        return order

    def _install_services(
        self,
        plugin_id: str,
        declared: tuple[str, ...],
        activation: PluginActivation,
    ) -> None:
        actual = tuple(service.name for service in activation.services)
        undeclared = set(actual) - set(declared)
        missing = set(declared) - set(actual)
        if undeclared or missing:
            raise PluginRegistryError(
                f"plugin {plugin_id!r} service contract mismatch; "
                f"undeclared={sorted(undeclared)!r}, missing={sorted(missing)!r}"
            )
        if len(set(actual)) != len(actual):
            raise PluginRegistryError(f"plugin {plugin_id!r} returned duplicate service names")

        for service in activation.services:
            if service.name in self._services:
                owner = self._service_owners[service.name]
                raise PluginRegistryError(
                    f"service {service.name!r} from {plugin_id!r} collides with owner {owner!r}"
                )
            self._services[service.name] = service.value
            self._service_owners[service.name] = plugin_id

    def _remove_services_owned_by(self, plugin_id: str) -> None:
        owned = [name for name, owner in self._service_owners.items() if owner == plugin_id]
        for name in owned:
            self._services.pop(name, None)
            self._service_owners.pop(name, None)

    def _rollback_activation(self) -> None:
        for plugin_id in reversed(self._activation_order):
            plugin = self._plugins[plugin_id]
            try:
                plugin.deactivate(PluginContext(self._services))
            except Exception:
                pass
            self._remove_services_owned_by(plugin_id)
        self._activation_order.clear()

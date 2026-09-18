from __future__ import annotations

from dataclasses import dataclass

import pytest

from praxis_plugins import (
    PluginActivation,
    PluginContext,
    PluginManifest,
    PluginRegistry,
    PluginRegistryError,
    PluginService,
)


@dataclass
class StubPlugin:
    plugin_id: str
    requires: tuple[str, ...] = ()
    provides: tuple[str, ...] = ()
    log: list[str] | None = None

    @property
    def manifest(self) -> PluginManifest:
        return PluginManifest(
            plugin_id=self.plugin_id,
            version="1.0.0",
            kind="test",
            requires=self.requires,
            provides=self.provides,
        )

    def activate(self, context: PluginContext) -> PluginActivation:
        if self.log is not None:
            self.log.append(f"activate:{self.plugin_id}")
        services = tuple(PluginService(name, f"value:{name}") for name in self.provides)
        return PluginActivation(services=services)

    def deactivate(self, context: PluginContext) -> None:
        if self.log is not None:
            self.log.append(f"deactivate:{self.plugin_id}")


def test_dependencies_activate_before_dependents_and_deactivate_in_reverse():
    log: list[str] = []
    registry = PluginRegistry()
    registry.register(StubPlugin("dependent", requires=("base",), log=log))
    registry.register(StubPlugin("base", log=log))

    assert registry.activate_all() == ("base", "dependent")
    registry.deactivate_all()

    assert log == [
        "activate:base",
        "activate:dependent",
        "deactivate:dependent",
        "deactivate:base",
    ]


def test_missing_dependency_fails_closed():
    registry = PluginRegistry()
    registry.register(StubPlugin("dependent", requires=("missing",)))

    with pytest.raises(PluginRegistryError, match="requires missing plugin"):
        registry.activate_all()


def test_dependency_cycle_fails_closed():
    registry = PluginRegistry()
    registry.register(StubPlugin("a", requires=("b",)))
    registry.register(StubPlugin("b", requires=("a",)))

    with pytest.raises(PluginRegistryError, match="dependency cycle"):
        registry.activate_all()


def test_service_collision_rolls_back_prior_activation():
    log: list[str] = []
    registry = PluginRegistry()
    registry.register(StubPlugin("one", provides=("shared/x",), log=log))
    registry.register(StubPlugin("two", provides=("shared/x",), log=log))

    with pytest.raises(PluginRegistryError, match="collides"):
        registry.activate_all()

    assert registry.services() == {}
    assert "deactivate:one" in log


def test_plugin_cannot_export_undeclared_service():
    class BadPlugin(StubPlugin):
        def activate(self, context: PluginContext) -> PluginActivation:
            return PluginActivation((PluginService("extra/x", object()),))

    registry = PluginRegistry()
    registry.register(BadPlugin("bad"))

    with pytest.raises(PluginRegistryError, match="service contract mismatch"):
        registry.activate_all()

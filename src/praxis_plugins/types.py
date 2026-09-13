"""Core types for installable Praxis subsystems and extensions."""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Mapping, Protocol, runtime_checkable


@dataclass(frozen=True)
class PluginManifest:
    """Static identity and dependency contract for a plugin.

    ``kind`` is deliberately an open string rather than an enum. Praxis must be
    able to gain new subsystem classes without a kernel release merely to add a
    new enum member.
    """

    plugin_id: str
    version: str
    kind: str
    description: str = ""
    requires: tuple[str, ...] = ()
    provides: tuple[str, ...] = ()

    def __post_init__(self) -> None:
        for name, value in (
            ("plugin_id", self.plugin_id),
            ("version", self.version),
            ("kind", self.kind),
        ):
            if not value or not value.strip():
                raise ValueError(f"{name} must be a non-empty string")
        if len(set(self.requires)) != len(self.requires):
            raise ValueError("requires must not contain duplicates")
        if len(set(self.provides)) != len(self.provides):
            raise ValueError("provides must not contain duplicates")


@dataclass(frozen=True)
class PluginService:
    """One named service exported by an activated plugin."""

    name: str
    value: object

    def __post_init__(self) -> None:
        if not self.name or not self.name.strip():
            raise ValueError("service name must be non-empty")


@dataclass(frozen=True)
class PluginActivation:
    """Services exported by a successful plugin activation."""

    services: tuple[PluginService, ...] = ()


class PluginContext:
    """Read-only view of services already activated before a plugin."""

    def __init__(self, services: Mapping[str, object] | None = None) -> None:
        self._services = dict(services or {})

    def get(self, name: str, default: object | None = None) -> object | None:
        return self._services.get(name, default)

    def require(self, name: str) -> object:
        try:
            return self._services[name]
        except KeyError as exc:
            raise KeyError(f"required plugin service {name!r} is unavailable") from exc

    def services(self) -> Mapping[str, object]:
        return dict(self._services)


@runtime_checkable
class Plugin(Protocol):
    """Minimal lifecycle contract for a Praxis plugin."""

    @property
    def manifest(self) -> PluginManifest: ...

    def activate(self, context: PluginContext) -> PluginActivation: ...

    def deactivate(self, context: PluginContext) -> None: ...

"""Executor eligibility policies.

An ExecutorPolicy decides whether a given executor_id (and its advertisement)
is eligible to be selected for a capability. as_eligibility_callable adapts a
policy plus a snapshot of advertisements into the plain
Callable[[str], bool] shape that matching.match expects, so matching.py never
needs to import this module.
"""

from __future__ import annotations

import abc
from dataclasses import dataclass
from typing import Callable


class ExecutorPolicy(abc.ABC):
    @abc.abstractmethod
    def is_eligible(self, executor_id: str, advertisement: dict) -> bool: ...


_RECOGNIZED_AUTH_TRANSPORTS = frozenset(
    {"subscription_cli", "oauth_cli", "local", "metered_api", "api_key"}
)
_UNSAFE_BY_DEFAULT_AUTH_TRANSPORTS = frozenset({"metered_api", "api_key"})


@dataclass(frozen=True)
class AllowListPolicy(ExecutorPolicy):
    allowed_executor_ids: frozenset[str]

    def is_eligible(self, executor_id: str, advertisement: dict) -> bool:
        return executor_id in self.allowed_executor_ids


@dataclass(frozen=True)
class DenyListPolicy(ExecutorPolicy):
    denied_executor_ids: frozenset[str]

    def is_eligible(self, executor_id: str, advertisement: dict) -> bool:
        return executor_id not in self.denied_executor_ids


@dataclass(frozen=True)
class AuthTransportPolicy(ExecutorPolicy):
    denied_auth_transports: frozenset[str] = frozenset()
    allowed_auth_transports: frozenset[str] | None = None

    def is_eligible(self, executor_id: str, advertisement: dict) -> bool:
        capabilities = advertisement.get("capabilities")
        if not capabilities:
            return False
        for capability in capabilities:
            if not self._capability_is_eligible(capability):
                return False
        return True

    def _capability_is_eligible(self, capability: dict) -> bool:
        auth_transport = capability.get("auth_transport")
        if auth_transport not in _RECOGNIZED_AUTH_TRANSPORTS:
            return False
        if auth_transport in self.denied_auth_transports:
            return False
        explicitly_allowed = (
            self.allowed_auth_transports is not None
            and auth_transport in self.allowed_auth_transports
        )
        if auth_transport in _UNSAFE_BY_DEFAULT_AUTH_TRANSPORTS and not explicitly_allowed:
            return False
        if self.allowed_auth_transports is not None and not explicitly_allowed:
            return False
        return True


def as_eligibility_callable(
    policy: ExecutorPolicy, advertisements: list[dict]
) -> Callable[[str], bool]:
    lookup = {advertisement["executor_id"]: advertisement for advertisement in advertisements}

    def is_eligible(executor_id: str) -> bool:
        advertisement = lookup.get(executor_id)
        if advertisement is None:
            return False
        return policy.is_eligible(executor_id, advertisement)

    return is_eligible

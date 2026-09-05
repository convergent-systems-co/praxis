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

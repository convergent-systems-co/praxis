"""Executor abstraction: the Executor ABC and its request/handle/result types.

This module has no dependency on praxis_runtime or praxis_contracts, so every
adapter can be implemented and tested independently of the runtime engine.
"""

from __future__ import annotations

import abc
import enum
from dataclasses import dataclass, field


class ExecutorStatus(enum.Enum):
    """Lifecycle status of a single execution, as reported by an Executor."""

    QUEUED = "queued"
    RUNNING = "running"
    SUCCEEDED = "succeeded"
    FAILED = "failed"
    CANCELLED = "cancelled"


class ExecutorAvailability(enum.Enum):
    """Overall health of an Executor, independent of any single execution."""

    AVAILABLE = "available"
    DEGRADED = "degraded"
    UNAVAILABLE = "unavailable"


@dataclass(frozen=True)
class ExecutionRequest:
    """A request to launch a Promise-shaped unit of work on an Executor."""

    promise: dict
    parameters: dict = field(default_factory=dict)


@dataclass(frozen=True)
class ExecutionHandle:
    """An opaque reference to a single launched execution."""

    handle_id: str


@dataclass(frozen=True)
class ExecutionResult:
    """The outcome of a completed (or failed/cancelled) execution.

    `evidence` is a flat `{proof_type: claim}` dict; its keys must match the
    `proof_type` vocabulary used by the target node's `evidence_requirement`
    (see `src/praxis_runtime/transitions.py::_check_evidence` and
    `docs/runtime.md`). It cannot be passed straight through to
    `TransitionEngine.apply(..., evidence=...)`: that method requires
    `list[dict]` of raw proof-record documents (see
    `schemas/v1/proof-record.schema.json`). A caller that has the
    run/graph/node context this flat dict lacks (this module deliberately
    has none, so adapters stay independent of `praxis_runtime`) is
    responsible for converting each `evidence` entry into a proof-record
    document before dispatching into `TransitionEngine.apply` --
    `praxis_executors.registry.evidence_to_proof_records` is the reusable
    conversion function for this.
    """

    status: ExecutorStatus
    evidence: dict = field(default_factory=dict)
    payload: dict = field(default_factory=dict)


class ExecutorError(Exception):
    """Raised by an Executor implementation when an operation cannot proceed."""


class Executor(abc.ABC):
    """A pluggable backend capable of launching and reporting on executions."""

    @abc.abstractmethod
    def capabilities(self) -> dict:
        """Return a CapabilityAdvertisement-shaped dict for this executor."""

    @abc.abstractmethod
    def health(self) -> ExecutorAvailability:
        """Return this executor's current overall availability."""

    @abc.abstractmethod
    def launch(self, request: ExecutionRequest) -> ExecutionHandle:
        """Start executing the given request and return a handle to it."""

    @abc.abstractmethod
    def status(self, handle: ExecutionHandle) -> ExecutorStatus:
        """Return the current lifecycle status of a previously launched execution."""

    @abc.abstractmethod
    def cancel(self, handle: ExecutionHandle) -> None:
        """Request cancellation of a previously launched execution."""

    @abc.abstractmethod
    def result(self, handle: ExecutionHandle) -> ExecutionResult:
        """Return the final result of a previously launched execution."""

"""Grader protocol and overlay grader registry.

A `Grader` grades a `ProofRecord` into a `GradeResult`. There is a single
`Grader` protocol; the three grader kinds ("deterministic", "model", "human")
are distinguished only by the string they are registered under in a
`GraderRegistry`, not by separate classes, so the registry stays uniform.

A `"human"` grader's `grade()` must treat the human-authored
`ProofRecord.status` as authoritative pass/fail — the human decision *is*
the record. It must never infer approval from the absence of a rejection
or from any other implicit signal.

`GraderRegistry` has no globally-shared mutable singleton: `default_registry()`
returns a fresh, empty registry on every call. Callers (including
`TransitionEngine`) construct or receive a registry explicitly and register
graders into it, which is the actual mechanism that lets domain overlays
define specialized proof types and graders without changing core runtime
code.
"""

from __future__ import annotations

from typing import Protocol

from praxis_evidence.types import GradeResult, ProofRecord

_VALID_GRADER_KINDS = {"deterministic", "model", "human"}


class Grader(Protocol):
    def grade(self, record: ProofRecord) -> GradeResult: ...


class GraderRegistry:
    def __init__(self) -> None:
        self._graders: dict[tuple[str, str], Grader] = {}

    def register(self, proof_type: str, grader_kind: str, grader: Grader) -> None:
        if grader_kind not in _VALID_GRADER_KINDS:
            raise ValueError(
                f"invalid grader_kind {grader_kind!r}; must be one of {sorted(_VALID_GRADER_KINDS)}"
            )
        self._graders[(proof_type, grader_kind)] = grader

    def get(self, proof_type: str, grader_kind: str) -> Grader | None:
        return self._graders.get((proof_type, grader_kind))

    def kinds_for(self, proof_type: str) -> set[str]:
        return {kind for (pt, kind) in self._graders if pt == proof_type}


def default_registry() -> GraderRegistry:
    return GraderRegistry()

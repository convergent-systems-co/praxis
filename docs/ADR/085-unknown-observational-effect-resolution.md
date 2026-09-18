# ADR-085: Evidence-based resolution of the Goals `/4` observational effect

Status: Proposed
Predecessors: ADR-084; ADR-083; SPEC-047; SPEC-046; PLAN-013; PLAN-012

The `/4` dogfood execution established a narrow case in which a read-only
verification effect was dispatched once, recorded as `UNKNOWN` because a
positional validator rejected provider enumeration order, and later observed
again with the corrected identity-preserving validator. The original and
reconciliation observations are immutable, semantically equal, and contain no
mutation. The existing reconciliation command correctly records unresolved
evidence but has no safe resolution transition.

This decision adds a closed, append-only resolution contract for exactly that
case: `goals-established-state-publication/4`, operation
`publish-goals-from-established-state`, step `verify-draft`, adapter
`goals-recovery-github`, one original attempt, no later effects, and active
delegated authority at resolution time. A dedicated command, not ordinary
reconcile or execute, validates the complete lineage and both observations,
then uses the state-store terminal reconciliation primitive to atomically
append a resolution event and move the current effect from `UNKNOWN` to
`SUCCEEDED` without dispatching the provider operation.

The original UNKNOWN effect, observed result, and unresolved reconciliation
event remain immutable history. Resolution is an appended interpretation of a
read-only effect; it does not claim a provider mutation, erase uncertainty
from the original record, or authorize publication. The effect attempt count
remains one. Subsequent execution may consider only the next permitted effect,
`publish-existing-release`, after fresh authority and current-state checks.

The existing delegated child authority must still be active when resolution is
performed. Resolution neither renews nor extends it and cannot use expired
historical authority. Mutating effects and arbitrary UNKNOWN effects are out
of scope and require separate decisions.

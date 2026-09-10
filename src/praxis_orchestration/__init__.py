"""The executor-to-runtime orchestration seam.

This is the one package permitted to import `praxis_runtime`,
`praxis_executors`, and `praxis_policy` together. `praxis_policy` and
`praxis_executors.registry` are both documented as never importing
`praxis_runtime`, and pushing executor or policy imports into `praxis_runtime`
would invert that and put domain logic in the core. Anything that has to see
all three lives here instead.
"""

from praxis_orchestration.escalation import (
    ATTEMPT_TIERS,
    AttemptOutcome,
    EscalationResult,
    build_escalation_ladder,
    run_escalation_ladder,
)

__all__ = [
    "ATTEMPT_TIERS",
    "AttemptOutcome",
    "EscalationResult",
    "build_escalation_ladder",
    "run_escalation_ladder",
]

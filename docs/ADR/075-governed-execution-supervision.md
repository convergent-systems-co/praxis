# ADR-075: Governed Execution Supervision

- Status: Proposed
- Date: 2026-09-15
- Related: ADR-031, ADR-033, ADR-035, ADR-040, ADR-042, ADR-060, ADR-073, ADR-074

## Context

The accepted Goal-drive architecture owns bounded turns, progress, checkpoint
validation, authority boundaries, and durable recovery. The current provider
adapters do not expose meaningful activity while a turn is running, and the
controller records a turn only after the provider returns. A human therefore
cannot reliably understand or correct consequential longitudinal work before a
long provider execution completes.

`JAPETELLA_LONGITUDINAL_READY` requires both trustworthy execution and effective
human supervision. This requirement is about operational observability and
bounded intervention, not private chain-of-thought, token streaming, a TUI, or
a general observability platform.

## Decision

Praxis SHALL add a governed execution-supervision contract as a projection and
control-plane extension of the existing local append-only event model. It SHALL
not create a second governance or authority system.

The contract SHALL preserve these invariants:

- evidence is not authority;
- a provider or model statement is not a controller fact;
- human commentary does not silently become architecture authority;
- every intervention is scoped to an exact execution identity and provenance;
- uncertain execution SHALL shrink the next irreversible step;
- an intervention SHALL be honored at the earliest legitimate safe boundary.

The first supported client SHALL be a CLI/control-plane surface capable of
starting a bounded supervised execution, following an active execution,
inspecting meaningful activity, submitting supported intervention, suspending,
cancelling, resuming, and observing authority, blocker, and completion
transitions. A TUI is explicitly outside this decision.

## Trust and information boundaries

Controller-owned facts MAY assert only deterministic runtime observations and
validated transitions: exact Goal/work candidate, invocation and turn identity,
provider identity, objective, execution state, typed action start/result,
deterministically known affected workspace area, validation activity/result,
checkpoint creation, blocker classification, authority request, and
cancellation/suspension state.

Provider/model messages SHALL be separately typed as provider-originated
observations. They MAY be surfaced as meaningful user-facing output, but SHALL
not satisfy progress, validation, completion, authority, or checkpoint claims.

Operational payloads SHALL use an allowlist and sensitivity classification.
Private reasoning, hidden model state, credentials, key material, and policy-
forbidden raw tool inputs/outputs SHALL not be persisted or displayed. Unknown
sensitive content SHALL be redacted or represented only by an integrity-bound
reference and classification. Large transcripts SHALL be external artifacts or
bounded summaries, never unbounded event payloads.

## Continuous execution

Continuous mode SHALL be implemented as repeated, controller-owned bounded
transitions. Each transition must recover authoritative state, select one
authorized bounded unit, execute it, record evidence, evaluate progress,
completion, blockers, authority, revocation, and intervention state, and only
then decide whether another unit may begin.

Continuous mode SHALL remain observable and interruptible. It SHALL not broaden
permissions, bypass authority, suppress blockers, skip evidence or completion
qualification, or treat provider prose as a continuation decision.

## Consequences

Japetella qualification gains a concrete supervision gate without requiring a
presentation framework. Provider adapters require a streaming/activity callback
or equivalent bounded activity channel, while the controller remains the only
authority for transitions and claims. The release candidate must include this
contract and its qualification evidence before claiming longitudinal readiness.

## Non-goals

This ADR does not define a TUI, token-by-token streaming, distributed event
transport, arbitrary chat collaboration, unrestricted transcript retention, or
a general metrics/observability platform. #117 and #118 remain parked unless a
reviewed implementation dependency is demonstrated.

# SPEC-020: Resource Continuation and Handoff

- Status: Active
- Date: 2026-09-13
- Governing ADRs: ADR-001, ADR-003, ADR-004, ADR-020, ADR-031, ADR-034, ADR-035, ADR-038, ADR-047, ADR-050
- Depends on: SPEC-002, SPEC-003, SPEC-006, SPEC-010

## Purpose

Define the domain-neutral runtime contract for observing resource pressure, applying graph/package-provided continuation policy, checkpointing, handing execution across process/session boundaries, and resuming without changing run or agent identity.

## Contracts

`ResourceObservation` SHALL bind a stable observation ID, run ID, optional agent ID, opaque non-negative signal values, and durable evidence references.

`ResourceProfile` SHALL bind stable profile ID/version and ordered rules. Each rule names an opaque signal, non-negative inclusive threshold, and one generic action: `continue`, `constrain`, `checkpoint`, or `handoff`. Signal vocabulary and threshold calibration remain package/profile policy. Core SHALL NOT infer semantics from a signal name.

`ContinuationDecision` SHALL bind the exact profile/rule, observation digest, action, checkpoint reference where applicable, and handoff reference where applicable. Equivalent durable inputs SHALL produce the same decision identity.

`checkpoint` and `handoff` SHALL capture exact run, graph/version, optional agent, current node, transition count, completed evidence, quota counters, and continuation evidence. `handoff` additionally places the run in a typed suspended state. A failed authorization or failed event append SHALL leave the in-memory run unchanged.

Resume SHALL require the exact persisted handoff reference and SHALL traverse the normal deterministic `RunControl` authority boundary. Replay SHALL never re-perform completed external work. After restart and resume, run, graph/version, agent identity, checkpoint, pressure evidence, and subsequent execution evidence remain reconstructable from authoritative events.

## Authority and failure behavior

Resource measurement and profile evaluation do not authorize mutation. Applying a continuation decision requires a deterministic authorizer immediately before persistence. Handoff satisfaction grants no downstream capability; it only releases the exact continuation wait after run-control authorization.

Invalid profiles, negative observations, identity mismatch, unknown action, missing checkpoint/handoff reference, stale event version, denied authorization, and mismatched resume reference fail closed.

## Acceptance evidence

One SQLite restart integration SHALL exercise two unrelated domains with different signal names and thresholds. Each must observe pressure, obtain a governed handoff decision, persist a checkpoint, close and reopen the authoritative store, reconstruct identical run/agent/evidence state, resume using the exact handoff reference, complete through the kernel, and retain both pre-handoff and post-resume evidence. Neither profile or test may rely on software-delivery roles, turn counts, color tiers, bundle lanes, or `HANDOFF.md`.

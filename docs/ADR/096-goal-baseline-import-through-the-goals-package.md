# ADR-096: The Goal Baseline Import Boundary Is Reached Through the Goals Package

- Status: Accepted
- Date: 2026-09-18
- Governs: how a canonical Goal Baseline enters a fresh installation's
  GoalStore now that Goals commands live in the package surface
- Related: ADR-068, SPEC-031, ADR-095

## Context

ADR-068 and SPEC-031 accepted `praxis goal import` as the only
evidence-to-authority boundary for a Goal Baseline. The release
consolidation (ae6fd0f) removed the kernel `goal` dispatch when Goals
commands moved into `praxis.package.goals`, but no package operation
replaced it. The importer remained in the binary as unreachable code.
Qualifying the external bootstrap on a fresh installation showed the
consequence: after a verified install of the Goals package there was no way
to create a Goal, so `goal-drive` could never be given a durable Goal
identity.

## Decision

The import boundary is exposed as the `import` operation of the
`goals-lifecycle` invocation, dispatched by the registered first-party
adapter like every other lifecycle operation:

```
praxis goals-lifecycle --operation=import --input=<canonical-baseline.json>
```

SPEC-031 rules are unchanged: the document carries `schema_version` `1`,
its own canonical absolute path as `source_ref`, the SHA-256 of the
baseline's canonical payload as `source_digest`, and a complete baseline; an
exact duplicate is idempotent, a conflicting generation fails closed, a
non-root generation requires its stored predecessor, and import admits only
Goal state. The importer opens the governed repository through the same
bootstrap-backed path as every other lifecycle mutation.

The same qualification showed that an authority-backed acceptance was
stored but never bound into a Goal generation, while `goal-drive`
materializes work only from the baseline's embedded WorkPlan. The
`attach` operation closes that gap by calling the existing
`AttachAcceptedWorkPlan`: it creates the successor immutable generation
that carries the accepted plan, refuses a source that already carries one,
and requires the exact source digest and an effective acceptance authority.

```
praxis goals-lifecycle --operation=attach --input=<attach.json>
```

No kernel command is added. The published `praxis.package.goals@0.1.1`
invocation contract is unchanged; the `operation` option's descriptive text
will list `import` in the next package version.

## Consequences

- A fresh installation can create a Goal only through an installed,
  verified Goals package generation, never through the kernel.
- The unreachable kernel `goal` command is gone; its document validation is
  shared by the package operation.

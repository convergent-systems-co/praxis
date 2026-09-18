# Built-in Praxis Authority Model v1 Decision Packet v1

Status: Proposed — architecture-owner decision required

Date: 2026-09-15

## Finding

The current repository has a closed critical delegation need but an open
general vocabulary. `authority.delegate` is now intrinsic to the enrolled
root. Goals uses `workplan.accept` as a requested authority. Package and
provider capabilities are dynamic runtime lease inputs, and invocation
contracts explicitly state that discovery does not grant execution authority.

The existing lease evaluator's lexical scope compatibility is not adopted for
delegated-generation containment.

## Minimum model

ADR-074/SPEC-037 define a versioned built-in model with one delegation edge:
root `authority.delegate` to a distinct controller's exact `workplan.accept`
authority over one exact protected WorkPlan target and typed derived scope.
Runtime package capabilities remain lease-governed and extensible without
becoming governance authority.

## Parked features

The following are deliberately outside v1: runtime policy publication and
precedence ecosystems, arbitrary custom authority classes, arbitrary scope
hierarchies, package-defined governance delegation, self-target exceptions,
and automatic migration of historical open-string records.

## Advisory challenge

Architecture/security review: root enrollment does not become execution
authority; the only intrinsic root capability is `authority.delegate`; child
principal, authority, target, scope, operation, expiry, model, parent, and
revocation are exact and closed; unknown values fail closed; dynamic package
capabilities remain leases; restart revalidates immutable lineage.

The smallest remaining implementation can therefore use a built-in model
without inventing an extensible policy ecosystem.

## Decision requested

Accept, reject, or revise ADR-074/SPEC-037. Acceptance must confirm that
`workplan.accept` is the canonical downstream governed authority for the
current Goals path and that package/runtime capabilities remain outside the
governance model. This packet does not accept or authorize implementation,
bootstrap, or Japetella work.

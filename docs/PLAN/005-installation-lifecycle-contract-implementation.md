# PLAN-005: Installation Lifecycle Contract Implementation

- Status: PLANNED — ACCEPTED
- Accepted proposal digest: `sha256:a794d52a3bd1c660c6f73977b0c9d7903da6928c4c2e9d8fef2964b07961a12a`
- Acceptance: explicit architecture-owner decision; implementation planning authorized
- Governing proposal: ADR-075 and SPEC-038
- Predecessor proposal: PLAN-004, `sha256:16e52978e01b353df549e6af4de15f798c3537d2a69df8469ecdac17713b7b82`
- Related: PLAN-001, PLAN-003

## Delivery sequence

1. Freeze canonical lifecycle-plan, transition-journal, installation-manifest,
   artifact-reference, snapshot, reconciliation, and credential-lineage
   schemas.
2. Implement read-only plan/preview and deterministic blocker/readiness output.
3. Implement complete snapshot/export with external-artifact verification and
   fenced import.
4. Implement journaled migration, staged binary/package changes, crash
   recovery, retry classification, downgrade refusal, rollback, and forward
   recovery.
5. Integrate existing package successor, activation, update, disable, and
   rollback contracts without reviving approvals or leases.
6. Implement authority and credential reconciliation, including loss,
   expiration, compromise, replacement, revocation, and historical signature
   verification.
7. Extend doctor/readiness and lifecycle discovery using existing core and
   dynamic invocation ownership boundaries.
8. Qualify all interruption, corruption, restore, rollback, credential, and
   authority-divergence scenarios in isolated installations.
9. Qualify the schema-11 dogfood only after a complete snapshot and fresh
   authority decision; preserve its history and fence invalid authority.

## Future-surface integration

The lifecycle architecture records reviewer-role and protected-artifact
references when those surfaces exist. #117 and #118 are not prerequisites for
this architecture and must not become circular dependencies. Until then,
ADR-033 remains the acceptance mechanism and role/advisory evidence remains
non-authoritative.

## Advisory challenge evidence

The existing domain-neutral development-graph role boundary was used to
challenge the successor proposal:

| Role | Challenge | Disposition in ADR-075/SPEC-038 | Remaining risk |
|---|---|---|---|
| `architecture.lifecycle` | Prevent a lifecycle coordinator from becoming a competing domain owner. | Resolved by composing existing provider/domain contracts and assigning canonical lifecycle identity only to the coordinator. | Provider interface details remain implementation work. |
| `security.authority` | Prevent restore, rollback, credential replacement, or migration from reviving authority. | Resolved by explicit precedence, fencing, fresh-authority outcomes, and continuity decisions. | Reconciliation implementation remains unbuilt. |
| `persistence.data-lifecycle` | Determine complete backup contents and bind external artifacts. | Resolved by required component inventory and digest/provenance-bound `ArtifactRef`. | External provider availability and retention still require qualification fixtures. |
| `operations.recovery` | Prevent ambiguous interruption and unsafe retry. | Resolved by journal states, staged replacement, idempotency declaration, and reconciliation-required outcomes. | Cross-provider crash behavior remains unqualified. |
| `adversarial.invariant-review` | Attempt object substitution, stale restore, downgrade, and privilege revival. | Resolved at contract level by exact digests, current-lineage validation, and authority fencing. | Full adversarial execution evidence is not yet available. |

These are advisory conclusions derived from repository architecture and the
listed role responsibilities. The current repository cannot prove distinct
team execution identities or persist an independent protected-artifact review.
They are therefore evidence supporting the proposal, not acceptance authority.

## Governance gates

Each phase requires independent evidence and does not authorize the next phase.
No implementation step authorizes itself, and no migration step authorizes an
authority transition merely because it succeeded in changing representation.

## Exit criteria

An isolated installation must demonstrate preview, complete backup, fenced
restore, restart-safe migration, safe rollback, package successor handling,
credential lifecycle, authority reconciliation, historical verification, and
truthful readiness before the real dogfood installation is considered.

This plan is proposed and does not authorize implementation.

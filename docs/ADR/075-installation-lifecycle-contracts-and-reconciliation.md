# ADR-075: Installation Lifecycle Contracts and Reconciliation

- Status: Accepted
- Date: 2026-09-15
- Accepted proposal digest: `sha256:265e0c02fe3c8daab43bf20a4c062a88ef87fab79925576c2ded14ffd892ecff`
- Acceptance: explicit architecture-owner decision; implementation authorized, migration not authorized
- Predecessor proposal: ADR-074, `sha256:d7c8594cea7d3151230d2ea671885d529e457c8c13f3538d9fd5efe4a2c7d49d`
- Companions: SPEC-038, PLAN-005
- Related: ADR-011, ADR-031, ADR-032, ADR-033, ADR-036, ADR-047, ADR-065, ADR-069

## Context

ADR-074 established the required installation-lifecycle direction but left
four contracts insufficiently precise: lifecycle identity and journal state,
complete snapshots, interruption/downgrade recovery, and authority/credential
reconciliation. This proposal defines those boundaries without implementing
them or changing the status of ADR-074.

## Decision proposed

Praxis SHALL use one installation lifecycle boundary that composes existing
state, package, cryptographic, authority, evidence, and runtime providers. It
does not replace their semantic ownership.

The boundary SHALL use the canonical contracts and recovery rules in SPEC-038.
Every lifecycle transition SHALL preserve:

```text
software upgrade != authority upgrade
migration != historical reinterpretation
credential rotation != authority recreation
restored bytes != current authority
package restoration != package reactivation
evidence != authority
```

Lifecycle planning and execution SHALL remain separate from architecture
acceptance. ADR-033 remains the current human architecture-owner acceptance
mechanism. Future #117 team evidence and #118 protected-artifact governance
MAY provide stronger review and proposal surfaces, but this architecture does
not depend on either feature being implemented.

## Authority precedence

Current authority is evaluated in this order:

1. current installation identity and current authority lineage must validate;
2. explicit revocation, supersession, or expiry fences the affected authority;
3. exact current object and generation bindings must match;
4. restored historical authority is evidence only until separately reconciled;
5. when compatibility cannot be proven, current authority is fenced and new
   authority initialization is required.

No restore, rollback, migration, or credential replacement may make a stale
authority generation current by preserving its bytes.

## Consequences

The proposal makes lifecycle identity, snapshot completeness, crash recovery,
and reconciliation testable before implementation. It intentionally leaves
domain-specific effect semantics with existing providers and contracts.

This ADR is a successor proposal only. It is not accepted and does not
authorize lifecycle implementation or dogfood mutation.

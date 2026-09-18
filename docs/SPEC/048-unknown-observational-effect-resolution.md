# SPEC-048: Goals `/4` observational-effect resolution

Status: Proposed
Predecessors: SPEC-047; SPEC-046

## Closed scope

This contract applies only when all fields match exactly:

```text
contract: goals-established-state-publication/4
operation: publish-goals-from-established-state
step: verify-draft
adapter: goals-recovery-github
state: UNKNOWN
attempts: 1
later effects: none
abandonment: absent
```

The effect is observational. No upload, publish, release creation, or other
mutation may be resolved by this contract.

## Required evidence

The resolver loads and validates the exact request, ActionIntent, execution,
effect, delegated child generation, parent authority lineage, and all
predecessor bindings. It proves the delegated child governed the original
dispatch timestamp and remains active at resolution time. It requires the
immutable original observed result and the immutable
`goals-publication-recovery.reconciled` event with version `1` and outcome
`unresolved`.

Both observations must independently pass the current order-independent
verification validator. Their canonical semantic projections must be equal:
repository, owner/account, release ID, tag, commit, tree, draft/prerelease
state, and the exact name-keyed asset records and downloaded digests. Provider
enumeration order is excluded from semantic equality. Byte equality may be
recorded as supporting evidence but is not the general identity rule.

The resolver rejects missing, substituted, stale, contradictory, or malformed
observations; wrong lineage; invalid temporal authority; changed attempts;
later effects; abandonment; conflicting resolutions; and any path requiring
redispatch.

## Resolution transition

The dedicated CLI transition is:

```text
publisher goals-publication-recovery-resolve-observation
  --request-id <exact request ID>
  --effect-id <exact verify-draft effect ID>
  --confirmation "RESOLVE <canonical resolution digest>"
```

The resolution digest is deterministic over the exact request, intent,
execution, effect, delegated authority, original observation digest,
reconciliation event/payload digest, and canonical semantic observation
projection. The confirmation is required from the human operator for this
explicit state transition; it does not create authority.

On success, one transaction must append a distinct versioned resolution
command/event and call the existing terminal reconciliation primitive with
`SUCCEEDED` and the validated resolution evidence. The effect changes from
`UNKNOWN` to `SUCCEEDED`, retains attempts `1`, and retains its original
observed result. The unresolved reconciliation event is never rewritten.
Repeating the same exact resolution is idempotent. A different resolution
identity or evidence fails closed.

The resolver never calls adapter `Dispatch`, never retries, and never creates
later effects. A later execution may create only `publish-existing-release`,
after fresh authority and exact current-state validation; published-release
verification remains unavailable until publication succeeds.

## Authority and retention

The delegated child authority that governed the original verification must be
currently valid at resolution time. Resolution does not renew, extend, or
replace it. Expired authority, historical-only authority, or a second
authority generation fails closed. Historical evidence loading remains subject
to SPEC-047. Retention and destruction remain separate concerns.

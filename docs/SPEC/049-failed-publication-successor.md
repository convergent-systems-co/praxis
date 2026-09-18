# SPEC-049: Goals `/5` failed-publication successor

Status: Proposed
Predecessors: SPEC-048; SPEC-047; SPEC-046

## Contract

The contract is accepted only for:

```text
contract: goals-established-state-publication/5
operation: publish-verified-goals-from-established-state
adapter: goals-recovery-github
graph: publish,verify-published
```

The request must bind the exact `/4` predecessor request/version/digest,
ActionIntent/version/digest, execution identity, delegated child generation,
resolved `verify-draft` effect and resolution event/digest, and failed
`publish` effect. The failed publish effect must be `FAILED`, have exactly one
attempt, no observed result, and no reconciliation evidence. Its command and
step-admitted event must be present and agree. No `verify-published`, completion,
or abandonment record may exist.

## Pre-dispatch proof

Preparation proves local pre-dispatch failure from the durable effect state
and ledger: the publish command was admitted once; the effect reached terminal
`FAILED`; `observed_result` is absent; reconciliation evidence is absent; and
there is no provider observation or mutation event for the publish effect.
Any provider-started, ambiguous, or otherwise unclassifiable attempt is
rejected. A textual diagnostic alone is insufficient.

## Established state

The successor binds installation, repository and owner IDs, release ID, tag,
commit, tree, draft state, and exactly three assets. Each asset is bound by
ID, name, size, uploaded state, uploader, signed digest, and downloaded-byte
digest. The values in the `/5` intent and durable preconditions must agree
exactly with the resolved `/4` verification observation and a current
read-only provider observation. Provider ordering has no semantic meaning.

## Authority and preparation

The `/5` request requires a fresh owner decision and delegated child generation
with a new bounded expiration within the active authority model's 24-hour
bound. The `/4` authority is evidence only. Active `/4` authority is validated
normally; expired `/4` authority is loaded only through ADR-084/SPEC-047's
typed historical projection. Historical loading cannot delegate or execute.

The preparation surface is:

```text
publisher goals-publication-recovery-failed-publication-prepare
  --package-dir <exact package directory>
  --request-id <exact /4 predecessor request ID>
  --expires-at <fresh bounded /5 expiration>
```

Preparation performs no provider mutation and creates no authority.

## Execution and failure

`publish-existing-release` is dispatched only after fresh `/5` governance and
immediate exact-state revalidation. It updates the existing draft release;
assets are never uploaded or replaced. `verify-published-release` is available
only after publish succeeds and verifies the exact resulting release. Local
pre-dispatch failure is terminal and immutable. Provider ambiguity is
`UNKNOWN` and follows reconciliation; already-published state is not silently
treated as success.

The `/5` generation does not rewrite `/4`, claim that `/4` published anything,
or include the malformed preparation-only request in lineage.

## Identity and adversarial requirements

Canonical digests must bind the complete predecessor projection, chain
extension, current established state, and ordered effect scope. Reject
substituted lineage, resolution, failed effect, authority, release, asset,
temporal, graph, operation, or pre-dispatch evidence; replay and restart must
be exact and idempotent. Provider order changes are accepted only when the
identity-bound semantic inventory is unchanged.

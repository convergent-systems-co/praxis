# SPEC-038: Installation Lifecycle Contracts

- Status: Accepted
- Accepted proposal digest: `sha256:bf77c1cd1bdb7158f098649321bd22f8ab118b401265fd34c21e4932f5a13f16`
- Acceptance: explicit architecture-owner decision; implementation planning authorized
- Governing proposal: ADR-075
- Predecessor proposal: SPEC-037, `sha256:6794582d1ee0d563918f516139e97c45dc1bc3adafb57f4ec6e45b8e62e4c861`
- Related: ADR-011, ADR-031, ADR-032, ADR-033, ADR-036, ADR-047, SPEC-005, SPEC-011

## 1. Canonical lifecycle plan

`LifecyclePlan` is an immutable canonical record with:

- `plan_id`, `plan_version`, and content digest;
- installation identity and current installation-manifest digest;
- target binary, provider, schema, package, contract, credential, authority,
  evidence, and runtime identities/digests;
- ordered `TransitionStep` records;
- required preconditions and evidence references;
- preserved historical object references;
- authority requirements and the exact decision scope;
- snapshot requirements;
- reversible, irreversible, and recovery-only classifications;
- rollback or forward-recovery strategy;
- expected resulting readiness vector.

Each `TransitionStep` has a stable step ID, predecessor step IDs, component
class, current identity/digest, target identity/digest, precondition digest,
authority requirement, effect class, recovery strategy, and readiness impact.
Unknown component classes or missing identity/digest bindings fail closed.

## 2. Canonical transition journal

`TransitionJournal` is append-only and binds:

- journal ID/version/digest;
- exact plan ID/version/digest;
- installation identity;
- transition sequence and step ID;
- current state and prior state;
- precondition/evidence digest;
- snapshot digest;
- authority request/decision/generation references;
- recovery action and resulting readiness digest.

Allowed state transitions are:

```text
planned → approved → prepared → applying → committed
                         ├→ failed_recoverable
                         ├→ reconcile_required
                         └→ rolled_back

failed_recoverable → prepared | reconcile_required | rolled_back
reconcile_required → committed | rolled_back | fenced
```

The journal is immutable; corrections append a successor record. A transition
cannot be committed unless all step preconditions and the resulting component
manifest are revalidated against the plan digest.

## 3. Snapshot completeness and external artifacts

An installation snapshot is complete only when its manifest accounts for every
authoritative or recovery-required component:

- canonical state, events, projections/checkpoints, evidence, findings, and
  provenance;
- schema/provider metadata;
- package manifests, dependency locks, signatures, verification evidence,
  artifacts, and activation history;
- binary identity and digest;
- bootstrap metadata and protected-provider references;
- credential references, lineage, revocation, and historical verification
  metadata;
- runtime/supervisor recovery state;
- configuration and external dependency declarations.

Each component is either embedded or represented by an `ArtifactRef` containing
artifact identity/version, media type, size, content digest, source/provenance
digest, provider/reference, retention class, and availability evidence.

An external artifact is admissible only when its provider identity, immutable
reference, exact digest, retrieval/authenticity evidence, and retention policy
are recorded in the snapshot. A locator, filename, mutable URL, or successful
retrieval alone is not sufficient. Missing or unverifiable required external
artifacts make the snapshot incomplete and block executable restore.

Private keys are excluded by default. Permitted key export requires an
explicit destination-bound capability and encrypted profile.

Raw SQLite files may be retained as diagnostic material but are not the
canonical portable snapshot.

## 4. Crash, retry, downgrade, and rollback semantics

- Preparation must durably record the exact plan, preconditions, and snapshot
  before an irreversible step.
- SQLite/provider-local atomic work either commits or rolls back as one
  transaction.
- Atomic file replacement uses a staged artifact, digest verification, and a
  recoverable pointer/manifest update.
- Interruption before commit leaves the prior committed state authoritative.
- Interruption after an external effect or ambiguous process exit enters
  `reconcile_required`; it is never blindly retried.
- Retry is idempotent only for a plan/step/precondition digest that the
  provider declares idempotent. Otherwise reconciliation is required.
- Downgrade is refused when the target cannot read the current schema,
  contracts, or authority metadata. A binary rollback never rewinds state,
  authority, credentials, leases, or evidence.
- Rollback may restore only a retained compatible software/package closure;
  it never restores consumed approvals, revoked authority, or newer history.
- Forward recovery is allowed only when a declared adapter or reconciliation
  procedure proves the target representation and preserves historical meaning.
- Restart at every journal boundary must yield one deterministic state:
  committed, recoverable failure, reconciliation required, fenced, or rolled
  back.

## 5. Authority and credential reconciliation

Reconciliation evaluates current authority separately from historical records:

1. validate the current installation identity and authority-generation lineage;
2. apply explicit revocation, supersession, expiry, and compromise fences;
3. validate exact object, generation, proposal, contract, and digest bindings;
4. classify restored records as current-compatible, historical-only, stale,
   revoked, unverifiable, or requiring new authority;
5. permit current execution only for current-compatible records;
6. require fresh governance for incompatible or missing authority.

Credential states distinguish active, expiring, expired, retiring,
retired_verify_only, lost, compromised, revoked, and destroyed. Expired or
retired credentials may verify historical signatures only where policy permits;
they cannot create new signatures. Loss or compromise fences new use and may
require new installation-root enrollment. A replacement credential receives
no authority implicitly. A continuity record must bind predecessor reference,
replacement reference, reason, evidence, and an independent governed decision.

## 6. Readiness

Doctor SHALL report independently:

```text
crypto bootstrap
state store
schema/provider compatibility
governance root
authority topology
package/runtime closure
lifecycle journal/recovery state
installation readiness
```

Installation readiness does not imply Goal execution readiness or validity of
restored historical authority.

## 7. Qualification

Qualification SHALL cover interrupted migration, partial writes, corrupt and
incomplete snapshots, missing external artifacts, incompatible schemas,
package downgrade, stale restore, authority divergence, binary rollback after
revocation, credential expiration/loss/compromise/replacement, historical
signature verification, restart at every journal state, idempotent retry, and
preservation of historical evidence without authority revival.

This specification is proposed and does not authorize implementation.

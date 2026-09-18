# ADR-087: Installation Repair Authority and Reconstruction-Evidence Trust

- Status: Proposed — architecture-owner decision required
- Date: 2026-09-17
- Related: ADR-074 (Built-in Praxis Authority Model v1, itself still
  Proposed), ADR-075 / SPEC-038 / ADR-076 (Installation Lifecycle
  Contracts and successor lineage, Accepted), PLAN-005 / PLAN-006, PLAN-007
  (Derived Runtime-Registry Repair, drafted but NOT accepted pending this
  decision), issue #138 (migration-identity tracking defect), issue #139
  (installation-repair primitive gap)
- Triggering evidence: independent review of PLAN-007 established, from
  primary code (`internal/state/package_registry.go:200-241`,
  `pkg/contracts/authority_model.go:19-22`, `pkg/contracts/delegation.go:10`,
  `authority_model.go:78-79,119-120`), that PLAN-007's two central technical
  claims do not hold against current code: (1) `installed_packages` +
  `package_activation_receipts` + `invocation_registry` are not sufficient
  to deterministically reconstruct `invocation_runtime_bindings` for any
  entry point with a plugin executable binding — that path requires a
  `packagecatalog.VerifiedPackage`, obtainable only by re-running signature
  verification against the original artifact bytes and applicable trust
  roots; (2) `AuthorityRequest.RequestedAuthority` is a closed, code-enforced
  enum (`GovernedPackagePublish`, `GovernedPackageDeploy`,
  `AuthorityDelegateCapability`, each with a dedicated exact-match validator
  branch), and no existing `AuthorityGeneration`/`AuthorityRequest`/
  `AuthorityDecision` field represents a non-delegable authority semantic —
  delegation is structurally assumed available throughout the current model.

## Why this is a new ADR, not a PLAN-007 revision or an existing-ADR successor

**Correction (independent review, 2026-09-17):** an earlier draft of this
section incorrectly characterized the authority-kind enum in
`pkg/contracts/authority_model.go` as an "already-accepted, closed
contract." That claim is false and is withdrawn. The accurate provenance,
verified by checking every ADR that references
`GovernedPackagePublish`/`GovernedPackageDeploy`/the authority-kind
constants: **no Accepted ADR currently governs that vocabulary.** ADR-074,
ADR-078, and ADR-079 — every document that touches this vocabulary — are
all `Proposed`, none `Accepted`. The current closed `RequestedAuthority`
vocabulary is enforced entirely by production code and tests (the
dedicated exact-match validator branches in `authority_model.go`, e.g.
lines 78-79, 119-120): it is real, working, and genuinely closed at the
code level, but it has no accepted architectural document behind it.
ADR-074 being Proposed means it cannot supply accepted architectural
authority for this ADR to extend.

Given that, ADR-087 is not extending accepted architecture — it is
proposing an additional authority kind and associated semantics at an
architecture boundary that is **currently implemented but incompletely
documented/governed**, doing exactly what ADR-074/078/079 already do: each
is a freestanding proposal over unaccepted, code-only convention, not a
successor claiming inherited acceptance. No single existing ADR's lineage
legitimately covers both findings triggering this document (ADR-075/076
govern installation-lifecycle *transitions* in general but do not define
authority-kind vocabulary; ADR-074 attempts to define that vocabulary but
remains unaccepted). This decision sits at the intersection of two
unaccepted-or-partially-accepted lineages and is therefore drafted as a new,
freestanding ADR referencing both as context, not as a successor claiming
either one's acceptance.

## Decision requested

### 1-4. Installation repair authority: a new, closed, non-delegable authority kind

Adopt `RequestedAuthority = "installation.repair-derived-registry"` as a new
member of the v1 closed authority vocabulary (alongside
`GovernedPackagePublish`/`GovernedPackageDeploy`/`AuthorityDelegateCapability`
in `pkg/contracts/authority_model.go`), with:

- **Principal:** the installation-governance root principal only (the same
  principal class validated via `InstallationOwnerPrincipal`/
  `InstallationGovernanceScope`, `pkg/contracts/authority_request.go:69-86`)
  — no intermediate/delegated principal may originate a repair request.
- **Parent authority:** the installation-governance root generation
  (`ParentRef`/`ParentVersion`/`ParentDigest` empty-or-present-together per
  existing `AuthorityGeneration.Validate()`), matching how root-level
  authority is already structurally distinguished from delegated authority.
- **Non-delegability, as a complete architectural invariant, not a
  one-path claim (corrected):** an earlier draft of this ADR described a
  single new validator branch (rejecting
  `installation.repair-derived-registry` as the `Delegation` payload of a
  downstream `AuthorityRequest`, structurally identical to the existing
  closed-profile checks at `authority_model.go:78-79,119-120`) as though it
  were a complete closure. It is not. Independent review identified **two
  known `AuthorityGeneration` persistence surfaces** in
  `internal/goalstore/repository.go`:
  1. `SaveDelegatedAuthorityGeneration` /
     `SaveAuthorityDecisionAndDelegatedAuthorityGeneration` — gated by the
     delegation-containment validators (`authority_model.go`'s
     `ValidateBuiltinDelegation` and profile-specific checks). The
     validator branch described above closes **only this path**.
  2. `SaveAuthorityGeneration` (`repository.go:822-831`) — persists any
     `AuthorityGeneration` after shape/provenance validation only
     (`AuthorityGeneration.Validate()`), with **no authority-kind
     awareness at all**. This path is not touched by the delegation-profile
     validators and is not closed by the branch described above.

  The required architectural invariant is therefore stated independently of
  any single implementation mechanism:

      installation.repair-derived-registry authority
      MUST NOT appear in any child/lower delegated AuthorityGeneration,
      through any production path capable of persisting AuthorityGeneration
      state.

  This ADR does **not** prescribe a specific implementation for closing the
  `SaveAuthorityGeneration` path merely to satisfy this invariant on paper.
  Implementation and qualification MUST demonstrate complete enforcement
  across every current and future generation-creation/persistence path, not
  merely the delegation-containment path, before this authority kind may be
  considered non-delegable in practice.

  **Trusted-reachability is not a substitute for architectural
  enforcement.** `SaveAuthorityGeneration`'s only current caller is root
  bootstrap code (`cmd/praxis/authoritybootstrap.go:417`), so there is no
  live exploit today — but "only reachable from a currently-trusted caller"
  is a fact about the present call graph, not an architectural guarantee.
  A future caller, refactor, or new bootstrap/maintenance path could invoke
  `SaveAuthorityGeneration` directly with an `installation.repair-derived-registry`-scoped
  generation and bypass the delegation-containment check entirely, because
  that check does not sit on this path. The invariant above must hold
  architecturally — provably, across all paths — not merely hold today
  because the only caller happens to be trustworthy.
- **Scope:** exactly one derived-registry component class per request
  (`derived-runtime-registry` in v1); the request MUST bind the exact target
  table identity, so scope cannot silently broaden to "any table."
- **Expiry:** short-lived (hours), consistent with existing recovery-successor
  expiries used elsewhere this session.

### 5. Repair scope semantics

A repair authority grant authorizes exactly one `TransitionStep` component
class instance, bound by digest to one target table identity, one precondition
(source-evidence) digest, and one target (reconstructed-state) digest, per
the §9 atomic revalidation-at-commit discipline (the five concurrency
predicates, established via the new `installation-repair` aggregate type,
not a literal reuse of `CommitTransitionGuarded`). It does not authorize any
other mutation.

### 6-7. Package/artifact/signature evidence required for deterministic reconstruction

Reject PLAN-007's "three durable tables are sufficient" claim. For any
`invocation_runtime_bindings` row whose owning package uses a plugin
executable binding, deterministic reconstruction requires:

- the original package artifact bytes (already durable, referenced from
  `package_activation_receipts`);
- the original signature envelope (already durable per
  `package_activation_receipts:308,318-319` per the independent review's
  citation);
- a **currently valid** set of `SignatureVerifier`s / trust roots capable of
  re-verifying that signature today.

**Grounding (corrected): this is not an ADR-087 analogy, it is a direct
application of already-accepted SPEC-038 §5 (Accepted per the ADR-075/076
lineage), not merely "extended" from ADR-075's restored-bytes language.**
SPEC-038 §5 states the general rule this ADR applies to one table:
historical/restored records must be classified as one of
*current-compatible, historical-only, stale, revoked, unverifiable, or
requiring new authority*, and "permit current execution only for
current-compatible records." It further states: "Expired or retired
credentials may verify historical signatures only where policy permits;
they cannot create new signatures." The precise distinction required is:

    historical verification = valid historical evidence
        but NOT automatically
    current executable authority

Historical verification establishes that a package's content was
legitimately activated in the past — that fact is durable, immutable,
and never erased or reinterpreted. It does **not**, by itself, establish
that the same material may be used to reconstruct an **active current**
runtime binding — i.e. state that Praxis's own execution dispatch will
treat as currently authoritative (traced in the independent review:
`invocation_runtime_bindings` content is consumed directly by
`Store.ActiveInvocations` and alias resolution to decide what code
actually runs). Per SPEC-038 §5 rule 5, only records classified
current-compatible may feed current execution.

**Existing verified-package evidence MAY be reused to reconstruct an
active current runtime binding only if it is itself classified
current-compatible under SPEC-038 §5** — i.e., re-verification under
current governing trust semantics succeeds. It MUST NOT be reused merely
because verification succeeded historically at activation time. If a
signing key has since been rotated, retired, or revoked, the historical
evidence remains preserved exactly as recorded (never deleted, never
reinterpreted) — it is simply not eligible to reconstruct current
executable authority. This ADR does **not** design credential
reconciliation (SPEC-038 §5's continuity-record/replacement-credential
machinery, or ADR-075's deferred Step 6) — that remains a separate,
not-yet-designed governed transition. v1 must classify the outcome using
SPEC-038's own vocabulary (§10 below) and fail closed; it must not attempt
reconciliation, and it must not treat "package material exists and was
once verified" as sufficient grounds to mint a current binding.

**Exact reconstruction requirement (added per independent review):**
verifying the package's signature is necessary but not sufficient.
`resolveExecutableBinding` (`internal/state/package_registry.go:200-237`)
does not merely check that a package is authentic — it re-derives
`runtime_id`/`runtime_version`/`runtime_digest`/`ExecutableBinding` from the
verified package's manifest content, with byte-exact digest cross-checks at
every step (plugin-definition digest, executable-content digest, and an
explicit `binding.ExecutableDigest != executable.Digest` fail-closed check).
This cross-validation is what prevents a reconstructed binding from
introducing an executable identity that was not independently authorized by
the original package verification and activation. Repair MUST reuse this
exact production derivation and cross-check logic — not merely re-verify
the package's signature and then copy `runtime_id`/`runtime_version`/
`runtime_digest`/`executable_binding_json` from some other convenient
durable field (e.g. trusting `package_activation_receipts` content
verbatim, or any prior/stale `invocation_runtime_bindings` row). Doing so
would bypass the manifest-to-binding digest cross-validation and reopen
exactly the executable-identity-smuggling risk this ADR otherwise closes.
The required chain is:

```
package artifact
  ->
cryptographic verification under governing current trust semantics
  ->
VerifiedPackage
  ->
production executable-binding derivation (resolveExecutableBinding or its
exact equivalent, including all digest cross-checks)
  ->
reconstructed invocation_runtime_bindings row
```

No step in this chain may be skipped or replaced with a shortcut that
reads pre-computed values from a source not itself independently
re-validated by this same chain.

### 8. Trust ordering among evidence sources

This hierarchy is scoped explicitly and only to reconstructing **CURRENT
executable runtime bindings** — i.e., rows in `invocation_runtime_bindings`
that Praxis's own dispatch path will treat as presently authoritative for
what code runs. It is not a general evidence-trust ordering for historical
record-keeping, diagnostics, or evidence preservation, all of which retain
their own existing immutability guarantees regardless of this hierarchy.

Adopt this explicit hierarchy, most-authoritative first, for
`derived-runtime-registry` reconstruction of current bindings:

```
1. package artifact bytes + signature envelope, currently re-verifiable
   under current governing trust semantics (SPEC-038 §5 "current-compatible"
   classification)                     (cryptographic ground truth for
                                         CURRENT authority)
2. package_activation_receipts          (durable record that verification
                                          occurred, and against what content)
3. installed_packages                   (durable record of installation
                                          state)
4. invocation_registry                  (corroborating only — see below)
5. invocation_runtime_bindings           (the repair target itself; never
                                          a source for its own reconstruction)
```

**`invocation_registry` is explicitly NOT an independent trust root, and
this is stated as an architectural fact, not a cautious default: it is
produced by the exact same activation transaction that produced the
damaged derived state it would otherwise be used to validate.**
`package_registry.go:342-346` writes `invocation_registry` immediately
before writing `invocation_runtime_bindings` at line 374, inside one
transaction, from one code path. A table that shares its producer and
failure mode with the table being repaired cannot serve as independent
evidence of that table's correctness — it can only corroborate agreement
with genuinely independent evidence (the re-verified artifact/signature
chain and the activation receipts recording that verification). If
`invocation_registry` and the re-verified package/activation evidence
disagree, the repair MUST fail closed (`failed_recoverable` per §10) and
report the disagreement rather than trusting either side unilaterally.

### 9. Lifecycle aggregate/component identity for guarded repair (corrected)

**Withdrawn claim:** an earlier draft stated that installation-governance
identity is "versioned against the installation's own `AggregateVersion` in
the same sense other installation-governance transitions already are." This
is false and is withdrawn. Independent review traced both mechanisms
directly: `CommitTransitionGuarded`'s optimistic-concurrency check
(`compareAndAdvanceAggregate`, `internal/state/store.go`) versions strictly
by `(aggregate_id, aggregate_type)` pairs in the `aggregate_versions` table.
Installation-governance root/delegated generation identity, by contrast, is
persisted entirely through `internal/goalstore`'s secure-blob `Ref`/`Version`
generation-lineage chain (`SaveAuthorityGeneration`/
`SaveDelegatedAuthorityGeneration`) — a **completely separate persistence
and versioning mechanism**. No `AggregateType` in the codebase today is
`installation-governance` or any installation-root-generation type; nothing
currently bridges these two mechanisms. Using the installation-governance
generation as a `CommitTransitionGuarded` aggregate would therefore be
**new architectural behavior**, not existing precedent, and this ADR must
not claim otherwise.

**Required concurrency predicates.** Whatever aggregate/version mechanism
is used, the guarded commit MUST atomically establish, at commit time, that
all of the following still match what an approved repair proposal bound:

1. the approved repair proposal's own identity/digest;
2. the exact repair-target pre-state digest (the live
   `invocation_runtime_bindings` contents match what the proposal observed);
3. the exact authoritative source-evidence digest (the live
   re-verified package/activation evidence matches what the proposal bound);
4. the exact reconstructed-output digest (what would actually be written
   matches what was approved, not a recomputed value that happens to
   differ);
5. current repair authority (the decision has not expired, been revoked,
   or been superseded).

**ADR-level resolution (not deferred to PLAN-007 or implementation):**
option **A** — installation repair receives a legitimate, explicitly
defined lifecycle-component aggregate/version identity **within the
already-accepted `aggregate_versions` mechanism**, without any claim of
bridging to or reusing installation-governance's separate `Ref`/`Version`
chain. Specifically: a new `AggregateType = "installation-repair"`,
`AggregateID` bound to the installation's own stable identity (its
bootstrap/installation identity digest, not its governance generation
`Ref`), used exclusively for `derived-runtime-registry`-class repair
transitions and nothing else. This is a narrow, explicitly new use of
already-accepted machinery (`aggregate_versions`/
`compareAndAdvanceAggregate`), not a claimed reuse of an existing
installation-governance pattern, and it does not touch, version, or
contend with installation-governance's own generation lineage in any way.
The five predicates above are established by: (1)-(2) checked against the
`invocation_runtime_bindings` table's live digest inside the guarded
transaction; (3) checked against a fresh re-derivation of the source
evidence inside the same transaction; (4) checked by recomputing the
proposed output inside the same transaction and comparing to the approved
digest; (5) checked against the `AuthorityDecision`'s expiry/revocation
state inside the same transaction. A distinct commit function (not
`CommitTransitionGuarded` verbatim — PLAN-007 must not claim literal reuse)
is expected, shaped around this predicate set and DDL-plus-DML semantics
rather than the effects/command/event row model `CommitTransitionGuarded`
was built for.

Option **B** (leaving aggregate representation as an unresolved
architectural decision) is rejected as unnecessary: option A above
resolves it without inventing false precedent and without touching
installation-governance's own mechanism, so nothing here blocks ADR
acceptance on this specific question.

### 10. Recovery outcomes (corrected: mapped onto SPEC-038 §2/§4 taxonomy, not a single generic "fail closed" bucket)

**Withdrawn:** an earlier draft collapsed every failure mode into one
undifferentiated "fail closed" outcome. SPEC-038 §2 already defines an
accepted, named `TransitionJournal` state taxonomy (`planned → approved →
prepared → applying → committed`, with `failed_recoverable`,
`reconcile_required`, `rolled_back`, and (§4) `fenced` as first-class
outcomes, plus §4's rule that "restart at every journal boundary must
yield one deterministic state"). `derived-runtime-registry` repair reuses
this exact vocabulary rather than inventing a parallel one. Semantically
distinct conditions are not collapsed merely because all of them prevent
forward progress:

| Condition | SPEC-038 outcome | Notes |
|---|---|---|
| Missing reconstruction evidence for an active `invocation_registry` entry (artifact/receipt absent) | `failed_recoverable` | Repair simply did not reach `prepared`; re-running `propose` from scratch is always safe once evidence exists. |
| Current-trust verification failure (signing key rotated/retired/revoked/unverifiable) | `reconcile_required` | Per SPEC-038 §5: an `unverifiable`-classified record requires governed credential reconciliation, not a bare retry — explicitly out of this ADR's/v1's scope to resolve, but the *outcome itself* must be recorded as `reconcile_required`, not silently reported as a generic failure. |
| Stale proposal (approved proposal's bound digests no longer match current live state) | `failed_recoverable` | The TOCTOU guard (§9 predicates 1-4) rejects the transaction before any write; a fresh `propose` is the recovery path. |
| Source-state change between proposal and execution | `failed_recoverable` | Same guard as above; not distinct from "stale proposal" as an outcome, only as a cause. |
| Target-state change between proposal and execution (table already mutated by something else) | `failed_recoverable` | Same TOCTOU class; proposal's bound pre-state digest no longer matches. |
| Transactional failure before commit (any error inside the guarded transaction) | `failed_recoverable` | SPEC-038 §4: "SQLite/provider-local atomic work either commits or rolls back as one transaction" — old table state is authoritative, nothing partial is ever observable. |
| Successful commit | `committed` | Durable completion evidence recorded per §"Consequences." |
| Post-commit verification uncertainty (commit succeeded but `verify` cannot confirm dynamic resolution now works, or crashes before confirming) | `reconcile_required` | Per SPEC-038 §4: "interruption after an external effect or ambiguous process exit enters `reconcile_required`; it is never blindly retried." A committed table write is not "external" in the same sense as a provider mutation, but the *verification* step's own success/failure is exactly this class of ambiguity — the underlying data mutation is real and durable, but whether the repair actually achieved its purpose (restoring dynamic invocation) is unconfirmed. Do not re-run `execute`; re-run `verify` (idempotent, read-only) or escalate to `reconcile_required` handling. |
| Restart/crash strictly before commit | `failed_recoverable` (equivalent to never having started) | Per SPEC-038 §4: "interruption before commit leaves the prior committed state authoritative." Original broken table is exactly as it was; `propose`/`inspect` are safely re-runnable. |
| Restart/crash after commit but before `verify` | `reconcile_required` | The mutation is durable (SQLite transaction semantics guarantee this), but whether the *intended outcome* (dynamic resolution restored) has been externally confirmed is genuinely `UNKNOWN` until `verify` runs — this is precisely SPEC-038's reconciliation category, not a fresh failure and not a silent success. |

Fail-closed remains the default disposition for any condition not
explicitly classified above (unknown component class, missing digest
binding — per SPEC-038 §1: "Unknown component classes or missing
identity/digest bindings fail closed"), landing in `failed_recoverable`
unless evidence specifically indicates `reconcile_required` per the table
above. No condition results in a partial registry, a silent fallback to
stale verification, or an unconfirmed outcome being reported as
`committed`. Remediation of the evidence gap itself (credential
reconciliation) remains separate, not-yet-designed work (ADR-075's
deferred Step 6 / SPEC-038 §5 domain), explicitly out of this ADR's and
PLAN-007's scope — this ADR only requires that the *outcome* be classified
correctly using SPEC-038's existing vocabulary, not that reconciliation be
performed.

## Consequences

This decision, if accepted, authorizes:
- extending the closed `RequestedAuthority` vocabulary — itself currently
  code-enforced but not yet backed by any Accepted ADR — with one new,
  non-delegable, root-principal-only member;
- a complete architectural non-delegability invariant for that member,
  requiring implementation/qualification to prove enforcement across every
  `AuthorityGeneration` persistence path (not merely the
  delegation-containment path), with trusted-reachability of any currently
  unguarded path treated as a present-state fact, not an architectural
  guarantee;
- a reconstruction requirement that reuses the exact production
  manifest-to-binding derivation and digest cross-validation logic
  (`resolveExecutableBinding` or its exact equivalent), never a shortcut
  that copies pre-computed runtime-binding fields from a source not itself
  independently re-validated;
- an explicit evidence-trust hierarchy that subordinates `invocation_registry`
  to corroborating-only status;
- a distinct (not reused-verbatim) atomic commit path scoped to a new,
  explicitly independent `installation-repair` aggregate identity within
  the existing `aggregate_versions` mechanism — not a bridge to or reuse of
  installation-governance's own generation-lineage versioning;
- a recovery-outcome model mapped onto SPEC-038's existing named
  `TransitionJournal` taxonomy (`committed`/`failed_recoverable`/
  `reconcile_required`/`fenced`), not a single undifferentiated "fail
  closed" outcome.

It does **not** authorize: generic table repair, migration-content-identity
repair (#138's domain), credential/signature reconciliation design (remains
deferred), or any dogfood mutation. Implementation remains ungoverned until
this ADR (or a corrected successor) is explicitly accepted by the
architecture owner.

## PLAN-007 portions superseded/invalidated pending this decision

- **Delivery sequence step 3** ("compute the join... three tables") —
  invalid as written; must be replaced with the evidence-trust hierarchy in
  §8 above, including mandatory re-verification for plugin-executable rows.
- **§"Wire proposal review/authority"** claim of *"no new authority
  primitive, a new narrowly scoped capability only"* — invalid; this ADR's
  §1-4 is the missing new authority primitive PLAN-007 disclaimed needing.
- **Non-delegability claim** ("non-delegable in v1") — invalid as an
  assumed-free property; now requires proving the complete architectural
  invariant in §1-4 across every `AuthorityGeneration` persistence path
  (not just one validator branch on one path).
- **`CommitTransitionGuarded` reuse claim** ("reused... for TOCTOU-safe
  execution") — invalid as literal reuse; superseded by §9's new
  `installation-repair` aggregate type within the existing
  `aggregate_versions` mechanism (explicitly not a bridge to
  installation-governance's own Ref/Version chain).
- **Generic "fail closed" outcome handling** — superseded by §10's mapping
  onto SPEC-038 §2/§4's named outcome taxonomy
  (`committed`/`failed_recoverable`/`reconcile_required`/`fenced`); PLAN-007
  must not describe repair outcomes as a single undifferentiated bucket.
- **Adversarial matrix** — retained in spirit but requires redesign, not
  just addition: (a) a row for `invocation_registry` drift relative to
  re-verified package evidence; (b) a row for signing-key
  rotation/revocation since activation, classified `reconcile_required`
  per §10, not a bare failure; (c) every existing row must be re-classified
  against the §10 outcome taxonomy instead of a single "rejected"/"fails
  closed" outcome; (d) concrete fixture/execution/assertion specification
  per row (independent review's Finding C — still deferred to PLAN-level
  work, but now scoped against a taxonomy that actually distinguishes
  outcomes, not a flat pass/fail).
- **Qualification cases involving current-trust/reconciliation semantics**
  (any case touching signature re-verification, key rotation, or
  post-commit verification uncertainty) require redesign against §6/§10
  above — PLAN-007's original matrix did not distinguish
  `reconcile_required` from `failed_recoverable` at all.
- **Everything else in PLAN-007** (scope discipline, governance gates,
  exit-criteria mirroring, durable-evidence-on-any-outcome requirement,
  fail-closed corrected invariant for missing/incomplete evidence) remains
  sound and should be preserved once revised against this ADR, if accepted.

This ADR is proposed only. It does not authorize implementation, PLAN-007
revision-and-acceptance, or dogfood mutation.

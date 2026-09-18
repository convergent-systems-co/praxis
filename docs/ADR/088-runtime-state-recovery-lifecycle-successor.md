# ADR-088: Runtime-State Recovery as Two Ordered Lifecycle Transitions (Successor to ADR-076's Step 5/6 Deferral, Narrow)

- Status: **Accepted** — Thomas, architecture owner, accepted this ADR's
  exact scope as written on 2026-09-17, following two independent
  reviewer-model verification passes (a full 19-gate review that returned
  5 blocking + 5 non-blocking findings, and a focused re-verification
  confirming all 5 corrections resolved with 0 new regressions). This
  acceptance covers exactly the scope stated herein — the two-transition
  `storage_schema -> runtime_state` decomposition, the retained-history
  population model, the v1 plugin exclusion pending #140, and the
  mechanics of §2-§19 — and nothing broader. It is not a blank endorsement
  of the direction beyond this document's stated scope.
- Date: 2026-09-17
- Predecessor: `docs/ADR/076-installation-lifecycle-contracts-successor.md`,
  digest `sha256:6622f53cb6aa6993dd71127a14dbf693c9ad5074401b229fd746476b52d0df91`
  (verified by direct `shasum -a 256` of that file, matching its own
  self-declared digest)
- Companions: `docs/SPEC/039-installation-lifecycle-contracts-successor.md`,
  digest `sha256:9fe5af8c621df3d59e371c59db93d4e2ecdd8a7d3c159a7c682248cef14755fd`;
  `docs/PLAN/006-installation-lifecycle-contracts-successor.md`, digest
  `sha256:98df961c07ba71939a5172b13f93eec8b7263f024237740348c2128c7367d027`
  (both verified the same way)
- Supersedes as design direction (not by content): `docs/ADR/087-installation-repair-authority-and-evidence-trust.md`
  — ADR-087 remains proposed and not accepted (its own status line reads
  "Proposed — architecture-owner decision required",
  `docs/ADR/087-installation-repair-authority-and-evidence-trust.md:3`) and
  is preserved unmodified as
  historical evidence of the rejected separate-mechanism path. This document
  does not rewrite or accept it.
- Related: `docs/PLAN/007-derived-runtime-registry-repair.md` (also remains
  `PROPOSED — NOT ACCEPTED`; not revision-ready — see "PLAN-007 disposition"
  below), issue #138 (migration identity/content drift — this revision
  establishes new primary evidence of exactly this defect, §2 below), issue
  #139 (installation-repair primitive gap), issue #140 (package trust-root
  provenance gap — referenced, not solved, here), issue #136 (mandatory
  independent-model review at consequential graph boundaries)
- Governing human decisions: Thomas, architecture owner —
  (1) `CONCUR — OPTION A`: recovery is modeled as first-class lifecycle
  transition(s) under the accepted `ADR-076`/`SPEC-039`/`PLAN-006` lineage,
  using the existing `lifecycle_transition` journal, never a parallel
  installation-repair mechanism; (2) recovery is **two ordered transitions**,
  `storage_schema` then `runtime_state`, not one collapsed transition; (3)
  `invocation_runtime_bindings` is durable per-package-generation binding
  **history**, not an active-set-only table — inactive retained generations
  are legitimate and must survive repair.

## 1. What this decision states, precisely

Installation derived-runtime-registry recovery **is** two ordered
first-class lifecycle transitions under the accepted lifecycle
architecture — not one transition, and not a separate mechanism:

```
storage_schema  ->  runtime_state
```

Both use `LifecycleComponentClass` values already present in the closed,
`Validate()`-enforced enum (`pkg/contracts/lifecycle.go:18-27`):
`LifecycleSchema = "storage_schema"` and `LifecycleRuntime = "runtime_state"`.
Neither is new. This ADR does not introduce a new component class. No
primary evidence was found establishing that either enum value cannot
legitimately represent this recovery's structural and semantic halves
respectively, so both are used as-is. If future evidence establishes
otherwise, a new component class requires its own human architecture
decision — this ADR does not authorize introducing one.

Both transitions use the existing `internal/lifecycle.Journal` /
`lifecycle_transition` aggregate (`internal/lifecycle/migration.go:21,25`)
and the existing, already-implemented `LifecycleTransitionState` machine
(`pkg/contracts/lifecycle.go:311-326`), as two separate `LifecycleTransitionStep`
entries in one `LifecyclePlan` (§3 covers exactly how `runtime_state`'s
precondition is bound to `storage_schema`'s qualified output — they are
ordered, not merely co-scheduled). No new aggregate type, no
table-as-aggregate shortcut, no parallel journal, no second state machine.
ADR-087's `installation-repair` `AggregateType` proposal is explicitly not
adopted, for either transition.

**Why two transitions, not one:** the defect has two independent failure
modes with independent evidence and independent recovery obligations. A
structural column mismatch (`no such column: rb.runtime_id`) is provable
from schema introspection alone and has one deterministic correct
resolution (rename, preserving row identity). A semantic-content question
(is a given retained row's `runtime_id`/`runtime_version`/`runtime_digest`
value the value current production code would have produced) requires
per-binding-class evidence chains (§6/§7) that have nothing to do with
column names. Collapsing them into one transition would force `runtime_state`
semantic validation to also carry `storage_schema`'s all-or-nothing
mechanical precondition, and would force `storage_schema`'s renamed-value
carry-over to silently look like a semantic guarantee it is not (§2's
central prohibition). Keeping them separate lets `storage_schema` fail and
be retried/rolled back on purely structural grounds without ever touching
semantic reconstruction, and lets `runtime_state` refuse to start at all
against an unqualified schema (§3).

## 2. `storage_schema` transition: structural correction only

**Primary evidence of the exact defect, newly established in this
revision** (not previously documented in this ADR): the dogfood-authoritative
schema and current source both trace to a migration numbered identically
(`migrations/sqlite/0012_invocation_runtime_bindings.sql`) but with two
different bodies across this repository's reconciled history —
`git show 0084352:migrations/sqlite/0012_invocation_runtime_bindings.sql`
defines `handler_id TEXT NOT NULL, handler_version TEXT NOT NULL,
handler_digest TEXT NOT NULL`; `git show 2860381:migrations/sqlite/0012_invocation_runtime_bindings.sql`
defines `runtime_id TEXT NOT NULL, runtime_version TEXT NOT NULL,
runtime_digest TEXT NOT NULL` at the **same migration number**, both ending
in the identical `UPDATE schema_meta SET value='12' WHERE key='schema_version'`.
This is issue #138's defect made concrete: `schema_meta.value` alone cannot
distinguish which body a given installation was bootstrapped from, because
both bodies claim the same schema version. **Any precondition evidence this
transition relies on MUST come from direct schema introspection of the live
database (e.g. `PRAGMA table_info(invocation_runtime_bindings)`), never
from `schema_meta.value` alone.** Trusting the version number here would
repeat exactly the defect that caused the outage.

**Correction: the divergence is not column-name-only.** Comparing the two
bodies completely (not only the three renamed columns): the PK is unchanged
(`entry_point_id, package_version, content_digest`), and `package_id,
contract_digest, registered_at` are unchanged. But the `FOREIGN KEY`
clause also differs — `0084352`'s body declares
`FOREIGN KEY (entry_point_id, package_version, content_digest) REFERENCES
invocation_registry(entry_point_id, package_version, content_digest)` with
**no `ON DELETE CASCADE`**; `2860381`'s body (the current
`migrations/sqlite/0012_invocation_runtime_bindings.sql`) declares the
same FK **with** `ON DELETE CASCADE`. A dogfood installation bootstrapped
from the `0084352` body therefore differs from current source in both the
three column names **and** the FK's delete-action semantics. This ADR
withdraws any framing of this transition as "column rename only" — it is a
**complete live-schema-to-target-schema structural correction**, of which
the three-column rename is one part and the FK delete-action correction is
another. There may be no other undiscovered divergences, but this
transition's precondition evidence (below) must prove that, not assume it.

`0013_executable_invocation_bindings.sql`'s `executable_binding_json`
column addition is unaffected by either body and is not part of this
transition.

**This transition's step:**
- `Class = "storage_schema"`, `Current` = the component ref for the
  as-found (introspected, not assumed) complete table definition, `Target`
  = the complete table definition current source requires — column names,
  types, primary key, foreign keys and their delete/update actions, and
  indexes, not the three renamed columns alone.
- **Precondition and postcondition evidence MUST be captured via complete
  live schema introspection**, not a partial check of the three known
  renamed columns: `PRAGMA table_info(invocation_runtime_bindings)` for
  columns/types/PK, **and** `PRAGMA foreign_key_list(invocation_runtime_bindings)`
  for the FK's referenced table/columns and `ON DELETE`/`ON UPDATE`
  actions, together with any other schema/index introspection needed to
  fully characterize the table (e.g. `sqlite_master`/`PRAGMA index_list`
  for the existing `idx_invocation_runtime_bindings_package` index). A
  precondition check that only inspects the three column names and
  silently assumes the FK clause already matches would repeat exactly the
  class of defect (`schema_meta.value` bookkeeping presumed sufficient)
  that caused the original outage.
- `Effect = LifecycleAuthorityBound` (schema mutation on the authoritative
  installation database is not `LifecycleAutomatic`), `SnapshotRequired =
  true`. **`Reversible` depends on the mechanism SQLite actually requires**:
  SQLite's `ALTER TABLE` can rename a column but cannot alter an existing
  table's `FOREIGN KEY` clause in place — changing the FK's `ON DELETE`
  action requires the standard SQLite table-rebuild sequence (create the
  new table with the target definition, copy all rows, drop the old table,
  rename the new one into place, recreate the index), not a bare column
  rename. This ADR does not choose between "rename columns, then
  separately rebuild for the FK clause" and "one rebuild that fixes
  everything at once" — either is acceptable — but whichever is chosen
  must honestly declare its own `Reversible` value: a rebuild that
  preserves every row's content in a form from which the exact prior table
  definition and content can be reconstructed qualifies as `Reversible =
  true`; if it cannot, `Reversible = false` forces `Effect =
  LifecycleIrreversible`, which independently forces `SnapshotRequired =
  true` per `LifecycleTransitionStep.Validate()`, `pkg/contracts/lifecycle.go:186-188`.
- **Preserves every retained row's identity and content bytes exactly**,
  regardless of which mechanism (in-place rename vs. table rebuild) is
  used: the values that were `handler_id`/`handler_version`/`handler_digest`
  become the values of `runtime_id`/`runtime_version`/`runtime_digest` for
  every row in the table — active and inactive alike (§4). This transition
  performs no filtering, no selection, and no per-row judgment on binding
  *content*; it is a uniform structural operation over the complete
  retained population. (A table rebuild that copies rows already
  necessarily "touches" every row mechanically — that is not the kind of
  per-row judgment this prohibition refers to; it refers to never
  including or excluding a row based on its content or activation state.)
- **MUST NOT claim semantic requalification.** A row whose
  `runtime_digest` column now holds the byte-identical value that was
  previously in `handler_digest` has **not** been validated as the digest
  current production code would compute for that row. This transition's
  job ends at "the live table definition now exactly matches current
  source's expected shape — columns, types, PK, and FK delete-action alike
  — with every row's prior content preserved verbatim." Whether that
  content is semantically correct is entirely `runtime_state`'s question
  (§7/§8), out of scope here.
- Runs before `runtime_state` can be attempted at all: `runtime_state`'s
  precondition (§3) is bound to proof that this transition's step actually
  reached `committed` under the exact same plan `runtime_state` is itself
  bound to (§3 defines the exact, field-accurate mechanism), so
  `runtime_state` structurally cannot execute against the old schema (in
  either the column names or the FK clause), a partially-migrated shape,
  or an unqualified structural result.

## 3. Sequencing: `runtime_state` precondition bound to `storage_schema`'s qualified output

**Correction: `LifecycleTransitionJournal` has no `ResultingManifestDigest`
field, and `ReadinessDigest` does not record a step's `Target.Digest`.**
Primary evidence: `LifecycleTransitionJournal`'s complete field list
(`pkg/contracts/lifecycle.go:328-348`) is `JournalID, Version, PlanID,
PlanDigest, InstallationID, Sequence, StepID, PreviousState, State,
PreconditionDigest, SnapshotDigest, AuthorityRef, AuthorityVersion,
AuthorityDecisionRef, AuthorityDecisionDigest, AuthorityGenerationDigest,
RecoveryAction, ReadinessDigest, RecordedAt` — there is no
`ResultingManifestDigest` field anywhere in the journal entry.
`ReadinessDigest` is populated, in every entry `RunStep` appends, from
`req.Plan.TargetManifestDigest` (`internal/lifecycle/migration.go`'s
`journalEntry` function: `ReadinessDigest: req.Plan.TargetManifestDigest`)
— the **plan's** overall target-manifest digest, the same value on every
entry for every step in that plan, not any individual step's
`Target.Digest`. The prior text's citation was wrong, and the mechanism it
described cannot be implemented against the actual contract.

**Correct mechanism, using only fields that genuinely exist, requiring no
new durable field:** `LifecyclePlan.Digest` is computed
(`LifecyclePlan.ComputeDigest`, `pkg/contracts/lifecycle.go:251-263`) over
the plan's complete `Steps` array, which includes every step's `Current`
and `Target` component refs (and therefore `storage_schema`'s
`Target.Digest`) as an immutable part of what `PlanDigest` cryptographically
commits to. `RunStep` already only ever appends a `Committed` entry for a
step after checking `result.ResultingManifestDigest == step.Target.Digest`
at runtime (`internal/lifecycle/migration.go:269-272`) — this check happens
before commit, is not itself persisted as a separate digest field, and does
not need to be, because the following two facts, both already durable and
already checked elsewhere in this ADR, are jointly sufficient:

1. `runtime_state`'s own resume/execution is already required (§13.1) to
   verify `req.Plan.Digest` against the plan it is running, which — because
   `storage_schema` and `runtime_state` are two steps of **one** plan — is
   the identical `PlanDigest` that governs `storage_schema`'s step
   definition, including its exact `Target.Digest`, in that same plan.
2. `storage_schema`'s last journal entry for `(PlanID, StepID)` has `State
   = LifecycleCommitted`, which (by point 1's runtime check, already
   enforced by existing `RunStep` logic) can only have been appended after
   the driver's actual applied result matched that exact `Target.Digest`.

Therefore: **`runtime_state`'s `Preflight` MUST load the journal history
for the `storage_schema` step under the same `PlanID`, verify that step's
last entry has `PlanDigest == req.Plan.Digest` (the same equality check
§13.1 already requires generally) and `State == LifecycleCommitted`, and
refuse to proceed (never entering `applying`) otherwise.** No freshly
invented digest field is required — the binding falls directly out of
`PlanDigest` equality plus `storage_schema`'s own `committed` state, both
already-existing, already-verifiable facts. This also means `runtime_state`
and `storage_schema` MUST be steps of the same `LifecyclePlan` (not two
independently-digested plans that merely happen to run in the chosen
order) — this ADR requires that construction, not merely orders two
otherwise-unrelated plans.

## 4. Exact deferral text being narrowly superseded

ADR-076's own words: *"This artifact governs PLAN-005 Steps 1–4... Step 5
and later lifecycle work remain deferred."* PLAN-006 independently
confirms the same boundary. PLAN-005 defines:

- **Step 5:** "Integrate existing package successor, activation, update,
  disable, and rollback contracts without reviving approvals or leases."
- **Step 6:** "Implement authority and credential reconciliation, including
  loss, expiration, compromise, replacement, revocation, and historical
  signature verification."

**This ADR lifts the deferral only to the extent required for:**

```
LifecycleComponentClass:  storage_schema  (structural correction, §2)
LifecycleComponentClass:  runtime_state   (semantic validation, §7/§8)
v1 recovery target:       invocation_runtime_bindings
```

Specifically, it authorizes:
- the `storage_schema` structural rename described in §2, over the
  complete retained population of `invocation_runtime_bindings` (not a
  Step 5/6 matter at all — this is schema maintenance, not
  activation/rollback/credential logic, and requires its own authority
  bound per §9);
- reading (never mutating) `installed_packages`, `package_activation_receipts`,
  and `invocation_registry` as `runtime_state`-transition evidence sources
  for this one component class (a narrow slice of Step 5's "integrate...
  activation... rollback contracts" — read-only integration, not the
  general package-successor/update/disable/rollback lifecycle work Step 5
  otherwise names);
- for plugin executable bindings only, re-running package signature
  verification under current trust semantics as a precondition for
  `runtime_state` (a narrow slice of Step 6's "historical signature
  verification" — re-verification for reconstruction eligibility, not the
  general credential-loss/compromise/replacement/revocation reconciliation
  machinery Step 6 otherwise names).

**This ADR explicitly does NOT authorize:**
- general Step 5 implementation (package successor/update/disable/rollback
  as first-class lifecycle transitions in general);
- general Step 6 implementation (credential continuity records, replacement
  credentials, loss/compromise handling, the full SPEC-038 §5 six-way
  classification machinery as a governed, implemented system);
- generic installation repair of any other table or state class;
- arbitrary database repair;
- migration-history repair beyond the one narrow rename in §2 (issue
  #138's broader domain — general migration-identity tracking — remains
  separate future work);
- credential-reconciliation architecture beyond the one narrow
  re-verification precondition named above;
- ADR-087's separate-aggregate design;
- implementation code (this ADR is a decision record, not a PLAN or code);
- dogfood DB mutation;
- weather-app Goal resumption.

Everything else ADR-076 deferred remains deferred. A future PLAN or ADR
seeking to implement general Step 5/6 work must obtain its own separate
human architecture decision; this document is not precedent for that.

## 5. Lifecycle integration status — first production integration, not reuse of a running system

Primary evidence: `internal/lifecycle.Journal`/`RunStep` (`internal/lifecycle/migration.go`)
have **zero production importers** anywhere in this repository today — every
reference to `NewJournal`/`RunStep` outside `internal/lifecycle` itself is in
this ADR's own text or in test files for the package itself. The lifecycle
architecture (contracts, state machine, journal type) exists and is
implemented, but it has never been wired into any production command path.

**This ADR's `storage_schema`/`runtime_state` transitions are therefore the
first production integration of the lifecycle architecture, not an
extension of an already-operating subsystem.** Qualification of the
eventual replacement PLAN must treat every mechanism this ADR relies on —
the journal append path, the state-machine transition legality, the
guard/precondition mechanisms of §10-§13 — as exercised for the first time
under real production conditions, not as previously load-bearing,
already-hardened machinery. This materially raises the qualification bar
described in §18; it is not a reason to defer wiring it in, since ADR-076
already accepted the lifecycle architecture's design and this is precisely
the deferred integration work it anticipated.

## 6. Retained-generation population semantics — membership, activation, and binding identity as three separate questions

Primary evidence: `internal/state/package_registry.go:288,342,465`
(activation) and `internal/state/package_rollback.go:88,337`
(rollback/deactivation) all mutate `invocation_registry.active`, at times
independent of and later than the original activation transaction that
wrote `invocation_runtime_bindings`. No production code path deletes rows
from `installed_packages`, `invocation_registry`, or
`invocation_runtime_bindings`; the only production `INSERT` into
`invocation_runtime_bindings` is `activateVerifiedPackageTx`; uninstall
marks `installed_packages` removed but deletes nothing from either
invocation table. Disable and rollback both operate exclusively through
`invocation_registry.active`, never through deletion.

**Consequently, `invocation_runtime_bindings` is durable per-package-generation
binding history, not an active-set-only table.** Reconstruction must treat
the following as three orthogonal questions, not one linear trust-priority
list (correcting both ADR-087's central defect and this ADR's own earlier,
now-withdrawn active-set-only framing):

- **Retained-generation population (WHICH rows legitimately exist at
  all, active or not):** every `(entry_point_id, package_version,
  content_digest)` key that independent durable installation/activation
  provenance (`installed_packages`, `package_activation_receipts` where
  present, and the FK-enforced correspondence to `invocation_registry`,
  `foreign_keys` enabled) establishes as a legitimately-installed-and-activated-at-some-point
  generation. **Correction:** the current source's `0012` body declares
  this FK with `ON DELETE CASCADE` (§2), but §2's own primary evidence
  shows the historical (pre-`storage_schema`) dogfood table may not have
  that clause at all — so this FK-enforced-correspondence property holds
  for the *post-`storage_schema`* schema, once that transition has
  committed, not necessarily for the live table as first found. This
  provenance reasoning about retained-generation population applies from
  `runtime_state`'s perspective, i.e. after `storage_schema`'s precondition
  (§3) is already satisfied — it is not a claim about the as-found,
  possibly-`0084352`-shaped table. A
  corresponding `invocation_registry` row establishes structural
  retained-generation *membership* only — it does not, by itself,
  establish governance *legitimacy* of that membership; legitimacy is
  established by reconciling against the independent provenance sources
  above.
- **Current activation selection (WHICH retained generation is
  currently active):** `invocation_registry.active = 1` at repair time.
  This determines routing, not existence. A row with `active = 0` is not
  thereby illegitimate or a repair target for deletion — it is exactly
  the kind of row rollback/reactivation depends on being present later.
- **Binding identity/content (WHAT each existing binding's `runtime_id`/
  `runtime_version`/`runtime_digest`/`executable_binding_json` must be):**
  derived per binding class, per §7/§8 below — never read back from
  `invocation_registry` or any other pre-computed source.

**Non-deletion requirement, stated as an explicit invariant:** this
recovery, across both transitions, MUST NOT delete, and MUST NOT produce as
a side effect the loss of, any retained-generation row that independent
provenance shows was legitimately installed and activated at some point,
regardless of its current `active` value. `storage_schema` (§2) already
satisfies this by construction (uniform rename, no row selection).
`runtime_state`'s semantic-validation scope (§8) is narrower than the full
retained population for v1 (bounded to what the immediate dogfood failure
requires — the currently active set, which is what the failing query
actually joins against), but narrowing *validation* scope is not the same
as narrowing *preservation* scope: inactive retained rows survive
`runtime_state` untouched, with their `storage_schema`-renamed but
not-yet-semantically-revalidated content intact, explicitly flagged as such
(§8) rather than silently presented as validated.

## 7. Non-plugin / client-adapter binding reconstruction

Primary evidence, independently re-verified against the exact production
code:

```go
// internal/state/package_registry.go:336-353
for _, inv := range manifest.Invocations {
    body, err := json.Marshal(inv)                    // line 336
    ...
    // written verbatim as invocation_registry.contract_json:  line 343
    ...
    runtimeID := "client-adapter:" + inv.PackageID + ":" + inv.EntryPointID   // line 351
    runtimeVersion := inv.Version                                            // line 352
    runtimeDigest := digestPackageBytes(body)                                // line 353 — SAME body
```

`body` is the identical byte sequence written to
`invocation_registry.contract_json` and used to compute the non-plugin
`runtime_digest`.

**Correction: `runtime_id` does not come from the target row's own
`package_id` column.** Primary evidence establishes `invocation_registry.package_id`
(and, identically, `invocation_runtime_bindings.package_id`) is written
from `manifest.PackageID` — the *package manifest's* package identity
(`package_registry.go:343`: `INSERT INTO invocation_registry(...,package_id,...)
VALUES(?,inv.EntryPointID,manifest.PackageID,...)`, i.e. the row's
`package_id` column is populated from `manifest.PackageID`, not from
`inv.PackageID`). Production's `runtimeID` formula at line 351 instead uses
`inv.PackageID` — the field on the *invocation contract itself*
(`manifest.Invocations[i]`), the same struct whose full JSON encoding
(`body`) is `contract_json`. `inv.PackageID` and `manifest.PackageID` are
two distinct fields from two distinct structs that happen to hold the same
value in the ordinary case, but nothing in the schema or this code
guarantees they are equal, and reconstruction MUST use the one production
actually reads: **`inv.PackageID`, recovered only by decoding
`contract_json` back into the `InvocationContract` shape** — never by
reading the target row's `package_id` column and assuming it is the same
field. Reading the row's `package_id` column here would be exactly the
"read a different, unvalidated source" shortcut §7 (below) says this class
avoids; the target row's `package_id` is a different, independently-sourced
field, not a validated stand-in for `inv.PackageID`.

The corrected reconstruction chain, decoding `contract_json` once and
reading every field this class needs from that one decode:

```
invocation_registry.contract_json (durable, already-canonical bytes)
  ->
decode contract_json back into the InvocationContract shape
  (contracts.InvocationContract / the manifest.Invocations element type)
  ->
runtimeID = "client-adapter:" + (decoded).PackageID + ":" + (decoded).EntryPointID
  (both fields read from the DECODED CONTRACT, never from the target
  row's own package_id/entry_point_id columns, even though
  entry_point_id happens to be part of this table's own primary key —
  the decoded value is the authoritative source; the PK column may be
  used only as a cross-check, never as the derivation source itself)
  ->
runtimeVersion = (decoded).Version
  ->
runtimeDigest = digestPackageBytes(contract_json bytes exactly as stored)
  ->
reconstructed invocation_runtime_bindings row (executable_binding_json = NULL)
```

**Required cross-check, not a derivation shortcut:** if the decoded
`(decoded).PackageID`/`(decoded).EntryPointID` disagree with the target
row's own `package_id` column or its `entry_point_id` primary-key
component, that disagreement is itself evidence of corruption or drift in
exactly the row this recovery is trying to repair, and MUST be treated as
a precondition failure for that row (falling out under §8's missing/
unresolvable-evidence handling) — never silently resolved by preferring
one source over the other without recording the discrepancy.

No shortcut is taken here in the sense ADR-087 warned against (reading a
*different*, unvalidated source) — corrected, this derivation reuses the
exact production formula (`inv.PackageID`/`inv.EntryPointID`/`inv.Version`
from the decoded contract, not the row's own columns) against the exact
production input. Requiring package-signature re-verification for this
class, as ADR-087 did blanket-wide, is not supported by primary evidence
and is not adopted here.

## 8. Plugin executable-binding reconstruction

**Classification evidence source, stated explicitly (correcting an
omission, not a prior claim):** whether a given retained generation is
plugin-backed or a client-adapter binding MUST be determined from the
*source-side* evidence chain below — the package artifact's own manifest
and `resolveExecutableBinding`'s production classification logic
(`internal/state/package_registry.go:200-239`) as applied to that manifest
— never from any field already present in the target
`invocation_runtime_bindings` row being repaired (`runtime_id`,
`executable_binding_json`, or any other column). A row's own
`executable_binding_json` being `NULL` or non-`NULL` is exactly the kind of
"potentially corrupted target-state field" this recovery exists to not
trust; classification must not depend on it. If the source-side manifest
cannot be located or resolved for a given retained generation, that row's
classification is itself unresolved and it MUST NOT be defaulted into the
non-plugin path — it falls out under §11 as a missing-evidence precondition
failure, never a silent downgrade.

For any row whose owning package uses a plugin executable binding
(`resolveExecutableBinding` returns non-nil, `internal/state/package_registry.go:200-239`),
the complete production chain applies, with no step skipped or shortcut:

```
package artifact bytes (from durable package content storage)
  ->
dependency closure resolution (packagecatalog.VerifyPackage requires
  input.ResolvedDependencies; EffectiveCapabilities(manifest, dependencies)
  must be recomputed, not assumed — internal/packagecatalog/verification.go)
  ->
current governing package verification (signature re-check against
  currently governing trust semantics — see §9; this is the narrow Step-6
  slice this ADR authorizes, and only this slice)
  ->
VerifiedPackage
  ->
resolveExecutableBinding production derivation (internal/state/package_registry.go:200-239),
  including its existing byte-exact digest cross-checks (plugin-definition
  digest, executable-content digest, the explicit
  `binding.ExecutableDigest != executable.Digest` fail-closed check at
  line ~232)
  ->
proposed runtime binding (runtime_id/runtime_version/runtime_digest/
  executable_binding_json)
```

No step may be skipped, and no field may be populated from a source that
was not itself produced by this exact chain (e.g. never copy
`executable_binding_json` from a prior/stale `invocation_runtime_bindings`
row or from `package_activation_receipts` without re-deriving through
`resolveExecutableBinding`).

## 9. Current trust semantics (issue #140, referenced not solved) — v1 scope boundary

Plugin reconstruction (§8) depends on package signature verification
succeeding under *currently* governing trust semantics. Primary evidence
establishes this is presently sourced from `PRAXIS_TRUSTED_KEYS`
(`cmd/praxis/packages.go:417-424`), an unversioned process environment
variable with no durable provenance binding and no wiring to any
credential-state classification. **This ADR does not solve that gap** — it
is tracked separately as issue #140 (package trust-root provenance).

**Non-plugin/client-adapter reconstruction (§7) does not depend on #140 at
all** — it never reads `PRAXIS_TRUSTED_KEYS` or performs any signature
verification. #140 is exclusively a plugin-binding-path concern.

**Plugin bindings are OUT OF SCOPE for this v1 `runtime_state` transition
as a completable repair path.** If any row within `runtime_state`'s v1
validation scope (§8's active-membership set, per §6/§10) is plugin-backed,
current verification cannot be legitimately established against a durable
trust root — only an ambient environment variable — so that row's binding
is **not reconstructable under this ADR**. Per the complete-key-set
invariant (§10), this makes the *entire* `runtime_state` transition
non-executable for that plan — not a partial success, and not a silently
smaller reconstructed registry. This ADR does not claim plugin-binding
recovery is qualifiable today, and does not attempt to work around #140
opportunistically.

**Correction: failure routing is stated here precisely, against
`validLifecycleTransition`'s actual map (`pkg/contracts/lifecycle.go:311-326`),
not colloquially.** The prior text implied `prepared -> failed_recoverable`
and a "post-`applying`" outcome as if either were routine — neither is
legal: `LifecyclePrepared`'s only legal successor is `LifecycleApplying`
(`:315`), and `failed_recoverable`/`reconcile_required` are reachable only
from `applying` (`:316`); there is no post-`committed` outcome at all
(`committed` is terminal, §15). The exact, legal routing for a
plugin-classification discovery is:

| Current state | Condition | Legal next state | `RecoveryAction` required? |
|---|---|---|---|
| `planned` (not yet `prepared`) | plugin-backed row identified during precondition evaluation, before the plan is even proposed for `prepared` | the plan/step simply never advances to `prepared`/`applying` — no failure state is entered, because none was ever reached | n/a — no journal entry beyond `planned` exists for this to attach to |
| `applying` | plugin-classified row's trust verification definitively fails (e.g. key demonstrably absent/revoked under whatever trust-root source is actually configured) | `applying -> failed_recoverable` | yes — a concrete action such as `"plugin-verification-failed-pending-140"` |
| `applying` | plugin-classified row's trust verification is itself ambiguous (e.g. trust-root infrastructure unreachable, not a definitive revocation) | `applying -> reconcile_required` | yes — a concrete action such as `"plugin-verification-ambiguous-pending-140"` |

A plugin-backed row discovered before `applying` is simply never approved
into `applying` in the first place — this is not a distinct "failure"
state, it is the ordinary precondition-gating behavior already described
in §15's first crash-table row. Only a plugin-backed row's status changing
or first being discovered **during** `applying` produces a `failed_recoverable`/
`reconcile_required` journal entry, and only via the one legal edge each
of those states actually has.

## 10. Authority semantics — corrected, not falsely claimed

Primary evidence (independently re-verified, correcting ADR-087's original
premise): `RequestedAuthority` is **not** a globally closed enum at the
`AuthorityRequest`/`AuthorityGeneration` contract layer.
`AuthorityRequest.ValidateAt` (`pkg/contracts/authority_request.go`) treats
the three named constants (`GovernedPackagePublish`, `GovernedPackageDeploy`,
`AuthorityDelegateCapability`) as *exemptions* from a baseline/proposal/
review evidence requirement, not as an allowlist; any other non-empty
string validates provided it carries that evidence.
`AuthorityGeneration.Validate()` never inspects the authority string at
all. The genuinely closed layer is narrower: the delegation-*containment*
validators (`pkg/contracts/authority_model.go`'s profile-specific checks).
And the persistence surface for `AuthorityGeneration` state is broader than
any two named `Repository` methods — it is the exported `secure_blobs`
writer API (`PutSecureBlob`/`PutSecureBlobWithLock`/`PutSecureBlobsWithLock`/
`PutSecureBlobUnlessRevoked`, `internal/state/secure_blob.go`), reachable
from any in-tree caller.

**Required architectural invariant for both lifecycle transitions,** stated
without falsely claiming an existing global enum:

```
the repair authority exercised for the storage_schema and runtime_state
transitions
    MUST NOT appear in any child/lower delegated AuthorityGeneration,
    through any production path capable of persisting AuthorityGeneration
    state (the delegation-containment path AND the broader secure_blobs
    writer surface).
```

This requires: (a) a new containment/validation rule at the layer that IS
closed today (the delegation-containment profile validators), explicitly
rejecting this authority as any `Delegation` payload — the same pattern
already used for `package.publish`/`package.deploy`; and (b) an
implementation/qualification obligation to demonstrate, not merely assert,
that no other reachable `secure_blobs`-write path can mint this authority
for a delegated generation. This ADR does not prescribe the exact
enforcement mechanism for (b) — that is PLAN-level and implementation
work — but it requires that work to close the invariant completely, not
partially, before this authority kind may be considered non-delegable in
practice. Trusted-reachability of any currently unguarded path (e.g. the
fact that a given raw persistence function is today only called from
trusted bootstrap code) is a fact about the present call graph, not a
substitute for architectural enforcement, and must not be treated as one.

Principal/parent requirements: the installation-governance root principal
only (validated via `InstallationOwnerPrincipal`/`InstallationGovernanceScope`,
`pkg/contracts/authority_request.go:69-86`), root generation as parent
(no intermediate/delegated principal may originate this request), for
**both** transitions independently — `storage_schema` is not exempt merely
because it is "just" a rename.

## 11. Complete-key-set invariant — scoped correctly per transition

`storage_schema` (§2) is **unconditional**: it applies to the complete
retained population of `invocation_runtime_bindings`, active and inactive
alike, with no key-set filtering. There is no "expected set" to compute —
every existing row is in scope by construction, and the transition either
renames every row's columns successfully or does not commit at all (a
`storage_schema` rename step, if partially applied, must not be considered
`committed` — see §13).

`runtime_state`'s complete-key-set invariant is **scoped to v1's validation
target**, not the complete retained population (§6):

```
reconstructed binding key set (runtime_state, v1)
    ==
complete expected active membership key set
```

"Expected active membership" is exactly the set of `invocation_registry`
rows with `active = 1` at the moment the transition's precondition
evidence is captured (§6). Every member of that set requires a
successfully derived binding via §7 or §8 as applicable. If even one
expected-active entry lacks a legitimate, complete derivation (missing
evidence, failed current-trust verification per §9, or any other
disqualifying condition), the **entire `runtime_state` transition** is
non-executable — there is no partial success, no silently-smaller
reconstructed active set, and no row-by-row "best effort" outcome. This
directly preserves the fail-closed correction established earlier in this
session's PLAN-007 work and rejects ADR-087's later drift from it.

**This invariant governs semantic validation scope only.** It does not
authorize deleting, ignoring the preservation of, or silently
"deactivating" any inactive retained row outside this key set — §6's
non-deletion requirement applies regardless of `runtime_state`'s narrower
validation scope. A future transition (out of scope here, not designed by
this ADR) would extend semantic validation to the full retained population
if and when reactivation of a currently-inactive generation is attempted;
this ADR does not claim v1 already does that, and does not need to for the
weather-app-unblocking goal this recovery serves.

## 12. Control-plane independence

The repair lifecycle entry point MUST be reserved bootstrap/control-plane
behavior: a hardcoded case in `cmd/praxis/main.go`'s top-level switch (the
same pattern already used for `authority`/`migration`/`supervise`/
`resume`/`cancel`), never resolved through `resolveDynamicInvocation`,
never dependent on `invocation_runtime_bindings` content, and never itself
made resolvable through the dynamic registry it repairs. Primary evidence:
no existing hardcoded case wires into the `internal/lifecycle` journal
machinery today (`grep "case \"lifecycle"` in `cmd/praxis/main.go` returns
nothing) — this is new wiring required regardless of which lifecycle
component class is used, not something this ADR can claim already exists.
This applies identically to both `storage_schema` and `runtime_state`
entry points; neither may be reachable through the registry either is
repairing.

## 13. Restart-safe identity binding, `applying` recovery, and atomic journal/mutation commit

This section replaces this ADR's prior acceptance of an unbounded
journal/mutation atomicity gap and an unaddressed `applying`-restart defect.
Both are now analyzed against the exact production code, not asserted.

### 13.1 Plan/step identity binding at restart — corrected claim

**Prior text in this ADR overstated what `Journal.Append` guarantees.**
Primary evidence: `Journal.Append` (`internal/lifecycle/migration.go:66-100`)
enforces exactly one thing about ordering — that `entry.Sequence ==
len(history)+1`, where `history` is the *entire installation's* lifecycle
journal (all plans, all steps), via `j.Load`'s per-event
`AggregateVersion` check. It does **not**, by itself, verify that a
resumed `(PlanID, StepID)` is being resumed against the same `PlanDigest`
it was previously advanced under. `RunStep`'s own resume path
(`lastStepState`, `internal/lifecycle/migration.go:303-311`) matches
history entries by `PlanID` and `StepID` alone — `PlanDigest` is never
compared. A plan edited under the same `PlanID`/`StepID` (for example, a
corrected precondition list) would silently resume against a stale step
definition with no error.

**Required correction:** `RunStep` (or its successor implementation) MUST,
before treating any non-empty `current` state as resumable, verify that the
`PlanDigest` recorded on the last journal entry for that `(PlanID, StepID)`
equals `req.Plan.Digest` for the plan being run now. On mismatch, the step
MUST NOT resume silently. It must be routed through a legal transition
only — `applying -> reconcile_required` if the mismatch is discovered after
`applying` was already entered, or simply refusing to advance past
`planned`/`prepared` if discovered earlier (exactly as an ordinary
precondition failure is already handled, §15's first row). No illegal
transition is invented merely to fence this case; the existing
`reconcile_required` outcome, with a specific `RecoveryAction` such as
`"plan-digest-mismatch"`, is sufficient and already legal from `applying`.

### 13.2 `applying`-restart recovery — corrected claim

**Prior text in this ADR asserted crash-before-mutation "remains `prepared`
or `applying` with no committed table change," implying restart from
`applying` was unproblematic. That was not verified against the actual
resume path and is withdrawn.** Primary evidence: in `RunStep`
(`internal/lifecycle/migration.go:252-259`), when `current ==
LifecycleApplying` on resume, execution falls through the
`if current == LifecyclePrepared {...} else if current !=
LifecycleApplying {return error}` guard (the `else if` does not fire,
because `current` *is* `Applying`) directly into an **unconditional**
`appendState(contracts.LifecycleApplying, "")`. This constructs a journal
entry with `PreviousState = Applying, State = Applying`, which
`LifecycleTransitionJournal.Validate()` rejects — `Applying` has no
self-transition in `validLifecycleTransition` (`pkg/contracts/lifecycle.go:311-326`,
`Applying`'s only legal successors are `Committed`, `FailedRecoverable`,
`ReconcileRequired`, `RolledBack`). `journal.Append` therefore returns an
error, and the step is **permanently stuck** — it can never legally
progress past this point through the code as it stands today. This is a
real, currently-unreachable-to-recovery defect in the existing
implementation, not a hypothetical.

**Required correction, stated at the conceptual level this ADR operates
at** (implementation is future PLAN/code work, not authorized here):

- On resume, if the last recorded state for `(PlanID, StepID)` (with
  `PlanDigest` verified per §13.1) is `Applying`, the resume path MUST NOT
  append another `Applying` entry. `Applying` is entered exactly once per
  attempt; re-entry is illegal by design (§14 already established this),
  and correctly so — a second entry into `Applying` would be
  indistinguishable from a second real attempt for journal-reading
  purposes.
- Instead, resuming from `Applying` MUST establish the exact durable
  effect/mutation state via §13.3's atomic-commit invariant, which by
  construction eliminates the ambiguity a naive resume would otherwise
  need to guess at: because the domain mutation and its corresponding
  outcome journal entry commit together in one transaction (§13.3), any
  process that resumes and finds the journal still at `Applying` (no
  outcome entry beyond it exists) has thereby already learned that the
  paired domain mutation did **not** commit either — there is no case
  where the mutation committed but the outcome entry did not, and no case
  requiring separate out-of-band verification of "did the mutation
  commit." A resume from `Applying` may therefore safely re-attempt
  `driver.Apply` under the driver's own `Idempotent` contract (the same
  contract already governing the `FailedRecoverable -> Prepared` retry
  path today), transitioning out of `Applying` through one of its four
  legal successors based on the fresh attempt's outcome — never by
  re-appending `Applying`.
- No `Applying -> Prepared` transition is invented; none is legal, and
  none is required once §13.3 removes the ambiguity a resume would
  otherwise need that transition to paper over.

### 13.3 Atomic journal/mutation commit — new required capability

**Prior text in this ADR accepted "journal append and table mutation are
two separate durable operations" as a permanent, documented limitation.
That acceptance is withdrawn for the domain-mutation boundary** (predicate
6, the authority check, is addressed separately in §13.5 with its own
honestly-scoped bound).

Primary evidence: `SQLiteEventStore.Append` (`internal/state/eventstore.go:22-91`,
the production `eventstore.Store` backing `internal/lifecycle.Journal`) and
`invocation_runtime_bindings`'s mutations (`internal/state/package_registry.go`)
both operate against the same `*sql.DB` — the single authoritative
installation SQLite database — but `SQLiteEventStore.Append` opens and
commits its own `*sql.Tx` internally; it accepts no externally-supplied
transaction. There is today no tx-scoped variant of the journal event
insert that a `storage_schema`/`runtime_state` driver's `Apply` could join
with its own domain mutation in one transaction. This is a real capability
gap, not a fact about SQLite (SQLite itself supports exactly this: this
session already demonstrated the pattern of running a guard/mutation
sequence inside one transaction on this same single-connection store via
`CommitTransitionGuarded`, `internal/state/store.go:172-`).

**Required invariant:**

```
domain mutation (invocation_runtime_bindings, or the storage_schema
rename) commits
    if and only if
the corresponding lifecycle outcome journal event commits
```

**Required new capability (design-level, not prescribing an exact API):** a
tx-scoped lifecycle-journal-event-insert operation, usable from inside a
caller-supplied `*sql.Tx`, structured analogously to the internal
event-insert logic `SQLiteEventStore.Append` already contains but without
that function's own `BeginTx`/`Commit`. A `storage_schema`/`runtime_state`
driver's `Apply` then performs, inside one `*sql.Tx`:

```
begin transaction
  ->
(§13.5) revalidate current repair authority, in-transaction
  ->
revalidate predicates 2-5 (§13.4), in-transaction
  ->
perform the domain mutation (schema rename, or bindings write)
  ->
insert the lifecycle outcome journal event (tx-scoped, via the new
  capability above) — sequence/version-checked exactly as
  Journal.Append checks today, just without its own separate transaction
  ->
commit once
```

If any step fails, the whole transaction rolls back: no partial rename, no
partial bindings write, and no journal entry claiming an outcome that did
not durably happen. This is the exact mechanism that makes §13.2's resume
logic sound — there is no longer a durability gap between "the mutation
happened" and "the journal says so" for either transition.

**Implementation-local hazard the future PLAN/code must not reintroduce:**
any sequence/history read the tx-scoped insert needs (the equivalent of
`Journal.Load`'s query, used to compute the next `Sequence`) MUST be
performed against the *same* `*sql.Tx` this transaction already holds, not
via a second call into `Store`/`*sql.DB` outside that transaction. This
store's connection pool is documented as capped at exactly one connection
(`internal/state/store.go:150-152`'s own comment on `CommitTransitionGuarded`),
so any nested query against the pool while this transaction's connection is
checked out would deadlock, not merely race. This is an implementation
constraint on how the new capability in this section must be built, not a
change to the invariant itself.

### 13.4 Predicates revalidated inside the mutation transaction

Predicates 2-5 from this ADR's prior draft remain correct in substance and
now execute inside the single transaction of §13.3, not merely "as close
as possible" to it:

1. target pre-state/schema digest (the live `invocation_runtime_bindings`
   contents, or live schema shape for `storage_schema`, match what was
   observed at `prepared`);
2. expected population/membership digest (for `runtime_state`: a digest
   over the exact `invocation_registry` active-row key set observed at
   `prepared`; for `storage_schema`: a digest over the complete retained
   row-key population, since §11 makes it unconditional);
3. authoritative reconstruction evidence digest(s) (one per binding,
   covering §7's `contract_json` bytes or §8's re-verified package/manifest
   chain — `storage_schema` has no analogous per-row evidence digest,
   since it performs no semantic derivation);
4. proposed complete result digest (what will actually be written matches
   what was approved at `prepared`).

Each predicate's revalidation reads through the same `*sql.Tx` the mutation
itself uses, so no other writer can commit a conflicting change to the
schema, to `invocation_registry`, or to the provenance tables between this
check and the mutation's own commit — SQLite's single-writer lock,
acquired for the whole transaction, makes this true by construction, the
same guarantee `CommitTransitionGuarded`'s existing doc comment already
relies on.

### 13.5 In-transaction authority revalidation — TOCTOU gap withdrawn

**This ADR's prior text accepted a documented authority-check TOCTOU
window on the grounds that `secure_blobs` reads have no `*sql.Tx`
parameter. That acceptance is withdrawn.** `secure_blobs` (`internal/state/secure_blob.go`)
lives in the same authoritative SQLite database as the lifecycle journal
and `invocation_runtime_bindings` — the same single-writer-lock argument
already used to justify §13.4's predicate revalidation applies equally to
an authority read. `PutSecureBlobWithLock` (`internal/state/secure_blob.go:118-`)
already demonstrates the precedent pattern of taking a lock inside a
`*sql.Tx` against `secure_blobs` for exactly this kind of coordination.
This ADR does not claim any existing function already accepts an external
`*sql.Tx` for a `secure_blobs` read (none does today; this is a required
new capability, like §13.3's), but the underlying storage fact — one
SQLite file, one writer lock, reachable through one connection pool
already capped at one connection (`internal/state/store.go:150-152`'s own
documented invariant) — makes a tx-scoped read possible in principle, not
merely convenient. Asserting it cannot be done, as this ADR previously did,
was not supported by primary evidence.

**Required ordering, inside the single §13.3 transaction:**

```
begin governed mutation transaction
  ->
revalidate current repair authority (decision not expired/revoked/
  superseded), via a tx-scoped secure_blobs read against the same *sql.Tx
  ->
revalidate predicates 2-4 (§13.4)
  ->
mutate (schema rename, or bindings write)
  ->
insert the lifecycle outcome journal event (§13.3)
  ->
commit once
```

Any failed predicate — including authority failing revalidation — aborts
the whole transaction via rollback; no command, event, or mutation is
admitted. This closes the class of defect this session already found and
fixed once elsewhere (check-authority → gap → mutate) rather than
reintroducing it here with a documented excuse.

## 14. Journal evidentiary integrity under shared-transaction commit

Making the lifecycle journal event and the domain mutation share one
transaction (§13.3-§13.5) must not weaken the journal's role as an
append-only evidentiary record. The following properties are required and
must all hold simultaneously:

- **Append-only, unconditionally:** no code path introduced by this ADR
  may `UPDATE` or `DELETE` an existing `lifecycle_transition` journal
  event row. The shared transaction only ever adds one new event; it never
  rewrites history, exactly as `internal/eventstore`'s `events` table
  already has no update/delete path today.
- **Sequence/version checked exactly as today:** the tx-scoped insert
  capability (§13.3) must perform the same `expectedVersion+1`
  aggregate-advance check `SQLiteEventStore.Append` performs today
  (`compareAndAdvanceAggregate`, `internal/state/eventstore.go:41`) — this
  ADR does not relax that check to make the shared transaction easier to
  write.
- **Transition legality validated before commit, not after:** the outcome
  state being inserted must pass `LifecycleTransitionJournal.Validate()`
  (including `validLifecycleTransition`) before the transaction is allowed
  to reach its mutation step, not merely before the transaction commits —
  an invalid transition must never cause a partially-executed mutation to
  be attempted and then rolled back for an unrelated reason.
- **Exact plan/step identity bound:** the inserted event's `PlanID`,
  `PlanDigest`, and `StepID` must be the exact values validated in §13.1,
  not re-derived independently inside the transaction from a possibly
  different in-memory `LifecyclePlan` value.
- **No mutation without a valid paired event, and no claimed-committed
  event without a durable mutation:** because both inserts share one
  transaction and one commit (§13.3), these two properties are the same
  fact, not two separate guarantees to maintain by discipline — this is
  the entire point of §13.3's redesign, restated here explicitly for the
  independent reviewer to check directly against whatever implementation
  is eventually proposed.

**The mandatory independent-model reviewer for this work (§18, issue #136)
must specifically evaluate this section** — that the shared-transaction
design in §13 does not, in practice or in a proposed implementation,
create any path by which the journal could report a state that the
database does not durably reflect, or vice versa.

## 15. Recovery states — legal transitions only, mapped from the real state machine, crash table rebuilt around §13.3's atomic boundary

Using `pkg/contracts/lifecycle.go`'s actual `validLifecycleTransition`
table (`:311-326`), not ADR-087's invented/illegal one, and rebuilt around
the atomic `Apply` transaction §13.3 establishes (not the previously
assumed two-separate-operations model):

| Condition | Legal state(s) | Notes |
|---|---|---|
| Deterministic precondition rejection (missing evidence, membership row lacks a legitimate derivation, `storage_schema` not yet `committed` per §3, `PlanDigest` mismatch per §13.1), discovered at `Preflight`, before any journal entry for this attempt exists | **no journal entry is written at all** for this rejected attempt | `RunStep` calls `driver.Preflight` before its first `appendState(LifecyclePlanned)` (`internal/lifecycle/migration.go:202-`); a `Preflight` rejection returns before any `Planned` entry is appended, so there is no "step remains at `planned`" state to point to unless a `Planned` entry already exists from a genuinely separate, prior attempt. Never conflate "no entry was written" with "the step is durably `planned`." |
| Stale proposal (bound digests no longer match at the `prepared`→`applying` boundary) | `applying` → `failed_recoverable` | Only reachable from `applying` per the real state machine; the transition must actually have entered `applying` for this outcome to be legal. |
| Membership drift, or (for `storage_schema`) retained-population drift, between `prepared` and `applying` | `applying` → `failed_recoverable` | Predicate 2 (§13.4) catch, now checked inside the §13.3 transaction rather than best-effort beforehand. |
| Verification failure (plugin binding fails current-trust check, §9) | `applying` → `failed_recoverable`, or `applying` → `reconcile_required` if the failure mode itself is ambiguous (e.g. trust-root infrastructure unreachable rather than a definitive revocation) — see §9's routing table for the complete legal mapping | `reconcile_required` entries MUST carry a non-empty `RecoveryAction` (`LifecycleTransitionJournal.Validate()`, `pkg/contracts/lifecycle.go:388-390`) — this ADR requires that field be populated with a concrete, specific description, not a placeholder. |
| Crash before the §13.3 transaction begins | remains `prepared` or `applying` (from a prior attempt) with no committed table change and no new journal entry | Prior committed state is authoritative; nothing has been attempted yet for this specific attempt. |
| Crash during the §13.3 transaction (any point before its single commit) | remains at whatever state preceded this attempt (`prepared`, or `applying` if this is a resume) — no mutation, no new journal entry | SQLite's transactional rollback-on-abort guarantees neither the mutation nor the journal event is durable; §13.2 governs how the next resume attempt proceeds, and it is now unambiguous (no partial state to interpret). |
| §13.3 transaction commits | `applying` → one of `committed` / `failed_recoverable` / `reconcile_required` / `rolled_back`, chosen by the driver's actual result | Both the mutation and its paired journal event are durable together, by construction (§13.3, §14). |
| Crash after the §13.3 transaction's commit, before any subsequent externally-observed verification | the just-committed state stands; a later verification step is a **separate, later fact**, not a reinterpretation of this commit | The table mutation and its outcome event are already durably paired; whether the intended purpose (dynamic invocation resolution restored) is externally confirmed is separate, later work, exactly as before. |
| Genuinely ambiguous post-commit state (a later verification step cannot confirm the intended purpose — e.g. dynamic invocation resolution — was actually achieved) | **`committed` itself is never transitioned out of — it is terminal, with no legal successor of any kind (`validLifecycleTransition`, `pkg/contracts/lifecycle.go:311-326`, has no entry for `LifecycleCommitted`).** The ambiguity is instead addressed by proposing a **new, separate `LifecycleTransitionStep`** (its own `StepID`, in a new or amended plan) whose journal history starts fresh at `PreviousState == ""` → `State = LifecyclePlanned` (`LifecycleTransitionJournal.Validate()` requires exactly this for any entry with no `PreviousState`, `pkg/contracts/lifecycle.go:381-384`) and proceeds through the ordinary `planned → approved → prepared → applying → {outcome}` sequence to address the ambiguity as its own governed transition. | The journal is append-only (§14); the original `committed` entry is never rewritten, reinterpreted, or given a successor state. A later-discovered ambiguity about a `committed` transition's real-world effect is new information requiring a new governed transition, not a retroactive edit or extension of the old one. |
| Reconciliation (of a step that is itself already at `reconcile_required`, e.g. from the two rows above) | `reconcile_required` → `committed` \| `rolled_back` \| `fenced` | Exactly the three legal successors; this ADR does not design what drives that choice beyond noting it is governed, later work (issue #140-adjacent territory for the trust-verification case specifically), consistent with §9's scope limit. This row never applies to an already-`committed` step — see the row above. |

No row above assigns a transition the real state machine rejects. Every
`reconcile_required` row requires `RecoveryAction`. `committed` remains
terminal; there is no `committed -> reconcile_required` row above, and none
is legal — the "genuinely ambiguous post-commit state" row above explicitly
does not transition the committed step at all.

## 16. No partial repair (restated as a lifecycle-level invariant, not a repeated slogan)

`storage_schema` is atomic by construction (§2, §11: uniform operation,
unconditional key set, one commit per §13.3). `runtime_state`'s scoped
transition is atomic at the semantic level enforced by §11 and §13:
complete exact reconstructed active-membership registry, or no successful
repair. This follows directly from the complete-key-set invariant and the
atomic-transaction discipline of §13 — it is not a separate mechanism, it
is the composition of §11 + §13.

## 17. ADR-087 and PLAN-007 disposition

- **ADR-087** (`docs/ADR/087-installation-repair-authority-and-evidence-trust.md`)
  remains proposed and not accepted (status line: "Proposed —
  architecture-owner decision required", `:3`), unmodified, preserved as historical
  design evidence documenting the rejected separate-mechanism path
  (bespoke `installation-repair` aggregate, single "fail closed" outcome
  bucket, blanket plugin-style re-verification requirement for all binding
  classes, active-set-only population model, linear membership/binding
  trust hierarchy) and the reasoning that led two independent reviews to
  reject it. It is not rewritten to make this successor's direction appear
  obvious in hindsight.
- **PLAN-007** (`docs/PLAN/007-derived-runtime-registry-repair.md`) remains
  `PROPOSED — NOT ACCEPTED` and is **not revision-ready**. Its structural
  assumptions (single collapsed transition rather than the two-transition
  `storage_schema`/`runtime_state` ordering established here,
  three-table-join sufficiency, no-new-authority-primitive,
  non-delegability-by-one-validator, `CommitTransitionGuarded` literal
  reuse, single fail-closed outcome, `invocation_registry`-demoted trust
  hierarchy, active-set-only population semantics, and — per this ADR —
  its entire aggregate/journal framing) were invalidated deeply enough
  across two independent reviews that it must be **replaced**, not
  patched, once this ADR (or a corrected successor) is accepted. That
  replacement is separate future work, not authorized here.

## 18. Qualification implications for the eventual replacement PLAN

The eventual replacement PLAN must prove production-path behavior for, at
minimum:

- the historical `handler_*` schema, reproduced from primary source
  (`0084352`'s body of `migrations/sqlite/0012...sql`), as a real
  precondition-testing fixture, not merely asserted;
- `storage_schema` structural correction via schema introspection
  (`PRAGMA table_info`), never trusting `schema_meta.value` alone (§2);
- proof that every retained row's identity is preserved exactly through
  the rename, active and inactive alike;
- `runtime_state` non-plugin reconstruction (§7) against real
  `invocation_registry.contract_json` fixtures, including proof that
  decoding stored `contract_json` bytes back into the current
  `InvocationContract` shape and reading `PackageID`/`EntryPointID`/
  `Version` from that decode reproduces exactly what production wrote —
  i.e. that decode fidelity holds even if the contract's struct definition
  has itself evolved since the row was written (§7's decode-then-read
  requirement, and its required row-vs-decode cross-check, both depend on
  this);
- inactive historical generations preserved and unaltered by both
  transitions (§6, §11);
- rollback/reactivation still functioning correctly after repair;
- a plugin-backed row within `runtime_state`'s v1 validation scope causing
  the whole `runtime_state` transition to refuse completion, citing #140,
  with no partial registry result (§9, §11);
- missing/unresolvable activation-install provenance handled as a
  precondition failure, never a default classification (§6, §8);
- corrupted or missing binding-classification evidence handled per §8's
  source-side-only rule;
- stale `PlanDigest` on restart handled per §13.1, with no silent resume;
- restart from `Applying` handled per §13.2, with no illegal transition and
  no indefinite stuck state;
- authority expiry/revocation discovered exactly at the in-transaction
  revalidation point (§13.5), not merely at plan-approval time;
- any single failed in-transaction predicate (§13.4/§13.5) causing full
  transaction rollback, verified by direct inspection (no partial schema
  change, no partial bindings write, no orphaned journal event);
- crash injected at each boundary in §15's table, verified to land in the
  stated legal state and no other;
- restart and a fresh process after a fully committed transition, showing
  correct behavior with no re-attempt of `Apply`;
- journal append-only and sequence integrity maintained under the new
  tx-scoped insert path (§14), including under injected concurrent-writer
  contention;
- `runtime_state`'s precondition binding to `storage_schema`'s exact
  qualified output digest (§3), verified to actually block `runtime_state`
  from running against an unqualified or superseded `storage_schema`
  result;
- dynamic Goal invocation succeeding end-to-end against the repaired
  registry, in a non-dogfood environment first;
- the mandatory independent-model review gate (issue #136) exercised
  before any transition governed by this ADR is proposed against the real
  dogfood installation, with the reviewer specifically evaluating §14's
  evidentiary-integrity properties as directed there.

This ADR does not implement #136; it only requires the future PLAN to
honor it as a gate, consistent with already-accepted architecture's
general preference for independent verification of consequential
transitions demonstrated repeatedly this session.

## 19. Self-review — invalidated-design remnants checked and confirmed absent

This ADR was re-read in full after this revision specifically to search for
remnants of previously invalidated designs. Confirmed absent:

- **Active-set equality as the definition of legitimate existence:**
  withdrawn in §6/§11; `invocation_runtime_bindings` is now stated as
  retained history, with active-set equality scoped explicitly and only to
  `runtime_state`'s v1 semantic-validation boundary, never to what rows
  are allowed to exist.
- **Linear trust hierarchy:** §6 keeps membership, activation, and binding
  identity as three explicit, separately-sourced axes; §8's classification
  rule is source-side only, never target-field-derived.
- **Plugin recovery claimed complete in v1:** §9 explicitly states plugin
  bindings are out of scope for a completable v1 `runtime_state` repair and
  cites #140 without attempting to solve it.
- **Globally closed `RequestedAuthority`:** §10 retains the corrected,
  narrower closure claim (delegation-containment validators only), not a
  global enum claim.
- **Separate installation-repair aggregate:** §1 and §17 both explicitly
  reject this; both transitions use the existing `lifecycle_transition`
  aggregate exclusively.
- **`CommitTransitionGuarded` literal reuse:** §13.3/§13.5 cite it only as
  precedent for the tx-scoped-guard *pattern* and explicitly state no
  existing function is claimed to already support this use, and that new
  capability is required.
- **Authority TOCTOU accepted as a permanent limitation:** withdrawn in
  §13.5; the required ordering places authority revalidation inside the
  same transaction as the mutation and journal insert.
- **`committed -> reconcile_required`:** does not appear in §15's table;
  the "genuinely ambiguous post-commit state" row explicitly transitions
  nothing out of `committed` and instead requires a new, separate
  `LifecycleTransitionStep` starting fresh at `planned`; `committed` is
  stated as terminal in §15's closing sentence.
- **`prepared -> failed_recoverable` / any post-`applying` reconciliation
  implying a `committed` exit:** §9's failure-routing table states only
  the two legal `applying`-sourced edges and the "never advances past
  `planned`" non-edge for a pre-`applying` rejection; no other routing is
  stated anywhere in this ADR.
- **`applying -> prepared`:** does not appear anywhere in this revision;
  §13.2 explicitly states it is not invented and not required once §13.3's
  atomicity removes the ambiguity that transition would otherwise paper
  over.
- **Lifecycle described as already production-wired:** §5 is new in this
  revision and explicitly states zero production importers exist today,
  making both transitions the architecture's first production integration.
- **Nonexistent journal fields used as load-bearing mechanism:** §3 no
  longer cites `ReadinessDigest`/`ResultingManifestDigest` as if either
  recorded a step's `Target.Digest`; the sequencing binding is now derived
  from `PlanDigest` equality plus the `storage_schema` step's own
  `committed` state, both fields that genuinely exist on
  `LifecycleTransitionJournal`.
- **Structural divergence understated as "column rename only":** §2 now
  states the FK `ON DELETE CASCADE` divergence alongside the three-column
  rename, and requires live introspection (`PRAGMA table_info` +
  `PRAGMA foreign_key_list`) of the complete table definition, not just
  the three known columns.
- **Non-plugin `runtime_id` read from the target row's own `package_id`
  column:** §7 now requires decoding `contract_json` and reading the
  decoded contract's own `PackageID`/`EntryPointID`, with the row's own
  `package_id` column usable only as a cross-check, never as the
  derivation source.

This is the second self-review pass on this document, performed after a
mandatory independent review (a different-model reviewer) returned five
blocking findings (B1-B5, corresponding exactly to the five bullets above
introduced or revised in this pass) and five non-blocking implementation
findings (citation-line corrections applied throughout §7/§13.1/§13.2; a
tx-scoped-journal-read deadlock hazard now noted in §13.3; the §15
Preflight-rejection row corrected to state that no journal entry is
written, rather than that the step "remains at `planned`"; a
`contract_json` decode-fidelity qualification item added to §18; and an
ADR-087 status-line quotation corrected to its literal text). No further
contradiction was found in this pass beyond the specific findings that
prompted it.

## Consequences

If accepted, this ADR authorizes exactly: treating
`invocation_runtime_bindings` recovery as **two ordered** lifecycle
transitions under the existing `lifecycle_transition` journal —
`storage_schema` (§2, complete live-schema-to-target-schema structural
correction — column rename and FK delete-action correction alike — over
the complete retained population, no semantic claim) followed by
`runtime_state` (§7/§8,
semantic validation scoped to the active-membership set, §11) — with the
retained-generation population model (§6), the two binding-class
reconstruction chains (§7/§8), the narrow trust-semantics scope (§9), the
corrected authority invariant (§10), the complete-key-set/no-partial-repair
requirement (§11/§16), reserved control-plane reachability (§12), the
atomic journal/mutation transaction requirement replacing the prior
accepted TOCTOU/durability gaps (§13), the journal evidentiary-integrity
requirements (§14), and the legal-only recovery taxonomy (§15). It does not
authorize anything listed as excluded in §4, does not accept ADR-087, does
not make PLAN-007 revision-ready, and does not authorize implementation,
dogfood mutation, or weather-app work.

This ADR is **accepted**, as of the status line above, at exactly the
scope stated in this document. Independent review (§18, issue #136) has
already occurred for this ADR itself; it does not substitute for the
independent-model review §18 requires of the eventual replacement PLAN
before any transition governed by this ADR is proposed against the real
dogfood installation — that gate remains a separate, future requirement,
not satisfied by this acceptance.

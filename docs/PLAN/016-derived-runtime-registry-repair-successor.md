# PLAN-016: Derived Runtime-Registry Repair — `storage_schema`/`runtime_state` Lifecycle Implementation (Successor to PLAN-007)

- Status: **Accepted** — Thomas, architecture owner, accepted this PLAN on
  2026-09-17, following the required independent review (§4): an initial
  full review (6 blocking findings, B1-B6), a correction pass, a focused
  re-verification (5 of 6 verified, B2 not verified as specified), a
  second correction pass for B2, and a final focused verification
  confirming all six blockers resolved with 0 remaining blockers and 0
  architecture conflicts. This acceptance covers exactly the scope stated
  herein — WU1-WU10, the crash/restart suite (§2), the non-production-code
  deliverables (§3), the qualification matrix (§5), and the three
  human-authority STOP points (§6) — and authorizes proceeding with
  implementation of WU1-WU10. It does **not** authorize §8's dogfood
  ceremony, which remains separately gated by §6's STOP points 2 and 3.
  On 2026-09-17 the architecture owner additionally accepted the narrow WU4
  persistence-boundary correction recorded below: the prior exported thin
  sealed-record wrapper contradicted ADR-088 §10 because it remained directly
  reachable without semantic admission. The sole authority-generation write
  boundary now accepts the typed generation, validates that exact value, and
  internally serializes, seals, and persists it. No other authority semantics
  or ADR-088 scope changed.
- Date: 2026-09-17
- Governing ADR: `docs/ADR/088-runtime-state-recovery-lifecycle-successor.md`
  — **Accepted**, 2026-09-17 (see that document's status line for the exact
  scope accepted)
- Operative lineage this PLAN reconciles against: `docs/ADR/076-installation-lifecycle-contracts-successor.md`
  (Accepted successor), `docs/SPEC/039-installation-lifecycle-contracts-successor.md`,
  `docs/PLAN/006-installation-lifecycle-contracts-successor.md` (Accepted
  successor)
- Historical predecessor: `docs/PLAN/007-derived-runtime-registry-repair.md`
  — remains `PROPOSED — NOT ACCEPTED`, **unmodified**, preserved as
  historical evidence of a structurally invalidated design (single
  collapsed transition, three-table-join sufficiency claim,
  no-new-authority-primitive assumption, `CommitTransitionGuarded`
  literal-reuse claim, single fail-closed outcome bucket,
  `invocation_registry`-demoted trust hierarchy, active-set-only
  population semantics, and an aggregate/journal framing ADR-088 rejected).
  **This PLAN does not evolve from PLAN-007 and reuses none of its
  qualification.** It is a fresh document under the next available PLAN
  number, per this repository's convention of leaving a superseded
  proposal's own file and status line untouched while a successor document
  declares the supersession (the same pattern `docs/ADR/076-...md` uses
  toward `docs/ADR/075-installation-lifecycle-contracts-and-reconciliation.md`).
- Related: issue #136 (mandatory independent-model review at consequential
  graph boundaries), #138 (migration identity/content drift), #139
  (installation-repair primitive gap), #140 (package trust-root
  provenance — referenced, not solved, here)
- **Revision note:** this text incorporates six corrections (B1-B6) from an
  independent PLAN review, plus applicable non-blocking findings from the
  same review. Architecture changes introduced by this revision: **0** —
  every correction below is an implementation-plan fix within ADR-088's
  already-accepted scope; none reopens or extends ADR-088 itself.

## 0. Scope, exactly as ADR-088 authorizes and no further

**In scope**, matching ADR-088 §1-§4 exactly:

- Production wiring of `internal/lifecycle` (currently zero importers) into
  one reserved control-plane CLI path.
- The `Applying`-resume defect fix and restart-identity (`PlanDigest`)
  binding (ADR-088 §13.1-§13.2), together with the `RunStep`
  outcome-ownership refactor required to make the atomic Apply boundary
  below actually composable with the existing executor (§1 WU3).
- A tx-scoped lifecycle-journal-event-insert primitive and the atomic
  Apply-transaction redesign (ADR-088 §13.3-§13.5).
- The `storage_schema` transition for `invocation_runtime_bindings`
  (ADR-088 §2), including a defined, non-ambiguous already-canonical-schema
  case.
- The `runtime_state` transition, semantic-validation scope limited to the
  active-membership set, non-plugin/client-adapter reconstruction only
  (ADR-088 §6-§8, §11).
- The narrow root-owner-only authority invariant for both transitions,
  across the complete set of `AuthorityGeneration`-persistence surfaces
  ADR-088 §10 identifies as relevant (ADR-088 §10).
- A dedicated end-to-end qualification work unit (§1 WU10) proving the
  complete governed recovery path in a non-dogfood environment.

**Out of scope, explicitly, matching ADR-088 §4's exclusions:** general
Step 5/6 lifecycle implementation; plugin-binding recovery (blocked on
#140); generic schema/database repair beyond this one table; #138's
broader migration-identity-tracking fix; #140 itself; any new
authority-model redesign beyond the containment work ADR-088 §10 requires;
execution of the real dogfood recovery (§8 below defines that as a
separate, later ceremony this PLAN does not authorize); weather-app work.

This PLAN's job is to make implementation of the above **not require
inventing architecture** — every work unit below cites the exact governing
ADR-088 section and the exact files/packages it touches.

## 1. Work units

Work units are ordered by dependency. Each is independently committable
and independently reviewable. None combines unrelated concerns beyond
what B1/B2 below required folding together (each folded pair is one
tightly-coupled mechanism, not two unrelated concerns). Every work unit
that touches the authoritative dogfood database is explicitly marked
**NO DOGFOOD** below — none of WU1-WU10 touch it; only the ceremony in §8
(not authorized by this PLAN) does.

---

### WU1 — Reserved control-plane CLI entry point (ADR-088 §12)

**Depends on:** nothing (can land first, wires to a stub).

**Files:** new `cmd/praxis/lifecycle_recovery.go`, following the existing
`cmd/praxis/control.go`/`cmd/praxis/migration.go` pattern
(`runControlCommand`/`runMigrationCommand`); one new `case` arm added to
the `switch args[0]` in `cmd/praxis/main.go:run` (switch spans `:39-62`),
placed among the existing reserved cases (`"migration"`, `"resume",
"cancel"`, `"authority"`, `"supervise"`) — **before** the fallthrough to
`runDynamicInvocation` at `main.go:63`, never after.

**Acceptance criteria:**
- A new subcommand (e.g. `praxis lifecycle-recover`) is matched by an
  explicit `case` in `run`'s `switch`, dispatching to
  `runLifecycleRecoveryCommand(args[1:])` in the new file.
- `runLifecycleRecoveryCommand` imports `internal/lifecycle` and
  constructs its `Journal`/`RunStep` call graph directly — it has no
  dependency on `resolveDynamicInvocation`, `client.DispatchInvocation`,
  or any `invocation_runtime_bindings` read, by construction (it is wired
  before, not through, dynamic resolution).
- `cli_help.go`'s help-text registry is updated with this command's usage
  line, consistent with existing entries (matches the repo's existing
  convention, e.g. the `"install"` entry in `cmd/praxis/cli_help.go`).

**Production-path test:** a test asserting the new case is reached before
`runDynamicInvocation` for the chosen subcommand name, and a second test
asserting the command still resolves correctly even when
`invocation_runtime_bindings`/`invocation_registry` are absent or
malformed in the test database (proving the independence claim, not just
asserting it in prose).

**Authority boundary:** none yet — this work unit only wires a CLI path to
a not-yet-implemented handler; it performs no lifecycle transition and
touches no state.

**Blocking dependency for later work, not a human STOP:** none — this is
pure wiring, safe to land without further human sign-off beyond ordinary
code review.

---

### WU2 — `Applying`-resume fix and restart-identity (`PlanDigest`) binding (ADR-088 §13.1-§13.2)

**Depends on:** nothing (independent of WU1; can land in parallel).

**Files:** `internal/lifecycle/migration.go` — `RunStep` (the guard block
currently at `:252-259`, and the resume path feeding into it via
`lastStepState`, `:303-311`).

**Acceptance criteria:**
- Before treating any non-empty `current` state from `lastStepState` as
  resumable, `RunStep` verifies the `PlanDigest` of the last journal entry
  for `(PlanID, StepID)` equals `req.Plan.Digest`. On mismatch: if
  discovered before `applying` was ever entered for this attempt, the step
  simply does not advance (no illegal state is entered, matching §2's
  crash-suite row for pre-`applying` rejection); if discovered after
  `applying` was already durably entered by a *prior* attempt, the step
  transitions `applying -> reconcile_required` with `RecoveryAction =
  "plan-digest-mismatch"`.
- Resuming with `current == LifecycleApplying` no longer falls into the
  unconditional `appendState(contracts.LifecycleApplying, "")` at the
  current `:257`. The fixed code path re-attempts `driver.Apply` (subject
  to the driver's own `Idempotent` contract — **this check is REQUIRED on
  the `Applying`-resume path explicitly, not merely "subject to" it as
  loose prose**; today `RunStep` only consults `Idempotent` on the
  `FailedRecoverable` branch, `:222-226`, so the resume path must add its
  own equivalent call) and transitions out of `Applying` through one of
  its four legal successors based on the fresh attempt's outcome — it
  never appends a second `Applying` entry.
- `lastStepState`'s signature or its caller is extended to also return the
  matched entry's `PlanDigest` (or the resume path loads it separately),
  whichever is the smaller change — this PLAN does not mandate which,
  only that the check happens before any resume decision.

**Production-path test:** (a) inject a crash simulation that leaves the
journal at `Applying` with no further entries, resume `RunStep`, assert it
re-attempts `Apply` (after consulting `Idempotent`) and reaches a legal
terminal/interim state, never a second `Applying` entry and never an error
from `Validate()`; (b) construct two `LifecyclePlan` values with the same
`PlanID`/`StepID` but different `Digest` (a "modified plan"), advance the
first to a non-terminal state, then resume with the second, and assert the
mismatch is caught and routed legally, never silently resumed.

**Authority boundary:** none — this is pure state-machine logic, no new
authority surface.

**Blocking dependency for later work, not a human STOP:** WU5/WU7's
drivers may not be qualified against `RunStep` until this work unit's
tests pass — this is an ordinary implementation dependency, not one of
the three human-authority STOP points defined in §6.

---

### WU3 — Tx-scoped lifecycle-journal-event-insert primitive AND `RunStep` outcome-ownership refactor (ADR-088 §13.3-§13.5, §14)

**Corrected scope (was previously two loosely-related concerns implicitly
split across this work unit and `RunStep`'s untouched outcome-append
logic; they are one mechanism and are now one work unit, because ADR-088's
atomic Apply boundary is not achievable unless both change together).**
Primary evidence of why this must be one unit: today, after
`driver.Apply` returns, `RunStep` itself computes `sequence :=
len(history) + 1` (`internal/lifecycle/migration.go:227`, captured
*before* calling the driver) and then calls `appendState(Committed |
FailedRecoverable | ReconcileRequired | RolledBack, ...)`
(`:260-284`), which in turn calls `journal.Append` with that
already-computed `sequence`/expected-version. If a driver's `Apply` (WU5,
WU7) also durably appends its own outcome event inside its own shared
transaction, per ADR-088 §13.3, then `RunStep`'s subsequent
`appendState` call would attempt to append a **second** outcome entry
with a now-stale sequence number (`entry.Sequence != len(history)+1` at
`Journal.Append:80-82`, or the tx-scoped equivalent's identical check)
and fail — or, if not caught, would attempt an outright illegal
transition: `RunStep:269-271`'s `result.ResultingManifestDigest !=
step.Target.Digest` check, if it ever ran *after* a driver had already
committed a `committed` outcome in its own transaction, would try to
append `reconcile_required` from `committed` — not a legal edge under
`validLifecycleTransition` (`pkg/contracts/lifecycle.go:311-326`;
`Committed` has no successors). **This work unit exists specifically to
prevent that outcome.**

**Depends on:** WU2 landed (the atomic Apply transaction this enables is
meaningless without a correct resume path underneath it).

**Files:** `internal/state/eventstore.go` (new function alongside
`SQLiteEventStore.Append`, e.g. `AppendInTx` or equivalent — exact name is
implementation's choice, not architecture; **must also preserve the
`ensureEventCommand` side effect `SQLiteEventStore.Append` performs per
event today, so the tx-scoped insert does not silently drop command
provenance**); `internal/lifecycle/migration.go` (`Journal` gains a method
that accepts an externally-supplied `*sql.Tx` instead of opening its own;
**and `RunStep` itself is restructured**, per the ownership split below).

**Required ownership split (this is the core of this work unit, not an
optional detail):**

```
RunStep / lifecycle executor
    owns: loading history, computing the current legal state, deciding
    which LEGAL transition to attempt (planned/approved/prepared/applying
    progression, and — after Apply returns control — routing to whichever
    of the four Applying successors the result implies), and appending
    every journal entry that is NOT the outcome of a driver.Apply call
    (i.e. Planned/Approved/Prepared/Applying entries remain RunStep's
    sole responsibility, appended via the existing non-tx Journal.Append,
    exactly as today).

driver.Apply (WU5's storage_schema driver, WU7's runtime_state driver)
    owns: the ENTIRE atomic transaction for its own outcome — beginning
    the *sql.Tx, running WU4's authority guard, revalidating predicates,
    performing the domain mutation, computing the ACTUAL observed result
    (never merely echoing the intended target), and appending the OUTCOME
    journal entry (Committed / FailedRecoverable / ReconcileRequired /
    RolledBack) itself, via this work unit's tx-scoped primitive, inside
    that same transaction, before its own single commit.
```

**`RunStep` MUST NOT append a second outcome entry after a driver whose
`Apply` already committed one.** Concretely: `ApplyResult` (or its
replacement) gains a field the driver sets to indicate "I already
durably committed my own outcome entry inside my transaction" (e.g.
`OutcomeAlreadyRecorded bool`, exact name is implementation's choice).
When `RunStep` sees this set, it does **not** call `appendState` for the
outcome — it only updates its own in-memory understanding of `current`
(by reading back what the driver's transaction actually committed, or by
trusting the driver's returned outcome value, either is acceptable since
both are now the caller's next `Load()` away from being independently
re-verified) and returns. The pre-existing
`result.ResultingManifestDigest != step.Target.Digest` mismatch check
(`RunStep:269-271`) is **moved entirely inside the driver's own
transaction, before that transaction's commit** — the driver computes its
actual observed result, compares it to `step.Target.Digest` itself, and
chooses which outcome to append (`Committed` only if they match;
`ReconcileRequired` otherwise) **before** committing, never after. This
guarantees the mismatch case is caught pre-commit, so `committed` is never
durably recorded alongside an unqualified result, and `RunStep` never
needs to react to a mismatch after the fact by attempting an illegal
post-`committed` transition.

**Acceptance criteria (all required, per ADR-088 §14):**
- The new tx-scoped primitive performs the identical `expectedVersion+1`
  aggregate-advance check `compareAndAdvanceAggregate`
  (`internal/state/eventstore.go:41` today) already performs — not
  relaxed.
- It validates `LifecycleTransitionJournal.Validate()` (including
  `validLifecycleTransition`) **before** the caller's mutation step is
  reachable, not merely before the shared transaction commits — this
  constrains how WU4/WU5/WU7's `Apply` implementations must sequence their
  own calls, not just this primitive's internals.
- It performs no `UPDATE`/`DELETE` against the `events` table — append
  only, exactly as `SQLiteEventStore.Append` already guarantees today.
- It preserves the `ensureEventCommand` side effect (command-provenance
  row) per inserted event.
- It never opens or commits its own `*sql.Tx` — the caller (a driver's
  `Apply`) owns the transaction lifecycle entirely.
- Any history read this primitive needs (the equivalent of `Journal.Load`,
  to compute `Sequence`) is performed through the *same* caller-supplied
  `*sql.Tx`, never through `Store`/`*sql.DB` directly — this store's
  connection pool is capped at one connection
  (`internal/state/store.go:150-152`'s documented invariant), so a nested
  query outside the held transaction would deadlock, not merely race.
- `RunStep` is restructured per the ownership split above: it never
  appends a second outcome entry after a driver has already committed one
  inside its own transaction, and the `ResultingManifestDigest ==
  step.Target.Digest` check moves inside the driver's pre-commit path
  entirely, eliminating the post-`committed`-mismatch illegal-transition
  path that exists in the code today.

**Production-path test:** (a) unit test proving the primitive rejects an
out-of-sequence append inside a tx exactly as `Journal.Append` does
outside one; (b) unit test proving it rejects an entry that fails
`validLifecycleTransition` before any caller-visible mutation could have
been attempted; (c) a concurrency test that opens this transaction and
attempts a second, separate connection's write to the same aggregate,
proving SQLite's single-writer lock serializes them (this exercises the
same guarantee `CommitTransitionGuarded`'s existing doc comment,
`internal/state/store.go:149-160`, already relies on) with no deadlock
from the same-tx history read in the prior bullet; (d) **a test proving a
driver whose `Apply` durably commits its own `Committed` outcome causes
`RunStep` to append no further journal entry for that attempt** — asserted
by inspecting the journal's raw sequence count before/after, not by
inference from `RunStep`'s return value alone; (e) a test constructing a
result-mismatch case (driver's actual observed digest differs from
`step.Target.Digest`) and asserting the driver itself appends
`ReconcileRequired` pre-commit, with **no** `Committed` entry ever having
existed for that attempt at any point (not committed-then-corrected —
never committed at all).

**Authority boundary:** none directly in the primitive itself — WU4 builds
the authority check on top of it, as the first statement inside the
driver's transaction.

**Blocking dependency for later work, not a human STOP:** this is new
infrastructure directly implementing an ADR-088-required invariant
(§13.3), not itself introducing new architecture — no additional human
sign-off required beyond ordinary code review, per this PLAN's own
instruction not to require architecture sign-off for implementation of an
already-accepted invariant. WU5/WU7 cannot be qualified until this work
unit's tests pass.

---

### WU4 — In-transaction authority guard, across the complete relevant `AuthorityGeneration`-persistence surface (ADR-088 §13.5, §10)

**Corrected scope, twice now.** The prior version required a guard inside
`internal/state/secure_blob.go`'s four raw writers that could "refuse to
persist any `AuthorityGeneration` record whose content names the repair
authority as delegated to." **That is impossible as stated**, and this
correction replaces it with a mechanism that actually works against the
real call graph. Primary evidence, traced exhaustively (not sampled):
every caller that reaches those four raw writers seals its payload
**before** calling — e.g. `internal/goalstore/repository.go` (whose own
tests, `repository_test.go:153`, show the pattern directly:
`repo.Crypto.Seal(...)` runs, producing an `Envelope`, and only the sealed
`Envelope` is ever passed into `SecureBlobRecord`). `SecureBlobRecord`
itself (`internal/state/secure_blob.go:39-49`) carries only namespace/
object-identity/`ObjectDigest`/`Envelope` — no plaintext `AuthorityGeneration`
field exists on it to inspect. `state.Store` (`internal/state/store.go:20-25`)
holds only `*sql.DB`, no crypto/key material, so it structurally cannot
decrypt `Envelope` to read `AuthorityGeneration.Authorities` even if it
wanted to. A grep across the entire tree for both `PutSecureBlob*` calls
and the literal `"authority_generation"` namespace string confirms every
production write to that namespace already originates from exactly three
typed `internal/goalstore.Repository` methods — `SaveAuthorityGeneration`
(`repository.go:822`, root/non-delegated generations),
`SaveDelegatedAuthorityGeneration` (`:836`), and
`SaveAuthorityDecisionAndDelegatedAuthorityGeneration` (`:889`) — each of
which constructs the `contracts.AuthorityGeneration` value **in plaintext
Go structs**, before any `json.Marshal`/seal/persist call. No other
package writes to this namespace in production code today (only test
files construct raw writes directly, bypassing `Repository`, which is
exactly the bypass risk this correction must close for the future, not
just describe as absent today).

**Depends on:** WU3.

**Files:**
- `internal/state/secure_blob.go` — (a) a new tx-scoped **read** function,
  analogous in spirit to `PutSecureBlobWithLock` (`:118`), but a read
  against a caller-supplied `*sql.Tx` rather than a write with its own
  `BeginTx`, used to revalidate the repair authority decision is not
  expired/revoked/superseded (**unchanged from the prior draft** — this
  part of WU4 was never the defective part); (b) **a namespace-reserved
  bypass guard** — each of the four existing
  writers (`PutSecureBlob:92`, `PutSecureBlobWithLock:118`,
  `PutSecureBlobsWithLock:151`, `PutSecureBlobUnlessRevoked:187`) refuses
  every caller-supplied sealed record targeting `"authority_generation"`.
  The sole exported production persistence API for that namespace accepts a
  typed `contracts.AuthorityGeneration`, validates the semantic invariant on
  that exact value, canonicalizes/serializes it, derives its storage digest,
  seals those exact bytes, and invokes only private raw persistence
  primitives. It may support the existing locked and atomic
  decision+generation forms through typed options, but any ancillary sealed
  records must themselves be rejected if they target the reserved namespace.
  No exported API accepting a caller-constructed sealed authority-generation
  record remains.
- `internal/goalstore/repository.go` — the three typed entry points
  (`SaveAuthorityGeneration:822`, `SaveDelegatedAuthorityGeneration:836`,
  `SaveAuthorityDecisionAndDelegatedAuthorityGeneration:889`) switch from
  caller-side marshal/seal plus raw persistence to the sole typed state-store
  admission API. That API, not Repository call-graph convention, owns the
  unavoidable plaintext semantic check and rejects a generation naming either
  repair authority when **either** `ParentRef` or `DelegatedBy` is non-empty.
- `pkg/contracts/authority_model.go` — the delegation-containment rule
  ADR-088 §10 also names: rejecting the repair authority as any
  `Delegation` payload, following the same pattern already used for
  `package.publish`/`package.deploy`. This rule closes an already-closed
  allowlist layer (the three profile validators,
  `authority_model.go:78,119,165`, already reject any authority string not
  on their allowlist, so a repair authority is already rejected here by
  construction) — it is retained for defense-in-depth and explicit
  documentation, but it is **not** where ADR-088 §10(b)'s obligation is
  actually discharged; the two mechanisms above are.

**Acceptance criteria:**
- A tx-scoped `secure_blobs` read exists, taking a caller-supplied
  `*sql.Tx`, used to revalidate that the repair authority decision is not
  expired/revoked/superseded, executed as the **first** step inside the
  shared Apply transaction (WU3's transaction), before any predicate
  revalidation or domain mutation.
- The sole production persistence API accepts a typed
  `AuthorityGeneration`; semantic validation, serialization, digesting, and
  sealing occur within that boundary in that order on the exact same value.
  All three Repository flows use it.
- The namespace-reserved bypass guard (Files item, `secure_blob.go` (b)) is
  present in **all four** generic `secure_blobs` writer entry points and
  rejects every caller-constructed sealed write to the
  `authority_generation` namespace. No exported raw/dedicated sealed-record
  exception exists; the typed admission API is the only production route.
- The delegation-containment rule (`authority_model.go`) explicitly rejects
  the repair authority (`storage_schema`/`runtime_state` operation strings)
  from appearing in any `Delegation` payload.
- Existing valid `AuthorityGeneration` creation/persistence behavior for
  every authority kind other than the repair authority is unchanged. The
  generic typed path rejects every repair-bearing generation. ADR-089 / issue
  #142 supplies the sole legitimate admission path: immutable canonical-root
  succession with exact durable proposal, review, decision, predecessor
  supersession, and successor lineage. The generic authority model is not
  broadened and generic delegation remains unable to mint repair authority.
- No code path exists where the domain mutation (WU5/WU7) can be reached
  without the tx-scoped authority read having already run and passed,
  inside the same transaction — this is enforced by construction (the
  guard is the first statement inside the shared `*sql.Tx`, not a separate
  pre-check), closing the "check authority → gap → mutate" pattern
  ADR-088 §13.5 requires closed.
- Principal/parent requirements match ADR-088 §10's last paragraph: root
  installation-governance principal only, root generation as parent, for
  both `storage_schema` and `runtime_state` independently — the guard does
  not special-case `storage_schema` as "just a rename."

**Production-path test:** (a) revoke/expire the authority decision *after*
`prepared` but *before* the Apply transaction begins, assert the
transaction's guard step catches it and the whole transaction rolls back
(no partial mutation, no journal entry); (b) attempt to construct a
`Delegation` payload naming the repair authority as delegable, assert the
existing profile-allowlist rejection plus the new explicit rule both
reject it; (c) attempt the mutation with a non-root-principal-issued
authority decision, assert rejection; (d) **call
`Repository.SaveDelegatedAuthorityGeneration`/
`SaveAuthorityDecisionAndDelegatedAuthorityGeneration` (the real typed
  production entry points) with a delegation naming the repair authority**
  and assert rejection before persistence; separately prove both non-empty
  `ParentRef` and non-empty `DelegatedBy` with empty `ParentRef` are rejected;
  (e) call `Repository.SaveAuthorityGeneration` with an otherwise legitimate
  root-shaped installation-repair generation and assert it is rejected, then
  exercise ADR-089's governed succession production path and assert that only
  its atomic typed transition persists the exact repair-bearing successor;
  (f)
**attempt to call each of the four raw `secure_blobs` writer functions
directly** (`PutSecureBlob`, `PutSecureBlobWithLock`,
`PutSecureBlobsWithLock`, `PutSecureBlobUnlessRevoked`) with
`Namespace: authorityGenerationNamespace` and a pre-sealed envelope,
bypassing `Repository` entirely, and assert **each of the four** rejects
the write outright (the namespace-reserved bypass guard, not a semantic
check — this test asserts structural impossibility of bypass, not content
  inspection), and prove no exported authority-generation sealed-record
  writer exists; (g) call each of the four raw writer functions with an
**unrelated** namespace and assert they behave exactly as before,
  unaffected by this change; (h) assert the typed boundary's persisted
  plaintext bytes equal the canonical serialization of the exact validated
  input value.

**Authority boundary:** this work unit **is** the authority boundary for
both transitions — every downstream driver (WU5, WU7) depends on this
guard being called first, unconditionally, inside their shared
transaction.

**Blocking dependency for later work, not a human STOP:** none beyond
ordinary code review — this implements ADR-088 §10's already-decided
invariant across the surface ADR-088 itself names; it does not invent new
authority semantics. The narrow accepted correction places typed plaintext
admission plus canonical serialization/sealing inside `internal/state` for
this reserved namespace only; generic sealed-record writers remain
namespace-string-only and must reject that namespace outright.

---

### WU5 — `storage_schema` driver (ADR-088 §2)

**Depends on:** WU2, WU3, WU4.

**Files:** new `internal/lifecycle/storage_schema_binding.go` (or similar
— a new `StepDriver` implementation, per the existing `StepDriver`
interface, `internal/lifecycle/migration.go:129-134`), reading/writing
against `internal/state` (the same package that owns `package_registry.go`
and the SQLite connection pool).

**Acceptance criteria:**
- `Preflight` performs **complete live schema introspection** of
  `invocation_runtime_bindings` — `PRAGMA table_info`, `PRAGMA
  foreign_key_list`, and `sqlite_master`/`PRAGMA index_list` for the
  existing `idx_invocation_runtime_bindings_package` index — never reads
  `schema_meta.value` as evidence of actual shape. It compares the
  introspected shape against three recognized cases, not two:
  1. the `0084352` historical shape (`handler_id/handler_version/
     handler_digest`, FK with no `ON DELETE CASCADE`) — proceeds toward
     `Apply`'s rebuild path;
  2. the current `2860381`/canonical shape (`runtime_id/runtime_version/
     runtime_digest`, FK **with** `ON DELETE CASCADE`) — **proceeds
     toward `Apply`'s no-op path, defined precisely below (correcting the
     prior ambiguous either/or)**;
  3. any other introspected shape — `Preflight` refuses
     (`ErrDowngradeRefused` or an ordinary precondition error, per
     `StepDriver.Preflight`'s contract); an unrecognized shape is never
     silently coerced into either of the first two paths.
- **Already-canonical case, precisely defined (closes the prior
  ambiguity):** when live introspection matches case 2 above, `Apply`
  still executes — it does **not** cause a `Preflight` refusal, because a
  `Preflight` refusal writes no journal entry at all
  (`internal/lifecycle/migration.go:202-207`, before the first
  `appendState`), which would leave this step permanently at `planned` and
  make `runtime_state` (WU6) permanently unreachable, exactly the outcome
  this correction exists to prevent. Instead: `Apply` observes the
  already-canonical live shape (the same introspection `Preflight`
  performed), performs **zero domain mutation** (no `ALTER`/rebuild
  statements are issued — this is legitimately a no-op at the SQL level),
  computes `ApplyResult.ResultingManifestDigest` from that same live
  introspection (which by construction equals `step.Target.Digest`, since
  the live shape already matches the canonical target), and appends a
  `Committed` outcome via WU3's tx-scoped primitive inside the same
  transaction as any other `Apply` call — this is a legal `applying ->
  committed` transition under `validLifecycleTransition`
  (`pkg/contracts/lifecycle.go:316`) exactly like the rebuild path;
  nothing about the lifecycle contract itself needs to change to represent
  it, because "no rows changed" is a property of the domain mutation, not
  of the journal transition, which behaves identically either way.
- `Apply` performs the structural correction (case 1's rebuild) or the
  no-op (case 2, above) inside WU3/WU4's shared transaction: guard (WU4)
  → live re-introspection as the in-transaction predicate 1/2 revalidation
  (ADR-088 §13.4, using the *same* `*sql.Tx`) → the structural mutation
  (or no mutation, for the no-op case) → the driver's own pre-commit
  result-digest check against `step.Target.Digest` (per WU3's corrected
  ownership split) → the tx-scoped journal insert (WU3). For case 1, if
  SQLite cannot alter the FK's `ON DELETE` action in place (it cannot —
  confirmed limitation), `Apply` performs the standard SQLite table-rebuild
  sequence (create target-shape table, copy every row unmodified except
  the three renamed columns, drop old table, rename new table into place,
  recreate the index), followed by `PRAGMA foreign_key_check` before
  commit to positively confirm the rebuilt table's FK integrity — entirely
  inside the one shared transaction, so a crash mid-rebuild leaves the
  original table untouched (SQLite DDL is transactional). No table in this
  schema references `invocation_runtime_bindings` as a parent (verified:
  no other migration declares `REFERENCES invocation_runtime_bindings`),
  so the rebuild's drop/rename does not risk orphaning a dependent FK
  elsewhere — this is stated as a verified fact this driver's `Preflight`
  may rely on, not an assumption to re-derive at runtime.
- Every row is copied unconditionally in case 1 — active and inactive
  alike — with no `WHERE active = 1` filter anywhere in this driver. This
  is the concrete implementation of ADR-088 §11's "unconditional"
  requirement.
- `Apply`'s returned `ApplyResult.ResultingManifestDigest` is computed from
  the **post-mutation (or, for the no-op case, post-observation) live
  introspection performed inside this same transaction**, not merely
  echoed back from `step.Target.Digest` — this is what makes ADR-088 §3's
  "self-report is precondition-gated by live introspection" chain actually
  true in the implementation, not just true in the ADR's prose.
- `Reversible`/`Effect` on the `LifecycleTransitionStep` this driver is
  invoked with must match what the chosen rebuild strategy can actually
  reverse (ADR-088 §2's requirement) — this work unit's test suite must
  include a rollback-path test proving whichever value is declared.
- No row content interpretation, no semantic claim about `runtime_digest`
  correctness — this driver only ever reads/writes column shape and
  verbatim byte content (or, in the no-op case, reads and asserts shape
  without writing anything).

**Production-path test:** (a) start from a fixture database built from the
exact `0084352` migration body (reproduced verbatim, not paraphrased),
including populated `handler_*` rows both active and inactive; run
`Preflight`+`Apply`; assert the resulting live schema exactly matches
`PRAGMA table_info`/`PRAGMA foreign_key_list` introspection of a database
built from the current `2860381` migration body, every row's PK and byte
content survived unchanged except the three renamed columns, and
`PRAGMA foreign_key_check` reports clean; (b) inject a crash mid-rebuild
(abort the transaction before commit), assert the original table is
completely intact afterward; (c) run against a database already in the
*current* canonical shape, assert `Apply` performs the defined no-op path,
issues zero mutating SQL statements (asserted by inspecting the test
database's write log, not inferred), and still reaches `Committed` with a
`ResultingManifestDigest` equal to `step.Target.Digest`, and assert WU6's
`runtime_state` step correctly becomes reachable afterward (this last
assertion is what proves the ambiguity is actually resolved, not merely
declared resolved in prose).

**Authority boundary:** WU4's guard, invoked first inside this driver's
`Apply`.

**NO DOGFOOD** — all tests run against local fixture databases built from
the exact historical/current migration bodies, never the authoritative
installation.

**Blocking dependency for later work, not a human STOP:** none beyond
ordinary code review for the driver itself. **Executing this driver
against the real dogfood database is explicitly not authorized by this
PLAN** — see §8.

---

### WU6 — `storage_schema` → `runtime_state` sequencing binding (ADR-088 §3)

**Depends on:** WU5.

**Files:** wherever the `LifecyclePlan` for this recovery is constructed
(new, e.g. `internal/lifecycle/runtime_recovery_plan.go`) and
`runtime_state`'s driver `Preflight` (WU7).

**Acceptance criteria:**
- Both `storage_schema` and `runtime_state` are steps of **one**
  `LifecyclePlan` (not two independently-digested plans run in sequence) —
  this is a construction requirement on whatever builds the plan, not a
  new contract field.
- `runtime_state`'s `Preflight` loads the `storage_schema` step's journal
  history for the same `PlanID`, verifies `PlanDigest == req.Plan.Digest`
  (reusing WU2's equality check, not a separate implementation) and
  `State == LifecycleCommitted` for that step's last entry, and refuses to
  proceed otherwise — never entering `applying`. This is satisfied
  identically whether `storage_schema` reached `Committed` via WU5's
  rebuild path or its no-op path — WU6 does not need to distinguish them.
- No new journal field is introduced. This binding uses only
  `PlanID`/`PlanDigest`/`StepID`/`State`, all of which already exist on
  `LifecycleTransitionJournal`.

**Production-path test:** (a) attempt to run `runtime_state` against a
plan whose `storage_schema` step never committed — assert refusal before
`applying`; (b) attempt to run `runtime_state` under a plan with a
`PlanDigest` that doesn't match the `storage_schema` step's recorded
history (simulating a substituted/stale plan) — assert refusal; (c) run
the full two-step sequence against WU5's fixture database end-to-end
(both the rebuild-path fixture and the already-canonical/no-op-path
fixture), assert `runtime_state` only becomes reachable after
`storage_schema` genuinely committed, in both cases.

**Authority boundary:** none new — this is a precondition check reusing
WU2's and WU4's mechanisms.

**Blocking dependency for later work, not a human STOP:** none beyond
ordinary code review.

---

### WU7 — `runtime_state` retained-population reconciliation and non-plugin/client-adapter reconstruction (ADR-088 §6-§9, §11)

**Depends on:** WU4, WU6.

**Files:** new `internal/lifecycle/runtime_state_binding.go` (a second
`StepDriver`), reading `installed_packages`, `package_activation_receipts`
(where present), `invocation_registry`, and the now-structurally-corrected
`invocation_runtime_bindings` (never mutating the first three).

**Acceptance criteria:**
- **Population reconciliation:** for every `(entry_point_id,
  package_version, content_digest)` key present in `invocation_registry`
  with `active = 1` (the v1 semantic-validation target, per ADR-088 §11),
  cross-reference against `installed_packages`/`package_activation_receipts`
  to establish legitimacy — a structural `invocation_registry` row alone
  does not establish legitimacy, per ADR-088 §6. Rows failing this
  cross-reference are precondition failures, not silently accepted or
  silently dropped.
- **Missing-row case, stated explicitly:** if an active `invocation_registry`
  key has no corresponding `invocation_runtime_bindings` row at all (not
  merely one with stale/incorrect content), the complete-key-set invariant
  requires an **INSERT** of a newly-derived row for that key, not only
  re-derivation of existing rows' content — this driver's reconstruction
  logic must handle both "row exists, content needs correction" and "row
  is entirely absent" as the same derive-then-write path, differing only
  in whether the write is an `INSERT` or an `UPDATE`.
- **No deletion, anywhere in this driver, of any row** —
  `invocation_runtime_bindings` rows with `active = 0` (in the joined
  `invocation_registry` sense) are read for population-accounting purposes
  only if needed for the legitimacy cross-check, and are never written to,
  deleted, or excluded from the post-repair table.
- **Non-plugin/client-adapter reconstruction**, for each row in the active
  set classified non-plugin (WU8 owns classification): decode
  `invocation_registry.contract_json` back into the `InvocationContract`
  shape; set `runtime_id = "client-adapter:" + (decoded).PackageID + ":" +
  (decoded).EntryPointID`, `runtime_version = (decoded).Version`,
  `runtime_digest = digestPackageBytes(contract_json bytes exactly as
  stored)` — reusing the exact `digestPackageBytes` formula already in
  `internal/state/package_registry.go`, not a reimplementation. Cross-check
  the decoded `PackageID`/`EntryPointID` against the row's own
  `package_id`/`entry_point_id`; on mismatch, treat as a precondition
  failure for that row (per ADR-088 §7's corrected cross-check
  requirement), never silently prefer one source.
- **Complete-key-set invariant** (ADR-088 §11): if even one entry in the
  active-membership set lacks a legitimate derivation (missing provenance,
  failed cross-check, or — per WU9 — a plugin classification), `Apply`
  returns a non-success `ApplyResult` for the **entire** transition; no
  partial write to `invocation_runtime_bindings` is ever committed. This
  is enforced by computing and validating every row's proposed
  reconstruction **before** any write is issued inside the shared
  transaction, not by writing as it goes and rolling back on first
  failure (both achieve the same all-or-nothing outcome, but computing
  first avoids doing conditional work inside the lock-held transaction
  unnecessarily).

**Production-path test:** (a) fixture database with a mix of active
non-plugin rows, inactive historical rows (multiple generations of the
same entry point), an active key with no existing binding row at all, and
rows lacking install/activation provenance; assert only the active,
provenance-legitimate, non-plugin rows are reconstructed (via `INSERT` for
the missing-row case and `UPDATE` for existing rows needing correction),
inactive rows are byte-identical before/after, and the provenance-missing
row causes the whole transition to fail closed; (b) corrupt a row's
`package_id` to disagree with its `contract_json`'s decoded `PackageID`;
assert the cross-check catches it and the whole transition fails closed;
(c) after a successful repair, run the existing rollback/reactivation code
path (`internal/state/package_rollback.go:88,337`) against an inactive
historical generation and assert it still functions.

**Authority boundary:** WU4's guard.

**NO DOGFOOD.**

**Blocking dependency for later work, not a human STOP:** none beyond
ordinary code review for the driver itself; dogfood execution not
authorized here (§8).

---

### WU8 — Authoritative binding classification, without requiring current-trust verification (ADR-088 §8)

**Corrected mechanism (the prior text required constructing a
`packagecatalog.VerifiedPackage` merely to classify a row, which is
circular — verification requires current trust semantics, the exact path
ADR-088 §9 excludes from v1 even for plugin-backed rows this driver must
only *detect and refuse*, not verify).**

**Depends on:** nothing beyond WU7's scaffolding (can be developed in
parallel with WU7's non-plugin path, merged together).

**Files:** part of WU7's driver, or a small shared helper it calls.

**Acceptance criteria:**
- Classification (plugin vs. non-plugin) is determined by: (1) locating
  the retained generation's durable manifest bytes from the authoritative
  durable source — **`installed_packages.manifest_json`**, written at
  package-activation time (`internal/state/package_registry.go:301`; this
  is the authoritative source this driver uses, not
  `package_activation_receipts.manifest_bytes`/`artifact_bytes`, which
  additionally carries signed-artifact content this classification step
  does not need); (2) decoding those bytes into the
  `packagecatalog.Manifest` shape (the same struct
  `VerifiedPackage.Manifest()` would return, but reached here by direct
  JSON decode of the durable manifest — **no `VerifiedPackage`
  construction, no signature verification, no trust-root dependency of any
  kind**); (3) calling that decoded manifest's own
  `ExecutableBinding(entryPointID)` method
  (`internal/packagecatalog/manifest.go:152`) — the identical predicate
  `resolveExecutableBinding` itself uses as its first branch
  (`internal/state/package_registry.go:201`) to decide plugin vs.
  non-plugin — and using its boolean result as this driver's
  classification. This answers only "is this entry point plugin-backed
  according to the manifest's own structure," never "is the plugin package
  currently trusted" — the latter question is exactly what v1 does not
  need answered to satisfy ADR-088 §9's refusal requirement.
- Classification is **never** determined by reading the target row's own
  `runtime_id`, `executable_binding_json`, or any other
  `invocation_runtime_bindings` column.
- If the durable manifest bytes cannot be located or decoded for a given
  retained generation, that row's classification is itself unresolved and
  is treated as a precondition failure — never defaulted to non-plugin.

**Production-path test:** (a) construct a row whose
`executable_binding_json` is `NULL` (looks non-plugin from the target
side) but whose `installed_packages.manifest_json` decodes to a manifest
whose `ExecutableBinding(entry_point_id)` returns `true`; assert the
driver correctly routes it to WU9's refusal path rather than the
non-plugin reconstruction path — this is the exact downgrade-confusion
scenario ADR-088 §8 requires closed, and must be a real test, not just a
code-review assertion; (b) assert this classification succeeds and
correctly returns "non-plugin" **without any `PRAXIS_TRUSTED_KEYS`
environment variable set and without any package signature being
constructed or checked** — proving the classifier genuinely has no #140
dependency, closing B5's circularity concern as a tested property, not
merely a design claim.

**Authority boundary:** none additional.

**Blocking dependency for later work, not a human STOP:** none.

---

### WU9 — Plugin-backed bounded refusal (ADR-088 §9)

**Depends on:** WU7, WU8.

**Files:** part of WU7's driver.

**Acceptance criteria:**
- **No plugin-binding reconstruction code is implemented in this PLAN.**
  When WU8's classification identifies any row in the active-membership
  set as plugin-backed, the driver's `Apply` returns a non-success outcome
  for the **entire** `runtime_state` transition (routed through
  ADR-088 §9's exact routing table: `applying -> failed_recoverable` for a
  definitive classification, `applying -> reconcile_required` for genuine
  ambiguity, each with a specific `RecoveryAction` citing #140) — never a
  partial registry containing only the non-plugin rows.
- This detection is deterministic and occurs during the same
  population-scan pass as WU7's reconstruction attempt, not a separate
  best-effort check.

**Production-path test:** fixture database with one plugin-backed active
row alongside several legitimate non-plugin rows; assert the whole
transition refuses (citing #140 in `RecoveryAction`) and **zero** rows are
written to `invocation_runtime_bindings`, including the otherwise-valid
non-plugin ones.

**Authority boundary:** none additional — this is a refusal path, not a
mutation path.

**Blocking dependency for later work, not a human STOP:** this is the
required *absence* of plugin recovery, not new work requiring sign-off.
**Building an actual plugin-recovery path is out of scope for this PLAN
entirely** and requires a separate future ADR/PLAN once #140 is resolved.

---

### WU10 — End-to-end qualification (owns qualification-matrix row 28)

**New work unit (was previously an unowned matrix row referencing WU1,
WU5, WU7 without any work unit actually specifying the required chain of
steps, fixtures, and assertions).**

**Depends on:** WU1, WU2, WU3, WU4, WU5, WU6, WU7, WU8, WU9 all landed and
independently passing their own production-path tests.

**Files:** a new integration test (e.g.
`internal/lifecycle/runtime_recovery_e2e_test.go` or an equivalent
integration-test location this repository already uses for multi-package
tests), exercising WU1's CLI entry point end-to-end rather than calling
internal packages directly, so the test exercises the actual reserved
control-plane path a real operator would use.

**Required, exact test chain (this is the acceptance criterion — the test
must perform every step below, in order, against one fixture installation
that is torn down and reconstructed between process-restart steps, not
reused in-memory):**

```
1. build a fixture installation database from the exact `0084352`
   migration body (handler_* columns, FK without ON DELETE CASCADE),
   populated with a realistic mix of active and inactive
   invocation_registry/invocation_runtime_bindings rows, at least one
   entry point with multiple retained generations, and durable
   installed_packages/package_activation_receipts provenance for every
   legitimate row — all non-plugin/client-adapter (plugin rows are
   exercised separately by WU9's own test, not blended into this
   end-to-end happy-path proof)
   ->
2. invoke WU1's CLI entry point (not an internal function call) to run
   the storage_schema transition against this fixture, with valid
   WU4-satisfying authority
   ->
3. terminate the test process/goroutine state and reconstruct the
   lifecycle executor from only the fixture database's durable state
   (simulating a real process restart) — no in-memory state carried over
   ->
4. assert the storage_schema step's journal shows Committed, and the live
   schema now matches the canonical `2860381` shape via direct PRAGMA
   introspection (not by trusting the journal alone)
   ->
5. invoke WU1's CLI entry point again to run the runtime_state transition
   against the same fixture, with valid (separately obtained, per WU4's
   "both transitions independently" requirement) authority
   ->
6. terminate and reconstruct again, simulating a second process restart
   ->
7. assert the runtime_state step's journal shows Committed, and every
   active-membership invocation_runtime_bindings row now holds
   production-correct runtime_id/runtime_version/runtime_digest values
   (compared against independently computed expected values, not against
   the driver's own output circularly)
   ->
8. issue a real dynamic Goal invocation against the fixture installation
   through the ordinary dynamic-dispatch path (resolveDynamicInvocation)
   and assert it resolves successfully — this is the actual outage this
   whole recovery exists to fix, proven end-to-end
```

**Constraints, asserted programmatically, not merely narratively true:**
- no manual SQL statement is executed anywhere in this test outside the
  two governed transitions invoked via WU1's CLI path (asserted by
  restricting the test's own database access to read-only assertions
  between steps, never a write);
- no direct DB patch of any kind between phases;
- no dynamic-registry bypass — step 8 specifically must go through
  `resolveDynamicInvocation`, not a direct binding lookup;
- no hidden fixture mutation between phases — the fixture is built once,
  at step 1, and never touched except through steps 2 and 5's governed
  transitions;
- every historical row present in step 1's fixture (including inactive
  generations) is still present, byte-identical where untouched, after
  step 7;
- authority is obtained through the same code path WU4 tests exercise,
  not fabricated inline for this test alone.

**Authority boundary:** exercises WU4's guard twice, independently, as
required.

**NO DOGFOOD** — entirely fixture-based, as with every other work unit.

**Blocking dependency for later work, not a human STOP:** this work unit
is the final implementation-side gate before §4's independent review; it
does not itself require human authorization beyond ordinary code review,
but §4's independent reviewer treats a failing or missing WU10 as
disqualifying for the PLAN as a whole.

---

## 2. Crash/restart and journal-integrity qualification (ADR-088 §13, §14, §15)

Not a separate implementation work unit — a required **test suite**
spanning WU3-WU7, executed against WU5's and WU7's actual drivers, at
minimum:

- crash injected before the Apply transaction begins;
- crash injected during the Apply transaction, at each of: after guard,
  after predicate revalidation, mid-mutation, after mutation before the
  driver's own pre-commit result check, after that check before the
  tx-scoped journal insert, after journal insert before commit;
- transaction rollback from a failed in-transaction predicate — assert
  zero schema change, zero bindings write, zero orphaned journal event;
- successful commit — assert both the mutation (or no-op observation, for
  WU5's already-canonical case) and the journal outcome event are present,
  together, always, and that `RunStep` appended no second outcome entry
  (per WU3's ownership-split test);
- restart/new process immediately after a committed `storage_schema`
  transition, then again after a committed `runtime_state` transition —
  assert no re-attempt of `Apply`, no illegal transition, continuation (or
  correct terminal recognition) on the next `RunStep` invocation;
- stale `PlanDigest` restart (reuses WU2's test, exercised again through
  the full driver stack, not just the state-machine unit test);
- `Applying`-state restart (reuses WU2's test, same note, including the
  `Idempotent` consultation WU2 requires);
- authority expiry/revocation injected immediately before the
  in-transaction guard runs (reuses WU4's test, exercised through the full
  driver stack);
- journal append-only and sequence integrity under injected
  concurrent-writer contention (a second goroutine/process attempting a
  conflicting write during the held transaction) — assert it blocks or
  fails cleanly, never corrupts sequence;
- every `reconcile_required` entry produced anywhere in this suite carries
  a non-empty, specific `RecoveryAction` — asserted programmatically
  against `LifecycleTransitionJournal.Validate()`, not just visually
  inspected;
- no `UPDATE`/`DELETE` against the journal's underlying `events` table is
  ever issued by any code path exercised in this suite (a direct SQL
  assertion against the test database's write log, not an inference from
  passing behavior).

Every test above is **local, fixture-based** — none touches the
authoritative dogfood database.

## 3. Non-production-code deliverables

- **`contract_json` decode/schema-evolution fidelity test (corrected —
  was previously tautological):** the prior version of this test decoded a
  blob written by the *current* `InvocationContract` struct definition and
  re-marshaled it with that same current definition, proving nothing about
  schema evolution. **Replaced requirement:** the fixture bytes MUST come
  from a durably-persisted, historically-written `contract_json` body —
  either a byte-for-byte copy of a real row's stored bytes from a fixture
  database built under an earlier version of the `InvocationContract`
  struct (paralleling the `0084352`-lineage fixture approach WU5 already
  uses), or, if this repository has never actually changed
  `InvocationContract`'s wire shape, a fixture explicitly constructed to
  represent the oldest schema version this driver claims to support. The
  test decodes those durable bytes through the **current production**
  decode path (the same `json.Unmarshal` call WU7's driver actually uses,
  not a standalone helper), derives `runtime_id`/`runtime_version`/
  `runtime_digest` through WU7's actual production derivation, and
  compares the result against an **independently established expected
  value** (computed by hand from the known historical bytes' content, not
  by re-running the same decode/derive path a second time — a test that
  only compares a helper's output to itself proves nothing, per the
  independent review that flagged this). **If this repository cannot
  produce a genuinely-historical `contract_json` fixture (i.e. the wire
  shape has in fact never changed since the earliest retained generation),
  this work unit's test MUST say so explicitly and assert the negative
  finding as its result — WU7 must then explicitly document that decode
  fidelity across schema evolution is unproven and untested, rather than
  silently proceeding as if the current-only round-trip test (which
  remains a legitimate, weaker check to keep as an additional guard) had
  established that broader claim.**
- **Durable recovery evidence audit**: a checklist (not new code) verifying
  every artifact ADR-088 §14 requires is actually durable and immutable
  after WU3-WU10 land: plan/proposal identity, pre-state introspection
  evidence, structural observation, semantic source evidence, authority
  decision reference, every transition attempt, every committed outcome,
  every `RecoveryAction`, post-transition qualification evidence. This
  audit is performed as part of §4's independent review, not invented as
  new storage beyond what the journal and existing evidence tables
  (`package_activation_receipts`, etc.) already provide.

## 4. Independent-review graph gate (issue #136)

Before this PLAN (or any commit implementing it) is considered qualified,
and **before §8's dogfood ceremony may even be proposed**, a reviewer
whose **model identity differs from the producer model identity for the
work under review** must independently verify, against actual production
code and actual test execution (not prose review alone):

- every work unit's acceptance criteria are actually met by the delivered
  code, not merely asserted in a PR description;
- every production-path test in §1/§2/§3 exists, is not disabled/skipped,
  and actually exercises the scenario it claims to (specifically including
  WU10's end-to-end chain and WU4's four-writer-function authority test —
  the two corrections most likely to be shortcut under implementation
  pressure);
- the qualification matrix in §5 is fully covered;
- no blocking finding from this review is left unreconciled.

This PLAN does not hard-code which two models satisfy this — any pairing
where reviewer identity differs from producer identity for the specific
work being reviewed qualifies, following the same practice already used
for ADR-088's own two independent review passes. Blocking findings must be
durably reconciled (fixed, or the PLAN itself revised) before proceeding
to §8. **The absence of an available independent reviewer is not a pass
condition — it is a blocking condition; this PLAN may not be treated as
qualified without an actually-performed independent review.**

Independent review is additionally scheduled at architectural dependency
boundaries where downstream work will assume an invariant without re-proving
it. For work of PLAN-016's size, the target topology is roughly three review
cuts: (1) authority/persistence boundary, (2) first real mutation/recovery
boundary, and (3) final end-to-end qualification. These intermediate cuts do
not replace or weaken the mandatory final independent review above.

## 5. Qualification matrix

The independent review in §4 must confirm production-path coverage of, at
minimum, each item below, cross-referenced to the work unit that owns it:

| # | Case | Owning work unit |
|---|---|---|
| 1 | Divergent historical `handler_*` schema (exact `0084352` fixture) | WU5 |
| 2 | Missing current `ON DELETE CASCADE` FK behavior, plus `PRAGMA foreign_key_check` clean after rebuild | WU5 |
| 3 | Structural table rebuild preserves every row identity | WU5 |
| 4 | Already-canonical schema case reaches `Committed` via a defined no-op, with zero mutating SQL issued, and `runtime_state` becomes reachable afterward | WU5, WU6 |
| 5 | Structural result digest binds actual observed post-mutation (or post-observation) schema, not just declared target | WU5, WU6 |
| 6 | Stale/substituted structural result rejected by `runtime_state` | WU6 |
| 7 | Non-plugin-only retained history reconstructed correctly, including newly-inserted missing rows | WU7 |
| 8 | Inactive historical generation preserved byte-identical | WU5, WU7 |
| 9 | Rollback/reactivation works after full recovery | WU7 |
| 10 | Plugin-backed retained generation causes v1 bounded refusal, zero partial writes | WU8, WU9 |
| 11 | Plugin classification succeeds with zero #140/trust-root dependency (no `PRAXIS_TRUSTED_KEYS`, no signature check) | WU8 |
| 12 | Missing activation/install provenance handled as precondition failure | WU7 |
| 13 | Corrupted plugin/non-plugin classification evidence handled per source-side-only rule, using durable manifest bytes only | WU8 |
| 14 | `runtime_id` target-row substitution/cross-check-mismatch caught | WU7 |
| 15 | `contract_json` decode/schema-evolution fidelity against genuinely historical bytes, or an explicit documented negative finding | §3 |
| 16 | Stale `PlanDigest` restart | WU2, WU6 |
| 17 | Wrong/mismatched `StepID` identity | WU2 |
| 18 | `Applying`-state restart, no illegal transition, `Idempotent` consulted | WU2 |
| 19 | `RunStep` never appends a second outcome entry after a driver's own transaction already committed one | WU3 |
| 20 | Pre-commit result-mismatch never produces a `Committed` entry that is later "corrected" — it is never committed at all | WU3, WU5, WU7 |
| 21 | Authority expiry/revocation immediately before mutation | WU4 |
| 22 | Authority invalidation caught inside the shared transaction | WU4 |
| 23 | The sole typed persistence boundary rejects repair authority for non-empty `ParentRef` or `DelegatedBy`, seals the exact validated value internally, and all four raw `secure_blobs` writers structurally reject reserved-namespace bypass; unrelated namespaces unaffected | WU4 |
| 24 | Tx-scoped journal read does not deadlock under the capped single-connection pool | WU3 |
| 25 | Failed in-transaction predicate leaves journal and domain state completely unchanged | WU3, WU4, WU5, WU7 |
| 26 | Crash/rollback at each boundary during the Apply transaction | §2 |
| 27 | Committed Apply always has both mutation (or no-op observation) and journal outcome, never one without the other | §2 |
| 28 | Append-only journal history, no update/delete, under load | §2 |
| 29 | Illegal lifecycle transition rejected by `Validate()` | §2 (regression-tested against `validLifecycleTransition`) |
| 30 | Every `reconcile_required` entry carries a `RecoveryAction` | §2 |
| 31 | Restart/new process after `storage_schema` alone | §2 |
| 32 | Restart/new process after `runtime_state` alone | §2 |
| 33 | Full end-to-end: historically divergent fixture → governed `storage_schema` → restart → governed `runtime_state` → restart → dynamic Goal resolution succeeds, no manual SQL, via WU1's real CLI path | WU10 |
| 34 | No historical evidence (journal, `package_activation_receipts`, etc.) altered by any test run | §2, §3 |
| 35 | Mandatory different-model independent final review completed and findings reconciled | §4 |

## 6. Explicit STOP points requiring human authority

Beyond the per-work-unit implementation-dependency notes above (which are
ordinary engineering sequencing, not human-authority gates — none of
WU1-WU10 requires human sign-off beyond ordinary code review), this PLAN
requires explicit human (Thomas, architecture owner) authorization at
exactly these three points, and no others:

1. **Before §8's dogfood ceremony is even proposed** — after §4's
   independent review passes and §5's matrix is fully green in a
   non-dogfood environment, a human must explicitly authorize proceeding
   to plan (not yet execute) the real ceremony.
2. **Before the real `storage_schema` transition executes against the
   authoritative dogfood database** (inside §8's ceremony, not this
   PLAN) — a human must explicitly authorize that specific execution,
   separately from the general go-ahead in point 1.
3. **Before the real `runtime_state` transition executes against the
   authoritative dogfood database** — same, separately from point 2, even
   though `storage_schema` will already have completed by then.

## 7. Historical PLAN disposition

`docs/PLAN/007-derived-runtime-registry-repair.md` is **not modified by
this PLAN**. It remains `PROPOSED — NOT ACCEPTED`, preserved exactly as
written, as historical evidence of a design two independent reviews (one
producing ADR-087, one producing this ADR-088 lineage) found structurally
invalid. This PLAN does not claim continuity with it, does not reuse any
of its qualification claims, and does not present its own acceptance as a
natural evolution of PLAN-007 — it is a fresh document governed entirely
by ADR-088.

## 8. Ceremony reference (not authorized here)

For completeness, the eventual real-dogfood-recovery ceremony this PLAN's
qualified implementation will support, once independently reviewed,
qualified per §4-§5, and explicitly authorized per §6:

```
inspect authoritative dogfood installation (live introspection, WU5's
  Preflight, read-only)
  ->
prepare exact storage_schema recovery plan (WU5/WU6's plan construction)
  ->
obtain required repair authority (WU4's guard's precondition, a real
  human-authorized decision, not fabricated for this ceremony)
  ->
execute storage_schema (WU5, inside WU3/WU4's atomic transaction — rebuild
  path or no-op path, whichever the live installation's actual state
  requires)
  ->
qualify the exact structural output (live re-introspection, part of WU5's
  Apply, already inside the same transaction)
  ->
prepare exact runtime_state recovery plan, bound to the qualified
  storage_schema result (WU6/WU7)
  ->
obtain required repair authority (again, independently, per WU4's
  "both transitions independently" requirement)
  ->
execute runtime_state (WU7, inside WU3/WU4's atomic transaction)
  ->
qualify (post-transition verification, outside the transaction, a
  separate later fact per ADR-088 §15)
  ->
restart / new process
  ->
prove dynamic Goal invocation resolves end-to-end
```

**No direct SQLite surgery at any point.** This PLAN does not authorize
executing this ceremony — §6 governs exactly when that authorization may
even be requested.

## 9. Self-review

This PLAN was checked against accepted ADR-088 for:

- **Scope expansion:** none found — §0 restates ADR-088's exact in/out
  scope, and every work unit (including the new WU10 and the expanded
  WU3/WU4) cites the specific ADR-088 section it implements; no work unit
  introduces a capability ADR-088 did not authorize. Architecture changes
  introduced after initial acceptance: **1 narrow enforcement-boundary
  correction**, recorded in WU4 and the status note above.
- **Missing authority boundary:** none found post-correction — WU4 now
  owns the already-closed delegation-profile layer and a sole typed
  persistence API that validates the exact plaintext generation before it
  internally serializes and seals it. All raw sealed-record writers reject
  the reserved namespace with no exported raw exception, closing ADR-088
  §10(b) structurally rather than trusting the current call graph.
- **Illegal lifecycle transition:** none found — WU2/WU3/WU6 explicitly
  encode only `validLifecycleTransition`'s real edges; the prior
  post-`committed`-mismatch illegal-transition path is eliminated by
  WU3's ownership-split correction (the mismatch check now runs
  pre-commit, inside the driver's own transaction, so `committed` is
  never reached in that case at all); §2's crash suite tests against the
  same map, not an invented one.
- **Plugin recovery leakage:** none found — WU8/WU9 exist specifically to
  guarantee bounded refusal; WU8's corrected classification mechanism
  still performs zero verification, zero trust-root dependency; no work
  unit implements plugin reconstruction; §5 items 10-11 require this be
  proven, not assumed.
- **Active-set-only reconstruction:** none found — WU7 explicitly reads
  and preserves inactive rows unmodified; only the *validation* scope
  (not the *preservation* scope) is limited to active membership, matching
  ADR-088 §11 exactly.
- **Historical-row deletion:** none found — no work unit issues a `DELETE`
  against `invocation_runtime_bindings`, `invocation_registry`, or
  `installed_packages`; WU5 explicitly copies every row unconditionally in
  its rebuild path and mutates nothing in its no-op path.
- **Non-atomic mutation/journal path:** none found post-correction —
  WU3's ownership-split refactor is precisely what makes WU3/WU4 the
  explicit, now-actually-composable foundation every mutating driver
  (WU5, WU7) depends on; the previous version's atomicity claim is now
  backed by a concrete `RunStep`-restructuring requirement rather than
  merely asserted.
- **Authority TOCTOU:** none found — WU4's guard is placed first inside
  the shared transaction by construction, not as a separate pre-check, now
  proven across all four writer surfaces rather than one.
- **Dynamic-registry recovery dependency:** none found — WU1 explicitly
  tests independence from `resolveDynamicInvocation`/dynamic dispatch;
  WU10's end-to-end test additionally proves the *opposite* direction
  works (a real dynamic invocation succeeds *after* recovery, via the
  ordinary dynamic path, not a bypass).
- **Invented result fields:** none found — WU6 explicitly reuses only
  `PlanID`/`PlanDigest`/`StepID`/`State`, no new journal field; WU3's
  `OutcomeAlreadyRecorded`-style addition is an in-memory `ApplyResult`
  signal between a driver and `RunStep` within one process invocation, not
  a new durable journal field.
- **Tautological/helper-only tests:** the prior `contract_json` fidelity
  test (§3) is corrected to require genuinely historical fixture bytes and
  an independently-established expected value, or an explicit documented
  negative finding if no such fixture can be produced; WU10's end-to-end
  test now exists with a fully specified fixture/steps/assertion chain,
  closing the prior unowned matrix row.
- **Implementation decisions that actually require architecture
  authority:** none identified beyond what ADR-088 already decided; where
  this PLAN leaves an exact mechanism name/signature to implementation's
  discretion (e.g. WU3's exact new-function names), it is explicitly
  flagged as such and does not affect any invariant ADR-088 requires.

This PLAN did not accept itself. It required, and received, the
independent review in §4 before acceptance (see the status line above for
the exact review history), consistent with issue #136 and the practice
already followed for ADR-088 itself. Acceptance of this PLAN authorizes
implementation of WU1-WU10 exactly as specified; it does not itself
authorize §8's dogfood ceremony, which requires its own separate
human-authority STOP-point sign-offs (§6, points 2-3) after WU1-WU10 are
implemented and independently qualified per §5.

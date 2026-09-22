# FAA / Keychain migration decision — pre-FAA installation crossing into the accepted PRE-V4 candidate

**Phase:** migration-design / decision only. **Disposition: MIGRATION_READY** (the repository already defines the exact governed mechanism this installation needs; nothing new was designed).

**Not authorized by this phase and not performed:** mutating live authority state, creating the production FAA anchor, atomic core-binary replacement, package deployment, PRE-V4 activation, Proposal v4 materialization, Gates A/B/C. This document is analysis and a proposed execution plan only.

**Scope:** the live installation at `PRAXIS_DB=/Users/polliard/.praxis/praxis.db`, `PRAXIS_BOOTSTRAP_RECORD=/Users/polliard/.praxis/bootstrap.json`, currently served by the installed core `807359e48b2626abb1bcfca3ec302da743f1a0238644d6b0c64a0f888ec48fe7` (`praxis version`: `2.0.0-dev`, `commit f539b74f7678e323f4dcfbe820bffa833d2b5c44`, `modified: false`, `vcs_time: 2026-09-19T18:31:09Z`), crossing to the Review-#10-accepted candidate core `42cb404ff30b77b375505a2593f5541b9be63587b39e87c1792d9231851de821` (source `25c7330`).

## 1. Exact current installed authority state

Inspected directly and read-only (`sqlite3 -readonly`, no write, no Keychain access — see evidence file). `secure_blobs` row counts by namespace in the live database:

```
authority_decision                    10
authority_generation                   3
authority_generation_invalidation      1
authority_request                     11
goal_baseline                         13
provider_workspace                     5
publisher_governance                  11
root_authority_succession_decision     1
root_authority_succession_proposal     1
root_authority_succession_review       1
work_plan_acceptance                   6
work_plan_proposal                    10
work_plan_review                      10
```

Zero rows exist for `authority_generation_live`, `authority_decision_live`, `governance_fact`, `goal_safety_classification`, and `authority_revocation` — these namespaces are entirely absent from the live database. SQL tables: `publisher_generations` (the mutable `state.Store.PublisherGeneration` record) = 0 rows; `authority_model_active` = 1 row (a pointer at `publisher_governance/active-authority-model:v6/1`, updated `2026-09-18T18:18:05Z`); `installed_packages` = 3; `capability_leases` = 0.

**The installed core (`f539b74`, `vcs_time` 2026-09-19T18:31:09Z) itself predates every repair in this whole PRE-V4 chain** — Repair 4 (I12 liveness, 2026-09-21) is the *first* repair to write `authority_generation_live`/`authority_decision_live`/`goal_safety_classification` rows at all, and Repair 5 (FAA, also 2026-09-21) is the first to write `governance_fact` rows. The installed binary's code never wrote any of them: their absence is not evidence of retirement or loss, it is the expected state of a pre-Repair-4, pre-FAA installation. `root_authority_succession_*` (1/1/1) and exactly one `authority_generation_invalidation` row are consistent with exactly one historical root-succession event having already retired an earlier root generation, in-arch, under the pre-liveness code — this is an inference from the counts, not independently confirmed (the rows are encrypted `secure_blobs` envelopes; confirming their content needs the running application's crypto provider, which this phase does not invoke against the live installation).

## 2. Exact incompatibility with the accepted candidate

Two independent gates in the accepted candidate, both of which the installed state fails today, for different reasons:

**(a) Liveness (I12, Repair 4).** `internal/goalstore/liveness.go`'s `requireLive`/`requireLiveInTx` requires a *positive* sealed liveness row (`authority_decision_live` or `authority_generation_live`) before any decision or generation grants anything. Because the installed binary never wrote one, **every one of the 10 `authority_decision` and 3 `authority_generation` records currently in the store fails `requireLive` today**, independent of the FAA question — this is not a hypothetical migration risk, it is the store's actual current state under the accepted candidate's rules.

**(b) Forward Authority Anchor (D1, Repair 5).** The accepted candidate's production entrypoint (`cmd/praxis`) unconditionally wires the Keychain-backed FAA — `governanceAnchorFactory` in `cmd/praxis/governance_anchor.go` always returns `crypto.NewPlatformAnchor()`; there is no environment variable, flag, or configuration that omits it (`withGovernanceAnchor`'s own comment: "no backend, no governed operation (fail closed)"). Since this installation has never run an FAA-aware binary, **no Keychain item exists yet for this installation's digest**. The first governed operation the new core attempts will call `faa.Load`, get `faa.ErrMissing`, and every governed operation will refuse with `ErrGovernanceAnchorMissing` ("an installation that predates the anchor... needs a governed re-anchor"). `GovernanceStatus().Relation` would read `"missing"`.

Both gates independently and correctly refuse this installation's pre-existing state. Neither can be worked around; both are closed exactly the same governed way (§5).

## 3. Authority that legitimately needs to survive

The **installation root** — the single root-shaped authority generation enrolled by the OS user who owns this installation (`polliard`), which is the provenance root for every other decision/generation/delegation in the store. Nothing else. Delegated generations (package-manager delegation, publisher delegation, any `authority_request`-derived decision) are **not** re-admitted by a re-anchor; they must be re-established through their ordinary governed ceremonies after the root is current again, exactly as a brand-new installation would establish them.

## 4. Minimum authority for the installation to remain operable after replacement

The re-admitted root, with fresh liveness re-stamped on it by the re-anchor ceremony itself (`Repository.completeRootReadmission`, called automatically as the last step of `Reanchor`). That is sufficient for `praxis authority` root-scoped operations (delegation, root succession) to resume. It is **not** sufficient for package-manager or publisher operations that depend on a currently-live delegated generation — those need their own fresh delegation ceremony issued *by* the now-current root, post-migration, which this phase does not perform and does not propose performing.

## 5. Authority that must NOT survive, and the proposed transition

**Everything except the single re-admitted root must not survive as currently-granting.** This is not a design choice made in this phase — it is the mechanism the repository already implements exactly for this situation, unmodified:

`praxis authority governance-reanchor` (`cmd/praxis/governance_reanchor.go`, `internal/goalstore/reanchor.go`). Its `PlanReanchor`/`Reanchor` pair explicitly handles a **missing** anchor (`plan.Cause == "missing"`) as a first-class case, not an error — the exact case an installation that predates the FAA is in. It:

- computes a plan naming the installation, the store's current fact-chain sequence/head (0/empty here, since no `governance_fact` rows exist), the anchor's relation (`missing`), and the **one** un-invalidated, un-retired, root-shaped authority generation belonging to this installation's owner (`installationRootCandidate` — refuses with an explicit error if it finds zero or more than one candidate, rather than guessing);
- requires an **interactive terminal**, the **authenticated OS user matching the root's enrolling user**, and a **typed confirmation string bound to the SHA-256 digest of exactly that plan** (`"REANCHOR " + planDigest`, optionally `" CLASSIFY " + goal-ids`);
- appends exactly one durable, sealed `governance_fact` (`Kind: reanchor`) recording the plan's cause, the store/anchor sequences and heads it saw, the re-admitted root's identity, the confirming OS user, and a ceremony digest;
- creates the Keychain anchor item for this installation (`FAA.Set` with no prior expectation, since none exists) **inside the same database write transaction that inserts the fact** — if the Keychain write fails, the whole transaction rolls back and nothing is persisted (§8);
- as its last step, re-stamps fresh liveness on **only** the named root generation (`completeRootReadmission`) — nothing else in the store becomes live by this action.

This is precisely the diagram the requesting instruction sketched: old authority is retained only as historical evidence (the pre-existing `authority_generation`/`authority_decision` rows are untouched, not deleted — they simply remain non-live); the re-anchor is the explicit contemporary decision with provenance to the plan digest and the OS user's authenticated confirmation; it explicitly retains exactly the root and nothing else; current FAA/liveness lineage is established from that point forward. **No different architecture is proposed; the existing mechanism is verified against the actual installation and reported here.**

## 6. New decisions/authority that require Thomas

1. **Whether to migrate this installation at all**, and if so, whether now or after further hardening/decisions below — this document is the input to that decision, not the decision itself.
2. **The re-anchor ceremony itself** is inherently a human decision: it requires Thomas, interactively, at a terminal, typing the exact confirmation string after reading the plan the tool prints (own OS user identity is checked by the tool; it cannot be scripted or delegated).
3. **Whether and how to re-establish delegated authority afterward** (package-manager delegation, publisher enrollment) — none of that is proposed, decided, or performed by the re-anchor itself, and is explicitly out of scope for this phase per the authority boundary (governed deployment of the exact goals package candidate is a separately authorized later blocker).
4. **Whether to classify any existing Goal as safety-bearing at re-anchor time** (`--classify-goal`, restriction-only) — the live `goal_baseline` table has 13 rows; whether any of these Goals should be marked safety-bearing during the ceremony is a product/governance judgment this document does not make.
5. **Order relative to atomic binary replacement** (§11) — a separate, already-identified blocker in `candidate-activation-requirements.json`, unaffected by this document except that this document now gives it an exact, evidenced execution plan instead of an open question.

## 7. Keychain effects and expected prompts

- **Item created:** one dedicated, Keychain-anchor item scoped to this installation's digest (first-ever creation for this specific installation — no Keychain state currently exists for it, confirmed indirectly by the absence of any `governance_fact` row in the store, which is the only thing that would make an anchor "consistent").
- **Executable identity / ACL:** per Repair 5's measured design, the dedicated Keychain item's ACL is bound to the creating binary's code identity. The **first** governed command run with the new core will be the first process ever to touch this item.
- **Prompt expected:** yes, on that first access — this matches the already-documented activation blocker ("expect the standard macOS Keychain access prompt for the anchor's password items... the first time the installed core is a new binary"). This has not been triggered in this phase; no command was run against the live Keychain or live database in write mode.
- **Unattended restart:** not possible for the *migration step itself* (it requires an interactive terminal and a typed, plan-digest-bound confirmation by design — this is intentional, not a gap). Ordinary *use* after a successful re-anchor (i.e., a later, unattended restart of the governed CLI/daemon) is unaffected as long as the Keychain item remains accessible to the same code identity; every anchor **advance** thereafter also re-keys the dedicated file via `SecKeychainChangePassword` (Repair 6), measured only on macOS 26.6.2 arm64 — where that symbol is unavailable, advances (not the initial re-anchor create) fail closed as unavailable.
- **Failure behavior if Keychain access is unavailable:** the re-anchor's `FAA.Set` runs *inside* the database write transaction; if it fails, the whole transaction (including the new `governance_fact` row) is rolled back — the installation is left exactly as it was (unanchored, "missing"), not partially migrated. Confirmed from `internal/goalstore/reanchor.go`'s `Reanchor`, which returns the DB-record/undo pair to `AppendGovernanceFact` only after `FAA.Set` succeeds.
- **Portability:** this is the same measured-Darwin-only, `PRAXIS_REQUIRE_KEYCHAIN`-independent boundary already carried by every prior repair/review in this chain (residual #5). Nothing here extends or narrows it. This installation target *is* the measured environment (macOS, arm64) — the "current Darwin/arm64 evidence" is sufficient for *this* installation target specifically, though not for any other platform.

## 8. Interruption / rollback semantics

Traced directly against `internal/goalstore/reanchor.go`:

- **New core installed, no migration run yet:** every governed operation calls `governanceSnapshot`/`admissionStamp`, which calls `FAA.Load`, gets `ErrMissing`, and refuses closed with `ErrGovernanceAnchorMissing`. No old authority is read as current; nothing is silently granted. This is the safe, intended resting state — the installation simply cannot perform governed operations until re-anchored. Confirmed as the direct consequence of `governanceAnchorFactory` always wiring FAA and `CheckAuthorityInForceInTx`/`requireLiveInTx` both requiring `snap.Anchored` truthy plus a positive liveness row.
- **Re-anchor interrupted mid-write (process killed, machine crash):** the Keychain `FAA.Set` call happens *inside* the same SQL transaction callback that inserts the `governance_fact` row (§5, §7); a failure or interruption before both complete rolls the SQL transaction back entirely — there is no partially-applied state where the fact exists without the anchor or vice versa in the "missing → first create" path.
- **Interruption between transaction commit and the final `completeRootReadmission` liveness re-stamp:** `Reanchor`'s own comment states this explicitly — "the re-anchor is durable but the root re-admission is incomplete; run the re-anchor again to complete it." The fact and anchor are already durable and consistent at this point; re-running `governance-reanchor` detects `plan.Consistent` (fact chain already matches anchor) and completes only the idempotent liveness re-stamp, writing no second fact.
- **Keychain creation/access fails outright:** covered above — whole transaction rolls back, installation stays unanchored/"missing," another attempt is simply another ordinary re-anchor attempt.
- **Historical DB state restored after migration:** the restored DB has fewer (or different) `governance_fact` rows than the Keychain anchor now records → `faa.Compare` reports the store **behind** the anchor → every governed operation refuses (`ErrGovernanceRolledBack`) until another owner-confirmed re-anchor is performed. The restored old `authority_decision`/`authority_generation` rows are **not** resurrected as live by this restoration — they still lack liveness rows.
- **Historical Keychain state restored after migration:** before migration, no Keychain item exists for this installation at all, so there is no "earlier" Keychain state to restore *to* other than absence. If the anchor item were deleted after a successful migration (the platform-level residual already documented as R-K1 — another process of the same OS user can silently delete a Keychain item), the next governed operation would again see `ErrGovernanceAnchorMissing` ("missing") and require yet another re-anchor ceremony, voiding whatever was granted since the first one. This is exactly the accepted, documented residual, not a new gap this migration introduces.
- **DB and Keychain generations disagree in any other way (ahead / unrelated):** same family, same result — `ErrGovernanceAhead`/`ErrGovernanceUnrelated`, refuse closed, require another owner-confirmed re-anchor. No case in this family silently grants anything.
- **The old binary run again after migration:** the old binary's code never reads or writes `governance_fact`/liveness rows at all — it would continue operating exactly as before, oblivious to the new layer, potentially writing new `authority_generation`/`authority_decision` rows through its own (pre-liveness) code paths. Under the *new* core, any such rows are simply not live (same as every pre-migration row) — they do not become current merely by being written by the old binary; they'd need their own fresh liveness through a new-core-run governed ceremony. This makes the two binaries safely non-interfering with respect to *authority currency*, though running the old binary post-migration is still operationally confusing and should be avoided by discipline, not because it can silently regrant authority.
- **New binary restarts after migration:** ordinary restart — `governanceSnapshotOnce` reads store then anchor (in that load-bearing order, per the code comment on transient-lag handling) and finds them consistent; root generation's liveness row persists in the SQLite file; no special handling needed. This matches the restart/skew probes already run for Repairs 5–7.

No traced interruption path resurrects retired authority, manufactures currentness, or grants anything beyond the single named root. The one operationally relevant conclusion: **fail-closed on this installation is the default and the safe state, both before and during any interrupted migration attempt** — replacing the binary and *not* migrating is itself a safe (if inert) state, not a dangerous one.

## 9. Restart/skew behavior

Covered by the existing restart/skew probes from Repairs 5–7 (unchanged by this migration analysis, since the migration adds no new mechanism, only exercises the existing one against this specific installation's actual data). No new restart/skew testing is proposed as part of this decision phase; it would be exercised naturally by the qualification step in §10 once migration is authorized and executed.

## 10. Qualification required before atomic replacement

In addition to the already-documented blockers in `candidate-activation-requirements.json` (unaffected by this phase):

1. This migration decision itself reviewed/authorized by Thomas.
2. `praxis authority governance-status` run (read-only, first live Keychain read for this installation) with the **new candidate** core against the live `PRAXIS_DB`/`PRAXIS_BOOTSTRAP_RECORD`, confirming `relation: "missing"` and exactly one root candidate is reported once `governance-reanchor` prints its plan (it refuses with a named error otherwise — see §12 stop point).
3. The interactive `governance-reanchor` ceremony itself, run by Thomas, at a terminal, as the authenticated enrolling OS user.
4. Post-reanchor `governance-status` confirming `relation: "consistent"`, `anchored: true`, exactly one `reanchors` entry, and the root's liveness now present.
5. Only then is atomic core-binary replacement eligible — not before, per §2's incompatibility.

## 11. Exact migration execution sequence (proposed, not performed)

1. `praxis authority governance-status` (installed OLD core, PRAXIS_DB/PRAXIS_BOOTSTRAP_RECORD as currently set) — establish the pre-migration baseline the old core sees (it will not recognize the command's concepts the same way, since it predates FAA; this step is really "confirm the old core has no notion of this," a negative check).

   Correction: the old core's `praxis authority` subcommand list does **not** include `governance-status`/`governance-reanchor` at all (confirmed: `authority --help` on the installed binary lists `bootstrap|delegate|request-inspect|model-preview|model-adopt|model-abandon|model-status|package-deploy-*|root-successor-*|installation-repair-*` — no `governance-status`, no `governance-reanchor`). So step 1 is **run the accepted candidate core** (not yet installed as `/Users/polliard/bin/praxis`; run its built path directly) with `governance-status`, still fully read-only, against the live `PRAXIS_DB`.
2. Inspect the printed relation; expect `"missing"` per §2. **Stop and report** if anything else is printed (§12).
3. Run the candidate core's `authority governance-reanchor` (no `--classify-goal` unless Thomas decides to add one per §6.4). Read the printed plan carefully — cause, store/anchor sequences, orphan count, named root ref/version/digest, root's enrolling OS user.
4. Confirm the printed root identity and enrolling OS user are the expected ones **before** typing the confirmation string (this is the human control point the mechanism is built around).
5. Type the exact `REANCHOR <plan-digest>` confirmation (or the `CLASSIFY` variant if Thomas decided to classify Goals).
6. Expect a macOS Keychain prompt (first-ever access for this installation's anchor item) and approve it, without ever typing the Keychain item's own password (Praxis generates and holds it).
7. Run `governance-status` again (still with the candidate core, still read-only) and confirm `relation: "consistent"`.
8. **Only then**, and only with Thomas's separate authorization for atomic core-binary replacement (an already-identified, still-ungranted blocker), proceed to replacement — outside this phase's authority.

## 12. Exact stop/rollback points

- **Step 2**, if `governance-status` reports anything other than `"missing"` (e.g., `"invalid"`, `"unreadable"`, or an unexpected `"consistent"` implying an anchor already exists under this digest from an untracked prior process) — stop, do not proceed to re-anchor, report the exact relation observed.
- **Step 3**, if `PlanReanchor`/`installationRootCandidate` reports zero or more than one candidate root (`"re-anchoring needs exactly one un-retired installation root in the store, found %d; resolve root succession first"`) — stop; this would mean §1's inference (exactly one root, already resolved by the one historical succession) is wrong, and root succession must be resolved with its own governed ceremony first, which is outside this phase.
- **Step 4**, if the printed root identity or enrolling OS user is not what Thomas expects — do not type the confirmation string; the ceremony has no effect until the exact confirmation is typed, so declining costs nothing and leaves the installation exactly as it was.
- **Step 6**, if the Keychain prompt is denied or fails — the whole transaction rolls back (§8); the installation remains unanchored/"missing," safely retryable.
- **Step 7**, if `relation` is not `"consistent"` after the ceremony reports success — stop before proceeding to any later step; this would itself be a defect in the accepted candidate and must be reported, not silently worked around, per this phase's own constraint against repairing an accepted candidate here.

## 13. Evidence that would prove migration succeeded

- `governance-status` output: `anchored: true`, `relation: "consistent"`, `store_seq == anchor_seq`, non-empty matching `store_head`/`anchor_head`, exactly one entry in `reanchors` naming the expected root, `orphaned_facts: 0`.
- A first governed root-scoped operation (e.g., `authority delegate` targeting a fresh delegation, or `authority model-status`) succeeding without `ErrGovernanceAnchorMissing`/`ErrGovernanceRolledBack`/`ErrLivenessMissing`.
- The pre-existing 10 `authority_decision` / 3 `authority_generation` rows remaining present and byte-unchanged in the store (proving nothing was deleted or rewritten — only a new `governance_fact` row and a re-stamped liveness row for the root were added).
- A restart of the governed CLI reproducing `relation: "consistent"` without a further prompt (steady-state, not just immediately post-ceremony).

## 14. Post-install evidence still required

Everything already listed as a blocker in `candidate-activation-requirements.json` and not addressed by this document: separate explicit human authorization for atomic core-binary replacement itself; governed deployment of the exact Goals package candidate after replacement; a **separate** final activation manifest (not a rewrite of the pre-activation record) with the exact qualification/specification-bundle digests; persisting that manifest at an explicit durable path; read-only restart/skew probes run against the *replaced, migrated* installation specifically (the existing probes were run against test/qualification environments, not this live installation); and, consistent with this whole program's practice, a fresh independent review of the executed migration once it actually happens — this document is not that review.

---

## Summary answers to the required output fields

- **Disposition:** `MIGRATION_READY`. The repository already defines the exact mechanism (`governance-reanchor`) for an installation that predates the FAA; no new architecture is proposed.
- **Exact migration proposed:** run the accepted candidate core's `authority governance-status` (read-only) then `authority governance-reanchor` (interactive, owner-confirmed) against the live installation, before any binary replacement.
- **Exact human decisions required:** whether/when to migrate at all; performing the interactive confirmation itself; whether to re-establish delegated authority afterward and how; whether to classify any Goal at ceremony time; separately, whether/when to authorize atomic binary replacement.
- **Exact command/process that would execute it:** §11, steps 1–7 (read-only status check, interactive re-anchor with typed plan-digest confirmation, read-only status re-check).
- **Expected before/after authority identities:** before — `governance-status` relation `"missing"`, zero live decisions/generations; after — relation `"consistent"`, exactly one live root generation (re-stamped by the ceremony), all pre-existing delegated decisions/generations still present but still non-live, awaiting their own fresh ceremonies.
- **Keychain effects:** one new dedicated anchor item created for this installation's digest; expect one macOS access prompt on first touch; subsequent anchor advances re-key it (measured Darwin-only, per Repair 6).
- **Failure/rollback behavior:** §8 — every traced interruption path is fail-closed and non-resurrecting; worst case is "run the re-anchor ceremony again."
- **Qualification evidence:** §10.
- **Next transition eligible if Thomas authorizes this:** atomic core-binary replacement (itself still requiring its own separate authorization per the existing activation blockers — this document does not grant it).

No candidate implementation, evidence, artifact, or review was modified. No live authority state, Keychain item, or installed binary was mutated. The only live-system interaction performed was two read-only `sqlite3 -readonly` queries against `/Users/polliard/.praxis/praxis.db` (row counts only, no decryption, no Keychain access) — see `migration-decision/live-installation-inspection-evidence.md`.

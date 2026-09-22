# Forward Authority Anchor (FAA): architecture, invariants and atomicity

Authority provenance: Thomas resolved the earlier AUTHORITY_CONFLICT (`../AUTHORITY_CONFLICT.md`, preserved unchanged) with decision **D1**: rollback and prefix-truncation of the governance store ARE inside the PRE-V4 defended adversary, and Praxis may keep **minimal forward-only governance freshness state outside the mutable governance-store rollback domain**, as a platform-neutral contract with the **macOS Keychain as the first backend**. Not authorised, and not built: any network service, GitHub witness, remote ledger, TPM dependency, cloud service or other trust infrastructure; the FAA grants, decides, recreates and substitutes for nothing.

## 1. Invariants

Derived from the Repair 5 experiments (`../probes/`) and the counterexamples N14 and N15, not copied from the task text.

**I13 — Temporal authority.** A governance fact is consumable only if the store's authenticated fact chain is exactly the chain the forward authority anchor pins. Consequently, once authority or classification has been retired, no earlier authentic state, restored whole, restored in part, or replayed row by row, can make it executable again; and a store that is behind, ahead of, unrelated to, or unable to be compared with the anchor yields no governance at all rather than an older governance.

**I14 — Subject-governance continuity.** Entry of a subject into the governed safety domain is an anchored fact. Erasing, truncating or rolling back the store's evidence cannot make that subject indistinguishable from a never-governed one: either the fact survives, or the missing suffix is detected against the anchor and governance is refused until a governed re-anchor.

**I12 (statement unchanged, implementation completed).** Classification and every retirement predicate are monotone decreasing over the *whole* Goal-attributable evidence set, not over the three rows Repair 4 read. This is enforced by a registry of evidence kinds plus a coverage test (§6).

**Composition.** I3 (historical authority is not current authority): I13 is its temporal form. I9 (downgrade resistance): I14 makes classification survive erasure. I10 (outward-effect equivalence): every outward-effect admission (recovery/publication admission, repair guard) re-checks freshness in the admitting transaction. I11 (authenticated completion consumption): gate completions re-validate their decision through the same predicate, so a resurrected decision cannot re-legitimise a completion. None of the three is weakened; the FAA only ever *removes* permission (a refusal), except the single owner-attested re-admission of the installation root in a governed re-anchor (§5), which restores the owner's own root authority and nothing else.

## 2. The contract (platform-neutral)

`internal/faa`:

* `State{Installation, Seq, Head}` is everything the anchor stores: the length and hash-chain head of the governance fact chain.
* `Anchor` is a forward-only cell: `Load`, compare-and-set `Set` (strictly forward, same installation, read-back verified), `Revert` (only for the caller's own failed commit), `Reset` (only a governed re-anchor overwrites unreadable content).
* The chain is authenticated in the store: each fact is a sealed row (same envelope service and AAD as every governance row), chained by `Prev`, with `Head = sha256(prev || canonical fact)`.
* Backends: `internal/crypto` (`KeychainAnchor`, darwin; on other platforms there is **no backend and governed operations fail closed**, exactly like governed bootstrap), and test doubles under `internal/faa/faatest` that nothing outside tests imports. Production has one selection path (`governanceAnchorFactory`, default platform backend); there is no environment variable, flag or file that selects another.

## 3. What is a fact, and why only these

A transition is a fact iff **losing or replaying it would broaden permission**. That is retirements (a decision retired; a generation invalidated, revoked or superseded), classification (a Goal entered the governed safety domain), and re-anchoring itself. Admissions (a new decision or generation) are *not* facts: losing an admission narrows permission. Instead each admission's sealed liveness record carries `admitted_at_seq`, the anchor sequence at admission; a re-anchor at sequence S voids every admission stamped below S. This keeps anchor writes to the few transitions that need them.

The chain starts with a **genesis fact bound to a random nonce**. Without the nonce the empty-chain anchor state would be computable from the public installation digest, and "restore a snapshot from before the first fact and set the anchor to genesis" would be undetectable (see §7, Keychain semantics).

## 4. Atomicity across two trust domains

SQLite and the Keychain cannot share a transaction. The protocol is chosen so that **every interleaving of a crash leaves either a consistent state or a fail-closed stranded one, never a state that broadens permission**.

Appending a fact (`Store.AppendGovernanceFact`), all inside one SQLite write transaction (the writer lock serialises every appender in every process):

1. take the writer lock; read the fact rows through the transaction; authenticate and verify the chain; load the anchor; require them consistent;
2. if the transition is already recorded, stop (idempotence);
3. seal the next fact; **advance the anchor** (compare-and-set, read-back verified);
4. insert the fact row and **commit**. If the commit fails, revert the anchor.

Retirements are **write-ahead**: the fact is committed first, then the existing effects (liveness row deleted, revocation/invalidation row written) run. The fact is the authority; the effects are derived and idempotent.

| Crash / failure point | State left | Verdict |
|---|---|---|
| before step 3 | nothing changed | consistent |
| anchor write fails | transaction rolled back, no fact | consistent (`TestFAAAnchorWriteThatDoesNotStickIsRefused`, `FailSet`) |
| backend reports success but did not persist | read-back mismatch, rolled back | consistent; refused |
| after the anchor advanced, process dies before commit | **anchor one ahead of the store** | **stranded, fail closed**: every governance read refuses (store behind anchor); recovery is the governed re-anchor (`TestFAACrashAfterAnchorAdvanceBeforeCommitStrandsFailClosed`) |
| commit fails (error, cancellation) | anchor reverted by the writer | consistent (`TestFAACommitFailureRevertsTheAnchor`) |
| after commit, before the effects | fact says retired; liveness/revocation rows still present | **retired** (restrictive); the owner's retry completes the effects and appends nothing (`TestFAACrashBetweenFactAndEffectsLeavesTheDecisionRetired`) |
| root succession: fact committed, succession transaction fails | predecessor retired by fact, successor not admitted | no current root until the same acceptance is retried (availability hazard, restrictive) |
| initialisation: anchor created, store not committed | anchor at sequence 0, empty store | the next initialisation replaces it (nothing depends on it) |
| re-anchor: fact and anchor committed, root re-admission not | consistent store, root not current | the same ceremony completes exactly the re-admission the fact names (`TestFAAReanchorInterruptedBeforeRootReadmissionIsCompletedByTheSameCeremony`) |

The reverse order (commit, then advance) was rejected: a crash between would leave a store that has lost or replayed a retirement the anchor never learned of, which is the resurrection the FAA exists to prevent.

**Readers.** Because a writer advances the anchor before it commits, a reader can observe the two differing by one transition for the length of one write. A refusal that is a plain lag (behind/ahead) is re-observed a few times with backoff before it stands; an invalid chain, an unrelated head or an unreadable anchor is refused at once. In-transaction consumers (recovery admission, the repair guard) read the chain through their own transaction and do not retry.

**Concurrency.** `TestFAAConcurrentAppendersAndReadersKeepAValidChain`: four repositories over one store and anchor append 48 facts while readers consume; the chain ends gapless and consistent.

## 5. Backup restore and re-anchoring (governed recovery)

A restored older store, a lost/unreadable/reset anchor and an interrupted write all present identically (store and anchor disagree) and are all refused. The only way out is `praxis authority governance-reanchor`, an owner ceremony:

* **Authority** is derived from existing installation-owner authority: an interactive terminal; the authenticated OS user equal to the OS user recorded in the installation root's enrolment provenance; and a typed confirmation bound to the digest of exactly the plan shown (cause, sequences, heads, the root, the owner-named Goals). Nothing new is invented; the same ceremony machinery as `authority decide`.
* **Effect, deliberately narrow:** one durable `reanchor` fact bridging exactly from the verified chain (recording the cause, the store and anchor sequences and heads, the root, the OS user and the ceremony digest); every admission stamped before it is **void**; **only** the installation root generation named in the fact is re-admitted; the owner may add classifications (restrictions only). Orphaned fact rows are preserved in a separate namespace, not consulted.
* **It cannot** grant a decision, approve a gate, broaden a Goal or WorkPlan, or make anything retired current. A decision that was in force in a restored backup and revoked in the lost interval is void, not live (`TestFAAGovernedReanchorAfterRestoreDoesNotResurrectRetiredAuthority`, `TestRepair5WholeDatabaseRollbackIsRefusedAndRecoveredOnlyByTheGovernedCeremony`). New authority is established the ordinary way and is current (`TestFAAAuthorityAdmittedAfterAReanchorIsCurrent`).
* `praxis authority governance-status` is read-only and works exactly when the store is not consumable: relation (consistent, behind, ahead, unrelated, missing, unreadable), both sequences and heads, orphan count, and every recorded recovery with its cause, root, OS user and ceremony digest.

**Residual of recovery, stated.** Recovery from a backup cannot know which classifications existed in the lost interval; the owner names Goals with `--classify-goal`, otherwise a Goal whose entire governance history lay in the lost interval reads as never governed. A gate decision voided by recovery cannot be re-decided under the same request identity (the same as after any revocation); a new request is needed.

## 6. Consumers (equivalent-path search)

Every consequential consumer of current governance state was walked. "Predicate" is the shared freshness predicate (`governanceSnapshot`: anchor loaded, chain authenticated and verified, related to the anchor) plus retirement facts and admission stamps.

| Consumer | Change |
|---|---|
| authority decision consumption (`LoadAuthorityDecision`) | freshness first; then liveness with retirement and admission-stamp checks |
| generation currentness (`ValidateAuthorityGeneration`, lineage walk, plan authority) | through `requireLive`, same predicate |
| installation root (`loadInstallationRoot`) | snapshot first; a retired or pre-recovery root is not a candidate |
| root succession | supersession anchored write-ahead; successor stamped |
| classification (`GoalSafetyKernel`, `markGoalSafetyBearing`) | anchored fact is authoritative; anchored before the row and before the proposal; registry-driven derivation as the in-store fallback |
| import (`Repository.Save`) | via classification |
| activation | unchanged (already positive-presence); a protected request whose proposal is missing now fails closed |
| publication fence, ordinary re-request | via `requireLiveIdentity` |
| recovery admission (in-transaction) | anchored chain read through the admitting transaction |
| governed repair guard (`internal/lifecycle`) | `Governance` hook supplied by the composition root, in the guard's transaction |
| completion consumption (I11) | re-validates its decision through `LoadAuthorityDecision` |
| restart | no cache; every consumption re-reads anchor and chain |
| package/goals lifecycle | authority flows through the same repository |
| `pending` listing and other readers | refuse with the freshness error rather than list a stale store |
| cached or derived currentness | none exists; the admission stamp is sealed, not cached |

**Registry-driven classification (not an eight-row enumeration).** `goalEvidenceExtractors` maps every record kind that evidences a Goal's governed status to an extractor; `nonEvidenceNamespaces` names every other governance namespace with a reason; `TestGoalEvidenceRegistryCoversEveryGovernanceNamespace` parses the source for every `*Namespace` constant and fails on any that is neither, so a new kind of governance record cannot be silently ignored.

## 7. What the Keychain does and does not provide (executed evidence)

Requirement of D1: the anchor must be outside the store's rollback domain and not writable by the store's adversary. Executed against the real Keychain for the FIRST Repair 5 design (single login-Keychain item; `../probes/keychain_cross_process_probe_test.go.txt`, `keychain-cross-process-probe.log`; preserved, superseded by §7.1 for the overwrite finding):

* A SQLite-file writer cannot reach it (no Keychain API is involved). **Provided.**
* Another process of the same OS user using the Apple-signed `security` tool: **reading** the item blocked awaiting a user prompt (would not complete non-interactively); **overwriting** it with `add-generic-password -U` **succeeded silently**, and the anchor then read as corrupt.

What this means, and does not: an attacker who can drive Keychain writes can **destroy** the anchor (the store is refused, recovery is the governed ceremony) but cannot **set it to a chosen earlier valid value**, because that needs `(Seq, Head)` and the head is inside sealed rows and unreadable without the key or an approved Keychain read; the genesis nonce closes the one value that would otherwise be computable (the empty chain). What remains, and is **not** claimed to be closed:

* **R-K1: anchor destruction plus whole-installation wipe.** Deleting the item and emptying the store is indistinguishable from a new installation. It is total erasure including the anchor; it needs a write-protected anchor.
* **R-K2: restore of the whole login Keychain together with the database** (a machine-level restore) is a consistent earlier state of both domains; the Keychain backend cannot distinguish it.
* **R-K3: a holder of the storage key or an approved Keychain accessor** can forge anything the OS-user/keychain trust root can. This is the accepted bootstrap trust root and is unchanged.

### 7.1 Access-control hardening (Thomas's follow-up decision; implemented after the first Repair 5 qualification)

The first Repair 5 candidate stored the anchor in one login-Keychain item and left the overwrite finding above open. Thomas directed that unauthorized mutation must not be silently permitted merely because the result fails closed, without weakening the governed re-anchor and without creating another recovery or authority path. Measured on this machine (`../probes/acl-hardening/`):

* **The item's own access list cannot do it.** An item created with every modification-related access entry restricted to the creating binary was still **overwritten and deleted silently** by another process of the same user (`item-acl-and-dp-keychain-measurement.log`). Changing an item's access list afterwards needs the Keychain password (status -25293), so nobody can loosen it silently either, but modification and deletion are not gated by it. Only reading is.
* **Data-protection Keychain / access-control flags are unavailable** to an ad-hoc-signed command-line tool (status -34018, missing entitlement); using them would need a signed application identity and a provisioning profile, i.e. new trust infrastructure that D1 did not authorize.
* **A locked, password-protected Keychain file does refuse another process** (`dedicated-keychain-scratch-measurement.log`, and against the real implementation `hardened-anchor-foreign-process-attempts.log`): a differently signed process without the password, with user interaction disabled, is refused on **read, overwrite and add** (status -25293, or -128 where the platform reports the cancelled prompt). It is **not** refused on **delete**, and while the file is unlocked for a genuine operation an overwrite succeeds.

What was implemented (`internal/crypto/faa_keychain_darwin.go`): the anchor lives in a **dedicated Keychain file per installation** (`~/Library/Keychains/praxis-faa-<hash>.keychain-db`). Its password is **32 random bytes generated by Praxis** (never the user's login password, never typed or shown) and held in a **separate login-Keychain item** whose access list trusts only the code identity that created it. The file is **locked except for the milliseconds of one operation**, is re-locked before every return, and also locks itself after five seconds so that a crashed process cannot leave it open. An in-process mutex and an advisory file lock serialise sessions. **Reading** the anchor still requires the password item; **updating or adding** requires the password.

Effect: a process that does not hold the password can no longer silently overwrite the anchor or plant an item in it (previously one command did). Failures are classified so that the governed re-anchor stays the only way back: a missing keychain file is a **missing** anchor; a missing, replaced or malformed password item, or a file that cannot be unlocked with it, is a **corrupt** (unreadable) anchor, which the ceremony's existing `unreadable` path repairs by tearing down the pair and creating a fresh one holding the new anchor value; environment failures (no access, prompt cancelled, framework error) stay **unavailable** and the ceremony refuses them. Nothing here grants authority, adds a recovery route, or changes what a re-anchor admits.

What the platform still does not let any Keychain backend prevent, and what remains true (all fail closed, none can forge a valid anchor state, because the value is `(Seq, Head)` and the head is not computable):

* **Silent deletion** of the anchor item, of the dedicated file, or of the password item (measured: delete succeeds silently). The result is a missing or corrupt anchor: the store is refused until the governed ceremony. This is the destruction case of **R-K1**, unchanged in kind.
* **An overwrite during the unlocked window of a genuine operation** (milliseconds, once per governed read or write) would produce a value that fails validation, i.e. a corrupt anchor. Not closed, only narrowed.
* **Per-build access prompts.** The password item and the anchor item trust the exact binary that created them (an ad-hoc-signed Go binary changes identity with every build), so after a core replacement the first use by the new binary raises the standard macOS "allow access" prompt, exactly as the installation storage-key item already does. In a non-interactive session it fails closed as unavailable.
* **Untested here:** the five-second self-lock (it exists to bound a crash and cannot be exercised without waiting on a crashed process) and the cross-process advisory lock (only the in-process mutex is exercised by tests).


## 8. Coverage

* **Exhaustive over enumerated finite classes:** the 256-subset powerset over the eight Goal-attributable evidence rows, with the anchor detached (exactly one subset, deleting all eight, reads as legacy) and anchored (none); the 4096-store mixed-snapshot enumeration (every assignment of six row groups to any of four honest states never regains a retired decision, an invalidated generation, or un-classifies a Goal).
* **Explicit state-transition tests** where mutation cannot represent the threat: whole-database rollback, replay, truncation, gap, corruption, every anchor state, crash boundaries, commit failure, lying backend, concurrent writers and readers, real-Keychain end to end, governed recovery.
* **Mutation inventory** over the FAA guards (74 new mutations) in addition to the carried-forward catalogue; syntactic mutation is not used to claim rollback resistance.
* **Not claimed:** rollback resistance beyond §7; exhaustive powerset over the whole database (2^30 and beyond); that the Keychain item cannot be destroyed.

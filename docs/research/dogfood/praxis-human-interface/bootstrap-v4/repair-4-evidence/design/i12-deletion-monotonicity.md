# Repair 4 design: the invariant behind N8 and N9

Written before the guards were extended past the two preserved probes. It records the general invariant, how it reconciles with I1–I11 and the frozen PRE-V4 semantics, the adversary model it is stated against, and the equivalent-path search it drove.

## 1. What N8 and N9 have in common

| | N8 | N9 |
|---|---|---|
| governing fact | a decision was revoked | a Goal is safety-bearing |
| how the fact is stored | presence of a row in `authority_revocation` | presence of a row in `goal_safety_classification` |
| how the consumer reads it | "no revocation row" ⇒ the decision is in force | "no classification row" ⇒ the Goal is a legacy Goal |
| effect of a keyless `DELETE` | revoked authority returns; a fresh controller mints a new sealed gate completion | the downgrade fence disappears; a stripped import succeeds and a worker is dispatched with activation absent |

Both are the same defect: **a consequence-permitting predicate was defined by the absence of a row**. Removing a row therefore *enlarged* the set of permitted consequences. The rows themselves were sealed and authentic; nothing forged them. The attacker only needed write access to the SQLite file, not the storage key.

## 2. The invariant

The candidate formulation supplied with the task was:

> Loss, deletion, corruption, or absence of governance-relevant state must not broaden the set of permitted consequences. Absence must not mean "never existed" when historical existence would impose stronger current governance.

Reconciled against I1–I11 it is a *strengthening common to* I3, I9, I10 and I11, not a restatement of any one of them:

* **I3** (historical authority is not necessarily current executable authority) says a revoked or superseded fact must not be current. It does not say how "revoked" is known. N8 shows the *how* is part of the invariant.
* **I9** (downgrade resistance) says classification may not depend on a self-declared field. N9 shows it may not depend on the *survival of one row* either.
* **I10** (outward-effect equivalence) and **I11** (authenticated completion consumption) inherit the failure because the decision they re-validate can be resurrected.

Nothing in I1–I8 conflicts with it. It adds a property none of them stated, so it is added as an invariant rather than folded into I3 or I9:

> **I12 — Deletion-monotonicity.** Every predicate that permits a consequence is monotone *decreasing* in the set of governance rows. Removing, corrupting or failing to read any row may leave permission unchanged or smaller; it may never enlarge it. In particular a predicate of the form "no negative record exists" is anti-monotone and is not admissible as evidence that anything is permitted.

Corollaries that fix the mechanism:

1. **Permission requires positive presence.** A decision or an authority generation grants something only while a sealed *liveness* record for exactly that decision or generation exists. Absence of the liveness record is refusal, exactly like absence of an authenticated seal is refusal for a completion (I11).
2. **Retirement is deletion of the positive record, ordered before the negative one.** Revoking or invalidating deletes the liveness record and then writes the revocation or invalidation. A crash between the two leaves the fact not live, which is the fail-closed direction and is completed by a retry. Deleting the negative record afterwards changes nothing.
3. **Admission is atomic with liveness.** The decision, generation or successor and its liveness record commit in one transaction (a decision's transaction also carries the revocation/invalidation fences). No replay is ever needed to create a liveness record, and an exact replay of a stored decision is therefore *never* allowed to recreate a retired one.
4. **Classification is derived from positive evidence.** Governance can only be *added* by classification (a safety-bearing Goal is under more governance than a legacy one), so it cannot be made monotone by a single positive row. It is instead re-derived from every surviving safety-bearing artifact of the Goal (the classification row, else any proposal or accepted-plan generation carrying the binding). A single-row deletion cannot un-classify a Goal while any such artifact survives, and an unreadable candidate row is an error rather than "unclassified".
5. **Unreadable is refusal.** Corrupt, undecryptable or mismatched governing rows fail the predicate; they never read as absent.

## 3. Adversary model

| Adversary | Capability | Repair 4 outcome |
|---|---|---|
| **A1 — keyless delete-only** | any number of `DELETE`s against the SQLite file; no key, no sealed rows other than those already present | **Covered.** No combination of deletions widens authority, un-classifies a Goal that has a surviving safety-bearing artifact, or resurrects a revoked or superseded fact. Verified by single-row and paired sweeps over every sealed row and every plaintext row of the fixture. |
| **A2 — keyless delete + replay of previously copied sealed rows** | A1, plus a byte-exact copy of an earlier state of the same installation's rows, re-inserted | **Residual, documented, reproducible** (`probes/residual_rollback_replay_test.go.txt`). This is a rollback of authenticated state. Closing it needs an anchor outside the database file that only moves forward (for example a monotonic counter in the OS keychain). That is a new architectural component, not a repair of a frozen predicate, and it is deliberately not built. |
| **A3 — total erasure of a Goal's history** | delete the classification row and every proposal, acceptance and generation of one Goal | **Residual.** With no surviving artifact there is no positive evidence left to derive classification from. Same anchor as A2. |
| **A4 — storage-key holder** | can seal any record | Unchanged accepted bootstrap trust root (OS user / installation keychain). Not addressed and not claimed. |

The accepted trust assumption is that A4 is trusted. Review #4 correctly held that treating *arbitrary database-file write access* as equal to A4 would nullify the purpose of authenticated records. Repair 4 therefore raises the floor to "A1 cannot widen anything" and states the A2/A3 boundary precisely instead of folding it into A4.

## 4. Frozen PRE-V4 semantics

No product semantics or authority are added. The liveness record is the implementation of the *existing* predicate "the decision or generation is current authority" (I3), which frozen PRE-V4 already required. The same authority gates, the same WorkPlan safety binding, the same ceremony, the same activation manifest and the same exact-JSON discipline apply. The storage key remains the trust root: liveness rows are sealed with the same envelope service and AAD as every other governance row.

Migration consequence, stated because it follows from the invariant: a decision or generation written by code that predates liveness records has no liveness record and is therefore not in force. An installation that predates this candidate obtains liveness records for its root through the governed root succession (which now writes them atomically) and obtains fresh decisions through the ordinary ceremony. Nothing is backfilled silently; a silent backfill would itself be an unauthenticated authority grant.

## 5. Equivalent-path search

Every place a governing fact is decided by absence was enumerated by searching for each store read whose *not found* branch permits progress (`ErrSecureBlobNotFound`, `IsSecureBlobRevokedInTx`, raw `SELECT COUNT(*)` fences), then classifying the fact.

| # | Governing fact | Consumer(s) | Was | Now |
|---|---|---|---|---|
| 1 | decision revoked | `LoadAuthorityDecision`, gate reconcile, work-plan acceptance bridge, controller drive, I11 completion re-verification | **absence ⇒ in force (N8)** | requires decision liveness; revocation retires it |
| 2 | generation invalidated / superseded | `ValidateAuthorityGeneration`, `ValidateAuthorityGenerationLineage`, in-transaction fences of `PutSecureBlobUnlessRevoked` / `PutSecureBlobsUnlessRevoked`, `IsSecureBlobRevokedInTx` (lifecycle repair guard) | **absence ⇒ live (N10)** | requires generation liveness at every link; one shared in-transaction predicate |
| 3 | current installation root | `loadInstallationRoot` ("exactly one root without an invalidation") | **absence ⇒ current, and deleting a successor plus the predecessor's supersession revives the predecessor (N11)** | requires root liveness; succession retires the predecessor and admits the successor in one transaction |
| 4 | Goal is safety-bearing | `Repository.Save`, proposal / review / acceptance / attachment layers, controller, runtime, materialization | **absence ⇒ legacy (N9)** | derived from surviving positive evidence; unreadable ⇒ error |
| 5 | revoked decision eligible for ordinary re-request | `verifyHistoricalReRequestPredecessor` | **deleting the revocation made a revoked decision look merely expired (N12)** | predecessor decision must still have its liveness record |
| 6 | publication authority still current | `CheckGoalsPublicationInvalidation`, recovery admission re-check | **absence of revocation / invalidation ⇒ current (N13)** | both additionally require liveness |
| 7 | delegations and ceilings | delegated child generations | child carries its own liveness, written atomically with the decision; ceilings are evaluated from the live parent chain | positive |
| 8 | activation requirement | `requireSafetyActivation` (binding carried by the plan/proposal), activation manifest file | positive (present ⇒ verify; absent ⇒ refuse) | unchanged, already monotone |
| 9 | evidence requirements / consent | completion seals, owner-ceremony evidence, request-to-decision binding | positive presence required | unchanged, already monotone |
| 10 | gate state | pending gate (no decision) vs decided | deleting a decision returns a gate to *pending*, which needs a fresh ceremony to act; never grants | unchanged; monotone |
| 11 | consequence eligibility | ledger completions, settlement, admission fence, activity (plaintext, unauthenticated by design) | absence facts | not authenticated by this repair; sweeps show no single-row deletion lets a fresh controller mint a completion after revocation, because every consequence they gate is re-established from sealed state at the point of effect (I10, I11) |

Rows 2, 3, 5 and 6 are the additional defects N10–N13. They share the boundary with N8/N9 and are repaired by the same invariant, not by separate fixes.

## 6. What is deliberately not claimed

* Universal mutation completeness. The inventory is defined over an enumerated universe; Review #4 rightly noted the previous universe *excluded* raw row deletion. The new universe includes it, and the exclusion is removed from the claims-not-made list only for the deletion (A1) case.
* Rollback resistance (A2) and erasure resistance (A3).
* Any change to the accepted storage-key trust root.

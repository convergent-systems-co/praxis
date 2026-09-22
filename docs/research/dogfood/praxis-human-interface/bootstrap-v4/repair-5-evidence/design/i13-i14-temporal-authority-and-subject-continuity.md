# Repair 5 design record: N14 and N15, the invariant set, and the architecture boundary

Written before any change to the frozen Repair 4 candidate. Nothing in the repository tree outside `repair-5-evidence/` was modified. The one source prototype lives in a scratch copy and is preserved here only as a patch (`prototype-n15-inarch.patch`).

## 1. Result in one paragraph

N15 as reproduced is closable inside the current architecture, and the closure is quantifiable: the Repair 4 classifier is fooled by **32 of the 256** subsets of the eight authenticated rows that evidence a Goal's safety governance, and a classifier that consumes *every* surviving authenticated lineage record is fooled by **exactly one** (the subset that deletes all eight). That one subset, and N14 itself, are the same object: **a consistent earlier state of the rollback domain**. Any predicate computed only from the SQLite file evaluates a restored earlier state exactly as it evaluated it then, so no database-local mechanism (liveness rows, sequence numbers, hash chains, state commitments, referential-integrity checks, further lineage consumption) can refuse it. Refusing it needs one bit of forward-only state that an attacker with write access to the file cannot roll back, and the frozen PRE-V4 architecture contains no such state. Choosing one is a consequential trust-domain decision that is not authorized. Disposition: **AUTHORITY_CONFLICT**.

## 2. The state/transition class N14 and N15 belong to

Let D be the contents of the SQLite file, the *rollback domain*. The adversary (Review #4/#5 capability, no storage key) can read D, copy any rows at any time, delete any rows, and insert any previously copied row byte-for-byte. Every governance predicate P the kernel evaluates is a function of D (plus wall-clock and process memory that does not survive restart).

| Class | Adversary action | Resulting D' | N-finding |
|---|---|---|---|
| R1 replay of a retired row | delete the negative record, re-insert the copied positive record | mixed-time state, equal to an earlier state **on every row the predicate reads** | N14 (Review #5) |
| R2 consistent snapshot restore | replace the file with an earlier copy | exactly an earlier honest state | N14 generalised |
| E1 selective erasure with surviving lineage | delete some evidence rows, other authenticated rows remain | a state that is *not* any honest earlier state (implies governance that its remaining rows contradict) | N15 as reproduced |
| E2 prefix truncation | delete the whole closure of rows written after time t | exactly the honest state at time t | N15 boundary |

R1 and R2 are one class for a predicate that reads a bounded row set: the adversary restores that row set to time t. E2 is R2 realised by deletion instead of by copying (append-only histories can always be truncated by deleting a suffix). E1 is the only class in which the adversary produces a state that no honest history produced, so it is the only class an in-domain predicate can detect.

## 3. Existing invariants violated

* **I3** (historical authority is not current authority): R1/R2 make a retired decision current again.
* **I10 / I11**: the restored decision authorises a new outward durable consequence, a new sealed completion, which then counts as effective and completes the Goal.
* **I9** (downgrade resistance): E1/E2 make a governed Goal read as legacy.
* **I12** (deletion-monotonicity): violated by E1 exactly as Review #5 says. I12 was stated over a fixed row set; the Repair 4 implementation consulted three of the eight rows that imply governance, so it was not monotone over the true evidence set. This is an **implementation gap in I12, closable**, not a gap in its statement.

## 4. Invariant set (derived, minimal)

**I12 (strengthened statement, unchanged intent).** *Every* authenticated record that implies a subject was governed is an input to the classification predicate, so permission is monotone decreasing over the whole Goal-attributable evidence set, not over the subset a particular implementation chose to read. Closable in-architecture; see §6.

**I13 — Temporal authority (anti-rollback).** Once authority or classification has been retired, no earlier authentic state becomes executable merely because it is replayed or restored. Authenticity (origin and integrity) is not currency.

**I14 — Subject-governance continuity.** Loss of the currently enumerated governance evidence must not make a previously governed subject indistinguishable from a never-governed subject when surviving authenticated evidence establishes that governance existed.

Relationship: I13 and I14 are both consequences of one principle, *currentness and continuity cannot be inferred from the rollback domain alone*. I12 is complemented, not superseded: I12 (over the full evidence set) is the strongest thing a predicate can satisfy **inside** the domain; I13/I14 state what is required **beyond** it. I14 is satisfiable in-domain only to the boundary E2 (§5); I13 is not satisfiable in-domain at all.

## 5. Why the boundary is not a matter of effort (executed, not only argued)

**Argument.** Let A be any in-domain currentness or continuity predicate (a liveness row, a per-decision sequence number, a governance hash chain, a Merkle or sealed state commitment, a referential-integrity rule, a "seen" high-water row written by consumers). A reads D only. The adversary restores a copy of D taken at time t (or, equivalently for a bounded row set, the rows A reads). Then A(D') = A(D_t). A honest system at time t accepted, so A accepts. A predicate that must refuse D' cannot be a function of D.

**Evidence.**

* `probes/boundary_probes_test.go.txt` **P1**: a byte-consistent earlier copy of the database, restored over the current one, leaves *no trace of the revocation anywhere in the database* (`SELECT count(*) FROM secure_blobs WHERE namespace='authority_revocation'` is 0) and the decision loads. Log: `probes/boundary-probes-repair4-candidate.log` and, on the in-architecture prototype, `probes/boundary-probes-inarch-prototype.log` (same result).
* **P2**: deleting the whole Goal-attributable evidence closure leaves an honest pre-governance state; `GoalSafetyClassified` is false **even on the prototype that consumes every surviving lineage record**.

**Evaluated and rejected as in-domain "fixes" for N14** (each raises the number of rows the adversary must copy, none changes the capability class): per-decision sequence numbers; a governance hash chain; a sealed state-root row updated by every transition; consumer "high-water" rows; tombstones instead of deletion; linking liveness rows to a second row rewritten at each retirement. For N14 the adversary already needs one copied row; each of these makes it two or three (the old root or head, the old liveness row), and copying rows is free for a writer of the file. Presenting any of them as closing N14 would be false.

**What does not help either.** Process memory (a controller can remember retirements it observed, but N14 requires a restart, and "seen" state must persist somewhere). SQLite triggers, `secure_delete`, schema tricks and file permissions live in or beside the same file the adversary writes. The rowid of a reinserted row is chosen by the adversary.

**What an anchor would have to be.** State that (a) a writer of the SQLite file cannot roll back, (b) moves only forward, and (c) is consulted by the kernel at every consumption of governed authority and every classification decision. The frozen PRE-V4 kernel contains none: the storage key is held in the macOS Keychain (`internal/crypto/keychain_darwin.go`; no first-party backend is registered on other platforms, `bootstrap_platform_other.go`) but as a static secret, the bootstrap record is a static file, the activation manifest is a static file, and the frozen plan states no adversary model for database rollback (`plans/workplan-v4-codex.md` §9 and §14 leave the deployment threat model open).

## 6. What can be closed inside the existing architecture

**N15, E1: closable.** `GoalSafetyKernel` must treat every authenticated record that implies the Goal was put under the kernel as positive evidence, not three. The in-architecture prototype (patch preserved) additionally consults: work-plan reviews and acceptances (each embeds the proposal and the accepted plan with their `safety` bindings), protected authority requests and decisions for the Goal (`ceremony_profile` set, which frozen plan §9 items 2–4 make mandatory for kernel requests), and completion seals (object id `goal/version`). Result over the full powerset of the eight evidence rows in the gate fixture:

| classifier | subsets that make the governed Goal read as legacy |
|---|---|
| Repair 4 candidate | **32 / 256** (minimal: `class+proposal+baseline2`) |
| in-architecture prototype | **1 / 256** (`all eight`, i.e. E2) |

(`probes/evidence-powerset-repair4-candidate.log`, `probes/evidence-powerset-inarch-prototype.log`.) The preserved Review #5 N15 probe is refused by the prototype (`probes/review5-probes-inarch-prototype.log`); the existing goalstore, goaldrive and cmd/praxis suites pass on it. This is **not** a claim that N15 is repaired: the last subset is the boundary.

Further in-architecture hardening found by the equivalent-path search, not exploited in Review #5, recommended for the eventual repair:

* `safetyBindingForRequest` returns `nil, nil` when the referenced proposal is absent ("does not exist yet"); a *protected* request whose binding cannot be resolved should be refused rather than proceed without an activation check.
* Reads whose *not found* branch means "pending" or "no revocation" were audited in Repair 4; no further absence-as-permission read was found.

**N14: not closable in-domain** (§5). No source change is proposed for it, because none would change what a copy-capable adversary can do.

## 7. Equivalent-path search

Every path in the requested list was walked against the source at the frozen candidate. "In-arch" means closable inside the current architecture; "anchor" means closable only with forward-only state outside the rollback domain.

| Path | Reading of the source | Disposition |
|---|---|---|
| authentic stale state regains current authority | retirement is deletion of `authority_decision_live` / `authority_generation_live` (decision revocation, generation invalidation, root succession); each is replayable | **anchor** (N14) |
| deletion converts governed state into legacy/default permission | `GoalSafetyKernel` reads 3 of 8 implying rows; `safetyBindingForRequest` not-found → nil | classification: **in-arch to boundary E2**, then **anchor**; binding default: **in-arch** |
| rollback changes currentness | every predicate is a function of D | **anchor** |
| restart changes authority interpretation | no persistent cache of decisions; each load re-derives from D; `qualifiedTrees` is process-global (documented residual, lost on restart, recompute-and-compare inside a run applies) | no new path; restart is what makes R1 durable |
| partial surviving evidence ignored | N15 (3 of 8 rows consulted) | **in-arch** |
| recovery / publication / import / execution disagree about temporal authority | all use the same liveness predicate since Repair 4; they agree, and all are defeated together by R1 | not a disagreement; **anchor** |
| a previously governed identity loses its governed marker | classification row, baseline plan pointer, request `ceremony_profile` | **in-arch** to E2 |
| two individually fail-closed rules compose into permission | "missing classification → derive" + "no source → legacy" (N15); no other pair found | **in-arch** to E2 |
| replay of authentic historical rows | liveness rows; ceremony evidence, seals, requests and decisions are positive-only and immutable (replay is idempotent) | liveness: **anchor** |
| multi-row selective erasure | powerset over the 8 evidence rows | **in-arch** to E2 |
| rollback of part of a lineage / of mutually consistent subsets | a consistent subset equals an honest earlier state on the rows read | **anchor** |
| stale-but-valid envelopes | envelope AAD binds namespace, id, version and digest, not time | **anchor** |
| cross-generation / cross-Goal substitution | AAD binds identity; attach binds Goal identity (R1-G, Repair 3); seals bind `goal/version` | closed (Repair 3/4) |
| cache versus durable currentness | no decision cache | none |
| split-snapshot observations | root generations and liveness are read in one statement since Repair 4 | closed |
| ordinary re-request | requires the predecessor's liveness (N12) | inherits N14 |
| legacy/default fallbacks | as above | as above |
| combinations where no single deletion suffices but the set does | powerset | **in-arch** to E2 |

## 8. Semantic coverage boundary of the adversarial inventory

The Repair 4 mutation inventory (192 mutations) is unchanged and remains valid only for the frozen source; no source change was made, so it was not re-run. This repair adds **exhaustive enumeration over the finite equivalence class that matters** rather than a mutation count: the powerset over the eight Goal-attributable evidence rows (256 states, all executed, keyless SQL only, with restart). It is exhaustive for "which combinations of deletions of Goal-attributable governance rows turn a governed Goal into a legacy one". It is **not** a powerset over the whole database (about thirty sealed rows plus ledger tables, 2^30 and beyond) and does not claim rollback coverage: rows that are not Goal-attributable evidence cannot change this Goal's classification, and rollback is characterised by §5, not enumerated.

## 9. Decisions

The smallest decision is one question:

> **D1. Must the PRE-V4 safety kernel defend against an adversary who can restore previously valid database state (rollback, including prefix truncation), or is that outside the adversary the kernel is required to withstand?**

If the answer is *outside*, no anchor is needed; if *inside*, D2 follows.

| Alternative | What it changes | What it does not |
|---|---|---|
| **A. Rollback and prefix truncation are outside the defended adversary.** Apply the in-architecture N15 closure (32→1) and the binding default; state the boundary in the frozen plan's threat model and in every claim. | No new component. I13/I14 are recorded as *not required*; I3/I9 claims are scoped to "no in-domain adversary short of restoring earlier state". N14 remains reproducible and is documented as accepted. | Does not close N14 or E2. Review #6 must accept the scope; Review #5 stated that acceptance does not follow from difficulty. |
| **B. A forward-only epoch held in the OS Keychain.** Same trust domain as the storage key (the frozen OS-user/keychain root); the adversary of this review (SQLite writer without the keychain) cannot roll it back. Every retirement, classification and admission advances it; consumers refuse a database whose epoch is behind. | Closes R1, R2 and E2 for the stated adversary. New: a Keychain item and its lifecycle, a governed re-anchor procedure for legitimate backup restore, a two-step advance ordering (advance before commit), fail-closed behaviour when the item is unavailable. **darwin only**: no first-party backend exists elsewhere. | Does not resist a holder of the Keychain item (already the accepted A4 root). Does not resist deletion of the Keychain item itself, which must fail closed. |
| **C. Fresh owner ceremony at consumption.** A per-consumption interactive challenge whose nonce lives only in process memory, so a replayed database cannot supply it. | Needs no stored anchor. Changes product semantics: unattended controller consumption of a gate decision becomes interactive. | Human availability at every consumption; not compatible with the frozen unattended drive. |
| **D. External witness (Git remote, transparency log, third-party service).** | Closes R1/R2/E2 given the witness. New trust service and availability dependency. | Requires a remote to exist for every Goal; not authorised. |
| **E. A local file witness outside the database.** | Trivial to build. | A writer with the OS-user access needed to alter the database can alter it as well; closes no more than an attacker who happens to touch only the file. Not recommended; recorded for completeness. |

Recommendation (not a decision): apply the in-architecture N15 closure and the binding default regardless of D1, because they are within authority and strictly reduce the evidence the adversary must erase; then decide D1. If D1 is *inside*, alternative B is the only one supported by evidence that stays within the existing OS-user/keychain trust domain, and D2 is whether darwin-only is acceptable.

## 10. State of the tree

The frozen Repair 4 candidate is untouched: source manifest, qualification results, activation requirements, candidate binary and package identities are unchanged and still verify (`verify/preactivation_evidence.py` PASS). Nothing was committed, pushed, installed, deployed or activated. Proposal v4 is unmaterialized; Gates A, B and C are undecided.

# Repair #5 terminal disposition: AUTHORITY_CONFLICT

Review #5 (`../../reviews/bootstrap-v4-kernel-astra-review-5.md`, SHA-256 `a5b3443a24de86f434332b77cd263a8a872747ce98a18ebc7c47ca1b4420e9d1`) found N14 (authentic stale liveness becomes current authority) and N15 (selective erasure produces legacy permission).

## Why this is an AUTHORITY_CONFLICT and not a repair

* Both preserved Review #5 probes were reproduced unchanged on the frozen Repair 4 candidate first (`probes/review5-probes-baseline-RED-before-repair5.log`).
* N15 as reproduced **is** closable inside the current architecture, and a prototype shows how far: the powerset over the eight Goal-attributable evidence rows goes from **32 of 256** subsets that fool the classifier to **1 of 256**. The remaining subset, and N14 itself, are a *consistent earlier state of the SQLite file*.
* No predicate that reads only that file can refuse a consistent earlier state (executed boundary probes P1, P2 in `probes/`; argument in `design/i13-i14-temporal-authority-and-subject-continuity.md` §5). Refusing it needs forward-only state outside the rollback domain. The frozen PRE-V4 architecture has none, and choosing one is a consequential trust-domain decision I am not authorised to make.

## The smallest decision

> **D1. Must the PRE-V4 safety kernel defend against an adversary who can restore previously valid database state (rollback, including prefix truncation), or is that outside the adversary the kernel is required to withstand?**

Alternatives A to E, what each changes, and the evidence for each are in `design/…` §9. Nothing was chosen.

## Not done, by design

The frozen Repair 4 candidate was not modified: no source change was made to the repository tree, so its identities, qualification and mutation inventory remain exactly as reviewed, and `verify/preactivation_evidence.py` still passes. The in-architecture N15 prototype exists only as `prototype-n15-inarch.patch` (against `internal/goalstore/safety_classification.go`; **not applied, not qualified, no regression tests, no mutation coverage**). Nothing was committed, pushed, installed, deployed or activated. Proposal v4 is unmaterialized; Gates A, B and C are undecided; the active core `807359e4…48fe7` is unchanged.

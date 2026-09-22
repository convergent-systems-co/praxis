# Repair 5 evidence

Terminal disposition: **REPAIR_5_QUALIFIED** (a frozen PRE-ACTIVATION candidate, ready for Astra Review #6). History: the repair first ended in **AUTHORITY_CONFLICT** (`AUTHORITY_CONFLICT.md`, preserved unchanged); Thomas resolved it with decision **D1** (rollback and prefix-truncation of the governance store are inside the defended adversary; a minimal forward authority anchor outside the store is authorised, macOS Keychain first, no other trust infrastructure), and the repair continued from the preserved evidence. After the first qualification Thomas directed a follow-up: harden the Keychain item's access control so unauthorized mutation is not silently permitted, without weakening the governed re-anchor or adding another recovery or authority path. That is implemented (design §7.1), the qualification was re-run on the modified source, and the pre-hardening qualification logs, inventory and report are preserved under `qualification/superseded-pre-acl-hardening/`.

Target review: Astra Review #5 (`../../reviews/bootstrap-v4-kernel-astra-review-5.md`, SHA-256 `a5b3443a24de86f434332b77cd263a8a872747ce98a18ebc7c47ca1b4420e9d1`; evidence `../../reviews/bootstrap-v4-kernel-astra-review-5-evidence/`).

| Path | Content |
|---|---|
| `AUTHORITY_CONFLICT.md` | the earlier terminal disposition and the single decision D1 (unchanged) |
| `design/i13-i14-temporal-authority-and-subject-continuity.md` | the preserved analysis: state/transition classes, the impossibility argument, in-architecture closure, alternatives A–E (written before D1; unchanged) |
| `design/forward-authority-anchor.md` | the authorised architecture: invariants I13/I14, the contract, facts, the atomicity/crash matrix, backup/re-anchor semantics, the per-consumer search, the measured Keychain semantics and residuals, the coverage boundary |
| `probes/` | Review #5 probes replayed **unchanged** before (RED baseline) and after the repair; boundary probes, the 256-subset powerset (Repair 4 candidate, in-architecture prototype), the Keychain cross-process access probe (first design) and `probes/acl-hardening/` (ACL hardening: item-ACL and data-protection measurements, the dedicated-keychain measurements, and the foreign-process attempts against the real hardened implementation), with overlays; `PASS` in a probe log means the counterexample/boundary reproduced |
| `prototype-n15-inarch.patch` | the earlier in-architecture N15 prototype (superseded by the implemented registry; kept as evidence) |
| `qualification/` | final qualification logs, preserved Review #2/#3/#4/#5 replays, the base-vs-candidate stale-attestation scans, the flaky-test remeasurement, the powerset/mixed-snapshot artifact log |
| `mutation/` | harness, catalogue, inventory builder, `muts.json` and every counted result set |
| `mutation-inventory.md` / `.json` | the consolidated guard/mutation inventory with the Repair 5 forward-authority-anchor guards |

Regeneration order (source frozen, artifacts built): `../verify/rebuild_source_manifest.py` -> `../verify/regenerate_candidate_evidence.py` -> `../verify/preactivation_evidence.py`.

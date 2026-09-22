# Repair 6 evidence

Terminal disposition: **REPAIR_6_QUALIFIED — PRE-ACTIVATION** (a frozen review candidate, ready for a fresh independent Astra Review #7). Target review: Codex Review #6 (`../../reviews/bootstrap-v4-kernel-codex-review-6.md`, SHA-256 `aacd65d5d688c1fa6671880ae94583806f64465899044c8dce5f49d4bf6f2d95`; evidence `../../reviews/bootstrap-v4-kernel-codex-review-6-evidence/`), finding **N16**.

| Path | Content |
|---|---|
| `design/rekey-lifecycle.md` | the repair: re-key on every advance and undo, the pending-password protocol, the crash/failure matrix, the `SecKeychainChangePassword` dependency stated as far as the evidence goes, the corrected R-K2 |
| `mutation/` | harness, catalogue (304 mutations: 279 carried and re-anchored + 25 Repair 6), `muts*.json`, every counted result set, run logs, `build_inventory.py` |
| `mutation-inventory.md` / `.json` | the consolidated inventory: 274 killed, 30 classified survivors, 0 unexplained, with re-run notes |
| `probes/replay_review_probes.py`, `probes/image_probe_replay.py` | replay of every preserved review probe (Reviews 2-6) and Review 3's process-image A/B drive, unchanged, with outcome-set comparison |
| `qualification/` | `run_qualification.sh`, every log the qualification record is generated from, `probe-replays/`, `rebuild.log` |
| `superseded-repair-5-candidate/` | the Repair 5 final candidate, byte-exact and re-verified against Review #6's identity table: core, plugin, package archive and manifest, source manifest, qualification results, activation requirements, pre-activation verification |
| `superseded-implementation-and-qualification-report-repair5.md` | the Repair 5 report, reconstructed by reversing Repair 6's two in-place edits; its SHA-256 equals Review #6's recorded `d5e674b3…` |

Regeneration order (source frozen): `../verify/rebuild_source_manifest.py` -> build core, plugin (`go build`) and package (`../verify/build_goals_package.go`) -> `qualification/run_qualification.sh` -> the two probe scripts -> `../verify/regenerate_candidate_evidence.py` (refuses to write a result its logs do not support) -> `../verify/preactivation_evidence.py`.

Earlier evidence (`../repair-2-evidence` ... `../repair-5-evidence`) is preserved unchanged.

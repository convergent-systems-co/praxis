# Repair 7 evidence

Terminal disposition: **REPAIR_7_QUALIFIED — PRE-ACTIVATION** (a frozen review candidate, ready for a fresh independent Astra Review #8). Target review: Astra Review #7 (`../../reviews/bootstrap-v4-kernel-astra-review-7.md`, SHA-256 `d1fd04448200cd13fb79716b0ea14f988e053966538a125699bb13393771d20c`; evidence `../../reviews/bootstrap-v4-kernel-astra-review-7-evidence/`), finding **N17**.

| Path | Content |
|---|---|
| `design/generation-currentness.md` | the invariant, the repair, the equivalent-path audit (G1-G7) and its RED-first verification, the recurrence guard, residuals |
| `mutation/` | harness, catalogue (364 mutations: 304 carried and re-anchored + 60 Repair 7), `muts*.json`, every counted result set, run logs, `build_inventory.py` |
| `mutation-inventory.md` / `.json` | the consolidated inventory: 318 killed, 46 classified survivors, 0 unexplained, with re-run notes |
| `probes/replay_review_probes.py`, `probes/image_probe_replay.py` | replay of every preserved review probe (Reviews 2-7) and Review 3's process-image A/B drive |
| `qualification/` | `run_qualification.sh`, `focused.log`/`vet-all.log`/`diffcheck.log`/`rebuild.log`, `red-n17-unfixed-resolver.log` (RED baseline), every other log the qualification record is generated from |
| `superseded-repair-6-candidate/` | the Repair 6 final candidate, byte-exact and re-verified against Review #7's identity table: core, plugin, package archive and manifest, source manifest, qualification results, activation requirements, pre-activation verification, and its full report |

Regeneration order (source frozen): `../verify/rebuild_source_manifest.py` -> build core, plugin (`go build`) and package (`../verify/build_goals_package.go`) -> `qualification/run_qualification.sh` plus a focused/vet/diffcheck/rebuild capture -> the two probe scripts -> `../verify/regenerate_candidate_evidence.py` -> `../verify/preactivation_evidence.py`.

Earlier evidence (`../repair-2-evidence` ... `../repair-6-evidence`) is preserved unchanged.

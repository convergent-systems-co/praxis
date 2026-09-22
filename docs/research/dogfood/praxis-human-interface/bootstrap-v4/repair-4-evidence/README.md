# Repair 4 evidence

Independent-review target: Astra Review #5. Repairs Astra Review #4 (`../../reviews/bootstrap-v4-kernel-astra-review-4.md`, SHA-256 `71ed5bc796749fe48f71f78f4cff637cc9cdd5293bc05d974ff589d97657c8a6`; preserved evidence `../../reviews/bootstrap-v4-kernel-astra-review-4-evidence/`).

| Path | Content |
|---|---|
| `design/i12-deletion-monotonicity.md` | the invariant behind N8 and N9 (I12), its reconciliation with I1–I11 and frozen PRE-V4, the adversary model (what is and is not covered), and the equivalent-path matrix; written before the guards were extended beyond the two probes |
| `probes/` | the Review #4 probes replayed **unchanged** (log), and the documented-residual probe (delete + replay of a copied sealed row) with its overlay; `PASS` there means the residual reproduces |
| `qualification/` | final qualification logs (focused, `make test-current`, race, vet, Python, specification verifiers, historical conformance and the base-vs-candidate stale-attestation scans, diff check), the preserved Review #2, #3 and #4 probe replays including the A/B process-image drive, and the flaky-test measurement |
| `mutation/` | harness, catalogue, inventory builder, `muts.json` and every result set that is counted; the inventory now includes a *raw row deletion* universe entry, which Review #4 correctly noted the earlier inventory excluded |
| `mutation-inventory.md` / `.json` | the consolidated guard/mutation inventory: universe, exclusions, counts, every non-killed mutation with its class and reason |

Regeneration order (source frozen, artifacts built): `../verify/rebuild_source_manifest.py` -> `../verify/regenerate_candidate_evidence.py` -> `../verify/preactivation_evidence.py`. Review probe sources are the preserved, unmodified `.txt` files under `../../../reviews/` (and `probes/` here); replays run them through `go test -overlay` and never modify the repository.

The Review #3 evidence, Repair 3 evidence (`../repair-3-evidence/`) and every earlier attestation are unchanged.

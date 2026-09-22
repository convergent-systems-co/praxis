# Repair 3 evidence

Independent-review target: Astra Review #4. Repairs Astra Review #3 (`../../reviews/bootstrap-v4-kernel-astra-review-3.md`, SHA-256 `0ce799f512a1fac53499aa1c1f36bc4bd7349ab20ec3a1ba40fcc6114915fd92`).

| Path | Content |
|---|---|
| `design/invariant-boundary-matrix-premap.md` | the invariant -> finding -> boundary map written BEFORE any source edit |
| `qualification/` | final qualification logs (focused first run with the pre-existing flake and its re-run, `make test-current`, race, vet, Python, specification verifiers, historical conformance, diff check), preserved Review #2 and #3 probe replays (including the A/B process-image drive), the base-vs-candidate stale-attestation scans (identical, 1,236 entries), and `flaky-test-measurement.txt` |
| `mutation/` | harness (`harness.py`), catalogue (`catalogue.py`), inventory builder (`build_inventory.py`), `muts.json`, and every pass: exploratory partial run, first full pass, targeted pass, the final main pass and the final targeted re-run. Only `final-main-results.json` + `final-rerun-results.json` are counted (see the inventory) |
| `mutation-inventory.md` / `.json` | the consolidated guard/mutation inventory: universe, exclusions, counts, every survivor with its class and reason |
| `fork-reports/` | unedited reports of the two forked implementer sessions (N3; N4/N5/N6) and the N4/N5/N6 fork's own mutation log |

Regeneration order (source frozen, artifacts built): `verify/rebuild_source_manifest.py` -> `verify/regenerate_candidate_evidence.py` -> `verify/preactivation_evidence.py`. The Review #2 and #3 probe sources are the preserved, unmodified `.txt` files under `../../../reviews/`; replays run them through `go test -overlay` and never modify the repository.

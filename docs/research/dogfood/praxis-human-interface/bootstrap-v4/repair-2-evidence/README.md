# Second-repair evidence

Raw outputs for the repair of the second independent review
(`../../reviews/bootstrap-v4-kernel-codex-review-2.md`, SHA-256
`c2704e9d12ac6fd1efd515755a9afde2716fa3c19e1823e73b2095c171d2eb30`). The preserved
review evidence directory is unchanged.

| File | Content |
|---|---|
| `focused.log`, `test-current.log`, `race.log`, `vet.log`, `vet-all.log`, `python.log`, `specification-bundle.log`, `historical.log`, `preactivation-verifier.log` | final qualification runs on the frozen source |
| `independent-probe-replay.log`, `independent-probe-replay-summary.log` | the preserved review probes replayed through overlays; `FAIL` means the counterexample no longer reproduces, `PASS` is the retained positive control |
| `cmd_praxis_probe-as-replayed.go.txt`, `internal_goaldrive_probe-as-replayed.go.txt` | the scratch copies actually replayed, adapted only for `fileSafetyActivation` and the `settleCompletion` argument |
| `image-replacement-replay.log` | the reviewer's real process-image probe replayed against the repaired verifier |
| `mutation-checks.log` | each enforcement was disabled in turn; `KILLED` means its regression failed |

The historical conformance failure is reported truthfully and no attestation was edited.

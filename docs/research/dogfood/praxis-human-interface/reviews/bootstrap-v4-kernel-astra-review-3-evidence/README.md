# Third-review independent reproduction evidence

Companion to `../bootstrap-v4-kernel-astra-review-3.md`. Nothing here modifies the repository: probe sources are stored as `.txt`
and were run through `go test -overlay` (the overlay files map a scratch path to a temporary `*_test.go` name inside the
package; the original overlay JSONs contained absolute scratch paths and are reproduced under `qualification/replay-overlay.json`
as the pattern). To replay, copy a `.txt` probe to a scratch file, write an overlay `{"Replace":{"<repo>/<pkg>/zz_probe_test.go":"<scratch file>"}}`,
and run `go test -overlay=<overlay> ./<pkg> -run <TestName> -count=1 -v`. `PASS` on a probe named R1*/R2* usually means
the counterexample was REPRODUCED (the probe asserts the defect); read each log line's `REPRODUCED`/`REFUSED` text.

| Dir | Content |
|---|---|
| `r1-authority/` | R1-A/B/C (Safety-strip persists through `Save` and CLI import = N1), R1-D revocation refusals, R1-E ceremony mismatch matrix, R1-F storage-key holder forges ceremony (trust boundary), R1-G cross-Goal attach, R1-H forged gate completion (N2). `probes-run.log` is the lead reviewer's re-run. |
| `r2-execution/` | goal-drive probes: F1 completed gate not re-verified (N2), F2 assume-unchanged/skip-worktree (N5), F3 output bound + surviving child (N6), gate-dispatch variants (I7, no counterexample), pending-gate/explicit-objective observations, `e2e-pathA-run.log` (Path A end to end, with `bootstrap_v4_repair_modified_test.go.txt`), 21-mutation replay (`mutate.py`, `mutation*.json`, `mutation.log`). |
| `r3-activation/` | A/B process-image probes (N3), exact-JSON probe (N4), publication-window probe, independent bundle recomputation, pre-activation recomputation, plugin/package notes, fork-3 area report. |
| `qualification/` | `make test-current`, race, vet, Python, diff-check, historical conformance logs; base-versus-candidate stale-attestation scan (`stale-base.txt`, `stale-cand.txt`, `zz_scan_test.go.txt`); replay of the implementer's preserved review-2 probes (`preserved-probe-replay.log`). |

Probe executables, Go caches, scratch database fixtures, and a full rsync copy used for mutation runs are not preserved.

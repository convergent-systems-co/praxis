# Second-review evidence

The `*_review_test.go.txt` files are independent Go tests loaded through an overlay; they do not replace implementation or existing test files. They reuse supplied fixtures for temporary encrypted installations. Fixture activation checks substitute executable/VCS verification because a Go test executable lacks the required build identity. Manifest bytes, plan binding and package identity still use production checks. The separate `image_probe.go.txt` executable uses the real process-image verifier.

From the reviewed repository, replay with:

```sh
python3 docs/research/dogfood/praxis-human-interface/reviews/bootstrap-v4-kernel-codex-review-2-evidence/run_probes.py "$PWD"
```

The runner creates a temporary Go cache, overlay files, test databases and probe executables. It does not change the repository's source, active installation, or candidate artifacts. It requires the reviewed Go dependencies and toolchain. A repaired implementation should make the corresponding counterexample assertion fail; these are reproduction tests, not desired-behavior regression tests.

`probes-final.log` preserves the final independent fixture run. `image-probe.log` preserves the separately executed actual image-replacement demonstration. `current.log` records the initial sandbox cache failure; `current-retry.log` records the successful writable-cache run. `race.log`, `vet.log`, `python.log`, and `historical.log` preserve the other qualification outputs. An empty `vet.log` accompanies successful exit 0. `results.json` records outcomes and identities.

The reviewed production core was independently rebuilt in temporary storage and matched SHA-256 `45b9a89c1a318c7db4dc899ddf944f657b392dd03e74e9abbdef74d763a96a57`. Probe executables and test databases are not candidate installation artifacts and are not included here.

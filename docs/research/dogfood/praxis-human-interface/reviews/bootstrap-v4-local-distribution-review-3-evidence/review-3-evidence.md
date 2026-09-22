# Review 3 evidence — L3 deterministic containment interleaving

Reviewed repair: `23766e1a95b6e5b99287a53cfda832bdf222a158`; HEAD: `94e17831e6e063a39536c9f2af9e77d04f130f12`.

Scratch clone (removed after review): `/private/tmp/praxis-local-review3.mewZGR`, detached at `23766e1`. Reviewer-only probe: `internal/distribution/reviewer_l3_race_test.go`.

The successful probe created a regular in-root `root/pivot/pkg/1/artifact.tar.gz` containing `inside` and an external `outside/pkg/1/artifact.tar.gz` containing `OUTSIDE`. It then executed: successful `noSymlinksBelow(root, path)`; rename `root/pivot` to `root/saved`; symlink external directory at `root/pivot`; then `os.ReadFile(path)`. The read returned `OUTSIDE`.

Exact command/result:

```
go test -count=1 ./internal/distribution -run '^TestReviewerCheckThenOpenInterleaving$' -v
=== RUN   TestReviewerCheckThenOpenInterleaving
--- PASS: TestReviewerCheckThenOpenInterleaving
PASS
```

The test PASS is positive evidence of escape: it succeeds only when the post-check pathname read returns external bytes. This exactly models `readContainedFile`'s Lstat walk followed by unanchored `os.ReadFile`, with adversarial mutation between them.

Delta inspection:

```
git diff --quiet 9ad5731..23766e1 -- internal/packagecatalog/verification.go internal/goalspublication/acquisition.go internal/distribution/github_releases.go
# exit 0: no differences
```

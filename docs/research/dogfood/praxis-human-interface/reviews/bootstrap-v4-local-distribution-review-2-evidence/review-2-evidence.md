# Review 2 evidence — LocalFirstParty repair `9ad5731`

## L2 scratch-only reproduction

Scratch clone: `/private/tmp/praxis-local-review2.EeXu0A`, detached at `9ad57319f50fc19854b6671da9e8be09d8902ecc`. Reviewer-only file: `internal/distribution/reviewer_local_containment_test.go` (not present in the candidate).

The probe:

1. creates `root := t.TempDir()` and separate `outside := t.TempDir()`;
2. writes a correctly pinned `alias/pkg@1` package fixture only below `outside`;
3. creates `root/alias -> outside/alias` as a directory symlink;
4. invokes `LocalFirstParty{Root: root}` with local ref `alias/pkg`.

Exact command and result:

```
go test -count=1 ./internal/distribution -run '^TestReviewerSymlinkedPackageDirectoryEscapesRoot$'
ok github.com/convergent-systems-co/praxis/internal/distribution
```

The test treats success of `Resolve`, `Info`, and `ResolveLocked` as the observed escape, so its PASS is positive evidence of L2. The adapter's `Abs`/`Rel` predicate accepted lexical `root/alias/pkg/1`; `os.ReadFile` followed `root/alias` to the external fixture.

Focused regressions:

```
go test -count=1 ./internal/distribution ./cmd/praxis -run 'Test(LocalFirstPartyRefusesPathTraversalEvenWithMatchingManifestID|ParsePackageDeployRef.*Local|LocalUnsignedPackageStillFailsVerification|LocalInvalidSignatureStillFailsVerification|LocalTransportDoesNotChangeVerificationOutcome|ResolverVerifiesImmutableTransitiveLocksAcrossCatalogTransports|ParsePackageDeployRefGitHubBehaviorUnchanged)$'
ok github.com/convergent-systems-co/praxis/internal/distribution
ok github.com/convergent-systems-co/praxis/cmd/praxis
```

## Independent clean builds

Two fresh detached clones at `9ad5731`, with external output and distinct fresh `GOCACHE`:

```
git clone --no-local --quiet . /private/tmp/praxis-local-build-a.*
git clone --no-local --quiet . /private/tmp/praxis-local-build-b.*
git -C <clone> checkout --quiet 9ad57319f50fc19854b6671da9e8be09d8902ecc
GOCACHE=/private/tmp/praxis-local-cache-a.* <clone-a>/docs/research/dogfood/praxis-human-interface/bootstrap-v4/verify/build_candidate_artifacts.sh /private/tmp/praxis-local-out-a.*
GOCACHE=/private/tmp/praxis-local-cache-b.* <clone-b>/docs/research/dogfood/praxis-human-interface/bootstrap-v4/verify/build_candidate_artifacts.sh /private/tmp/praxis-local-out-b.*
```

Independent output shown by the first completed build:

```
core              5b2d3cbfd1d6f0e1b7c57c0b3b8f326b1303470efd2d70140d13917aec4bdbac
plugin            5829df4fbb3812c66e4440d946a033ecd7d3648a30301167469357c669d767a4
package archive   b59415c360a426b50e8331e9bf3e1968840c40abf61991d5bd317fd46700a255
package manifest  7dd37cef062fe46f2b4859cfc5e12fd286dbde26f3c69dbf74cc7e474c36732f
```

`go version -m` core contains `vcs.revision=9ad57319f50fc19854b6671da9e8be09d8902ecc` and `vcs.modified=false`; `strings` found zero instances of its scratch checkout path.

The second independently built core at `/private/tmp/praxis-local-out-b.t0gyxx/praxis-candidate` has the same SHA-256 as `/private/tmp/praxis-local-out-a.GRTAjm/praxis-candidate`; `cmp -s` returned success.

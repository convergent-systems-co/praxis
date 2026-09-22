# Astra Review #10 evidence

## Commands and paths

All commands were run 2026-09-22 on the reviewed Darwin arm64 host.

```
git clone --no-local --quiet . /private/tmp/praxis-review10-c.ZnTwB8
git clone --no-local --quiet . /private/tmp/praxis-review10-d.TYOSp3
git -C <clone> checkout --quiet 25c73305fbca532e6b402b9d4aa1ed3412305169
GOCACHE=/private/tmp/praxis-review10-cache-c.* \
  /private/tmp/praxis-review10-c.ZnTwB8/docs/research/dogfood/praxis-human-interface/bootstrap-v4/verify/build_candidate_artifacts.sh \
  /private/tmp/praxis-review10-out-c.UgyRe9
GOCACHE=/private/tmp/praxis-review10-cache-d.* \
  /private/tmp/praxis-review10-d.TYOSp3/docs/research/dogfood/praxis-human-interface/bootstrap-v4/verify/build_candidate_artifacts.sh \
  /private/tmp/praxis-review10-out-d.yQcWAF
```

Both output directories were outside their worktrees, avoiding a generated unignored file changing the Go VCS-status input mid-build. SHA-256 and byte comparison:

```
42cb404ff30b77b375505a2593f5541b9be63587b39e87c1792d9231851de821  core C = core D
08514f2078c0e4a7a9093e41b4a78ec72069f7053a6e5bf670ff426ec1888829  plugin C = plugin D
06e0ebaea3c0ad549d7efdf37f9894283360b36b9dfa4ccf8fa9e0a45d96cc62  archive C = archive D
e8894b79d8e7589cff37e98f93e31e22adb7fba885a02c19ac46156e534111d2  manifest C = manifest D
cmp: all four pairs equal
```

`go version -m` of independently built core: `CGO_ENABLED=1`, `GOOS=darwin`, `GOARCH=arm64`, `vcs.revision=25c73305fbca532e6b402b9d4aa1ed3412305169`, `vcs.modified=false`.

`strings` searches for `/private/tmp/praxis-review10-`, `/Users/polliard/workspace/convergent-systems-co/praxis`, `/tmp/`, `/private/`, `/Users/`, `/workspace/`, and `/var/folders/` returned zero checkout/source-root matches in both independently built binaries. `otool -L` reported only system framework/library load paths.

## Current identities recomputed from on-disk candidate

```
core                         42cb404ff30b77b375505a2593f5541b9be63587b39e87c1792d9231851de821
Goals plugin                 08514f2078c0e4a7a9093e41b4a78ec72069f7053a6e5bf670ff426ec1888829
package archive              06e0ebaea3c0ad549d7efdf37f9894283360b36b9dfa4ccf8fa9e0a45d96cc62
package manifest             e8894b79d8e7589cff37e98f93e31e22adb7fba885a02c19ac46156e534111d2
source manifest              65b3dd2ba52008c0ac48e185b7397a0e9d5dcb21211d033d188d745b653c1fd1
qualification results        affdb9b1e3c8fd93c8f4c072b31536c3320a46e6b2e76ccfe8b6c3c2abd1b4fb
activation requirements      a961ad31d27e4f84c40ac74f3071a54d8a459fcf168c322047c6dfd2192dff8d
preactivation verification   725143048481f12d88922be0ce1aa2abcece772e6761de47b9e2310e97278235
specification manifest       5a33e46f15f59978ea72972a91879d3feff39355581fe48cb451d30c03a96215
active installed Praxis      807359e48b2626abb1bcfca3ec302da743f1a0238644d6b0c64a0f888ec48fe7
```

## Source boundary and direct probes

`git diff --name-only 25c7330..9edca1e -- <all 125 manifest paths>` produced zero paths. The full post-source diff is restricted to bootstrap-v4 evidence/report/generator paths.

```
go test -count=1 ./internal/goalstore ./internal/goalspublication \
  -run 'Test(RetiredDelegatedPublisherGenerationDoesNotResolvePackagePublishAuthority|InvalidatedDelegatedPublisherGenerationPreventsBuildSigningPreview|RetiredPackageManagerGenerationDoesNotResolveDeploymentAuthority|EveryImmutableGenerationConsumerIsClassified|RetiredRootShapedGenerationCannotAuthenticateTheAbandoningOwner)$'
# PASS

PRAXIS_REQUIRE_KEYCHAIN=1 go test -count=1 ./internal/goalstore ./internal/crypto \
  -run 'Test(Repair6OpaqueKeychainFileReplayIsRefused|KeychainAnchorOpaqueFileReplayIsRefusedOnceTheAnchorHasAdvanced|Anchor.*(Rekey|Interrupted|Undo|Pending))'
# PASS on both packages
```

The qualification log predicates were independently read from `verify/regenerate_candidate_evidence.py` and their referenced log files. No candidate generator or preactivation verifier was run, because they write existing candidate records.

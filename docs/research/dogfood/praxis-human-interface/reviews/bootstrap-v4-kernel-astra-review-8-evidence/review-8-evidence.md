# Astra Review #8 evidence — Repair #7 PRE-ACTIVATION candidate

Review date: 2026-09-22. Base HEAD: `ff14600` (`ff146000aadae0ef60981d445056089f52815869`).

## Identity and provenance — ESTABLISHED

The candidate remains uncommitted (`build_modified:true`) and `candidate-activation-requirements.json` says `COMPLETE_PRE_ACTIVATION_CANDIDATE`. No `praxis-human-interface-proposal-v4.json` exists in the tree. The active core and candidate are distinct.

| Item | Independently recomputed SHA-256 |
|---|---|
| active `/Users/polliard/bin/praxis` | `807359e48b2626abb1bcfca3ec302da743f1a0238644d6b0c64a0f888ec48fe7` |
| candidate core | `86d79f214dac247cc0c56e99b024d23049092494c233097967ffa05600f64996` |
| Goals plugin | `b2f8ea655c2199573dfa244102d06ce770eb2c9c4c889974c1d2b880b722d6ff` |
| package archive | `b36792eaee121f5ed906a020cf1716634244a1d2959e4a39ea8363651a1fbd84` |
| package manifest | `fc3e692563f552ca0ee43ea901673ebf5cab2390d3b5d516ca0f197df2bb11d4` |
| source manifest | `7e02999f0ec246e44ba7d248ae539ef9f859a8d406f6508c91293b26343c3c2b` |
| qualification results | `e5bd0da0e3a2fc892f0453467e7706a817308aee808fad72817c83d742d08d9c` |
| activation requirements | `2cff69707b140ae9219e9996a2cdc4fae5b39ea37741c2398b6b4916a7b312cf` |
| validation entrypoint | `5c7f1924c19784d966f94d386ddcbd67ace8adcdf5f4f94f9b0020a33bea8f0c` |
| validation profile | `12d3f0a1e97a33591d1a320213ba55e9db008bc9d234297c3057b45bc2d1a55a` |
| specification-bundle manifest | `5a33e46f15f59978ea72972a91879d3feff39355581fe48cb451d30c03a96215` |
| canonical specification contract | `17e36c3351d9e510e710c74d3cf4170441f08c150a780e030dbed0896a3cf40a` |

All 125 entries in `source-manifest.json` match their current source bytes. The specification counts are 22 candidates, 68 relationships, and 31 requirements. `preactivation-verification.json` matches these identities; it was not regenerated because that verifier writes candidate evidence.

The preserved Repair #6 candidate is byte-identical to Review #7's recorded identities: core `a57fbce…fdd2`, plugin `6e10e6…2fb4`, archive `ed9f6e…8cab`, manifest `7529e5…7bbf`, source manifest `ffc63f…9e20`, qualification `428cc5…981b`, activation requirements `42ba96…cb0`, and preactivation `87d07e…2b99`. The preserved Repair #6 report hashes to `f3598e5fb397135a722d2135104db55deae660992eec7e546f3f167d7a3299b9`; Review #7 hashes to `d1fd04448200cd13fb79716b0ea14f988e053966538a125699bb13393771d20c`.

## N17 closure — ESTABLISHED

`ResolvePackagePublishAuthority` uses `currentAuthorityGenerations` before selecting a child and calls `requireCurrentLineage` before return (`internal/goalstore/publisher_authority.go`). `currentAuthorityGenerations` fails on an anchor/store disagreement and filters invalidation, liveness-digest, anchored retirement and re-anchor void; the lineage helper applies currentness to every ancestor (`internal/goalstore/generation_currentness.go`). `BuildSigningPreview` and final `SignWithPreview` both use this resolver (`internal/publisher/sign.go`).

An independent test was constructed in a scratch copy (not added to the candidate): it created a real anchored fixture and a valid preview; retired its delegated child via `SaveAuthorityGenerationInvalidation` while retaining a current store; then attempted both a new `BuildSigningPreview` and `SignWithPreview` using the pre-retirement preview. Both refused and the counting protected signer remained at zero calls.

Two scratch mutations were independently detected:

1. Replacing `currentAuthorityGenerations` with `ListAuthorityGenerations` **and** removing `requireCurrentLineage` made the independent probe fail: `BuildSigningPreview accepted a child generation retired in a current store`.
2. Removing `LoadCurrentAuthorityGeneration`'s call to `requireCurrentGeneration` made `TestLoadCurrentAuthorityGenerationRefusesARetiredGeneration` fail: `a retired generation loaded as current`.

Focused production regressions passed for N17, package-manager lineage, owner-currentness, and consumer-registry coverage. The registry is only a bounded structural inventory; direct source search of immutable readers and direct generation-namespace use found no unclassified authority-exercising production path beyond its documented E/M/C entries. This is an **INFERRED**, not universal, equivalent-path conclusion.

## Mutation audit — ESTABLISHED for inspected classifications

The inventory reports 364 mutations: 315 killed, 27 redundant-layer, 17 redundant-by-construction, 3 equivalent, 2 unmodeled, zero unexplained. Spot checks:

* R7.03 is redundant-by-construction: `AuthorityGeneration.Validate` itself requires `State == active`, and `LoadAuthorityGeneration` validates and verifies its digest before a record reaches the resolver.
* R7.04/R7.05 are a coherent version/digest pair; their joint R7.04j is present and recorded killed.
* R7.09 is masked by canonical scope equality and namespace membership; joint R7.09j is present and recorded killed by the bare-scope regression.
* R7.21's cyclic parent construction would require self-referential digest preimages; this is an inference from the hash-binding construction, appropriately recorded as redundant-by-construction rather than killed.
* R7.25 is masked by the resolver's independent lineage check; joint R7.25j is present and recorded killed.
* R7.35 is masked by decision-time validation of the same operational generation; joint R7.34j is present and recorded killed.
* R7.58 is an expiry layer masked by resolver expiry; the direct boundary regression is present. Its stated isolation limitation remains.

No false kill was found in these spot checks. The mutation suite remains bounded and does not prove all future consumers safe.

## Qualification and Keychain — ESTABLISHED / UNKNOWN

Actual logs show 12 tested packages plus `faa/faatest` with no tests, all `ok` in `focused.log` and `race.log`; `vet-all.log` says `vet exit 0`. `probe-replay-summary.log` contains all eight named Review 2–6 outcome sets as identical; `image-probe-replay-summary.log` records a 14-scenario normalized transcript identical to Repair 5.

This environment permits a mandatory real-Keychain run. `PRAXIS_REQUIRE_KEYCHAIN=1` passed the opaque-file N16 replay and the focused advance/undo rotation, crash-boundary, refusal, pending/current matrix, missing-entrypoint, stranded-state, and production replay tests. This establishes the tested Darwin backend behavior only; portability outside the recorded platform remains UNKNOWN. Repair #7 does not modify Keychain source, and R-K2 remains accurately stated: coordinated rollback also needs the corresponding historical login-Keychain password state.

## Residuals — ESTABLISHED as stated; not cleared

`consumePackageApproval` checks approval identity, revocation, use count, and expiry but never revalidates its authority lineage: it is a bearer approval through its maximum lifetime. `CheckAuthorityInForceInTx` returns nil when FAA is absent. `state.Store.PublisherGeneration` is a distinct mutable `State == active` record used by the signing path and was not covered by the immutable-generation registry. The exact signed-Goals recovery integration tests are skipped unless `PRAXIS_GOALS_QUALIFICATION_ASSETS` is supplied. These are accurately documented residuals/boundaries; none was independently cleared.

No candidate implementation, test, artifact, existing evidence, active installation, deployment, activation, Proposal v4, or Gates A/B/C state was modified.

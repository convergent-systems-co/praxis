# Goals initial-publication implementation qualification

Date: 2026-09-16. Source implementation evidence, not operational authority.

Accepted baseline: `9cdb6a2a8e4397c4afddff327079b1a180814856`, tree
`11d04b606c8b810d579017f4957bb8ca75e7b1cb`. Governing artifacts: ADR-078,
SPEC-041, PLAN-007 and their separate acceptance record. Their frozen bytes
remain unchanged. The commit containing this record binds the integrated tree;
the manifest below identifies the exact source and test bytes qualified.

## Exact denominator

1. Closed v4 GOALS_INITIAL_PUBLICATION edge, restricted to the fixed installation,
   publisher and existing Goals package. Existing owner delegation, protected
   request/decision/generation records and exact intent binding are reused.
   Historical v1/v2/v3 validators and digests retain their meaning. Existing v3
   deployment generations remain usable under v4 without replacement or renewal.
2. System-produced ActionIntent and prepare/execute/inspect/reconcile CLI surfaces.
   Deterministic descriptor/tree/parentless commit, explicit metadata, stable
   repository/owner/account IDs, asset inventory, expiry and execution nonce.
3. Fixed GitHub refs, draft release, three unchanged assets, read-back, publish
   and final read-back. No force, replacement, alternate destination or adoption
   of matching pre-existing objects. Credentials provide transport only.
4. Existing commands/events/effects and coordinator. Atomic deterministic admission,
   one pending-effect claimant, exact durable authority and observed outcomes.
   Completion binds each admitted effect's payload and result. Reconciliation
   appends observed evidence; an ambiguous mutation is never blindly repeated.
5. Local acquisition completion/authority/effect/remote-identity checks before
   existing package verification. Raw downloaded signature bytes are retained
   alongside the parsed envelope and compared without reserialization. Existing
   deployment authorization remains separate.
6. Focused adversarial tests against isolated databases, local scripted GitHub/Git
   transports and the exact unchanged signed Goals bytes. No live GitHub writes.

## Qualification executed successfully

```sh
PRAXIS_GOALS_QUALIFICATION_ASSETS=/private/tmp/goals-package-4af02ee3-a \
GOCACHE=/private/tmp/praxis-readonly-recovery-gocache \
go test -race ./internal/goalspublication ./internal/goalstore ./pkg/contracts \
  ./internal/distribution ./internal/effect ./internal/state ./cmd/praxis
```

The affected packages also passed ordinary `go test`, including
`./internal/packagecatalog`. `git diff --check` passed.

Tests cover the six items: historical expiry versus new permission; missing,
legacy and substituted authority; publisher/generation/provenance revocation or
substitution; field-by-field intent tampering; concurrent consumers and replay;
stable destination substitution; nonempty repositories and existing releases;
non-forced atomic ref creation and refusal to adopt matching refs; exact release
settings; moved tags, substituted asset IDs/sizes/bytes; interruption before
admission and before outcome persistence at every step; lost responses at every
step; expiry fencing; durable read-only reconciliation after authority loss;
missing/altered acquisition lineage and raw signature-envelope substitution;
unchanged deployment authority and unchanged original signing identities.

The public historical fixture is `internal/goalspublication/testdata/public-signing-lineage.json`.
It contains no private signing or installation keys. Tests create synthetic
publication grants and model adoption only inside fresh temporary databases.
The exact signed archive is deliberately not committed. Integration cases require
`PRAXIS_GOALS_QUALIFICATION_ASSETS`; without it they explicitly skip, so a default
CI pass alone does not reproduce this exact-byte qualification.

## Required bounded corrections and limits

Historical request validation previously consulted wall-clock time while loading
signing evidence at its original signing instant. Explicit `ValidateAt`/`DigestAt`
keep the canonical bytes unchanged, permit historical verification and retain
current-time expiry checks for new execution. This is a trust blocker correction
inside the accepted signing-lineage requirement.

Acquisition previously retained only the parsed signature envelope. Retaining its
raw bytes is necessary to enforce the accepted exact envelope identity; the
existing signature/package verifier and generic verification policy are unchanged.

No new tables, schema version or migration definitions are needed. The sole new
model is the source-defined bounded v4 successor; it was not adopted in dogfood.
No installation, authority ceremony, re-signing, publication, acquisition,
deployment, activation or distribution-repository mutation occurred.

Ambiguous GitHub responses can remain unresolved permanently when original
attribution cannot be proved. Matching bytes alone never authorize recovery.
This deliberately qualifies safety, not automatic completion after every network
failure. Real GitHub execution is a separately authorized operational transition.
Issue #127 and all accepted scope-firewall exclusions remain parked.

Next operational step: use the qualified binary to produce and review the
installation-specific `praxis authority model-preview` for v3 to v4. Adoption
requires separate explicit owner authorization; preparation and exact publication
delegation follow adoption, and are not authorized by this test record.

## Qualified source and fixture SHA-256 identities

| Path | SHA-256 |
| --- | --- |
| `cmd/praxis/authoritybootstrap.go` | `ad26c4f2e891a1e71120f1673ba81b7826a08bd05be54593a7f7ae14a70aaa7a` |
| `cmd/praxis/authoritymodel.go` | `b54fd54f2a64fd57f1ad07e96e79d3f2b8994eb645e071a2e6031d8b72271daf` |
| `cmd/praxis/cli_help.go` | `65cdb06150b81bc5ff409a26600fef11029690851266a2718d342c0fc2e566e7` |
| `cmd/praxis/goals_publication.go` | `d07bac73108319e308259177a305c9d2c38faa67156b4e7186d9724b1ac39c8d` |
| `cmd/praxis/packages.go` | `0b145ba5bbe5f03674f0244adb246352dbfd524a7ca8d2b61304e4afa7801693` |
| `cmd/praxis/publisher.go` | `7fa8b58c0328d13cab09ba0a3bb69e83bd518f0a6d63ddd0430271438f7cc29f` |
| `internal/distribution/distribution.go` | `39b4e4e8f4f130f177006cb67b441dce5569dd2dbb9bbcd94f89c8040c16b069` |
| `internal/distribution/github_releases.go` | `0b5b0e6e713758dd051b8790147581cf53c6b104a0db181f117031f64507e816` |
| `internal/goalspublication/acquisition.go` | `c9c44e237206cd63b3d2ba6bd78a9fd72e9081d9beaac097ef6a75bba6779b99` |
| `internal/goalspublication/assets.go` | `a7f8f5a38cc0ec4c93baabd403f99d8d9331bd8f9facba4a85d0d01c751166ba` |
| `internal/goalspublication/execute.go` | `fdcfa742caea1fa0097bfd147e3ff453e884a14ea9e5ab749ae24848c847d577` |
| `internal/goalspublication/github.go` | `6c2d6c0502f4b25efe0f1953d4837b176e33c5d6aee1d8eef0f7c3141a7a9e6e` |
| `internal/goalspublication/github_test.go` | `eef9cd697fd8326db36ec288a15988940601f0eaf0d0e92bdc9b7134f48248d0` |
| `internal/goalspublication/publication_test.go` | `516e80e7a2d9d7ddd9bcc7382ba92e5f6cf7998c8b00806502286b0f8a391759` |
| `internal/goalspublication/reconcile.go` | `919be4a8f1a3c2a4fef4737dc6b23169c44347c50e9399cd1abf188c7dac3d6b` |
| `internal/goalspublication/testdata/public-signing-lineage.json` | `d9a9fed381030cbcd929132e9d4e8c1cafeddf34600ee11e0fb9a56e65f35679` |
| `internal/goalstore/goals_publication.go` | `4bb61dbb7d0bf45ba7430b11cd058814a531a37f537339dd7108243737d6e510` |
| `internal/goalstore/package_manager_authority.go` | `00db58f8efa14186dcbf8a6792f6b3fccd7d5f8628f1af49fdf97e10ecf6f71e` |
| `internal/goalstore/publisher_authority.go` | `572db155d2f135c77a569a82923e1bf203db1e82f59c3bc6e51ca904a2bbb317` |
| `internal/goalstore/publisher_governance.go` | `6fbb4af98f71d6b6a1a85be61a077ec8dbde520dbb657d0eec55057391f9f6d3` |
| `internal/goalstore/repository.go` | `9299081437b3580756f5ad0007a444937d17473129a7644908ee41db0edbc7b6` |
| `pkg/contracts/authority_model.go` | `17da75623ddbc184f6c25489ba3e25971e6c227f90058f9d11793677efed29fd` |
| `pkg/contracts/authority_request.go` | `0e4244e012db314355194210bb1de56dbe67daf826fbfc039d7a4bc4ee18e8c7` |
| `pkg/contracts/goals_publication.go` | `84223b97c6991c33b3e03a664930597cefbe2d455e77313fa774371185ed9bb3` |
| `pkg/contracts/goals_publication_test.go` | `3311eb54fcceeed88cc878e6da54e1006a0b0320e375abe37052c0be98a68f3c` |

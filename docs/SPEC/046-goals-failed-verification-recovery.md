# SPEC-046: Goals failed-verification recovery

Status: Proposed
Predecessors: SPEC-045

Contract `goals-established-state-publication/4` binds the exact `/3` ordered
lineage plus failed request, intent, authority generation, execution, manifest,
archive, signature, and verify-draft effect identities and evidence. The first
three effects must be `succeeded`; verify-draft must be terminal `failed` with
one attempt. The remote repository, draft release, and exact asset identities
are bound at preparation.

The successor permitted effects are exactly:
`verify-draft-assets,publish-existing-release,verify-published-release`.
Preparation is non-authoritative and read-only externally; it creates only a
fresh ActionIntent and pending request. Historical UNKNOWN and FAILED outcomes
remain unchanged, and a later successful verification is a new effect.

For `/4`, current asset state is normative: `asset_inventory=established`,
`asset_count=3`, and the manifest/archive/signature IDs, names, sizes, and
signed digests are represented identically in parameters and preconditions.
The execution boundary validates this representation before any effect.

# SPEC-032: First-Installation Governance Principal Bootstrap

Status: Active — post-release dogfood

Authority: ADR-069

## Contract

`praxis authority bootstrap --scope <scope>` is an explicit core operation.
It MUST require `PRAXIS_BOOTSTRAP_RECORD`, a configured production bootstrap
provider that opens successfully, an explicit `PRAXIS_DB`, an authenticated
local OS session, and interactive confirmation of the exact displayed
principal/scope. Non-interactive/model/provider input MUST fail closed.

The operation MUST derive a deterministic human principal from the
non-secret BootstrapRecord digest, persist exactly one version-1
`AuthorityGeneration` in encrypted GoalStore state, and bind its provenance to
that bootstrap digest. The generation digest MUST be computed from the
canonical non-digest generation fields. The initial scope is caller-supplied,
least-scope, and required; it MUST NOT imply unrestricted installation,
repository, provider, organization, or external authority.

The operation MUST be idempotent for the exact existing generation and MUST
reject a second or conflicting root enrollment. Missing/changed bootstrap
material, corrupted state, stale generation, or inaccessible protection MUST
fail closed. The owner metadata string is descriptive only and cannot
authenticate a principal.

Enrollment MUST NOT create an AuthorityRequest/AuthorityDecision, accept or
attach a WorkPlan, grant provider authority, or invoke a worker. Later
authority decisions MUST still bind to the exact persisted generation and
scope through ADR-064/SPEC-027.

## Evidence

Qualification MUST cover non-interactive/model rejection, wrong or changed
bootstrap binding, duplicate/conflicting enrollment, restart recovery,
generation invalidation, and provider-subprocess inability to establish root
authority. Platform-native authentication and key protection remain below the
generic bootstrap boundary.

# SPEC-031: Goal Baseline Import

Status: Accepted — post-release dogfood

`praxis goal import <canonical-baseline.json>` is an explicit local mutation
boundary. The document must contain `schema_version: "1"`, an absolute
`source_ref`, a `source_digest`, and a complete `baseline` object. The source
digest is the SHA-256 digest of the baseline's canonical payload; the payload
must contain the exact immutable Goal ID, version, predecessor digest, and
baseline digest.

The importer opens the configured production bootstrap provider before opening
the encrypted GoalStore. It verifies canonical identity and digest, requires a
stored predecessor for any non-root generation, and then uses the normal
immutable GoalStore insertion path. Existing identical generations are
idempotent; corrupt, expired, or conflicting existing records fail closed.

Import admits only Goal state. It never admits a WorkPlan as executable
authority, creates authority records, or invokes a provider. Repository path,
file naming, issue text, and summary manifests have no authority.

Qualification covers invalid digest, missing provenance, summary/arbitrary JSON,
missing predecessor, duplicate exact import, conflicting generation, and
restart-readable recovery.

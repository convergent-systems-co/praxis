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

## Prose intake

`praxis goals-lifecycle --operation=intake --goal-id=<id> --input=<document>`
(ADR-101) derives a root Baseline from a prose Goal document and admits it
through the same import boundary; it is not a second way into the GoalStore.

Derivation is pure and deterministic (`packages/goals.BaselineFromProse`):

- `original_intent` is the document verbatim; `refined_outcome` is the intent
  text between an optional `# ` title and the first `## ` heading and must be
  non-empty.
- `## Scope` (free text) fills `scope`; `## Non-goals`, `## Constraints` and
  `## Success criteria` (bullet lists, an indented line continues the previous
  bullet) fill `non_goals`, `constraints` and `success_criteria`. Any other
  section is preserved only in `original_intent`.
- A recognized section that is empty, not a bullet list where one is required,
  or repeated is refused; nothing is silently dropped.
- `rigor` is `structured` and `recommendation_mode` is `review_all`. The
  document's SHA-256 is bound as `goal-document:sha256:<hex>` evidence, and an
  assumption states the fields were derived, not authored.
- The generation defaults to `1`, has no predecessor and no WorkPlan.

Admission binds `source_ref` to the document's canonical absolute path and
`source_digest` to the canonical Baseline payload, exactly as import does. An
exact repeat is idempotent; different content under the same generation fails
closed. Intake never creates a proposal, review, request, decision or
acceptance and never invokes a provider.


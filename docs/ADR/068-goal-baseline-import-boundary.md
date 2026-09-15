# ADR-068: Explicit Goal Baseline Import Boundary

- Status: Accepted — post-release dogfood
- Governing: ADR-033, ADR-040, ADR-043, ADR-064

Repository artifacts are evidence, not Goal authority. A canonical
`GoalBaseline` may enter the encrypted GoalStore only through the explicit
`praxis goal import` boundary. The importer verifies the baseline's canonical
digest, binds non-authoritative source provenance, requires every declared
predecessor generation, and persists through `goalstore.Repository`.

Import is idempotent for an exact existing generation and fails closed for a
same-ID/version conflict. It does not create a WorkPlan, grant provider or
authority permissions, or select execution. Summary manifests and arbitrary
JSON are not import documents.

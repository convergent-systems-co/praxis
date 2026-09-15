# ADR-069: First-Installation Governance Principal Bootstrap

- Status: Accepted for post-release dogfood integration
- Date: 2026-09-15
- Related: ADR-023, ADR-043, ADR-064, ADR-065

## Context

The encrypted GoalStore can persist authority generations, but first
installation had no governed way to establish the initial human governance
principal. Key bootstrap proves possession of the protected installation key;
the metadata `owner` field is not authentication, and conversation/provider
output is not authority.

## Decision

Praxis exposes an explicit core `authority bootstrap` operation. It requires:

1. an already initialized, explicitly configured production bootstrap record;
2. successful opening of the configured platform protection backend;
3. an authenticated local OS session;
4. interactive confirmation of the exact principal and least-scope authority.

The principal is deterministically bound to the non-secret bootstrap-record
digest as `installation-owner:<bootstrap-digest>`, with governance kind
`human`. The initial generation is persisted in the encrypted GoalStore as
version `1`, with provenance bound to the bootstrap digest and current OS
session identity. The caller must supply one explicit scope; no unrestricted
installation, repository, provider, organization, or external-system
authority is implied. The operation is core CLI functionality, not a dynamic
package/provider command, and models/providers cannot invoke it as an
authority shortcut.

Only one installation root generation is permitted. Exact repeated enrollment
is idempotent; a different bootstrap binding, principal, or scope fails closed.
The generation is immutable and uses the existing invalidation/supersession
records for later lifecycle changes. Enrollment creates neither an
AuthorityDecision nor a WorkPlan.

## Trust boundary

The trust anchor is the combination of protected-key possession, the current
OS user/session, and explicit interactive confirmation. The bootstrap record's
owner string, an environment variable, a model statement, and an email string
alone are not authentication. Installation ownership is distinct from
repository, provider, organization, and external-system authority.

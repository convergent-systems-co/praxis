# Reachable Bounded Authority Delegation Acceptance

Status: Accepted

Date: 2026-09-15

The architecture owner accepted the exact successor artifacts:

- ADR-073, `sha256:194ba86edd2ca8c7fc2c540ed66de21e08f73aabdf5ffb403a9d8c93129e025d`
- SPEC-036, `sha256:6bf6411a880852a42a1b39cbd8fde55dc13006036bca59361a0e0ab051612527`

ADR-072 and SPEC-035 remain unaccepted historical proposal evidence.

The enrolled root has exactly the intrinsic governance capability
`authority.delegate`. It has no implicit general decision, execution,
invocation, package, provider, repository, Goal, or WorkPlan authority.

The first child transition must bind the exact root generation, delegation
request, authenticated root decision, typed policy containment relation, and
child generation lineage. Self-targeted delegation is denied by default.

This record authorizes implementation and qualification of these semantics. It
does not itself create authority, approve a request, bootstrap an installation,
or authorize Japetella execution.

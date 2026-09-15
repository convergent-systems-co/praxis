# Security and Authority Model

Praxis constrains authority; it does not make arbitrary hosts, models, plugins, or provider accounts intrinsically safe.

## Boundary rules

- Models propose and perform bounded work; deterministic runtime code owns transitions.
- Client prompts, skills, hooks, and untrusted workspace content are evidence or input, not authority.
- Approvals are persisted, intent-bound, finite-use, and consumed atomically where required.
- Capability leases bind operation, scope, expiry, provider instance, and runtime session.
- Package activation requires exact signed bytes, content digests, dependency locks, policy review, and independent local approval.
- Plugins are supervised and isolated providers; installation does not grant a lease.
- Evidence may support a decision but cannot assert that the decision already happened.

## Fail-closed behavior

Praxis rejects unknown schema versions, ambiguous provider readiness, unsupported state-provider semantics, stale sessions, invalid digests, missing evidence, incompatible package generations, unsafe paths, and required cryptographic profiles that the provider cannot satisfy.

## Credentials and data

Provider credentials remain in provider-managed stores or environment/configuration mechanisms. Praxis redacts credential-shaped provider output and does not treat model text as a secret-safe channel. Back up and protect `PRAXIS_DB`, run directories, trusted publisher keys, and package artifacts according to their sensitivity.

## Qualification context

The first Praxis 2 release is qualified against the frozen 38-claim original-intent denominator and the versioned current-release oracle. Historical oracle bytes remain immutable evidence for their original replay context. See [ADR-049](ADR/049-goal-conformance-and-blind-self-improvement.md), [SPEC-018](SPEC/018-goal-conformance-and-self-improvement.md), and the exact artifacts linked in the [release notes](../RELEASE_NOTES.md).

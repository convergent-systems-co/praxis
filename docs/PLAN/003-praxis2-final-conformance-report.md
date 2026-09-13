# PLAN-003: Praxis 2 Final Conformance Report

- Status: Final release-conformance record
- Date: 2026-09-13
- Branch: `redesign/praxis2`
- Governing plan: PLAN-001
- Security review: PLAN-002

## Result

Praxis 2 satisfies the reconciled redesign master plan subject only to the final full-head CI result recorded after this report lands.

This report maps the late architecture changes added during closure to executable evidence so the branch does not reach completion through documentation-only claims.

## Provider abstraction

**Governing:** ADR-047, SPEC-016.

Implemented:

- semantic provider capability profile in `internal/stateprovider`;
- SQLite as reference provider rather than architectural contract;
- fail-closed capability requirement checks;
- event replay/optimistic append conformance through the provider surface;
- package registry access through the provider surface.

Evidence includes positive SQLite provider conformance and negative `unknown` capability rejection.

**Disposition:** conformant.

## Universal package model

**Governing:** ADR-048, SPEC-011, SPEC-017.

Implemented:

- package as universal distribution unit;
- typed graph, agent-definition and plugin contents;
- immutable generation/content digest binding;
- graph-only and agent-only packages without executable plugin authority;
- mixed packages preserving plugin capability/lease boundaries;
- independent local agent identities instantiated from one installed definition;
- active invocation contracts atomically bound to installed package generations.

**Disposition:** conformant.

## Dynamic CLI surface

**Governing:** ADR-046, SPEC-004, SPEC-015.

Implemented:

- core CLI exposes only stable control-plane verbs;
- domain commands are resolved from the active invocation registry;
- core command shadowing is rejected;
- alias collisions fail without changing the existing owner;
- package update atomically changes the visible invocation generation;
- uninstall removes active invocation visibility while preserving durable historical state;
- help derives installed aliases from the registry rather than importing domain packages.

**Disposition:** conformant.

## Package distribution and lifecycle

**Governing:** ADR-025, ADR-043, ADR-046, ADR-048, SPEC-005, SPEC-011, SPEC-015, SPEC-017.

Implemented initial GitHub Releases adapter:

- `discover`, `info`, `install`, `update`, `uninstall`, `list`;
- universal discovery topic `praxis-package`;
- canonical `praxis-package.json`, `praxis-package.tar.gz`, and `praxis-package.sig.json` release assets;
- bounded manifest, signature and artifact payload sizes;
- artifact SHA-256 binding to manifest identity;
- signature envelope binding exact downloaded manifest digest and artifact digest;
- locally trusted publisher keys;
- algorithm-agile signature verifier interface;
- Ed25519 classical verifier as the currently available implementation;
- PQ-required/hybrid fail closed unless an actual PQ verifier provider exists;
- PQ-preferred classical fallback requires an explicit CLI opt-in;
- update capability/enforcement/crypto expansion requires explicit acceptance;
- package installation never grants plugin capability leases.

GitHub Releases is distribution transport only and is not root of trust.

**Disposition:** conformant.

## Run control and client authority

**Governing:** ADR-038, ADR-042, SPEC-002, SPEC-004, SPEC-006.

Implemented:

- read-only `status` database path;
- deterministic replay-derived status;
- `cancel` and `resume` through scoped `run.control` authority;
- atomic capability consumption and run observation commit through the SQLite committer;
- exact wait-reference validation for resume;
- CLI resume only clears authorized liveness state and does not execute nodes itself.

**Disposition:** conformant.

## Plugin authority

**Governing:** ADR-041, SPEC-007.

Implemented:

- exact instance/runtime-session binding;
- protocol negotiation and verified manifest/handshake identity;
- supervisor isolation eligibility and quarantine;
- restart ceilings;
- authoritative persisted lease reload immediately before dispatch;
- atomic bounded-use consumption;
- stale-session, denied-operation and replay rejection.

**Disposition:** conformant.

## Cryptography

**Governing:** ADR-043, SPEC-005.

Implemented:

- profile-driven crypto resolver;
- classical-compatible, PQ-preferred, PQ-required and hybrid-high-assurance profiles;
- explicit downgrade authorization only for PQ-preferred;
- encrypted secure blobs with authenticated envelope security metadata;
- package signature verification through algorithm/class provider interfaces;
- no claim of PQ support when no PQ verifier/provider is installed.

**Disposition:** conformant.

## Goals and planning amortization

**Governing:** ADR-044, ADR-045, SPEC-013, SPEC-014.

Implemented:

- domain-neutral Goals graph;
- progressive rigor and recommendation-delegation eligibility;
- recommendation delegation separated from execution authority;
- canonical Goal Baseline digest;
- selective dependency invalidation;
- applicability classification `reuse`, `delta`, `replan`;
- encrypted durable baseline adapter and real ephemeral mode;
- development planning baseline binding and amortization telemetry.

**Disposition:** conformant.

## Domain neutrality proving evidence

Software development proving package demonstrates fast/direct and architected Goals paths.

Structured research proving package demonstrates the same Goals/subgraph/runtime substrate without repository/development ontology.

**Disposition:** domain-neutral core claim survives implementation.

## Licensing and contribution rights

Repository licensing is PolyForm Noncommercial 1.0.0 with NOTICE and README alignment. Commercial use is not granted by the public repository license. Until a deliberate contributor-rights agreement is selected, external code contributions are not accepted; issue/discussion feedback remains possible.

**Disposition:** aligned with repository owner intent.

## Remaining operational risks

The final adversarial review lists operational risks that cannot be eliminated by Praxis architecture alone, including host/root compromise, publisher-key theft, dependency vulnerabilities, intentionally overbroad user grants, and model reasoning quality. None are unexplained acceptance gaps in the redesign master plan.

## Final release gate

The final remaining mechanical gate is:

1. full `Praxis 2 Go` workflow green at the current reconciled branch head after this report and the final PLAN-001 status update.

Once that gate is green, PLAN-001 may be marked complete at 100% without qualification.

# PLAN-002: Praxis 2 Adversarial and Security Architecture Review

- Status: Draft review record
- Date: 2026-09-13
- Scope: ADR-001 through ADR-039, SPEC-001 through SPEC-004, PLAN-001

## Review method

The design was reviewed from four hostile perspectives:

1. malicious or compromised LLM/executor;
2. malicious repository/document/package content;
3. malicious or compromised plugin/catalog artifact;
4. concurrency/replay/state-corruption failures that create security effects without a malicious model.

The review also challenged the architecture for accidental software-development coupling, hidden nondeterministic authority, token/performance regressions, and trust claims stronger than the implementation can enforce.

## Architectural strengths retained

The following decisions survived review and should remain foundational:

- general graph/agent ontology rather than development-specific core semantics;
- execution/learning/governance separation;
- human authority precedence;
- deterministic command/query/event boundary;
- append-only durable events with projections;
- local authoritative state;
- capability-based plugin/executor routing;
- package provenance/signing separated from safety judgment;
- client adapters separated from graph semantics;
- enforcement below the LLM;
- workspace intelligence as disposable derived state rather than repository authority.

## Findings and disposition

### Critical: process isolation was not yet a security boundary

**Attack:** install or compromise a plugin that declares only narrow capabilities but inherits the user's filesystem, network, environment, SSH agent, cloud credentials, or process authority. The plugin bypasses Praxis protocol checks directly.

**Disposition:** ADR-041 created. Plugin principals, capability leases, credential brokering, OS/resource isolation, authenticated local transport, and accurate `unconstrained` reporting are now architectural requirements.

### Critical: approvals could be replayed or applied to changed actions

**Attack:** obtain approval for one command, mutate arguments/target after approval, reuse an old approval, or exploit repository/external-state changes between check and execution.

**Disposition:** ADR-042 created. Security-sensitive approvals bind to canonical `ActionIntent`; commit-time revalidation, anti-replay, idempotency, and unknown-outcome reconciliation are required.

### High: prompt injection could poison reasoning and learning despite deterministic side-effect gates

**Attack:** malicious repository comments, docs, tool output, package metadata, or retrieved text instruct the model to reinterpret policy, create false memory, promote preferences, or request a permitted but unintended action.

**Disposition:** ADR-040 created. Content/data and authority are separate trust classes. Provenance survives context/retrieval. Content can influence proposals but cannot authorize them or self-promote into policy/memory/preferences.

### High: Workspace Intelligence can become a secret-exfiltration amplifier

**Attack:** context packing finds credentials, private keys, `.env` content, generated secrets, or sensitive files and efficiently sends them to a remote model.

**Disposition:** ADR-039's exclusion and local-first rules are retained, but its implementation spec must require deny-first secret/path classification, symlink/path traversal controls, sensitivity labels on evidence, destination-aware context release, and tests proving excluded content never enters embeddings, caches, context packs, telemetry, or remote inference.

### High: derived indexes can become stale/confused evidence

**Attack:** race source changes against an index/context pack so a model reasons or obtains approval against old content and executes against new content.

**Disposition:** ADR-039 version/digest binding plus ADR-042 commit-time preconditions. Workspace evidence used for effectful decisions must expose freshness identity; stale evidence causes refresh/re-evaluation when material.

### High: package signing can be mistaken for trust

**Attack:** a correctly signed malicious package requests dangerous capabilities or later expands them.

**Disposition:** ADR-025 already separates integrity/provenance from capability risk and local authorization. Retain. Add conformance tests that valid signatures never bypass capability review and capability expansion requires new authorization.

### High: client enforcement can be overstated

**Attack:** Praxis claims a session is controlled while the model has alternate shell/filesystem/network tools outside Praxis.

**Disposition:** ADR-038 already requires `client-unconstrained` classification and fail-closed behavior for exclusive-mediation packages. Retain and make this a release security test, not documentation only.

### Medium: resource exhaustion is an authority bypass by availability

**Attack:** plugin, graph, catalog package, or hostile workspace causes unbounded indexing, event generation, recursion, context expansion, process spawning, memory growth, or retry storms.

**Disposition:** existing scheduler/resource governance is directionally correct. SPECs must include quotas for graph depth/work, plugin requests, payload size, event rate, index size, context budgets, retries, and inference spend. Resource limits must be deterministic and enforced below the model.

### Medium: event/projection corruption can produce false authority views

**Attack:** malformed plugin/event payload or migration bug creates a projection that appears to grant authority not present in source events.

**Disposition:** authoritative authorization must be evaluated from validated canonical state/policy, never a convenience projection whose consistency is unknown. Replay/migration tests must include security-critical projections and corruption recovery.

### Medium: cross-machine synchronization can import stale or hostile authority

**Attack:** another machine replays old approvals, capability grants, learned state, or package trust decisions.

**Disposition:** portable state specs must classify which authority artifacts are synchronizable, machine-local, expiring, or non-transferable. One-shot approvals and runtime leases are not portable authority. Conflicts involving policy/trust fail closed.

### Medium: learning can optimize against security boundaries

**Attack:** repeated successful work causes adaptation to suppress checks, broaden retrieval, reduce approvals, or select a faster but less constrained client/plugin.

**Disposition:** ADR-022/023/038/040 jointly prohibit learned behavior from weakening deterministic controls. Security policy and enforcement properties are non-learnable authority unless changed through explicit governance.

### Medium: dependency and parser attack surface

**Attack:** malformed source triggers parser/indexer vulnerabilities; package dependency resolution introduces compromised transitive plugins.

**Disposition:** workspace parsers execute under ADR-041 isolation. Package lock/digest resolution and transitive capability aggregation must be specified. Install UI/policy evaluates the effective dependency capability set, not only the top-level package.

## Adversarial architecture conclusions

### No new core ontology is required

The review did not find a need to add development, repository, client, security-product, or provider-specific entities to Praxis core. Security requirements fit the existing principal/capability/command/event/package/plugin architecture.

### Determinism must extend through the commit point

The largest architectural risk was a false sense of determinism: validating a model request deterministically but then allowing ambient plugin/client authority, stale approvals, or changed targets to bypass the validated decision. ADR-038, ADR-041, and ADR-042 together close this conceptual gap.

### Provenance is now a cross-cutting primitive

Provenance is required not only for catalog packages and learning, but for repository evidence, model output, approvals, plugin identity, synchronization, context packs, and authority decisions. SPEC-001 should treat provenance/trust class as a reusable canonical contract family rather than duplicating ad hoc fields.

### Security claims must be capability claims

Praxis should never say an environment is `safe`, `sandboxed`, or `enforced` as a global boolean. It should report specific verified properties. Unknown means unavailable. This matches the demonstrated-capability philosophy already used for executors.

## Required specification changes

Before implementation decomposition, the specification set must add or update contracts for:

- trust class and provenance envelopes;
- plugin principal and effective isolation profile;
- capability leases and revocation;
- `ActionIntent`, approval binding, anti-replay, idempotency, and commit revalidation;
- workspace evidence sensitivity/exclusion and destination-aware context release;
- transitive package capability aggregation;
- security-critical resource quotas;
- sync portability classes for authority artifacts;
- security conformance fixtures.

## Required adversarial test corpus

At minimum, release qualification must include hostile fixtures for:

- prompt injection in source comments/docs/issues/tool output;
- secret files and symlink/path traversal;
- stale index/context pack versus changed repository;
- malicious signed package;
- transitive dependency capability expansion;
- compromised plugin attempting direct filesystem/network access;
- forged plugin identity/protocol request;
- approval replay and changed arguments;
- ambiguous external side-effect timeout/retry;
- unconstrained client alternate-tool bypass;
- event/projection corruption;
- cross-machine stale authority import;
- graph/retry/index/resource exhaustion;
- learning attempting to weaken security policy.

## Review result

**Architecture disposition: proceed to detailed specifications after incorporating ADR-040 through ADR-042 and the PLAN-001 changes from this review.**

No unresolved design fork from this review requires immediate human decision. The remaining work is specification precision and implementation/conformance evidence.
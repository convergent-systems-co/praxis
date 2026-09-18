# PLAN-002: Praxis 2 Final Adversarial and Security Review

- Status: Final review record
- Date: 2026-09-13
- Scope: current `redesign/praxis2` architecture, ADR-001 through ADR-048, SPEC-001 through SPEC-017, PLAN-001, and implemented Go runtime/tests

## Review method

The completed redesign was challenged from these hostile perspectives:

1. malicious/compromised LLM or executor;
2. malicious repository/document/tool output and prompt injection;
3. malicious or compromised package/plugin/publisher;
4. plugin escaping declared authority through ambient host privileges;
5. replay/TOCTOU/concurrency/state-corruption failures;
6. malicious or stale client integration bypassing deterministic mediation;
7. cryptographic downgrade/key substitution/profile confusion;
8. malicious/stale learned state or user-preference drift;
9. resource exhaustion and recursive graph/subgraph abuse;
10. storage-provider substitution weakening authority semantics;
11. package/update lifecycle introducing undeclared command/content surfaces;
12. accidental reintroduction of software-development assumptions into core.

## Findings closed by architecture and implementation

### Plugin ambient-authority bypass

**Threat:** a plugin declares narrow capabilities but directly accesses filesystem/network/process credentials outside Praxis.

**Controls:** authenticated plugin instance/session identity, explicit isolation profile, capability leases bound to exact instance/runtime session, supervisor/quarantine, protocol handshake, and atomic lease consumption immediately before dispatch. Unknown enforcement is not represented as enforced.

**Disposition:** closed at Praxis boundary. Host isolation still depends on the selected enforcement provider actually supplying the claimed OS controls; unsupported guarantees fail closed for packages requiring them.

### Approval/lease replay and TOCTOU

**Threat:** reuse authority, mutate target/arguments after approval, or consume authority separately from protected mutation.

**Controls:** canonical action intent, bounded/revocable leases, optimistic versions, atomic authorization+event/transition boundaries where security requires it, exact run scope, one-use consumption tests.

**Disposition:** closed for implemented local SQLite authority paths.

### Prompt injection and untrusted-content promotion

**Threat:** repository/docs/tool output causes the model to reinterpret policy, create false authority, or poison durable preferences/memory.

**Controls:** trust/provenance classes; untrusted content can influence proposals/evidence but cannot authorize; learning promotion requires governed evidence; policy/security invariants are non-learnable; workspace evidence remains derived.

**Disposition:** closed architecturally. Model reasoning can still be misled, but side effects remain deterministically mediated.

### Secret/context exfiltration

**Threat:** Workspace Intelligence efficiently discovers secrets and sends them to remote inference.

**Controls:** root confinement, traversal/symlink defenses, sensitivity labels, destination-aware release, exclusions, bounded context packs, crypto/profile requirements.

**Disposition:** closed at workspace release boundary; deployment-specific exclusion policy still matters.

### Stale evidence/indexes

**Threat:** approval/reasoning occurs against stale workspace evidence then executes against changed state.

**Controls:** evidence provenance/freshness identity, disposable derived indexes, baseline/applicability invalidation, deterministic revalidation at authoritative effect boundaries.

**Disposition:** closed conceptually and covered by freshness/invalidation fixtures.

### Signed malicious package

**Threat:** signature is mistaken for safety/authorization.

**Controls:** signature/integrity is separate from capability authorization; transitive capabilities are aggregated; capability/enforcement/crypto expansion requires explicit update review; install does not grant plugin leases.

**Disposition:** closed.

### Dynamic CLI command injection

**Threat:** package shadows `status`, `install`, or another control-plane command; alias update hijacks another package; stale command survives uninstall.

**Controls:** reserved core command set, atomic invocation registration, immutable package-generation binding, cross-package alias collision rejection, active-generation filtering, uninstall/deactivate removal.

**Disposition:** closed by ADR-046/SPEC-015 and package registry tests.

### Universal package content confusion

**Threat:** graph/agent/plugin semantics are conflated, pure declarative content gains executable authority, or agent package updates overwrite persistent identities.

**Controls:** typed immutable content references; package is distribution unit, plugin only executable content class; graph/agent definitions register without code; agent instantiation creates separate local identities/generations; plugin capability remains separately lease-gated.

**Disposition:** closed by ADR-048/SPEC-017 and graph/agent/mixed package fixtures.

### Storage backend semantic downgrade

**Threat:** replacing SQLite with a backend that lacks optimistic concurrency or atomic authority consumption silently weakens the runtime.

**Controls:** provider-neutral semantic capability profile, fail-closed `Require`, SQLite reference provider, provider conformance fixtures. Storage abstraction is semantic rather than CRUD-shaped.

**Disposition:** closed architecturally; every future provider must pass conformance before authoritative use.

### Cryptographic downgrade

**Threat:** relabel encrypted data, substitute key/profile metadata, or silently downgrade PQ-required protection.

**Controls:** algorithm-agile crypto profiles; PQ-required/PQ-preferred/hybrid/classical-compatible modes; envelope metadata authenticated as AAD; explicit key/suite/profile identifiers; provider capability resolution; no claim that unavailable PQ primitives are present.

**Disposition:** closed at profile/envelope layer. Actual PQ implementation remains provider-dependent and must truthfully advertise support.

### Run-control authority bypass

**Threat:** CLI/client cancels or resumes runs merely because it can access the local database.

**Controls:** read-only status path; mutation requires explicit authorizer or preferred atomic committer; SQLite committer consumes scoped `run.control` authority in the same transaction as the event transition.

**Disposition:** closed in runtime service; CLI mutation exposure must use this path only.

### Resource exhaustion

**Threat:** graphs/plugins/indexers consume unbounded work, nesting, retries or context.

**Controls:** graph/resource quotas, bounded retries, nesting limits, supervisor restart ceilings, context/token budgets.

**Disposition:** bounded at implemented runtime surfaces. Deployment resource limits remain defense in depth.

### Goals/recommendation authority confusion

**Threat:** user delegates recommendation acceptance and the model interprets that as authority to execute effects.

**Controls:** recommendation delegation only changes interaction/question surfacing; capability/policy/approval remain independent deterministic boundaries; Goal Baseline digest and selective invalidation detect mutation.

**Disposition:** closed.

## Remaining operational risks, not architecture blockers

1. **Host enforcement quality:** an OS/container/sandbox provider can only enforce what its platform actually supports. Praxis must report unsupported/unknown properties accurately.
2. **Third-party dependency vulnerabilities:** standard dependency/SBOM/update hygiene remains required.
3. **Publisher key compromise:** revocation/rotation policy mitigates but cannot prevent malicious releases signed by a stolen key before revocation.
4. **Model quality:** deterministic authority constrains damage but does not guarantee model reasoning correctness.
5. **User-granted authority:** Praxis cannot make an intentionally broad capability grant narrow; UX should keep scope and consequences explicit.
6. **Physical/local database compromise:** an attacker with arbitrary local process/root access can tamper with state or binaries. OS/user account security remains outside the cryptographic/application trust boundary unless an external attestation system is added.

None of these require a new core ontology or architectural redesign.

## Final adversarial checks required for release closure

The branch qualification suite SHALL include evidence for:

- optimistic version conflict/replay;
- restart/recovery equivalence;
- run cancellation durability;
- one-use plugin lease replay rejection;
- plugin instance/session mismatch;
- supervisor quarantine/restart ceiling;
- protocol/handshake mismatch;
- workspace traversal/sensitive-context rejection;
- crypto envelope tamper/profile-header mutation rejection;
- PQ-required downgrade failure;
- signed package without authority;
- transitive capability expansion review;
- dynamic alias collision/core-command shadow rejection;
- graph-only, agent-only and mixed package activation;
- independent agents from one installed definition;
- provider capability `unknown` fail-closed behavior;
- Goals recommendation delegation not granting execution authority;
- Goal Baseline canonical digest/selective invalidation;
- development fast path and architected Goals path;
- non-development research proving graph;
- read-only status and authorized run-control mutation boundaries.

## Final conclusion

**Architecture disposition: ACCEPT.**

No unresolved architectural/security fork requires a human decision before the Praxis 2 redesign branch can be treated as complete. Remaining work is release closure: user-facing CLI/documentation alignment, final full-head CI, and conformance reporting.

The central security claim remains intentionally narrow and defensible:

> Models may propose and perform bounded work, but authoritative state, capability, transition, effect, persistence, and package activation decisions are owned by deterministic runtime boundaries whose supported guarantees are explicitly represented and fail closed when required guarantees are unavailable.

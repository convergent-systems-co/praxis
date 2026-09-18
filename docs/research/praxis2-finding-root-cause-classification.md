# Praxis 2 Non-Satisfied Finding Root-Cause Classification

- Basis: frozen blind result `blind-learning-prompt-retirement-v38-final.json`
- Frozen result digest: `sha256:177c19c9b061bf05319acdea18ba5ffa1e8f4e4bb7f5d945b25fa334e5301ba7`
- Claim-set digest: `sha256:cc06b98af05518d4a10bfa25d69e5b1a2d0004c0a3e4867d1ff468cbfe0239a0`
- Scope: all 25 non-satisfied original-intent claims in that result
- Date: 2026-09-13

## Method

This classification is derived from the original ADR source attached to each claim and from an implementation evidence inventory performed independently of claim expectations. It does not use the withheld qualification oracle. Existing source files, interfaces, and unexecuted test files establish at most a contract; they do not establish behavioral, integration, enforcement, or lifecycle conformance.

The classifications below do not change a claim, criticality, required stage, or required evidence class. Compound claims remain open until evidence demonstrates every stated property. No completion percentage is assigned.

Classification meanings:

- **missing implementation**: a required executable mechanism does not exist;
- **missing integration**: pieces exist, but the required end-to-end behavior is not connected;
- **missing lifecycle/restart behavior**: behavior is process-local or lacks durable recovery evidence;
- **missing enforcement**: policy or eligibility exists, but the protected action can lack a fail-closed authoritative boundary;
- **missing evidence/test only**: the complete behavior appears integrated, but admissible execution evidence is absent;
- **ambiguous architecture requiring ADR/SPEC clarification**: original responsibilities cannot safely be implemented without resolving a genuine architectural ambiguity;
- **genuine contradiction**: admissible evidence demonstrates behavior opposite to original intent.

No current finding is classified as evidence-only, ambiguous architecture, or genuine contradiction. That is deliberate: inspection found substantive behavioral gaps behind every non-satisfied claim. Merely attesting the current unit tests would overclaim conformance.

## Root capability clusters

### A. Durable adaptive-behavior lifecycle

Claims: OI-006, OI-012, OI-013, OI-014, OI-015, OI-016, OI-017, OI-020.

The common missing capability is a durable, domain-neutral lifecycle that turns scoped execution evidence into behavioral/profile measurements, longitudinal diagnoses, generalized artifacts, governed candidate generations, routing inputs, drift/correction responses, and reversible activation. Current preference, profile, inference-routing, and learning components are disconnected local mechanisms. Fixing only contradiction-driven demotion would leave the same architectural break at the next boundary.

Core owns evidence identity/provenance, causal independence, immutable generations, lifecycle persistence, deterministic eligibility/authority gates, replay, and rollback. Packages/profiles own dimensions, evaluators, thresholds, target scopes, acceptable variance, and domain-specific interpretation. The initial deterministic-mechanism allowlist is a safe bootstrap compiler boundary, not the definition or eventual limit of learnable Praxis behavior. Future compiler/provider extensions must remain content-addressed, governed, independently evaluated, regression-tested, and promotion-controlled; core must not pre-encode every learnable behavior.

### B. Portable state and universal package lifecycle

Claims: OI-009, OI-011, OI-019, OI-035, OI-036, OI-037.

Canonical export/reconciliation, catalog retrieval, package verification, privacy/generalization, local authorization, atomic activation, typed content deployment, invocation publication, provider semantics, update/rollback, and restart currently exist only as partial and disconnected mechanisms. The architectural fix must be one transactionally governed lifecycle rather than separate claim fixtures.

### C. Mediated execution and recoverable resource control

Claims: OI-021, OI-024, OI-025, OI-026, OI-027, OI-029, OI-030.

The common gap is an end-to-end trusted execution envelope. Plugin supervision is in-memory and uses an abstract launcher; scheduling has deterministic ordering but no durable leases; effect coordination has no durable idempotent outcome lifecycle; client and untrusted-content checks are not demonstrated across every authoritative path; isolation records asserted properties rather than enforcing them. These claims require real process, restart, cancellation, authorization, effect, and isolation evidence rather than more policy objects.

#### Persisted child finding: plugin capability-set publication

During Cluster-C transport work, review found a material supporting defect at the capability authority boundary. Before correction, `ValidateHandshake` required equality between the signed manifest capability list and runtime advertisement, while `Registry.Resolve` then selected against the manifest list. This conflated permitted package capability, current instance availability, and routable eligibility. Removing only the equality check would have made under-advertised capabilities routable; adding a caller-set provider field would have let unvalidated data influence selection.

The finding is classified **missing enforcement** and affects the OI-021/OI-030/OI-037 lifecycle dependency, but does not add a denominator claim or close any OI. ADR-055 defines the authority relationship: runtime advertisement is a validated subset of the signed upper bound; only the handshake-validated ephemeral set is routable; grants remain separately leased. The before/after source identities are frozen in `docs/research/conformance/plugin-capability-set-finding-v1.json`. It was initially outside the narrow transport-edit scope, but remained relevant to the parent goal and was scheduled into the same Cluster-C dependency before evidence closure.

This record also captures the process classification: a validated material finding outside a child diff is persisted and scheduled, never silently discarded. Scope controls implementation order, not parent-goal relevance.

### D. Incremental workspace evidence lifecycle

Claim: OI-028.

Discovery, indexing, freshness, search, sensitivity release, and bounded context assembly exist as separate functions. A runtime has not demonstrated incremental change observation through a fresh, authorized context pack with persisted/reconstructable evidence identity.

### E. Cryptographic provider and key lifecycle

Claim: OI-032.

Profile resolution and envelope metadata checks exist, but real PQ-preferred signing/key establishment, hybrid/no-downgrade operation, key rotation, revocation, and retained verification/decryption identity are not implemented as one lifecycle. Fake capability providers cannot prove the compound claim.

### F. Executable Goal Baseline and progressive planning lifecycle

Claims: OI-033, OI-034.

Graph definitions, baseline values, classifiers, and scripted composition tests exist. They do not yet demonstrate an interactive domain-neutral Goals execution that compiles original intent into a durable baseline, then drives progressive, dependency-aware reuse, selective invalidation, and delta planning in materially different domains.

## Per-finding classification

| Claim | Classification | Evidence-independent diagnosis |
|---|---|---|
| OI-006 | missing integration | Deterministic preference selection exists, but explicit correction, contradictory behavior, confidence drift, durable history, and graph execution are not one integrated lifecycle. |
| OI-009 | missing lifecycle/restart behavior | Portability classification exists; canonical export/import, concurrent multi-machine reconciliation, conflict preservation, stale-lineage rejection, and restart reconstruction do not. |
| OI-011 | missing integration | Package manifests and local state exist, but install/bootstrap has no end-to-end privacy stripping and proof that reusable behavior is imported without personalized state. |
| OI-012 | missing implementation | No executable versioned preference-contract seeding and migration mechanism proves material-only setup or fail-closed incompatible evolution. |
| OI-013 | missing integration | A model-neutral router exists, but graph runtime does not persist tier selection, executor evidence, provider replacement, and observed outcome as one explainable execution. |
| OI-014 | missing enforcement | Routing filters local evidence fields, but demonstrated capability cannot yet be shown to remain subordinate to authoritative eligibility/capability/policy at the dispatch boundary. |
| OI-015 | missing implementation | No generalize/sanitize/evaluate/publish/adopt transfer flow exists; raw episodic-memory exclusion is not enforced across agents. |
| OI-016 | missing implementation | No longitudinal evaluator jointly detects regression, repeated inference, variance, and provider-portability failure from durable execution series. |
| OI-017 | missing integration | Immutable promotion/rollback exists, but contradictory evidence, context change, or explicit correction does not yet create and govern a fork/demotion generation back toward inference. |
| OI-019 | missing integration | Manifests, signature verification, dependency capability aggregation, and install states exist separately; no single lifecycle binds immutable bytes/provenance/signature/review/local trust to activation. |
| OI-020 | missing implementation | Evidence-class constants and profile distance exist, but profile history does not derive, preserve, and distinguish declared/inherited/observed/measured/confirmed values or expose divergence. |
| OI-021 | missing lifecycle/restart behavior | The supervisor is process-local, its launcher is abstract/faked, and streaming, cancellation propagation, negotiated process recovery, and durable quarantine/restart state are incomplete. |
| OI-024 | missing lifecycle/restart behavior | Ordering and read-only admission helpers exist; atomic durable lease acquisition/release, cancellation, expiry recovery, and bounded-starvation behavior across restart do not. |
| OI-025 | missing integration | Event append and an effect commit gate exist separately, but all mutations do not traverse one command path with durable idempotent pending/committed/unknown reconciliation outcomes. |
| OI-026 | missing integration | Invocation parsing/registry checks exist, but equivalent execution across multiple client adapters and safe degradation of unavailable optional affordances are not exercised through the runtime. |
| OI-027 | missing enforcement | Several local gates fail closed, but Praxis has not demonstrated that every authoritative action path is mediated below the LLM or rejected when required mediation is unavailable. |
| OI-028 | missing integration | Workspace functions are individually tested; no integrated incremental index-to-fresh-bounded-context execution with provenance and sensitivity authorization exists. |
| OI-029 | missing enforcement | Memory trust labels and learning filters exist, but untrusted data is not demonstrated end-to-end as unable to authorize commands, capabilities, effects, preferences, or durable authoritative memory. |
| OI-030 | missing enforcement | Isolation profiles record claimed states and gate selection, but no launcher applies and verifies effective process/filesystem/network/credential/resource restrictions. |
| OI-032 | missing implementation | Algorithm-profile resolution and symmetric envelopes exist; standardized PQ signing/KEM, hybrid operation, key lifecycle/rotation, and retained identity are absent. |
| OI-033 | missing integration | Planning classification and baseline data exist, but a real execution does not prove baseline reuse, freshness, dependency invalidation, and bounded delta planning together. |
| OI-034 | missing integration | The Goals graph schema exists, but interactive execution does not yet compile and persist a complete reusable baseline through all required conceptual responsibilities. |
| OI-035 | missing lifecycle/restart behavior | SQLite activation updates invocation metadata, but install/update/remove has not been demonstrated across restart and client-visible resolution through the stable control plane. |
| OI-036 | missing integration | A provider interface/profile exists, but runtime services still use mixed storage paths and have not passed one semantic conformance suite across provider substitution and mismatch. |
| OI-037 | missing lifecycle/restart behavior | Typed graph/agent/plugin contents can be registered, but clean install, activation, instantiation, update, rollback, removal, and restart are not one universal lifecycle. |

## Remediation order

1. Build cluster A as one durable adaptive-behavior lifecycle. Independently re-evaluate all eight claims; do not assume all close.
2. Build cluster B as one portable verified package/state lifecycle, including restart and privacy boundaries.
3. Build cluster C as one mediated execution envelope with real durable leases, process isolation, cancellation, and idempotent effect recovery.
4. Integrate workspace evidence (D), cryptographic/key providers (E), and Goal Baseline execution (F) into those substrate lifecycles.
5. After each cluster, create a new immutable execution attestation and frozen blind result, compare unrelated findings for regression, and retain every predecessor artifact unchanged.

Before accepting any closure, evaluate the evidence as though the target claim ID were unknown. Evidence tailored only to an expected status is inadmissible in spirit even if its file type matches the inventory.

## Successor blind audit

After the durable supervisor and scheduler substrate increments, the new blind
result `docs/research/conformance/blind-plugin-scheduler-substrate-v1.json`
was frozen with digest
`sha256:6c55874cf8d9290c7b3285c2826f40c0d843440f4cd2430dcb35116db3da7b57`.
It preserves the same 25 satisfied / 13 indeterminate state without loading
the withheld oracle. The substrate findings remain classified under Cluster C;
OI-021, OI-024, OI-025, OI-027, OI-029, and OI-030 require integrated lifecycle
and enforcement evidence beyond these components.

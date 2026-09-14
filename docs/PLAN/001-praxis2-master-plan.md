# PLAN-001: Praxis 2 Master Delivery Plan

- Status: REOPENED — GOAL CONFORMANCE
- Branch: `redesign/praxis2`
- Architecture authority: `docs/ADR/*`
- Implementation authority: `docs/SPEC/*`
- Reopened: 2026-09-13

## Purpose

Deliver Praxis 2 as a domain-neutral platform for persistent, self-improving agents and graphs with deterministic authority below the LLM.

The previous 100% marker is invalid as a goal-satisfaction claim. It proved completion against the then-current plan, not independent conformance to original intent. ADR-049/SPEC-018 add an independent Goal Conformance gate so an incomplete plan cannot define its own successful denominator.

## Delivery laws

1. ADRs define durable architectural decisions and rationale.
2. SPECs define executable contracts, invariants, failure behavior, security properties, and acceptance criteria.
3. PLANs define dependency order, integration gates, qualification, and completion evidence.
4. Existing/legacy implementation is salvageable evidence, not architectural authority.
5. Cross-domain behavior belongs in core contracts; domain behavior belongs in packages/graphs.
6. Client skills, hooks, prompts, MCP descriptions, and generated instructions are adapters, not authority.
7. If an LLM can choose to ignore a control, the control is advisory rather than enforcement.
8. Untrusted content is evidence/data, not authority.
9. Every external side effect remains within deterministic authority through the final commit/dispatch boundary.
10. Cryptography is profile-driven and algorithm-agile; Praxis-native asymmetric protection prefers standardized post-quantum mechanisms and fails closed on required-profile mismatch.
11. Expensive discovery and durable uncertainty are compiled once into reusable Goal/Planning Baselines when justified; downstream slices perform delta planning rather than rediscovery.
12. Planning rigor is progressive. Direct work retains a fast path; material work escalates to Goals/architecture/specification/planning.
13. Praxis core owns a stable control plane. Domain/package CLI commands are dynamically materialized from installed InvocationContracts and are never hard-coded into core.
14. A Praxis Package is the universal distribution unit. Graphs, agent definitions, preferences, templates, and executable plugins are typed package contents; plugin is not synonymous with package.
15. Authoritative persistence is defined by semantic provider contracts. SQLite is the default/reference implementation, not the architecture.
16. Distribution transport is replaceable. GitHub Releases is the initial adapter and never becomes root of trust.
17. Plan completion is not goal completion. Critical original-goal claims require independent admissible conformance evidence.
18. Blind qualification findings are frozen before any expected-gap oracle is loaded.
19. Self-improvement changes active behavior only through candidate evaluation, replay/regression, governed promotion, and rollback.

## Previously delivered waves

Waves 0-15 remain implementation evidence, but their COMPLETE labels are no longer sufficient for release closure. They must be re-evaluated through Goal Conformance. Existing implementation includes canonical contracts, authoritative event/state boundaries, graph runtime, plugin isolation/capabilities, Workspace Intelligence, inference routing, learning candidate/promotion primitives, agent identity/memory definitions, universal packages, personalization, client integration, portability classification, development/research proving domains, and adversarial/security qualification.

## Wave 16: Independent Goal Conformance and Governed Self-Improvement — IN PROGRESS

Required gates:

1. ADR-049 and SPEC-018 define blind goal conformance and post-freeze oracle separation.
2. Deterministic conformance evaluator classifies original-goal claims as satisfied/unsupported/contradicted/indeterminate.
3. Behavioral claims cannot be satisfied by prose-only evidence.
4. Findings are canonical and content-addressed before oracle comparison.
5. Blind Praxis audit runs using only original architectural intent plus implementation evidence.
6. Frozen findings are compared to a withheld external qualification oracle only afterward.
7. Material process failures produce governed learning candidates.
8. Candidate graph generation is immutable and cannot self-promote.
9. Candidate is sandboxed/replayed against original goal and regression corpus.
10. Security/policy/correctness regression gates precede promotion.
11. Promotion is versioned and rollback-capable.
12. Whole-system blind audit produces a machine-readable conformance report.
13. Findings rebuild this master plan denominator from demonstrated goal gaps.
14. All newly discovered critical gaps are implemented and re-audited until the blind audit is conformant.
15. Full branch CI is green at the final reconciled head.

### Versioned Wave 16 denominator

The initial original-intent denominator was frozen at 37 claims with claim-set digest `sha256:40162f681aa56bc3091ab4dd188d6857330bc8d495b11eea20e701c49bef2717`. Review of the capacity/handoff ownership decision exposed another incomplete original-goal decomposition: recovery and scheduler-lease claims did not explicitly require a domain-neutral resource-pressure-to-continuation lifecycle. The old denominator and every result produced from it remain immutable.

The controlled denominator transition adds OI-038, derived only from ADR-001, ADR-003, ADR-004, ADR-020, ADR-031, and ADR-034. ADR-050, SPEC-020, implementation, tests, and the reviewer's proposed API names are excluded as claim sources. The current denominator is frozen at 38 claims with claim-set digest `sha256:cc06b98af05518d4a10bfa25d69e5b1a2d0004c0a3e4867d1ff468cbfe0239a0`.

The corrected initial blind result is `docs/research/conformance/blind-source-qualified.json`, digest `sha256:03f7c9a8f07bd222989720f063fa0f354b4e087e578989fac322ab3f8f2f119c`. It found 2 satisfied, 12 unsupported, and 23 indeterminate critical claims. The post-freeze oracle qualification independently matched its positive control with one true positive and no false negative.

The denominator below is now fixed at version 38. New evidence may change finding states, but may not silently delete, weaken, or redefine a claim. Any later decomposition correction or genuinely superseded original goal requires another explicit, immutable denominator transition.

## Wave 17: Executed Evidence and Reproducible Attestation — IN PROGRESS

1. Define content-addressed execution attestations distinct from test source.
2. Bind command, exact source/evidence digests, environment/profile, exit state, and produced observations.
3. Verify attestations before raising evidence maturity to behavior/integration/lifecycle.
4. Record full Go, Python, clean-install, recovery, security, and conformance runs without allowing a test file to self-attest execution.
5. Preserve failed and partial runs as evidence.

Primary finding closures: all indeterminate claims; prerequisite for every later closure.

## Wave 18: Persistent Agent Runtime Composition — COMPLETE

1. Persist agent identity and immutable generations independently of executor/model identity.
2. Bind active operational graph versions, memory retrieval, preferences, capability history, policies, evaluation history, and lineage.
3. Execute receiving-goal, context/memory retrieval, action, evidence, reflection, and learning paths through an agent-owned graph.
4. Provide runtime-derived generation introspection and behavioral diff.
5. Prove provider replacement, process restart, rollback, and bounded-context retrieval.

Finding closures: OI-001, OI-002, OI-007, OI-008.

Closure: OI-001, OI-002, OI-007, and OI-008 are satisfied in frozen result `blind-agent-lifecycle.json` through authoritative restart, provider replacement, exact graph-version binding, full operational-role execution, scoped/bounded/supersession-aware persistent memory, immutable generation history, runtime-derived introspection, and lineage-preserving rollback.

## Wave 19: Goal Discovery and Actual Learning Loop — IN PROGRESS

Root-cause gate: before further Wave 19 remediation, all non-satisfied claims were classified independently in `docs/research/praxis2-finding-root-cause-classification.md`. OI-006, OI-012 through OI-017, and OI-020 are treated as one missing durable adaptive-behavior lifecycle, not as finding-specific patches. The remaining findings are grouped into portable package/state, mediated execution, workspace evidence, cryptographic lifecycle, and executable Goal Baseline capabilities. Claim definitions and frozen results remain unchanged, and no completion percentage is assigned.

1. Integrate process selection, adaptation, composition, novel candidate creation, and bounded one-off execution.
2. Observe repeated inference and outcomes; diagnose scope and repeated process behavior.
3. Generate immutable lower-inference candidates without encoding a qualification answer.
4. Replay original goals and an independent regression corpus, including model/provider portability.
5. Promote only through correctness/security/policy gates and distinct governance authority.
6. Persist failed candidates, active/rollback identities, promotion evidence, and restart recovery.
7. Implement prompt retirement, contradiction-driven fork/demotion, behavioral-profile evidence classes, and generalized cross-agent transfer.
8. Demonstrate longitudinal improvement in quality/variance/retries/intervention while reducing unnecessary repeated reasoning.

Finding closures: OI-003, OI-004, OI-005, OI-010, OI-013, OI-014, OI-015, OI-016, OI-017, OI-020.

Closure evidence: OI-003 is satisfied in `blind-process-discovery.json`. The resolver executes selection, contextual adaptation, exact-version fragment composition, governed candidate creation, and bounded one-off behavior while enforcing capability/policy eligibility below advisory inference.

OI-004 and OI-005 are satisfied in `blind-learning-prompt-retirement-v38-final.json`. Trusted repeated behavior is causally deduplicated and compiled into a content-addressed D0 candidate; original/regression equivalence plus security/policy gates precede distinct-authority promotion, and only the promoted generation retires redundant active prompt inference. Restart, rejection evidence, and rollback are retained.

Cluster-A substrate progress: SPEC-021 now defines a shared durable adaptive-behavior lifecycle. A domain-neutral event-backed ledger separately preserves content-addressed raw observations, derived measurements, explicit normalized scores, five non-interchangeable profile evidence classes, frozen policy thresholds, and traceable diagnoses. Software-delivery measures milliseconds/tokens/test counts while research measures seconds/evidence items/source diversity; neither is normalized for storage. Evaluator/version/transform and source evidence survive SQLite restart, and security/policy failures remain non-compensable. This substrate does not by itself close OI-006 or OI-012 through OI-017/OI-020; downstream preference, routing, transfer, and governed demotion integration remains required.

Contract-evolution checkpoint: ADR-051 records from repository history that adaptive observation/profile v1 was an untagged, unpublished redesign intermediate, not a compatibility commitment. Durable support starts at v2; pre-release v1 and unknown/future versions fail closed distinctly, while canonical v2 identity and authority-bound replay are regression tested. No finding status changes from this contract clarification.

Version-policy checkpoint: ADR-052 centralizes current, readable-historical, migratable, unsupported-pre-release, revoked/unsafe, and unknown dispositions in contract metadata with named deterministic upcasters. Adaptive replay is the first consumer. Signature/encryption envelopes, conformance attestations, continuation records, and package/graph/agent/preference/portable-state contracts require evidence-based adoption as their schema histories evolve; identity and aggregate-sequence versions are explicitly not conflated with schema compatibility. This checkpoint changes no finding status.

Reverse-adaptation progress: frozen package demotion policy can map a traceable diagnosis to an immutable child generation that restores bounded inference and removes only the contradicted deterministic rule. The compiled active generation is unchanged until exact-evaluation approval by a distinct authority; demotion, restart verification, rollback, and retained lineage work for software-delivery and research policies. Analysis policy/report pairs are now durable adaptive-ledger events. This remains cluster substrate/integration progress rather than an OI-017 closure until the whole cluster is independently attested and re-evaluated.

Preference-lifecycle closure: package-owned content-addressed contracts drive required-input discovery, safe default seeding, native values/scopes, and learnability. Profile divergence plus a frozen package drift policy produces a governed learned record; the exact generation-bound contract is resolved into every operational graph node. Explicit correction wins after SQLite restart and provider replacement. Generic append cannot mint learned or migrated authority, and one-to-one migration replay binds both contracts, the versioned transform, active origin, slot/value/scope/evidence, and preserved original authority. Historical preference attestations remain immutable; comprehensive successor `cluster-a-runtime-v5.json` binds the final sources and two-domain lifecycle. `blind-preference-lifecycle-v1.json` is retained with OI-012 indeterminate because its inventory mislabeled the executed runtime test as restart-only. Accepted successor `blind-preference-lifecycle-v2.json`, digest `sha256:95956a43b10e81cea320e9b36af0ce17232968c31506f3894e9a5ee48d4af6d7`, independently evaluates OI-006 and OI-012 as satisfied. Totals are 21 satisfied, 3 unsupported, and 14 indeterminate; no unrelated claim changed and the oracle was not loaded.

Evidence-routing progress: a frozen request now binds agent/run/goal/domain/behavior/context/tier, and deterministic eligibility evidence binds the exact request, executor, provider, authority, capability proof, and policy proof. Package routing policy selects typed native measurements, units, thresholds, objective direction, and independent-causation minimums; core preserves and validates those declarations without defining their domain meaning. Software-delivery latency in milliseconds and research verified-source counts route through the same mechanism, while a denied candidate cannot win through a favorable metric. Selection-only attestation `evidence-routing-v1.json` and integrated `adaptive-routing-runtime-v1.json` remain immutable. Successor `adaptive-routing-runtime-v2.json` proves the same dispatch/restart behavior using semantic evidence identities rather than incidental observation counts or positions. OI-013/OI-014 status is determined only by the next frozen blind audit.

Accepted routing results: immutable blind audit `blind-adaptive-routing-v1.json`, digest `sha256:fd22aa58dc4753de4779ba5d3a8116542c434806685fe5f8e5d5d6713cf573ba`, independently evaluated OI-013 and OI-014 as satisfied. Evidence-semantics review then classified exact counts as contract, fixture, implementation, or security cardinality and replaced count/position-shaped assertions in affected attested lifecycles with stable semantic identities. Successor attestations and `blind-adaptive-routing-v2.json`, digest `sha256:fd55e967abac466c065c048308514907971829b200838bffc405ef9f12a76024`, retain OI-001/OI-002/OI-004/OI-005/OI-007/OI-008/OI-010/OI-013/OI-014/OI-038 as satisfied under stronger evidence. The 38-claim denominator is unchanged; totals remain 15 satisfied, 7 unsupported, and 16 indeterminate. This closes only the demonstrated behaviors, not the remaining Cluster-A preference, transfer, longitudinal-analysis, demotion, or profile lifecycle claims.

Longitudinal-evaluation closure: a content-addressed plan binds exact package evaluator versions and frozen policy to an adaptive scope. Core loads durable observations, validates evaluator provenance/source scope, persists derived measurements, evaluates declared thresholds, and retains the full trace across restart without defining regression, repeated inference, variance, or provider-portability semantics. Software-delivery and research packages use distinct metric names and native raw units while jointly diagnosing those four architectural dimensions. Attestation `longitudinal-evaluation-v1.json` and frozen blind result `blind-longitudinal-evaluation-v1.json`, digest `sha256:5a472b7cd0948429c3e02a739842d1fd58e5ecf4fdfc766e55c200b4bc767528`, independently close OI-016. Totals are 16 satisfied, 6 unsupported, and 16 indeterminate; all other finding states are unchanged.

Cross-agent transfer progress: a content-addressed lifecycle now freezes the complete package scope/privacy/evaluator policy with exact source agent, generation, memory, content-digest, and causation lineage. Package processors generalize and sanitize domain material; core stores references rather than episodic payloads, rejects raw-content identity reuse or invented causation, treats privacy/security/policy failures as non-compensable, and requires distinct deterministic authority for publication and target-generation adoption. Software-delivery and research fixtures use different artifact kinds, source/target scopes, and sanitization controls; rejected privacy candidates remain durable and successful receiving-agent records replay after SQLite restart. This is not an OI-015 closure until successor execution evidence is frozen and the blind evaluator independently re-evaluates it.

Cross-agent transfer closure: successor attestation `cluster-a-runtime-v2.json` rebinds every affected state-directory observation and explicitly proves self-adoption and unauthorized adoption fail closed. Frozen audit `blind-cross-agent-transfer-v3.json`, digest `sha256:807e9fbed50c1aab74e16e07960b7da86e21f07bcea437fa18884cd1be7216bc`, independently evaluates OI-015 as satisfied with both lifecycle and security evidence. The v1/v2 audits remain immutable across the inventory-subject correction and final security extension. Totals are 17 satisfied, 5 unsupported, and 16 indeterminate; all unrelated claim states are unchanged.

Contradiction-demotion closure: an external two-domain integration persists native contradiction observations, the versioned derived equivalence measurement, and the exact analysis policy/report in the adaptive SQLite ledger, then restarts before deriving an immutable inference-restoring child. Proposal leaves the deterministic generation active; exact-evaluation approval by a distinct governor precedes activation, and restart/rollback retain both generations and source lineage. Successor `learning-runtime-v13.json` adds semantic source-identity and duplicate-causation checks. Frozen audit `blind-contradiction-demotion-v2.json`, digest `sha256:161889d19de605c2def1ae9c7f28009950c7cacb4fa25c82774506a90f4e13ab`, independently evaluates OI-017 as satisfied. Totals are 18 satisfied, 4 unsupported, and 16 indeterminate; all unrelated claim states are unchanged.

Profile-lifecycle progress: package evaluators now derive `observed` facts atomically with evaluator/version/transform/source lineage; the generic fact path rejects an observed enum without that derivation. A frozen package divergence policy selects opaque dimension/context, reference/current evidence classes, and threshold. Core preserves the five distinct evidence classes, compares only the declared normalized delta, persists the report, and re-evaluates it from authoritative history on restart. Software-delivery validation depth and research source breadth use different native observations, evaluators, dimensions, and policies. This does not close OI-020 until successor execution evidence and a blind re-evaluation are frozen.

Profile-lifecycle closure: comprehensive successor attestation `cluster-a-runtime-v4.json` binds the final atomic evaluator/fact append and every affected Cluster-A observation. Frozen audit `blind-behavioral-profile-v2.json`, digest `sha256:48101fc78022413985534e1b07eead8e7b7217bc4c04ea733502f6268a422645`, independently evaluates OI-020 as satisfied. The initial v1 profile audit remains immutable from before the generic observed-fact bypass was closed. Totals are 19 satisfied, 3 unsupported, and 16 indeterminate; all unrelated claim states remain unchanged.

## Wave 20: Portable State, Catalog, and Universal Package Lifecycle — IN PROGRESS

1. COMPLETE — Export/import canonical state rather than raw SQLite pages.
2. COMPLETE — Reconcile concurrent compatible, context-separated, conflicting, and stale-descendant state by lineage/provenance.
3. Prove package discovery, immutable resolution, provenance/signature verification, transitive capability review, and local authorization as one lifecycle.
4. COMPLETE — Prove private personalized state does not enter generalized/catalog artifacts.
5. Clean-install pure graph, agent-definition, and mixed packages; instantiate independent agents; update, disable, roll back, uninstall, and restart.
6. Prove dynamic CLI/client entry points follow active package generation atomically without rebuilding core.
7. COMPLETE — Prove provider substitution retains semantics and capability mismatch fails closed.

Finding closures: OI-009, OI-011, OI-019, OI-035, OI-036, OI-037.

Closure evidence: OI-009 is satisfied in `blind-portable-state-v1.json`. Content-addressed records and envelopes preserve source installation/agent/generation/session provenance, scope, evidence, trust, native payload, and immutable lineage. Two SQLite installations exchange semantic envelopes, union commutative learning, preserve context-separated preferences, retain divergent generation heads and policy conflicts without selection, reject duplicate/stale sequence misuse and portable runtime authority, preserve tombstones, and reconstruct the same state/conflicts after restart. Remaining Wave 20 package/catalog/provider claims are unresolved.

OI-011 is satisfied in `blind-catalog-bootstrap-v1.json`, digest `sha256:327b90084b156df28fb99f400a516e7722ac6e7d7218ec2c2e926357bf541205`. A transfer-lifecycle-minted generalized artifact crosses independent evaluation and distinct publication authority, is mapped by explicit package policy into a signed typed package, activates through local install authority, and reconstructs reusable behavior after SQLite restart. Private source text, agent/generation/memory identities, and raw source bytes are excluded from package manifest, archive, evidence, and persisted package rows. Software-delivery and research mappings prove that core preserves package-owned content semantics. The blind result was frozen without the qualification oracle; totals are 24 satisfied, 1 unsupported, and 13 indeterminate.

OI-019 is satisfied in `blind-distributed-package-trust-v1.json`, digest `sha256:6cbfe36574a20b2ab15851767bd42e34c1a5de69a90601f786d5687a59b712bf`. One lifecycle resolves an immutable dependency through a replaceable catalog, rejects adapter/signed-manifest split authority, verifies exact signatures and publisher/source provenance, computes the transitive capability/enforcement review, denies activation without separate local authority, then atomically activates the exact closure and reconstructs verification/approval lineage after SQLite restart. Totals are 25 satisfied and 13 indeterminate; no unsupported claim remains, no unrelated claim changed, and the oracle was not loaded.

OI-036 is satisfied in `blind-provider-substitution-v1.json`. The same canonical optimistic append/replay fixture runs through the semantic provider boundary against SQLite and the bounded in-memory provider; a provider missing required optimistic mutation capability fails before state changes. Successor attestations preserve prior evidence after the provider source changed.

Governed package-lifecycle progress: package verification now mints a sealed, content-addressed capability from exact manifest/artifact bytes, signature proofs, verified dependency evidence, transitive capabilities, enforcement requirements, provenance, and time. SQLite activation no longer accepts a bare manifest: it validates an exact activation intent, rechecks and atomically consumes its persisted local approval, records immutable verification/authority evidence, and publishes typed content and invocation registrations in the same transaction. The receipt and active client surface reconstruct after restart; caller-selected authority and failed registry collisions leave approval and package state unchanged. `package-lifecycle-v14.json` records the current focused execution; `cluster-a-runtime-v19.json` and `portable-state-v14.json` are successor attestations for previously satisfied evidence affected by state changes. Earlier attestations, including the invalid v8 Cluster-A capture with a misspelled observation identity, remain immutable historical evidence and are not used by the current inventory. OI-011 and OI-019 are independently closed; OI-035 and OI-037 remain open pending plugin lifecycle/isolation, graph dispatch across clients, and whole-lifecycle evidence.

Plugin-package binding progress: ADR-053 separates the versioned plugin definition from its opaque executable payload. Supported durable manifest semantics begin at v2; unsupported pre-release v1 cannot be upcast because it lacks executable identity. Package activation validates same-generation content ID/version, entrypoint, and digest binding and rejects orphaned or multiply referenced executables atomically. Restart resolution rechecks exact retained bytes and does not launch or grant a lease. Evidence fixtures use explicit lifecycle time and a clearly named authoritative store. Successor blind result `blind-plugin-package-substrate-v1.json`, digest `sha256:b37dcaec1d62d7791441602d2dfffadd9e96260198f96eaa5f64ff9a29c4cf8b`, retains 25 satisfied and 13 indeterminate claims without oracle access. This is package substrate only and does not close OI-021, OI-030, or OI-037 without governed real-process lifecycle/isolation evidence.

Plugin wire-contract progress: repository/tag/history review classifies the original `praxis.v1.PraxisPlugin` file as an unsupported pre-release skeleton, not a released external contract. ADR-054 nevertheless reserves every displaced handshake wire number/name, retains unchanged diagnostic identity, separates protobuf schema evolution from protocol-range negotiation, and removes plugin assertion of the negotiated protocol. Generated Go bindings and semantic compatibility tests prove old skeleton payloads cannot become current acceptance authority, runtime-bound instance/session identity cannot be overridden, incompatible ranges fail closed, and advertisement leaves the provider non-active. Successor attestations v20/v15/v15 preserve all source-affected accepted evidence. This is a contract-safety prerequisite, not OI-021/OI-030/OI-037 closure.

Capability-set review: ADR-055 evaluates manifest declaration, handshake advertisement, provider availability, routing, and lease authority as distinct facts. The signed manifest is a reviewed upper bound; a handshake-validated session may expose a safe subset, while any extra capability fails closed. A provider cannot publish caller-selected availability or `ready` state: supervisor publication requires the exact validated handshake result, and only its ephemeral advertised set is routable. Restart requires fresh validation and stale lease/session binding remains denied. The equality audit retained exact relations for signed bytes, dependency closures, AAD, and bound identity; it did not loosen them mechanically. Successor attestations v21/v16/v16 preserve source-affected accepted evidence. This remains enabling runtime substrate, not a finding closure.

Out-of-process transport progress: a separate executable fixture now uses the canonical Unix-socket gRPC service and exercises runtime-bound handshake identity, authority-derived protocol selection, unary execution, bidirectional streaming, deadline cancellation, crash observation, and fresh instance/session restart rejection. Successor attestations v27/v22/v22 preserve source-affected accepted evidence. The transport remains a substrate: package executable materialization, supervisor durable lifecycle/recovery, lease persistence integration, and effective OS isolation are still required before OI-021/OI-030/OI-037 closure.

Verified launch-boundary progress: `ResolvedPlugin.LaunchSpec` now carries the
exact digest-verified executable bytes reconstructed from active package state
into the host launcher. Launch validation rechecks identity, entrypoint, digest,
and combined manifest/policy isolation requirements; the local launcher
materializes only those bytes, strips ambient environment, and fails closed when
required isolation lacks an enforcer. This is an integration prerequisite, not
closure of OI-021/OI-030/OI-037: durable supervisor recovery, lease persistence,
real package update/revocation, and effective OS isolation remain outstanding.

Durable supervisor progress: ADR-056 and SQLite migration 0010 persist the
verified launch binding plus failure/quarantine/revocation state. Restored
`starting`/`ready`/`degraded` entries become stopped and require a fresh process
handshake; runtime advertisement and leases are never resurrected. This closes
no OI claim yet: complete process recovery, lease integration, update/revocation,
and host-enforced isolation evidence remain required.

Durable scheduler progress: ADR-057 and SQLite migration 0011 add an
authoritative resource-capacity/lease ledger. Deterministic ordered requests
expire stale leases and acquire all resources atomically; denied requests keep
no partial prefix, and explicit recovery fences expired leases for durable
observability across database restart. This is
admission substrate only: cancellation propagation, quota inheritance, retry
governance, child-work mediation, and complete OI-024 evidence remain open.

Package contract/transition progress: a contract-family catalog now rejects duplicate semantic ownership, and named package manifest, signature, verification, activation, deployment, transition, and rollback policies are the only current-version sources. Package release version remains distinct from manifest schema version; client invocation schema is owned by its separate client-contract catalog; SQLite receipt state evolves through migrations. Governed disable/remove operations bind exact generation and operation to an independently persisted approval, atomically withdraw active contents/aliases, retain receipts across restart, and reject caller mutation without consuming authority. OI findings remain open until resolution, privacy, plugin lifecycle, graph dispatch, and whole-lifecycle evidence are complete.

Rollback progress: core derives the exact dependency-first target closure from retained manifests, binds both current generation preconditions and target IDs/versions/digests to a rollback-specific contract and approval, revalidates retained manifest/artifact/signature/verification/content/invocation evidence, and restores the complete closure atomically. Successor generations remain immutable `rolled_back` history, client aliases follow the restored generation after SQLite restart, mutated targets preserve approval, and an unaffected active dependent blocks an incompatible generation switch. This is lifecycle substrate; findings remain open pending plugin/privacy/client-dispatch integration and independent whole-package evaluation.

Verified-content progress: package verification now proves the signed gzip-tar regular-file surface equals the typed manifest inventory, including per-content digests and safe canonical paths. Activation atomically retains the archive and selected content bytes; provider/state resolution rechecks exact bytes, and an active graph can be decoded and validated after SQLite restart rather than inferred from a metadata row. This is a missing deployability prerequisite, not closure of OI-019/OI-035/OI-037; dependency resolution, agent instantiation, plugin isolation, governed rollback, and complete dynamic invocation remain outstanding.

Dependency-deployment progress: a transport-neutral resolver walks signed exact dependency locks deterministically across multiple catalog kinds, rejects unavailable transports, identity/digest mismatch, cycles, and conflicting identities, and feeds only verifier-minted transitive packages into capability/evidence aggregation. GitHub Releases implements the initial locked-source adapter, while source locators remain non-authoritative transport metadata. Install/update now create a closure-bound deployment intent and activate every verified reachable generation dependency-first in one SQLite transaction. Missing, extra, reordered, lock-mismatched, or provenance-lineage-mismatched closure members fail closed; a root collision rolls back dependency writes and approval consumption. OI-019/OI-037 remain open pending governed rollback, privacy, plugin lifecycle/isolation, and whole-package lifecycle evidence.

Dynamic-client progress: the stable CLI resolves only the active persisted invocation registry and now emits exact package ID/version/digest plus graph ID/version. Valid-but-digest-mismatched registry bytes, stale aliases after update, and disabled aliases after restart fail closed; a disabled exact generation remains selectable for separately authorized removal. This establishes registry/client lifecycle semantics but does not yet prove graph dispatch or all client adapters, so OI-035 remains open.

Agent-definition progress: package-owned agent definitions now carry contract-owned version metadata and exact graph bindings. The state-provider boundary re-resolves digest-bound definition/graph bytes, compares an exact local identity/owner/governance intent, consumes persisted approval, and appends the initial agent generation atomically. Software-delivery and research packages instantiate independent identities that reconstruct through the agent runtime after SQLite restart; caller mutation leaves both approval and event state untouched. Generic agent creation no longer labels caller observations `user_confirmed`. OI-037 remains open pending complete dependency deployment, update/rollback, plugin lifecycle, and whole-package evidence.

## Wave 21: Runtime Mediation, Recovery, Isolation, and Cryptography — IN PROGRESS

1. Attest event/projection/checkpoint replay and causal observability after restart.
2. Attest negotiated out-of-process plugin lifecycle, streaming, cancellation, crash recovery, and authenticated instance binding.
3. Attest scheduler ordered acquisition, bounded starvation, cancellation release, lease expiry, and restart recovery.
4. Prove every mutation/effect entry surface traverses canonical command, policy, capability, approval, and effect commit boundaries.
5. Prove client degradation and required-mediation failure across supported adapters.
6. Prove untrusted content remains data through proposal, memory, and authority paths.
7. Prove effective plugin filesystem/network/process/credential/resource isolation or accurately fail closed where unavailable.
8. Attest canonical approval binding, atomic consumption, anti-replay, stale precondition rejection, and ambiguous-effect reconciliation.
9. Attest required PQ/classical/hybrid cryptographic profiles, downgrade resistance, durable algorithm identifiers, rotation, revocation, and authorization separation.
10. Attest Workspace Intelligence incremental freshness, path isolation, bounded context, and release authorization.

Finding closures: OI-021, OI-023, OI-024, OI-025, OI-026, OI-027, OI-028, OI-029, OI-030, OI-031, OI-032, OI-038.

Closure evidence: OI-023 and OI-031 are satisfied in `blind-attested-recovery-authority.json` by content-bound restart/replay and security/integration executions. OI-025 deliberately remains indeterminate because the narrower effect revalidation test does not prove complete mediation of every mutation/effect surface.

OI-038 is satisfied in `blind-resource-continuation.json`. Separate software-delivery and research profiles use different opaque pressure signals and thresholds while the same core mechanism governs event-led checkpoint, handoff, SQLite restart, exact-reference resume, and run/agent/evidence identity preservation. Calibration remains package/profile policy.

## Wave 22: Preferences, Goals, and Planning Lifecycle Qualification — IN PROGRESS

1. COMPLETE — Attest preference precedence, scope, explicit correction, drift, seeding, and contract migration.
2. Execute all Goals responsibilities interactively with interruption/resumption and baseline persistence.
3. Attest progressive rigor, baseline reuse, targeted invalidation, dependency-aware delta planning, and direct fast path.
4. Replay the historical plan-satisfied-itself scenario and unrelated omission regressions through the governed self-improvement generation.

Finding closures: OI-006, OI-012, OI-033, OI-034.

Closure evidence: OI-006 and OI-012 are satisfied in `blind-preference-lifecycle-v2.json`. The two-domain lifecycle proves governed drift and graph consumption, explicit correction across restart/provider replacement, material-only input/default behavior, and contract-bound migration with fail-closed incompatible reinterpretation. OI-033 and OI-034 remain unresolved.

## Wave 23: Final Blind Closure and Release Qualification — BLOCKED BY WAVES 17-22

1. Re-inventory implementation evidence without reading the oracle.
2. Freeze a new content-addressed blind result against the current 38-claim denominator.
3. Require every critical claim OI-001 through OI-038 to be `satisfied` by admissible evidence.
4. Load and score the withheld oracle only after freeze; require no false negative.
5. Run regression, adversarial/security, clean-install, restart/recovery, cross-provider, and full branch CI qualifications.
6. Reconcile ADR/SPEC/PLAN/report links to exact evidence and frozen digests.
7. Only after all gates pass may this plan return to COMPLETE and calculate 100% against the frozen denominator.

## Current evidence

Implemented in this reopened wave:

- `docs/ADR/049-goal-conformance-and-blind-self-improvement.md`
- `docs/SPEC/018-goal-conformance-and-self-improvement.md`
- `internal/conformance/evaluator.go`
- `internal/conformance/oracle.go`
- conformance boundary tests including behavioral prose rejection and order-stable freeze digests
- blind Praxis audit fixture derived from original ADR intent without oracle input
- immutable 37-claim initial denominator plus explicit 38-claim resource-continuation transition and content-digested evidence inventory
- immutable corrected blind report plus post-freeze semantic oracle score
- generic planning-process candidate generation, replay/regression comparison, independent-evidence/security/policy gates, governed promotion, failed-candidate retention, and rollback demonstration
- whole-system conformance report at `docs/research/praxis2-whole-system-conformance.md`
- frozen execution-attestation contract and command runner with stale/mutated/failed fail-closed validation
- self-improvement execution attestation and post-remediation blind result `blind-self-improvement-qualified.json` (OI-010 satisfied)
- persistent agent/runtime lifecycle result `blind-agent-lifecycle.json` (OI-001, OI-002, OI-007, and OI-008 satisfied)
- accepted recovery/authority result `blind-attested-recovery-authority.json` (OI-023 and OI-031 satisfied)
- retained rejected audit `blind-attested-existing-integrations.json`, documenting why graph composition evidence cannot close compound baseline-reuse claim OI-033
- SPEC-019 plus content-bound goal/process discovery result `blind-process-discovery.json` (OI-003 satisfied)
- ADR-050/SPEC-020 plus two-domain, SQLite-restart resource continuation result `blind-resource-continuation.json` (OI-038 satisfied)
- content-bound deterministic learning/prompt-retirement result `blind-learning-prompt-retirement-v38-final.json` (OI-004 and OI-005 satisfied)
- content-bound cross-agent generalization/privacy/publication/adoption result `blind-cross-agent-transfer-v3.json` (OI-015 satisfied)
- durable contradiction-to-inference fork/demotion result `blind-contradiction-demotion-v2.json` (OI-017 satisfied)
- evaluator-bound five-class behavioral profile/divergence result `blind-behavioral-profile-v2.json` (OI-020 satisfied)
- governed preference drift/correction/graph-consumption and contract-migration result `blind-preference-lifecycle-v2.json` (OI-006 and OI-012 satisfied)
- canonical multi-machine export/import/reconciliation result `blind-portable-state-v1.json` (OI-009 satisfied)

## Completion calculation

No percentage is asserted while the blind audit is establishing the true denominator. This is intentional: assigning a percentage before independent gap discovery would repeat the planning error ADR-049 exists to prevent.

## Definition of completion

Praxis 2 is complete only when a blind whole-system conformance run finds every critical original-goal claim satisfied by admissible evidence, all material findings have passed the governed self-improvement/remediation loop, the reconciled plan reflects those findings, and final CI/conformance evidence is green.

package conformance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// OriginalIntentSources is deliberately limited to the pre-conformance Praxis 2
// architecture corpus. ADR-049, plans, implementation, tests, and qualification
// fixtures cannot enter the denominator through this list.
var OriginalIntentSources = []string{
	"docs/ADR/001-praxis2-purpose-and-design-laws.md", "docs/ADR/002-core-ontology.md",
	"docs/ADR/003-persistent-agent-identity.md", "docs/ADR/004-agents-as-versioned-graphs.md",
	"docs/ADR/005-goal-and-process-discovery.md", "docs/ADR/006-learning-and-deterministic-extraction.md",
	"docs/ADR/007-advisory-state-and-prompt-retirement.md", "docs/ADR/008-contextual-preferences-and-drift.md",
	"docs/ADR/009-memory-and-retrieval.md", "docs/ADR/010-agent-lineage-generations-and-introspection.md",
	"docs/ADR/011-local-first-portable-state.md", "docs/ADR/012-governed-self-modification.md",
	"docs/ADR/013-catalog-and-bootstrap-packages.md", "docs/ADR/014-preference-contracts-and-install-seeding.md",
	"docs/ADR/015-inference-boundary-and-reasoning-tiers.md", "docs/ADR/016-demonstrated-capabilities-and-routing.md",
	"docs/ADR/017-cross-agent-knowledge-transfer.md", "docs/ADR/018-measurement-and-inference-redundancy.md",
	"docs/ADR/019-stabilization-adaptation-and-demotion.md", "docs/ADR/020-domain-overlays-and-process-packages.md",
	"docs/ADR/021-privacy-scope-and-catalog-contribution.md", "docs/ADR/022-execution-learning-and-governance-separation.md",
	"docs/ADR/023-human-authority-and-learning-precedence.md", "docs/ADR/024-multi-machine-state-reconciliation.md",
	"docs/ADR/025-catalog-trust-provenance-and-signing.md", "docs/ADR/026-behavioral-profiles-for-graphs-and-agents.md",
	"docs/ADR/027-praxis2-migration-and-compatibility.md", "docs/ADR/028-go-core-and-language-agnostic-plugin-boundary.md",
	"docs/ADR/029-grpc-protobuf-plugin-transport.md", "docs/ADR/030-canonical-domain-model-and-scope-inheritance.md",
	"docs/ADR/031-durable-event-log-and-projection-model.md", "docs/ADR/032-sqlite-local-authoritative-state-store.md",
	"docs/ADR/033-architecture-decision-lifecycle-and-authority.md", "docs/ADR/034-slice-scheduling-and-resource-concurrency.md",
	"docs/ADR/035-command-query-and-authoritative-mutation-boundary.md", "docs/ADR/036-versioned-canonical-contracts-and-schema-evolution.md",
	"docs/ADR/037-llm-client-integration-as-adapter-capabilities.md", "docs/ADR/038-enforcement-below-the-llm.md",
	"docs/ADR/039-workspace-intelligence-plugin.md", "docs/ADR/040-untrusted-content-and-instruction-data-separation.md",
	"docs/ADR/041-plugin-isolation-least-privilege-and-capability-leases.md", "docs/ADR/042-approval-binding-anti-replay-and-side-effect-commit.md",
	"docs/ADR/043-cryptographic-agility-and-post-quantum-preference.md", "docs/ADR/044-front-load-uncertainty-and-reuse-planning-artifacts.md",
	"docs/ADR/045-design-graph-as-domain-neutral-uncertainty-compiler.md", "docs/ADR/046-dynamic-package-command-registration-and-distribution.md",
	"docs/ADR/047-authoritative-state-provider-abstraction.md", "docs/ADR/048-packages-as-universal-distribution-unit.md",
}

// PraxisOriginalIntentClaims is the versioned denominator derived from the
// source corpus above. OI-038 is an explicit denominator transition after an
// ownership review exposed an incomplete decomposition in the initial 37
// claims. It contains no implementation plans, tests, code layout, remediation
// ADRs, or qualification expectations.
func PraxisOriginalIntentClaims() []Claim {
	return []Claim{
		claim("OI-001", "persistent agents retain identity and lineage across model or provider replacement", "ADR-001:4-5;ADR-003;ADR-010", true, StageLifecycle, "restart_test"),
		claim("OI-002", "persistent agents execute versioned operational graphs around bounded inference points", "ADR-004", true, StageIntegration, "integration_test"),
		claim("OI-003", "goal resolution selects, adapts, composes, creates, or bounds a process from goal and context", "ADR-001:6;ADR-005", true, StageIntegration, "integration_test"),
		claim("OI-004", "learning converts repeated successful inference into evaluated lower-inference behavior", "ADR-001:1-2;ADR-006;ADR-007", true, StageIntegration, "integration_test"),
		claim("OI-005", "superseded prompt instructions retire when deterministic mechanisms replace them", "ADR-007", true, StageBehavior, "runtime_test"),
		claim("OI-006", "preferences resolve deterministically by authority and scope and react to explicit correction and drift", "ADR-008;ADR-014;ADR-023", true, StageIntegration, "integration_test"),
		claim("OI-007", "durable memory is typed and selectively retrieved without unbounded prompt growth", "ADR-001:10;ADR-009", true, StageIntegration, "integration_test"),
		claim("OI-008", "agent generations are immutable append-only lineage with runtime-derived introspection and rollback", "ADR-010", true, StageLifecycle, "restart_test"),
		claim("OI-009", "canonical local state exports, imports, and reconciles across machines without last-writer-wins loss", "ADR-011;ADR-024;ADR-027", true, StageLifecycle, "restart_test", "integration_test"),
		claim("OI-010", "behavioral change uses a separate candidate generation, independent evaluation, governed promotion, and rollback", "ADR-012;ADR-022", true, StageLifecycle, "integration_test", "restart_test"),
		claim("OI-011", "catalog packages bootstrap reusable behavior without importing private personalized state", "ADR-013;ADR-017;ADR-021", true, StageIntegration, "integration_test", "security_test"),
		claim("OI-012", "preference contracts seed only material choices and migrate without silent reinterpretation", "ADR-014", true, StageBehavior, "runtime_test"),
		claim("OI-013", "reasoning tiers and executor selection remain model-independent and observable", "ADR-015;ADR-018", true, StageIntegration, "integration_test"),
		claim("OI-014", "routing uses demonstrated capability evidence without overriding eligibility or authority", "ADR-016", true, StageIntegration, "integration_test", "security_test"),
		claim("OI-015", "cross-agent learning transfers generalized scoped artifacts rather than raw episodic memory", "ADR-017;ADR-021", true, StageIntegration, "integration_test", "security_test"),
		claim("OI-016", "longitudinal evaluation detects regressions, repeated inference, variance, and model-portability failures", "ADR-018", true, StageIntegration, "integration_test"),
		claim("OI-017", "contradictory evidence can fork or demote deterministic behavior back toward inference", "ADR-019", true, StageIntegration, "integration_test"),
		claim("OI-018", "core execution, authority, evidence, and learning remain domain-neutral while overlays extend domain semantics", "ADR-002;ADR-020", false, StageContract, "code"),
		claim("OI-019", "distributed packages have immutable provenance, signatures, capability review, and explicit trust decisions", "ADR-025", true, StageIntegration, "integration_test", "security_test"),
		claim("OI-020", "behavioral profiles distinguish declared, inherited, observed, measured, and confirmed evidence", "ADR-026", true, StageBehavior, "runtime_test"),
		claim("OI-021", "the Go core supervises language-neutral out-of-process plugins with negotiated lifecycle, streaming, cancellation, and recovery", "ADR-028;ADR-029", true, StageLifecycle, "integration_test", "restart_test"),
		claim("OI-022", "canonical versioned contracts preserve stable identity, scope inheritance, compatibility, and migrations", "ADR-030;ADR-036", false, StageContract, "code", "schema_test"),
		claim("OI-023", "authoritative events and projections support replay, checkpoint recovery, and causal observability after restart", "ADR-031;ADR-032", true, StageLifecycle, "restart_test"),
		claim("OI-024", "slice scheduling uses ordered leases with bounded starvation, cancellation release, and recovery semantics", "ADR-034", true, StageLifecycle, "integration_test", "restart_test"),
		claim("OI-025", "every mutation and external effect traverses one command boundary with durable idempotent outcomes", "ADR-031;ADR-035", true, StageIntegration, "integration_test"),
		claim("OI-026", "client-neutral invocation contracts execute consistently across adapters and unavailable optional affordances degrade safely", "ADR-037", true, StageIntegration, "integration_test"),
		claim("OI-027", "authoritative actions are enforced deterministically below the LLM and fail closed when required mediation is absent", "ADR-038", true, StageIntegration, "security_test"),
		claim("OI-028", "workspace intelligence incrementally indexes authoritative sources and emits bounded fresh evidence context", "ADR-039", true, StageIntegration, "integration_test"),
		claim("OI-029", "untrusted content can influence proposals but cannot authorize actions or silently become authoritative memory", "ADR-040", true, StageIntegration, "security_test"),
		claim("OI-030", "plugin capability denial is backed by effective process, filesystem, network, credential, and resource isolation", "ADR-041", true, StageIntegration, "security_test"),
		claim("OI-031", "approval binds canonical action intent and is atomically revalidated and consumed at the side-effect commit boundary", "ADR-042", true, StageIntegration, "security_test", "integration_test"),
		claim("OI-032", "cryptographic profiles perform agile PQ-preferred signing and key establishment without silent downgrade and preserve rotation", "ADR-043", true, StageIntegration, "security_test", "integration_test"),
		claim("OI-033", "planning rigor is progressive and reusable baselines drive dependency-aware delta planning", "ADR-044", true, StageIntegration, "integration_test"),
		claim("OI-034", "the domain-neutral Goals graph interactively compiles original intent, uncertainty, decisions, models, specifications, and plans into a reusable baseline", "ADR-045", true, StageIntegration, "integration_test"),
		claim("OI-035", "installed packages atomically add and remove validated client-visible commands without rebuilding the core", "ADR-046", true, StageIntegration, "integration_test"),
		claim("OI-036", "runtime services use semantic authoritative-state provider contracts and fail closed on provider capability mismatch", "ADR-047", true, StageIntegration, "integration_test"),
		claim("OI-037", "pure graph, agent, and mixed packages install, activate, update, and roll back as universal distribution units", "ADR-048", true, StageLifecycle, "integration_test", "restart_test"),
		claim("OI-038", "domain-configured resource pressure can govern durable graph handoff and resume while preserving run, agent, and evidence identity", "ADR-001:3-5,11;ADR-003;ADR-004;ADR-020;ADR-031;ADR-034", true, StageLifecycle, "restart_test"),
	}
}

func claim(id, statement, source string, behavioral bool, stage EvidenceStage, kinds ...string) Claim {
	class := Structural
	if behavioral {
		class = Behavioral
	}
	return Claim{ID: id, Statement: statement, RequiredEvidence: kinds, Class: class, Criticality: Critical, RequiredStage: stage, SourceRef: source}
}

type InventoryArtifact struct {
	ID             string
	Kind           string
	Stage          EvidenceStage
	Ref            string
	ClaimIDs       []string
	AttestationRef string
	Observation    string
}

const (
	clusterARuntimeAttestation    = "docs/research/conformance/attestations/cluster-a-runtime-v45.json"
	goalsSessionAttestation       = "docs/research/conformance/attestations/goals-session-v6.json"
	planningLifecycleAttestation  = "docs/research/conformance/attestations/planning-lifecycle-v1.json"
	dynamicCLIAttestation         = "docs/research/conformance/attestations/dynamic-cli-lifecycle-v1.json"
	packageLifecycleQualification = "docs/research/conformance/attestations/universal-package-lifecycle-v1.json"
	cryptoLifecycleAttestation    = "docs/research/conformance/attestations/crypto-lifecycle-v1.json"
	trustBoundaryAttestation      = "docs/research/conformance/attestations/untrusted-content-boundary-v1.json"
	clientSurfaceAttestation      = "docs/research/conformance/attestations/client-surface-v1.json"
	portableStateAttestation      = "docs/research/conformance/attestations/portable-state-v37.json"
	packageLifecycleAttestation   = "docs/research/conformance/attestations/package-lifecycle-v37.json"
	pluginLifecycleAttestation    = "docs/research/conformance/attestations/plugin-lifecycle-v1.json"
	schedulerLifecycleAttestation = "docs/research/conformance/attestations/scheduler-lifecycle-v1.json"
)

// PraxisEvidenceInventory starts from observable artifacts. Claim mappings are
// explicit and reviewable; LoadEvidence hashes the actual file-tree bytes.
func PraxisEvidenceInventory() []InventoryArtifact {
	return []InventoryArtifact{
		{ID: "agent-definition", Kind: "runtime_test", Stage: StageBehavior, Ref: "internal/agent/definition_test.go", ClaimIDs: []string{"OI-001"}},
		{ID: "agent-runtime-restart", Kind: "restart_test", Stage: StageLifecycle, Ref: "internal/agent", ClaimIDs: []string{"OI-001"}, AttestationRef: clusterARuntimeAttestation, Observation: "TestPersistentAgentExecutesOperationalGraphAcrossRestartAndProviderReplacement"},
		{ID: "agent-operational-runtime", Kind: "integration_test", Stage: StageLifecycle, Ref: "internal/agent", ClaimIDs: []string{"OI-002"}, AttestationRef: clusterARuntimeAttestation, Observation: "TestPersistentAgentExecutesOperationalGraphAcrossRestartAndProviderReplacement"},
		{ID: "agent-memory-runtime", Kind: "integration_test", Stage: StageLifecycle, Ref: "internal/agent", ClaimIDs: []string{"OI-007"}, AttestationRef: clusterARuntimeAttestation, Observation: "TestPersistentMemoryRetrievalIsScopedBoundedAndSupersessionAware"},
		{ID: "agent-lineage-runtime", Kind: "restart_test", Stage: StageLifecycle, Ref: "internal/agent", ClaimIDs: []string{"OI-008"}, AttestationRef: clusterARuntimeAttestation, Observation: "TestAgentGenerationIntrospectionAndRollbackPreserveHistoryAcrossRestart"},
		{ID: "goal-process-discovery-runtime", Kind: "integration_test", Stage: StageIntegration, Ref: "internal/processresolver", ClaimIDs: []string{"OI-003"}, AttestationRef: "docs/research/conformance/attestations/goal-process-discovery.json", Observation: "TestGoalProcessDiscoveryResolvesAndExecutesAllModes"},
		{ID: "agent-memory", Kind: "security_test", Stage: StageBehavior, Ref: "internal/agent/memory_test.go", ClaimIDs: []string{"OI-007", "OI-029"}},
		{ID: "untrusted-content-boundary", Kind: "security_test", Stage: StageIntegration, Ref: "internal/trustboundary", ClaimIDs: []string{"OI-029"}, AttestationRef: trustBoundaryAttestation, Observation: "TestUntrustedContentRemainsDataThroughMemoryAndAuthorityBoundary"},
		{ID: "learning-extraction-runtime", Kind: "integration_test", Stage: StageLifecycle, Ref: "internal/learning", ClaimIDs: []string{"OI-004"}, AttestationRef: clusterARuntimeAttestation, Observation: "TestExecutionLearningCompilesBehaviorAndRetiresPromptThroughGovernedLifecycle"},
		{ID: "prompt-retirement-runtime", Kind: "runtime_test", Stage: StageLifecycle, Ref: "internal/learning", ClaimIDs: []string{"OI-005"}, AttestationRef: clusterARuntimeAttestation, Observation: "TestExecutionLearningCompilesBehaviorAndRetiresPromptThroughGovernedLifecycle"},
		{ID: "learning-promotion", Kind: "integration_test", Stage: StageLifecycle, Ref: "internal/learning", ClaimIDs: []string{"OI-010"}, AttestationRef: clusterARuntimeAttestation, Observation: "TestGenerationPromotionAndRollbackSurviveRestart"},
		{ID: "learning-restart", Kind: "restart_test", Stage: StageLifecycle, Ref: "internal/learning", ClaimIDs: []string{"OI-010"}, AttestationRef: clusterARuntimeAttestation, Observation: "TestGenerationPromotionAndRollbackSurviveRestart"},
		{ID: "contradiction-demotion-runtime", Kind: "integration_test", Stage: StageLifecycle, Ref: "internal/learning", ClaimIDs: []string{"OI-017"}, AttestationRef: clusterARuntimeAttestation, Observation: "TestDurableContradictionEvidenceForksBehaviorTowardInferenceAcrossDomains"},
		{ID: "preference-resolution", Kind: "runtime_test", Stage: StageBehavior, Ref: "internal/preference/resolver_test.go", ClaimIDs: []string{"OI-006", "OI-012"}},
		{ID: "preference-drift-runtime", Kind: "integration_test", Stage: StageLifecycle, Ref: "internal/agent", ClaimIDs: []string{"OI-006"}, AttestationRef: clusterARuntimeAttestation, Observation: "TestAgentGraphConsumesGovernedPreferenceDriftAndExplicitCorrectionAcrossRestart"},
		{ID: "preference-migration-runtime", Kind: "runtime_test", Stage: StageLifecycle, Ref: "internal/preference", ClaimIDs: []string{"OI-012"}, AttestationRef: clusterARuntimeAttestation, Observation: "TestPreferenceContractCorrectionMigrationAndRestartAcrossDomains"},
		{ID: "portable-state-integration", Kind: "integration_test", Stage: StageLifecycle, Ref: "internal/sync", ClaimIDs: []string{"OI-009"}, AttestationRef: portableStateAttestation, Observation: "TestCanonicalPortableStateReconcilesConcurrentMachinesAndSurvivesRestart"},
		{ID: "portable-state-restart", Kind: "restart_test", Stage: StageLifecycle, Ref: "internal/sync", ClaimIDs: []string{"OI-009"}, AttestationRef: portableStateAttestation, Observation: "TestCanonicalPortableStateReconcilesConcurrentMachinesAndSurvivesRestart"},
		{ID: "catalog-bootstrap-lifecycle", Kind: "integration_test", Stage: StageLifecycle, Ref: "internal/state", ClaimIDs: []string{"OI-011"}, AttestationRef: packageLifecycleAttestation, Observation: "TestCatalogBootstrapImportsGeneralizedBehaviorWithoutPrivateStateAcrossRestart"},
		{ID: "catalog-contribution-privacy", Kind: "security_test", Stage: StageIntegration, Ref: "internal/packagecatalog", ClaimIDs: []string{"OI-011"}, AttestationRef: packageLifecycleAttestation, Observation: "TestCatalogContributionRequiresGovernedGeneralizedPublication"},
		{ID: "distributed-package-trust-lifecycle", Kind: "integration_test", Stage: StageLifecycle, Ref: "internal/state", ClaimIDs: []string{"OI-019"}, AttestationRef: packageLifecycleAttestation, Observation: "TestDistributedPackageTrustBindsCatalogResolutionReviewAndLocalAuthorityAcrossRestart"},
		{ID: "distributed-package-local-authority", Kind: "security_test", Stage: StageIntegration, Ref: "internal/state", ClaimIDs: []string{"OI-019"}, AttestationRef: packageLifecycleAttestation, Observation: "TestVerificationCannotMintActivationAuthority"},
		{ID: "executor-routing", Kind: "integration_test", Stage: StageBehavior, Ref: "tests/test_executor_registry.py", ClaimIDs: []string{"OI-013", "OI-014"}},
		{ID: "adaptive-routing-integration", Kind: "integration_test", Stage: StageLifecycle, Ref: "internal/agent", ClaimIDs: []string{"OI-013", "OI-014"}, AttestationRef: clusterARuntimeAttestation, Observation: "TestOperationalGraphRoutesDispatchesObservesAndReplaysAcrossDomains"},
		{ID: "adaptive-routing-security", Kind: "security_test", Stage: StageLifecycle, Ref: "internal/agent", ClaimIDs: []string{"OI-014"}, AttestationRef: clusterARuntimeAttestation, Observation: "TestOperationalGraphRoutesDispatchesObservesAndReplaysAcrossDomains"},
		{ID: "longitudinal-evaluation-runtime", Kind: "integration_test", Stage: StageLifecycle, Ref: "internal/state", ClaimIDs: []string{"OI-016"}, AttestationRef: clusterARuntimeAttestation, Observation: "TestVersionedPackageEvaluatorsDiagnoseLongitudinalFailuresAcrossDomainsAndRestart"},
		{ID: "cross-agent-transfer-integration", Kind: "integration_test", Stage: StageLifecycle, Ref: "internal/state", ClaimIDs: []string{"OI-015"}, AttestationRef: clusterARuntimeAttestation, Observation: "TestCrossAgentTransferGeneralizesSanitizesPublishesAndAdoptsAcrossDomains"},
		{ID: "cross-agent-transfer-security", Kind: "security_test", Stage: StageLifecycle, Ref: "internal/state", ClaimIDs: []string{"OI-015"}, AttestationRef: clusterARuntimeAttestation, Observation: "TestCrossAgentTransferRetainsRejectedPrivacyCandidate"},
		{ID: "behavioral-profile-lifecycle", Kind: "runtime_test", Stage: StageLifecycle, Ref: "internal/state", ClaimIDs: []string{"OI-020"}, AttestationRef: clusterARuntimeAttestation, Observation: "TestBehavioralProfileDerivationAndDivergenceSurviveRestartAcrossDomains"},
		{ID: "canonical-contracts", Kind: "code", Stage: StageContract, Ref: "pkg/contracts", ClaimIDs: []string{"OI-018", "OI-022"}},
		{ID: "schema-conformance", Kind: "schema_test", Stage: StageContract, Ref: "tests/test_valid_contracts.py", ClaimIDs: []string{"OI-022"}},
		{ID: "plugin-lifecycle-integration", Kind: "integration_test", Stage: StageLifecycle, Ref: "internal/plugin/grpc_process_integration_test.go", ClaimIDs: []string{"OI-021"}, AttestationRef: pluginLifecycleAttestation, Observation: "TestOutOfProcessPluginHandshakeStreamingCancellationAndRestart"},
		{ID: "plugin-lifecycle-recovery", Kind: "restart_test", Stage: StageLifecycle, Ref: "internal/plugin/supervisor_test.go", ClaimIDs: []string{"OI-021"}, AttestationRef: pluginLifecycleAttestation, Observation: "TestSupervisorPersistsUnexpectedExitFromLocalProcess"},
		{ID: "event-recovery", Kind: "restart_test", Stage: StageLifecycle, Ref: "internal/state/run_control_integration_test.go", ClaimIDs: []string{"OI-023"}, AttestationRef: "docs/research/conformance/attestations/runtime-state-recovery.json", Observation: "TestQualificationRunControlSurvivesSQLiteRestart"},
		{ID: "projection-replay", Kind: "restart_test", Stage: StageLifecycle, Ref: "internal/projection/run_projection_test.go", ClaimIDs: []string{"OI-023"}, AttestationRef: "docs/research/conformance/attestations/runtime-state-recovery.json", Observation: "TestRunProjectionRebuildsFromAuthoritativeEvents"},
		{ID: "scheduler-admission-and-fairness", Kind: "integration_test", Stage: StageLifecycle, Ref: "internal/scheduler", ClaimIDs: []string{"OI-024"}, AttestationRef: schedulerLifecycleAttestation, Observation: "TestOrderRunnableBoundsStarvationUnderSustainedHigherPriorityLoad"},
		{ID: "scheduler-lease-recovery", Kind: "restart_test", Stage: StageLifecycle, Ref: "internal/state", ClaimIDs: []string{"OI-024"}, AttestationRef: schedulerLifecycleAttestation, Observation: "TestSchedulerResourceLeasesAreAtomicAndRecoverAfterRestart"},
		{ID: "effect-coordinator", Kind: "integration_test", Stage: StageIntegration, Ref: "internal/effect/coordinator_test.go", ClaimIDs: []string{"OI-025"}},
		{ID: "effect-approval-commit", Kind: "integration_test", Stage: StageIntegration, Ref: "internal/effect/coordinator_test.go", ClaimIDs: []string{"OI-031"}, AttestationRef: "docs/research/conformance/attestations/authority-effect-commit.json", Observation: "TestCommitRevalidatesImmediatelyBeforeDispatch"},
		{ID: "client-enforcement", Kind: "security_test", Stage: StageBehavior, Ref: "internal/client/enforcement_test.go", ClaimIDs: []string{"OI-026", "OI-027"}},
		{ID: "client-surface-equivalence", Kind: "integration_test", Stage: StageIntegration, Ref: "internal/clientadapt", ClaimIDs: []string{"OI-026"}, AttestationRef: clientSurfaceAttestation, Observation: "TestAdaptersPreserveInvocationSemanticsAndDegradeOptionalAffordances"},
		{ID: "workspace-runtime", Kind: "integration_test", Stage: StageBehavior, Ref: "plugins/workspace", ClaimIDs: []string{"OI-028"}},
		{ID: "plugin-isolation", Kind: "security_test", Stage: StageBehavior, Ref: "internal/plugin/isolation_test.go", ClaimIDs: []string{"OI-030"}},
		{ID: "approval-commit", Kind: "security_test", Stage: StageIntegration, Ref: "internal/state/authorized_transition_test.go", ClaimIDs: []string{"OI-031"}, AttestationRef: "docs/research/conformance/attestations/authority-effect-commit.json", Observation: "TestCommitTransitionAuthorizedLeaseConsumesOneShotAuthorityAtomically"},
		{ID: "crypto-profiles", Kind: "integration_test", Stage: StageIntegration, Ref: "internal/crypto", ClaimIDs: []string{"OI-032"}, AttestationRef: cryptoLifecycleAttestation, Observation: "TestKeyLifecycleRotationRevocationAndHistoricalVerification"},
		{ID: "crypto-profiles-security", Kind: "security_test", Stage: StageIntegration, Ref: "internal/crypto", ClaimIDs: []string{"OI-032"}, AttestationRef: cryptoLifecycleAttestation, Observation: "TestEnvelopePQRequiredFailsBeforeClassicalFallback"},
		{ID: "planning-runtime", Kind: "integration_test", Stage: StageIntegration, Ref: "packages/develop", ClaimIDs: []string{"OI-033"}, AttestationRef: planningLifecycleAttestation, Observation: "TestPlanningLifecycleReusesBaselineAndSelectivelyReplansSlices"},
		{ID: "goals-runtime", Kind: "integration_test", Stage: StageLifecycle, Ref: "packages/goals", ClaimIDs: []string{"OI-034"}, AttestationRef: goalsSessionAttestation, Observation: "TestGoalSessionCompilesFourMateriallyDifferentDomainBaselines"},
		{ID: "dynamic-package-cli", Kind: "integration_test", Stage: StageIntegration, Ref: "internal/state/package_registry_test.go", ClaimIDs: []string{"OI-035"}, AttestationRef: dynamicCLIAttestation, Observation: "TestPackageUpdateAtomicallyReplacesAliasAndContentSurface"},
		{ID: "dynamic-package-lifecycle-contract", Kind: "integration_test", Stage: StageLifecycle, Ref: "internal/state/package_registry_test.go", ClaimIDs: []string{"OI-037"}, AttestationRef: packageLifecycleQualification, Observation: "TestGovernedPackageRollbackRestoresExactClosureAcrossRestart"},
		{ID: "dynamic-package-lifecycle-restart", Kind: "restart_test", Stage: StageLifecycle, Ref: "internal/state/package_registry_test.go", ClaimIDs: []string{"OI-037"}, AttestationRef: packageLifecycleQualification, Observation: "TestVerifiedGraphArtifactSurvivesActivationAndRestart"},
		{ID: "state-provider", Kind: "integration_test", Stage: StageIntegration, Ref: "internal/stateprovider", ClaimIDs: []string{"OI-036"}, AttestationRef: portableStateAttestation, Observation: "TestEventRuntimeSemanticsSurviveProviderSubstitutionAndMismatchFailsClosed"},
		{ID: "resource-continuation-runtime", Kind: "restart_test", Stage: StageLifecycle, Ref: "internal/state/continuation_integration_test.go", ClaimIDs: []string{"OI-038"}, AttestationRef: "docs/research/conformance/attestations/resource-continuation-v2.json", Observation: "TestDomainNeutralResourceHandoffPreservesTwoDomainRunsAcrossSQLiteRestart"},
	}
}

func LoadEvidence(root string, inventory []InventoryArtifact) ([]Evidence, error) {
	var out []Evidence
	for _, a := range inventory {
		if a.ID == "" || a.Kind == "" || a.Ref == "" || len(a.ClaimIDs) == 0 {
			return nil, errors.New("inventory artifact requires id, kind, ref, and claims")
		}
		d, err := digestPath(filepath.Join(root, filepath.FromSlash(a.Ref)))
		if err != nil {
			return nil, fmt.Errorf("inventory %s: %w", a.ID, err)
		}
		stage := StageContract
		ref := a.Ref
		evidenceDigest := "sha256:" + d
		if a.AttestationRef != "" {
			attestationBytes, readErr := os.ReadFile(filepath.Join(root, filepath.FromSlash(a.AttestationRef)))
			if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
				return nil, fmt.Errorf("inventory %s attestation: %w", a.ID, readErr)
			}
			if readErr == nil {
				var attestation ExecutionAttestation
				if err := json.Unmarshal(attestationBytes, &attestation); err != nil {
					return nil, fmt.Errorf("inventory %s decode attestation: %w", a.ID, err)
				}
				if err := VerifyExecutionAttestation(attestation); err != nil {
					return nil, fmt.Errorf("inventory %s verify attestation: %w", a.ID, err)
				}
				if !containsString(attestation.Observations, a.Observation) {
					return nil, fmt.Errorf("inventory %s attestation lacks observation %s", a.ID, a.Observation)
				}
				for sourceRef, attestedDigest := range attestation.SourceDigests {
					sourceDigest, digestErr := SourceSetDigest(root, []string{sourceRef})
					if digestErr != nil {
						return nil, digestErr
					}
					if attestedDigest != sourceDigest {
						return nil, fmt.Errorf("inventory %s attested source %s is stale: got %s want %s", a.ID, sourceRef, attestedDigest, sourceDigest)
					}
				}
				outputDigest, outputErr := SourceSetDigest(root, []string{attestation.OutputRef})
				if outputErr != nil {
					return nil, outputErr
				}
				outputBytes, outputErr := os.ReadFile(filepath.Join(root, filepath.FromSlash(attestation.OutputRef)))
				if outputErr != nil {
					return nil, outputErr
				}
				if !goTestObservationPassed(string(outputBytes), a.Observation) {
					return nil, fmt.Errorf("inventory %s output does not prove passing observation %s", a.ID, a.Observation)
				}
				if attestation.SourceDigests[a.Ref] == "" {
					return nil, fmt.Errorf("inventory %s attestation does not bind primary source %s", a.ID, a.Ref)
				}
				if attestation.OutputDigest != outputDigest {
					return nil, fmt.Errorf("inventory %s attested output digest is stale", a.ID)
				}
				stage = a.Stage
				ref = a.AttestationRef
				evidenceDigest = attestation.Digest
			}
		}
		for _, claimID := range a.ClaimIDs {
			// Source bytes establish only that an implementation/test contract exists.
			// Runtime, integration, security, and lifecycle maturity require a
			// separate execution attestation; file names cannot self-attest behavior.
			out = append(out, Evidence{ID: a.ID + "#" + claimID, ClaimID: claimID, Kind: a.Kind, Subject: a.Ref, Ref: ref, Digest: evidenceDigest, Stage: stage, Supports: true})
		}
	}
	return out, nil
}

func goTestObservationPassed(output, observation string) bool {
	if observation == "" || strings.ContainsAny(observation, "\r\n") {
		return false
	}
	return strings.Contains(output, "--- PASS: "+observation+" (")
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func SourceSetDigest(root string, sources []string) (string, error) {
	paths := append([]string(nil), sources...)
	sort.Strings(paths)
	h := sha256.New()
	for _, ref := range paths {
		d, err := digestPath(filepath.Join(root, filepath.FromSlash(ref)))
		if err != nil {
			return "", err
		}
		_, _ = fmt.Fprintf(h, "%s\x00%s\n", ref, d)
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

func digestPath(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	if !info.IsDir() {
		b, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		h.Write(b)
		return hex.EncodeToString(h.Sum(nil)), nil
	}
	var files []string
	err = filepath.Walk(path, func(p string, entry os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(files)
	for _, p := range files {
		b, err := os.ReadFile(p)
		if err != nil {
			return "", err
		}
		rel, _ := filepath.Rel(path, p)
		_, _ = fmt.Fprintf(h, "%s\x00", filepath.ToSlash(rel))
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

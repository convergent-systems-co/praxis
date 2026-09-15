package contracts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"
)

// LifecycleComponentClass identifies the independently evolving installation
// surfaces. It is intentionally closed so a plan cannot silently introduce an
// unreviewed transition class.
type LifecycleComponentClass string

const (
	LifecycleBinary     LifecycleComponentClass = "binary"
	LifecycleSchema     LifecycleComponentClass = "storage_schema"
	LifecyclePackage    LifecycleComponentClass = "package_generation"
	LifecycleContract   LifecycleComponentClass = "contract"
	LifecycleCredential LifecycleComponentClass = "identity_credential"
	LifecycleAuthority  LifecycleComponentClass = "authority_generation"
	LifecycleEvidence   LifecycleComponentClass = "historical_evidence"
	LifecycleRuntime    LifecycleComponentClass = "runtime_state"
)

func (c LifecycleComponentClass) Validate() error {
	switch c {
	case LifecycleBinary, LifecycleSchema, LifecyclePackage, LifecycleContract,
		LifecycleCredential, LifecycleAuthority, LifecycleEvidence, LifecycleRuntime:
		return nil
	default:
		return fmt.Errorf("unknown lifecycle component class %q", c)
	}
}

type LifecycleEffectClass string

const (
	LifecycleAutomatic      LifecycleEffectClass = "automatic"
	LifecycleAuthorityBound LifecycleEffectClass = "authority_bound"
	LifecycleIrreversible   LifecycleEffectClass = "irreversible"
	LifecycleRecoveryOnly   LifecycleEffectClass = "recovery_only"
)

func (c LifecycleEffectClass) Validate() error {
	switch c {
	case LifecycleAutomatic, LifecycleAuthorityBound, LifecycleIrreversible, LifecycleRecoveryOnly:
		return nil
	default:
		return fmt.Errorf("unknown lifecycle effect class %q", c)
	}
}

type LifecycleEvidenceRef struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Digest string `json:"digest"`
	Source string `json:"source,omitempty"`
}

func (e LifecycleEvidenceRef) Validate() error {
	if e.ID == "" || e.Kind == "" || e.Source == "" {
		return errors.New("lifecycle evidence identity, kind, and source are required")
	}
	return ValidateSHA256Digest(e.Digest)
}

type LifecycleComponentRef struct {
	Class   LifecycleComponentClass `json:"class"`
	ID      string                  `json:"id"`
	Version string                  `json:"version"`
	Digest  string                  `json:"digest"`
}

func (c LifecycleComponentRef) Validate() error {
	if err := c.Class.Validate(); err != nil {
		return err
	}
	if c.ID == "" || c.Version == "" {
		return errors.New("lifecycle component identity and version are required")
	}
	return ValidateSHA256Digest(c.Digest)
}

type LifecycleAuthorityRequirement struct {
	Required                  bool   `json:"required"`
	Operation                 string `json:"operation,omitempty"`
	Scope                     string `json:"scope,omitempty"`
	RequestRef                string `json:"request_ref,omitempty"`
	RequestVersion            string `json:"request_version,omitempty"`
	RequestDigest             string `json:"request_digest,omitempty"`
	DecisionRef               string `json:"decision_ref,omitempty"`
	DecisionVersion           string `json:"decision_version,omitempty"`
	DecisionDigest            string `json:"decision_digest,omitempty"`
	AuthorityRef              string `json:"authority_ref,omitempty"`
	AuthorityVersion          string `json:"authority_version,omitempty"`
	AuthorityGenerationDigest string `json:"authority_generation_digest,omitempty"`
}

func (a LifecycleAuthorityRequirement) Validate() error {
	if !a.Required {
		if a.Operation != "" || a.Scope != "" || a.RequestRef != "" || a.RequestVersion != "" || a.RequestDigest != "" || a.DecisionRef != "" || a.DecisionVersion != "" || a.DecisionDigest != "" || a.AuthorityRef != "" || a.AuthorityVersion != "" || a.AuthorityGenerationDigest != "" {
			return errors.New("non-authority lifecycle step cannot carry authority bindings")
		}
		return nil
	}
	if a.Operation == "" || a.Scope == "" {
		return errors.New("authority-bound lifecycle step requires operation and scope")
	}
	for name, digest := range map[string]string{"request": a.RequestDigest, "decision": a.DecisionDigest, "authority generation": a.AuthorityGenerationDigest} {
		if digest == "" {
			continue
		}
		if err := ValidateSHA256Digest(digest); err != nil {
			return fmt.Errorf("lifecycle authority %s digest: %w", name, err)
		}
	}
	return nil
}

type LifecycleReadiness struct {
	CryptoBootstrap       string `json:"crypto_bootstrap"`
	StateStore            string `json:"state_store"`
	SchemaCompatibility   string `json:"schema_compatibility"`
	GovernanceRoot        string `json:"governance_root"`
	AuthorityTopology     string `json:"authority_topology"`
	PackageRuntimeClosure string `json:"package_runtime_closure"`
	LifecycleRecovery     string `json:"lifecycle_recovery"`
	Installation          string `json:"installation"`
}

func (r LifecycleReadiness) Validate() error {
	values := []string{r.CryptoBootstrap, r.StateStore, r.SchemaCompatibility, r.GovernanceRoot, r.AuthorityTopology, r.PackageRuntimeClosure, r.LifecycleRecovery, r.Installation}
	for _, value := range values {
		if value == "" {
			return errors.New("lifecycle readiness fields are required")
		}
	}
	return nil
}

type LifecycleTransitionStep struct {
	ID               string                        `json:"id"`
	Sequence         int                           `json:"sequence"`
	Class            LifecycleComponentClass       `json:"class"`
	Current          LifecycleComponentRef         `json:"current"`
	Target           LifecycleComponentRef         `json:"target"`
	Preconditions    []LifecycleEvidenceRef        `json:"preconditions"`
	PreservedHistory []LifecycleEvidenceRef        `json:"preserved_history,omitempty"`
	Authority        LifecycleAuthorityRequirement `json:"authority"`
	Effect           LifecycleEffectClass          `json:"effect"`
	SnapshotRequired bool                          `json:"snapshot_required"`
	Reversible       bool                          `json:"reversible"`
	RecoveryStrategy string                        `json:"recovery_strategy"`
	ReadinessImpact  string                        `json:"readiness_impact"`
}

func (s LifecycleTransitionStep) Validate() error {
	if s.ID == "" || s.Sequence < 1 || s.RecoveryStrategy == "" || s.ReadinessImpact == "" {
		return errors.New("lifecycle step identity, sequence, recovery, and readiness impact are required")
	}
	if err := s.Class.Validate(); err != nil {
		return err
	}
	if err := s.Current.Validate(); err != nil {
		return fmt.Errorf("current component: %w", err)
	}
	if err := s.Target.Validate(); err != nil {
		return fmt.Errorf("target component: %w", err)
	}
	if s.Current.Class != s.Class || s.Target.Class != s.Class {
		return errors.New("lifecycle step component class mismatch")
	}
	if err := s.Authority.Validate(); err != nil {
		return err
	}
	if err := s.Effect.Validate(); err != nil {
		return err
	}
	if s.Effect == LifecycleAuthorityBound && !s.Authority.Required {
		return errors.New("authority-bound lifecycle step requires authority")
	}
	if s.Effect == LifecycleIrreversible && !s.SnapshotRequired {
		return errors.New("irreversible lifecycle step requires a snapshot")
	}
	for _, evidence := range append(append([]LifecycleEvidenceRef{}, s.Preconditions...), s.PreservedHistory...) {
		if err := evidence.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type LifecyclePlan struct {
	PlanID                string                    `json:"plan_id"`
	PlanVersion           string                    `json:"plan_version"`
	Digest                string                    `json:"digest"`
	InstallationID        string                    `json:"installation_id"`
	CurrentManifestDigest string                    `json:"current_manifest_digest"`
	TargetManifestDigest  string                    `json:"target_manifest_digest"`
	Steps                 []LifecycleTransitionStep `json:"steps"`
	PreservedHistory      []LifecycleEvidenceRef    `json:"preserved_history"`
	SnapshotRequired      bool                      `json:"snapshot_required"`
	ExpectedReadiness     LifecycleReadiness        `json:"expected_readiness"`
}

func (p LifecyclePlan) Validate() error {
	if p.PlanID == "" || p.PlanVersion == "" || p.InstallationID == "" || len(p.Steps) == 0 {
		return errors.New("lifecycle plan identity, installation, and steps are required")
	}
	if err := ValidateSHA256Digest(p.Digest); err != nil {
		return err
	}
	if err := ValidateSHA256Digest(p.CurrentManifestDigest); err != nil {
		return err
	}
	if err := ValidateSHA256Digest(p.TargetManifestDigest); err != nil {
		return err
	}
	if err := p.ExpectedReadiness.Validate(); err != nil {
		return err
	}
	if !p.SnapshotRequired {
		for _, step := range p.Steps {
			if step.Effect == LifecycleIrreversible || step.SnapshotRequired {
				return errors.New("plan must require a snapshot for irreversible work")
			}
		}
	}
	seen := map[string]bool{}
	for i, step := range p.Steps {
		if err := step.Validate(); err != nil {
			return fmt.Errorf("step %d: %w", i, err)
		}
		if step.Sequence != i+1 || seen[step.ID] {
			return errors.New("lifecycle steps must have unique contiguous sequence")
		}
		seen[step.ID] = true
	}
	for _, evidence := range p.PreservedHistory {
		if err := evidence.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (p LifecyclePlan) ComputeDigest() (string, error) {
	copy := p
	copy.Digest = ""
	if err := copy.ValidateForDigest(); err != nil {
		return "", err
	}
	b, err := json.Marshal(copy)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func (p LifecyclePlan) ValidateForDigest() error {
	if p.PlanID == "" || p.PlanVersion == "" || p.InstallationID == "" || len(p.Steps) == 0 {
		return errors.New("lifecycle plan identity, installation, and steps are required")
	}
	if err := ValidateSHA256Digest(p.CurrentManifestDigest); err != nil {
		return err
	}
	if err := ValidateSHA256Digest(p.TargetManifestDigest); err != nil {
		return err
	}
	if err := p.ExpectedReadiness.Validate(); err != nil {
		return err
	}
	for i, step := range p.Steps {
		if err := step.Validate(); err != nil {
			return fmt.Errorf("step %d: %w", i, err)
		}
	}
	return nil
}

func (p LifecyclePlan) VerifyDigest() error {
	digest, err := p.ComputeDigest()
	if err != nil {
		return err
	}
	if digest != p.Digest {
		return errors.New("lifecycle plan digest mismatch")
	}
	return nil
}

type LifecycleTransitionState string

const (
	LifecyclePlanned           LifecycleTransitionState = "planned"
	LifecycleApproved          LifecycleTransitionState = "approved"
	LifecyclePrepared          LifecycleTransitionState = "prepared"
	LifecycleApplying          LifecycleTransitionState = "applying"
	LifecycleCommitted         LifecycleTransitionState = "committed"
	LifecycleFailedRecoverable LifecycleTransitionState = "failed_recoverable"
	LifecycleReconcileRequired LifecycleTransitionState = "reconcile_required"
	LifecycleRolledBack        LifecycleTransitionState = "rolled_back"
	LifecycleFenced            LifecycleTransitionState = "fenced"
)

func validLifecycleTransition(from, to LifecycleTransitionState) bool {
	allowed := map[LifecycleTransitionState][]LifecycleTransitionState{
		LifecyclePlanned:           {LifecycleApproved},
		LifecycleApproved:          {LifecyclePrepared},
		LifecyclePrepared:          {LifecycleApplying},
		LifecycleApplying:          {LifecycleCommitted, LifecycleFailedRecoverable, LifecycleReconcileRequired, LifecycleRolledBack},
		LifecycleFailedRecoverable: {LifecyclePrepared, LifecycleReconcileRequired, LifecycleRolledBack},
		LifecycleReconcileRequired: {LifecycleCommitted, LifecycleRolledBack, LifecycleFenced},
	}
	for _, candidate := range allowed[from] {
		if candidate == to {
			return true
		}
	}
	return false
}

type LifecycleTransitionJournal struct {
	JournalID                 string                   `json:"journal_id"`
	Version                   string                   `json:"version"`
	PlanID                    string                   `json:"plan_id"`
	PlanDigest                string                   `json:"plan_digest"`
	InstallationID            string                   `json:"installation_id"`
	Sequence                  int                      `json:"sequence"`
	StepID                    string                   `json:"step_id"`
	PreviousState             LifecycleTransitionState `json:"previous_state,omitempty"`
	State                     LifecycleTransitionState `json:"state"`
	PreconditionDigest        string                   `json:"precondition_digest"`
	SnapshotDigest            string                   `json:"snapshot_digest,omitempty"`
	AuthorityRef              string                   `json:"authority_ref,omitempty"`
	AuthorityVersion          string                   `json:"authority_version,omitempty"`
	AuthorityDecisionRef      string                   `json:"authority_decision_ref,omitempty"`
	AuthorityDecisionDigest   string                   `json:"authority_decision_digest,omitempty"`
	AuthorityGenerationDigest string                   `json:"authority_generation_digest,omitempty"`
	RecoveryAction            string                   `json:"recovery_action,omitempty"`
	ReadinessDigest           string                   `json:"readiness_digest"`
	RecordedAt                time.Time                `json:"recorded_at"`
}

func (j LifecycleTransitionJournal) Validate() error {
	if j.JournalID == "" || j.Version == "" || j.PlanID == "" || j.InstallationID == "" || j.StepID == "" || j.Sequence < 1 || j.RecordedAt.IsZero() {
		return errors.New("lifecycle journal identity, sequence, step, and timestamp are required")
	}
	if err := ValidateSHA256Digest(j.PlanDigest); err != nil {
		return err
	}
	if err := ValidateSHA256Digest(j.PreconditionDigest); err != nil {
		return err
	}
	if err := ValidateSHA256Digest(j.ReadinessDigest); err != nil {
		return err
	}
	if j.SnapshotDigest != "" {
		if err := ValidateSHA256Digest(j.SnapshotDigest); err != nil {
			return err
		}
	}
	if j.AuthorityGenerationDigest != "" {
		if err := ValidateSHA256Digest(j.AuthorityGenerationDigest); err != nil {
			return err
		}
	}
	if j.AuthorityDecisionDigest != "" {
		if err := ValidateSHA256Digest(j.AuthorityDecisionDigest); err != nil {
			return err
		}
	}
	if j.AuthorityVersion != "" && j.AuthorityRef == "" {
		return errors.New("lifecycle journal authority version requires authority reference")
	}
	if j.PreviousState == "" {
		if j.State != LifecyclePlanned {
			return errors.New("initial lifecycle journal state must be planned")
		}
	} else if !validLifecycleTransition(j.PreviousState, j.State) {
		return fmt.Errorf("invalid lifecycle journal transition %q to %q", j.PreviousState, j.State)
	}
	if j.State == LifecycleReconcileRequired && j.RecoveryAction == "" {
		return errors.New("reconciliation-required journal entry needs recovery action")
	}
	return nil
}

type LifecycleArtifactRef struct {
	ID                   string `json:"id"`
	Version              string `json:"version"`
	Digest               string `json:"digest"`
	MediaType            string `json:"media_type"`
	Size                 int64  `json:"size"`
	Provider             string `json:"provider,omitempty"`
	Reference            string `json:"reference,omitempty"`
	ProvenanceDigest     string `json:"provenance_digest"`
	Retention            string `json:"retention"`
	AvailabilityEvidence string `json:"availability_evidence"`
}

func (a LifecycleArtifactRef) Validate() error {
	if a.ID == "" || a.Version == "" || a.MediaType == "" || a.Size < 0 || a.Retention == "" || a.AvailabilityEvidence == "" {
		return errors.New("artifact identity, media type, size, retention, and availability evidence are required")
	}
	if err := ValidateSHA256Digest(a.Digest); err != nil {
		return err
	}
	if err := ValidateSHA256Digest(a.ProvenanceDigest); err != nil {
		return err
	}
	if a.Reference != "" && a.Provider == "" {
		return errors.New("external artifact reference requires provider identity")
	}
	if a.Reference == "" && a.Provider != "" {
		return errors.New("embedded artifact cannot name an external provider")
	}
	return nil
}

type LifecycleSnapshotEntry struct {
	Component LifecycleComponentRef `json:"component"`
	Artifact  LifecycleArtifactRef  `json:"artifact"`
}

func (e LifecycleSnapshotEntry) Validate() error {
	if err := e.Component.Validate(); err != nil {
		return err
	}
	return e.Artifact.Validate()
}

type LifecycleSnapshotManifest struct {
	SnapshotID                 string                    `json:"snapshot_id"`
	Version                    string                    `json:"version"`
	Digest                     string                    `json:"digest"`
	InstallationID             string                    `json:"installation_id"`
	InstallationManifestDigest string                    `json:"installation_manifest_digest"`
	RequiredClasses            []LifecycleComponentClass `json:"required_classes"`
	Entries                    []LifecycleSnapshotEntry  `json:"entries"`
	CapturedAt                 time.Time                 `json:"captured_at"`
}

func (s LifecycleSnapshotManifest) ComputeDigest() (string, error) {
	copy := s
	copy.Digest = ""
	if err := copy.ValidateForDigest(); err != nil {
		return "", err
	}
	b, err := json.Marshal(copy)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func (s LifecycleSnapshotManifest) ValidateForDigest() error {
	if s.SnapshotID == "" || s.Version == "" || s.InstallationID == "" || s.CapturedAt.IsZero() || len(s.RequiredClasses) == 0 {
		return errors.New("snapshot identity, installation, required classes, and capture time are required")
	}
	if err := ValidateSHA256Digest(s.InstallationManifestDigest); err != nil {
		return err
	}
	declared := map[LifecycleComponentClass]bool{}
	for _, class := range s.RequiredClasses {
		if err := class.Validate(); err != nil {
			return err
		}
		if _, exists := declared[class]; exists {
			return fmt.Errorf("snapshot repeats required component class %s", class)
		}
		declared[class] = false
	}
	components := map[string]bool{}
	for _, entry := range s.Entries {
		if err := entry.Validate(); err != nil {
			return err
		}
		if _, exists := declared[entry.Component.Class]; !exists {
			return fmt.Errorf("snapshot contains undeclared component class %s", entry.Component.Class)
		}
		key := string(entry.Component.Class) + "\x00" + entry.Component.ID
		if components[key] {
			return fmt.Errorf("snapshot repeats component %s/%s", entry.Component.Class, entry.Component.ID)
		}
		components[key] = true
		declared[entry.Component.Class] = true
	}
	for class, present := range declared {
		if !present {
			return fmt.Errorf("snapshot is incomplete: missing %s component", class)
		}
	}
	return nil
}

func (s LifecycleSnapshotManifest) VerifyDigest() error {
	digest, err := s.ComputeDigest()
	if err != nil {
		return err
	}
	if digest != s.Digest {
		return errors.New("snapshot manifest digest mismatch")
	}
	return nil
}

func (s LifecycleSnapshotManifest) Validate() error {
	if err := s.ValidateForDigest(); err != nil {
		return err
	}
	return ValidateSHA256Digest(s.Digest)
}

type LifecycleCredentialState string

const (
	CredentialActive        LifecycleCredentialState = "active"
	CredentialExpiring      LifecycleCredentialState = "expiring"
	CredentialExpired       LifecycleCredentialState = "expired"
	CredentialRetiring      LifecycleCredentialState = "retiring"
	CredentialRetiredVerify LifecycleCredentialState = "retired_verify_only"
	CredentialLost          LifecycleCredentialState = "lost"
	CredentialCompromised   LifecycleCredentialState = "compromised"
	CredentialRevoked       LifecycleCredentialState = "revoked"
	CredentialDestroyed     LifecycleCredentialState = "destroyed"
)

type LifecycleCredentialLineage struct {
	IdentityID           string                   `json:"identity_id"`
	CredentialRef        string                   `json:"credential_ref"`
	CredentialVersion    string                   `json:"credential_version"`
	State                LifecycleCredentialState `json:"state"`
	PredecessorRef       string                   `json:"predecessor_ref,omitempty"`
	Reason               string                   `json:"reason"`
	ContinuityEvidence   []LifecycleEvidenceRef   `json:"continuity_evidence,omitempty"`
	AuthorityDecisionRef string                   `json:"authority_decision_ref,omitempty"`
}

func (c LifecycleCredentialLineage) Validate() error {
	if c.IdentityID == "" || c.CredentialRef == "" || c.CredentialVersion == "" || c.Reason == "" {
		return errors.New("credential lineage identity, version, and reason are required")
	}
	switch c.State {
	case CredentialActive, CredentialExpiring, CredentialExpired, CredentialRetiring, CredentialRetiredVerify, CredentialLost, CredentialCompromised, CredentialRevoked, CredentialDestroyed:
	default:
		return fmt.Errorf("unknown credential state %q", c.State)
	}
	if c.PredecessorRef != "" && len(c.ContinuityEvidence) == 0 {
		return errors.New("credential replacement must retain continuity evidence")
	}
	for _, evidence := range c.ContinuityEvidence {
		if err := evidence.Validate(); err != nil {
			return err
		}
	}
	if c.State == CredentialActive && c.PredecessorRef != "" && c.AuthorityDecisionRef == "" {
		return errors.New("active replacement credential requires authority decision reference")
	}
	return nil
}

type LifecycleReconciliationOutcome string

const (
	AuthorityCompatible       LifecycleReconciliationOutcome = "compatible_authority_preserved"
	AuthorityHistoricalOnly   LifecycleReconciliationOutcome = "historical_only"
	AuthorityNewRequired      LifecycleReconciliationOutcome = "new_authority_required"
	AuthorityDecisionRequired LifecycleReconciliationOutcome = "authority_decision_required"
	AuthorityExternalRequired LifecycleReconciliationOutcome = "external_evidence_required"
	AuthorityUnrecoverable    LifecycleReconciliationOutcome = "unrecoverable_without_operator"
)

type LifecycleAuthorityReconciliation struct {
	ID                  string                         `json:"id"`
	CurrentAuthority    string                         `json:"current_authority,omitempty"`
	HistoricalAuthority string                         `json:"historical_authority,omitempty"`
	Outcome             LifecycleReconciliationOutcome `json:"outcome"`
	Reason              string                         `json:"reason"`
	Evidence            []LifecycleEvidenceRef         `json:"evidence"`
	DecisionRef         string                         `json:"decision_ref,omitempty"`
}

func (r LifecycleAuthorityReconciliation) Validate() error {
	if r.ID == "" || r.Reason == "" {
		return errors.New("authority reconciliation identity and reason are required")
	}
	switch r.Outcome {
	case AuthorityCompatible, AuthorityHistoricalOnly, AuthorityNewRequired, AuthorityDecisionRequired, AuthorityExternalRequired, AuthorityUnrecoverable:
	default:
		return fmt.Errorf("unknown authority reconciliation outcome %q", r.Outcome)
	}
	if r.Outcome == AuthorityCompatible && r.CurrentAuthority == "" {
		return errors.New("compatible authority outcome requires current authority")
	}
	if r.Outcome == AuthorityDecisionRequired && r.DecisionRef == "" {
		return errors.New("decision-required reconciliation needs decision reference")
	}
	for _, evidence := range r.Evidence {
		if err := evidence.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// StableSortedEvidence returns a copy ordered by evidence identity for callers
// that need deterministic digest construction without mutating the contract.
func StableSortedEvidence(in []LifecycleEvidenceRef) []LifecycleEvidenceRef {
	out := append([]LifecycleEvidenceRef(nil), in...)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

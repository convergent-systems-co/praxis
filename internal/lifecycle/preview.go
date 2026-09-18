// Package lifecycle contains read-only installation lifecycle planning and
// preview logic. It deliberately has no state-store or mutation dependency.
package lifecycle

import (
	"errors"
	"fmt"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type PreviewStatus string

const (
	PreviewReady             PreviewStatus = "ready"
	PreviewAuthorityRequired PreviewStatus = "authority_required"
	PreviewBlocked           PreviewStatus = "blocked"
)

type InstallationState struct {
	InstallationID    string
	ManifestDigest    string
	Components        []contracts.LifecycleComponentRef
	Readiness         contracts.LifecycleReadiness
	SnapshotAvailable bool
}

func (s InstallationState) Validate() error {
	if s.InstallationID == "" {
		return errors.New("installation identity is required")
	}
	if err := contracts.ValidateSHA256Digest(s.ManifestDigest); err != nil {
		return fmt.Errorf("installation manifest: %w", err)
	}
	if err := s.Readiness.Validate(); err != nil {
		return fmt.Errorf("installation readiness: %w", err)
	}
	seen := map[string]bool{}
	for _, component := range s.Components {
		if err := component.Validate(); err != nil {
			return fmt.Errorf("installation component: %w", err)
		}
		if seen[string(component.Class)+"\x00"+component.ID] {
			return fmt.Errorf("duplicate installation component %s/%s", component.Class, component.ID)
		}
		seen[string(component.Class)+"\x00"+component.ID] = true
	}
	return nil
}

type PreviewRequest struct {
	Plan                   contracts.LifecyclePlan
	Current                InstallationState
	AuthorityScopes        []string
	VerifiedSnapshotDigest string
}

type PreviewStep struct {
	ID                string                            `json:"id"`
	Sequence          int                               `json:"sequence"`
	Class             contracts.LifecycleComponentClass `json:"class"`
	Effect            contracts.LifecycleEffectClass    `json:"effect"`
	Reversible        bool                              `json:"reversible"`
	SnapshotRequired  bool                              `json:"snapshot_required"`
	AuthorityRequired bool                              `json:"authority_required"`
	RecoveryStrategy  string                            `json:"recovery_strategy"`
	ReadinessImpact   string                            `json:"readiness_impact"`
}

type Preview struct {
	PlanID            string                           `json:"plan_id"`
	PlanDigest        string                           `json:"plan_digest"`
	Status            PreviewStatus                    `json:"status"`
	Blockers          []string                         `json:"blockers,omitempty"`
	Steps             []PreviewStep                    `json:"steps"`
	PreservedHistory  []contracts.LifecycleEvidenceRef `json:"preserved_history"`
	CurrentReadiness  contracts.LifecycleReadiness     `json:"current_readiness"`
	ExpectedReadiness contracts.LifecycleReadiness     `json:"expected_readiness"`
	RollbackAvailable bool                             `json:"rollback_available"`
	SnapshotDigest    string                           `json:"snapshot_digest,omitempty"`
}

func PreviewLifecycle(req PreviewRequest) (Preview, error) {
	if err := req.Plan.Validate(); err != nil {
		return Preview{}, fmt.Errorf("lifecycle plan: %w", err)
	}
	if err := req.Plan.VerifyDigest(); err != nil {
		return Preview{}, fmt.Errorf("lifecycle plan digest: %w", err)
	}
	if err := req.Current.Validate(); err != nil {
		return Preview{}, fmt.Errorf("current installation: %w", err)
	}
	if req.Plan.InstallationID != req.Current.InstallationID {
		return Preview{}, errors.New("lifecycle plan targets a different installation")
	}
	if req.Plan.CurrentManifestDigest != req.Current.ManifestDigest {
		return Preview{}, errors.New("lifecycle plan current manifest does not match installation")
	}

	preview := Preview{
		PlanID:            req.Plan.PlanID,
		PlanDigest:        req.Plan.Digest,
		Status:            PreviewReady,
		Steps:             make([]PreviewStep, 0, len(req.Plan.Steps)),
		PreservedHistory:  append([]contracts.LifecycleEvidenceRef(nil), req.Plan.PreservedHistory...),
		CurrentReadiness:  req.Current.Readiness,
		ExpectedReadiness: req.Plan.ExpectedReadiness,
		SnapshotDigest:    req.VerifiedSnapshotDigest,
	}

	for _, step := range req.Plan.Steps {
		preview.Steps = append(preview.Steps, PreviewStep{
			ID: step.ID, Sequence: step.Sequence, Class: step.Class,
			Effect: step.Effect, Reversible: step.Reversible,
			SnapshotRequired:  step.SnapshotRequired,
			AuthorityRequired: step.Authority.Required,
			RecoveryStrategy:  step.RecoveryStrategy,
			ReadinessImpact:   step.ReadinessImpact,
		})
		if step.SnapshotRequired && !req.Current.SnapshotAvailable && req.VerifiedSnapshotDigest == "" {
			preview.Blockers = append(preview.Blockers, fmt.Sprintf("step %s requires a verified snapshot", step.ID))
		}
		if step.Authority.Required && !scopeGranted(req.AuthorityScopes, step.Authority.Scope) {
			preview.Blockers = append(preview.Blockers, fmt.Sprintf("step %s requires authority scope %q", step.ID, step.Authority.Scope))
		}
	}
	if len(preview.Blockers) != 0 {
		preview.Status = PreviewBlocked
		for _, step := range req.Plan.Steps {
			if step.Authority.Required && !scopeGranted(req.AuthorityScopes, step.Authority.Scope) {
				preview.Status = PreviewAuthorityRequired
				break
			}
		}
	}
	preview.RollbackAvailable = allStepsReversible(req.Plan.Steps)
	return preview, nil
}

func scopeGranted(granted []string, requested string) bool {
	for _, scope := range granted {
		if contracts.ScopeAllows(scope, requested) {
			return true
		}
	}
	return false
}

func allStepsReversible(steps []contracts.LifecycleTransitionStep) bool {
	if len(steps) == 0 {
		return false
	}
	for _, step := range steps {
		if !step.Reversible {
			return false
		}
	}
	return true
}

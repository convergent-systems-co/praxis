package goalstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/packages/goals"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func (r Repository) ReconcileAuthorityGate(ctx context.Context, baseline goals.GoalBaseline, candidate contracts.WorkCandidate, artifacts []contracts.GovernedArtifactEvidence, now time.Time) (contracts.AuthorityGateResult, error) {
	request, err := r.prepareAuthorityGate(ctx, baseline, candidate, artifacts, now)
	if err != nil {
		return contracts.AuthorityGateResult{}, err
	}
	if err := r.persistAuthorityGateRequest(ctx, request, now); err != nil {
		return contracts.AuthorityGateResult{}, fmt.Errorf("persist authority gate request: %w", err)
	}
	decision, err := r.LoadAuthorityDecision(ctx, request.ID, request.Version, now)
	if errors.Is(err, state.ErrSecureBlobNotFound) {
		return contracts.AuthorityGateResult{Request: request}, nil
	}
	if err != nil {
		return contracts.AuthorityGateResult{}, err
	}
	if err := decision.Validate(request, now); err != nil {
		return contracts.AuthorityGateResult{}, err
	}
	if err := r.validateGateDecisionAuthority(ctx, request, decision, now); err != nil {
		return contracts.AuthorityGateResult{}, err
	}
	if decision.Outcome != contracts.AuthorityApprove {
		return contracts.AuthorityGateResult{}, fmt.Errorf("authority gate %s was rejected", candidate.ID)
	}
	digest, err := decision.Digest()
	if err != nil {
		return contracts.AuthorityGateResult{}, err
	}
	return contracts.AuthorityGateResult{Request: request, Approved: true, DecisionDigest: digest}, nil
}

// prepareAuthorityGate is the shared front half of gate reconciliation and of
// gate-completion verification: activation, current plan authority, exact
// candidate specification, the exact qualified dossier, and the deterministic
// request that dossier implies. It persists nothing.
func (r Repository) prepareAuthorityGate(ctx context.Context, baseline goals.GoalBaseline, candidate contracts.WorkCandidate, artifacts []contracts.GovernedArtifactEvidence, now time.Time) (contracts.AuthorityRequest, error) {
	if baseline.WorkPlan == nil || baseline.WorkPlan.Safety == nil || candidate.Kind != contracts.WorkCandidateAuthorityGate || candidate.Provenance != contracts.ProvenanceAuthorityGate {
		return contracts.AuthorityRequest{}, errors.New("authority gate is not bound to an active safety-bearing WorkPlan")
	}
	// Activation is enforced here, at the shared boundary, and not left to the
	// caller: replaying an existing request is as protected as creating one.
	if err := r.requireSafetyActivation(ctx, baseline.WorkPlan.Safety); err != nil {
		return contracts.AuthorityRequest{}, err
	}
	// A gate is reachable only through the plan that contains it, so that
	// plan's acceptance must still be effective authority.
	if err := r.VerifyGoverningAuthority(ctx, baseline, now); err != nil {
		return contracts.AuthorityRequest{}, fmt.Errorf("gate is not governed by current WorkPlan authority: %w", err)
	}
	if err := candidate.ValidateAcceptedSpecification(); err != nil {
		return contracts.AuthorityRequest{}, err
	}
	contract, err := contracts.ParseAuthorityGateContract(candidate.Specification)
	if err != nil {
		return contracts.AuthorityRequest{}, err
	}
	artifact, dossier, err := contracts.ResolveGateDossier(candidate.ID, contract, artifacts)
	if err != nil {
		return contracts.AuthorityRequest{}, err
	}
	governanceScope, err := contracts.InstallationGovernanceScope(r.InstallationDigest)
	if err != nil {
		return contracts.AuthorityRequest{}, fmt.Errorf("authority gate requires the protected installation identity: %w", err)
	}
	return BuildAuthorityGateRequest(baseline, candidate, contract, artifact, dossier, governanceScope)
}

// BuildAuthorityGateRequest binds one exact gate question to the Goal
// generation, candidate, and qualified dossier it is about. Two scopes are
// kept distinct: governanceScope is the authority that must decide (the
// installation governance root, which the owner ceremony enforces), while the
// request's SubjectScope is the Goal generation being decided. Neither is
// substituted for the other.
func BuildAuthorityGateRequest(baseline goals.GoalBaseline, candidate contracts.WorkCandidate, contract contracts.AuthorityGateContract, artifact contracts.GovernedArtifactEvidence, dossier contracts.GateDossier, governanceScope string) (contracts.AuthorityRequest, error) {
	if err := artifact.Validate(); err != nil {
		return contracts.AuthorityRequest{}, err
	}
	idSum := sha256.Sum256([]byte(governanceScope + "\x00" + baseline.ID + "\x00" + baseline.Version + "\x00" + candidate.ID + "\x00" + candidate.SourceDigest + "\x00" + artifact.Digest))
	request := contracts.AuthorityRequest{
		ID: "goal-gate:" + hex.EncodeToString(idSum[:]), Version: "1",
		BaselineID: baseline.ID, BaselineVersion: baseline.Version, BaselineDigest: baseline.Digest,
		RequestedAuthority: "goal.gate.decide", RequestedScope: governanceScope, SubjectScope: contracts.GoalGateSubjectScope(baseline.ID, baseline.Version), Reason: contract.Question,
		AffectedWork: []string{candidate.ID}, Alternatives: append([]string(nil), dossier.OfferedAlternatives...), Recommendation: dossier.Recommendation, Status: contracts.AuthorityRequestPending,
		CeremonyProfile: "interactive-os-owner-v1", ActivationManifestDigest: baseline.WorkPlan.Safety.ActivationManifestDigest,
		GateCandidateID: candidate.ID, GateSpecificationDigest: candidate.SourceDigest,
		DossierRef: "checkpoint:" + artifact.Checkpoint + ":" + artifact.SourceRef, DossierDigest: artifact.Digest,
		DossierProducerCandidate: artifact.ProducerCandidateID, DossierRole: artifact.Role, DossierEvidenceClass: artifact.EvidenceClass,
		DossierSchemaID: artifact.SchemaID, DossierCheckpoint: artifact.Checkpoint,
	}
	if err := request.Validate(); err != nil {
		return contracts.AuthorityRequest{}, err
	}
	return request, nil
}

// validateGateDecisionAuthority proves the decider is the installation owner
// acting under the current installation-governance authority, independent of
// which CLI or caller produced the decision. It is applied both when a
// decision is recorded and when a recorded decision is consumed, so a decision
// made by another principal, under another scope, or by a revoked or
// superseded generation can never complete a gate.
func (r Repository) validateGateDecisionAuthority(ctx context.Context, request contracts.AuthorityRequest, decision contracts.AuthorityDecision, now time.Time) error {
	return r.validateOwnerDecisionAuthority(ctx, request, decision, now, true)
}

// persistAuthorityGateRequest is idempotent across turns and restarts: the
// request identity is deterministic in the exact gate, Goal generation, and
// dossier, so an existing record is accepted only when it is byte-identical in
// digest. Anything else under the same identity is a conflict, never replaced.
func (r Repository) persistAuthorityGateRequest(ctx context.Context, request contracts.AuthorityRequest, now time.Time) error {
	wanted, err := request.DigestAt(now)
	if err != nil {
		return err
	}
	existing, err := r.LoadAuthorityRequest(ctx, request.ID, request.Version, now)
	switch {
	case err == nil:
		existingDigest, digestErr := existing.DigestAt(now)
		if digestErr != nil || existingDigest != wanted {
			return errors.New("a different authority gate request already exists under this identity")
		}
		return nil
	case errors.Is(err, state.ErrSecureBlobNotFound):
		_, saveErr := r.SaveAuthorityRequest(ctx, request, now, nil)
		return saveErr
	default:
		return err
	}
}

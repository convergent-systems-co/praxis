package goalstore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/packages/goals"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// ErrSafetyPlanRequiresAttachment is returned when a safety-bearing WorkPlan
// reaches a persistence surface that is not the authenticated
// accepted-plan attachment transaction.
var ErrSafetyPlanRequiresAttachment = errors.New("a safety-bearing WorkPlan can enter a Goal generation only through the authenticated accepted-plan attachment transaction")

// VerifyGoverningAuthority proves that the authority governing a
// safety-bearing accepted WorkPlan is effective now. It re-resolves, from
// durable records, the exact acceptance decision the plan names and checks that
// it is an unrevoked approval of the exact request, issued by the current
// installation owner under an unrevoked, unsuperseded root generation, backed
// by a resolvable ceremony record. Immutable historic evidence stays readable
// (LoadAuthorityDecisionEvidence); this predicate governs only whether the plan
// may drive a new consequence. Non-safety plans are outside this boundary.
func (r Repository) VerifyGoverningAuthority(ctx context.Context, baseline goals.GoalBaseline, now time.Time) error {
	plan := baseline.WorkPlan
	if plan == nil || plan.Safety == nil {
		return nil
	}
	// The in-memory baseline must be byte-identical to the persisted,
	// authenticated generation, so an attached plan's candidates, bindings and
	// lineage cannot differ from what was attached.
	if err := baseline.VerifyDigest(); err != nil {
		return fmt.Errorf("baseline digest: %w", err)
	}
	stored, err := r.Load(ctx, baseline.ID, baseline.Version, now)
	if err != nil {
		return fmt.Errorf("load persisted Goal generation: %w", err)
	}
	if stored.Digest != baseline.Digest {
		return errors.New("baseline is not the persisted Goal generation")
	}
	return r.verifyPlanAuthorityLineage(ctx, *plan, now)
}

// VerifyPlanAuthorityLineage is the lineage half of VerifyGoverningAuthority as
// an exported operation: it proves, from durable records only, that the
// authority a plan names is the exact, ceremony-backed, owner-issued approval of
// exactly this plan, without requiring the plan to be a persisted generation.
// Attachment uses it at the moment a plan is being attached.
func (r Repository) VerifyPlanAuthorityLineage(ctx context.Context, plan contracts.WorkPlan, now time.Time) error {
	return r.verifyPlanAuthorityLineage(ctx, plan, now)
}

// verifyPlanAuthorityLineage is the lineage half of VerifyGoverningAuthority,
// applied without the persisted-generation match at the one moment a plan is
// being attached (its successor generation does not exist yet).
func (r Repository) verifyPlanAuthorityLineage(ctx context.Context, plan contracts.WorkPlan, now time.Time) error {
	if plan.Safety == nil {
		return nil
	}
	if plan.AuthorityRequestID == "" || plan.AuthorityRequestVersion == "" || plan.AuthorityDecisionRef == "" || plan.AuthorityDecisionVersion == "" {
		return errors.New("safety-bearing WorkPlan lacks authority-backed acceptance lineage")
	}
	request, err := r.LoadAuthorityRequest(ctx, plan.AuthorityRequestID, plan.AuthorityRequestVersion, now)
	if err != nil {
		return fmt.Errorf("load governing authority request: %w", err)
	}
	if request.RequestedAuthority != contracts.GovernedWorkPlanAccept || request.ProposalDigest != plan.ProposalDigest || request.BaselineDigest != plan.BaselineDigest {
		return errors.New("governing authority request is not the exact WorkPlan acceptance request")
	}
	if request.CeremonyProfile != contracts.OwnerCeremonyProfile || request.ActivationManifestDigest != plan.Safety.ActivationManifestDigest {
		return errors.New("governing authority request lacks the exact ceremony and activation binding")
	}
	decision, err := r.LoadAuthorityDecision(ctx, plan.AuthorityRequestID, plan.AuthorityRequestVersion, now)
	if err != nil {
		return fmt.Errorf("governing WorkPlan acceptance is not currently effective: %w", err)
	}
	if decision.Outcome != contracts.AuthorityApprove || decision.DecisionRef != plan.AuthorityDecisionRef || decision.DecisionVersion != plan.AuthorityDecisionVersion || decision.AuthorityRef != plan.AuthorityRef || decision.AuthorityVersion != plan.AuthorityVersion || decision.AuthorityDigest != plan.AuthorityDigest || decision.AuthorityGenerationDigest != plan.AuthorityGenerationDigest || decision.DecidedBy != plan.AcceptedBy {
		return errors.New("WorkPlan acceptance lineage does not match its durable authority decision")
	}
	return r.validateOwnerDecisionAuthority(ctx, request, decision, now, false)
}

// validateOwnerDecisionAuthority proves the decider is the installation owner
// acting under the installation-governance authority and that the decision is
// backed by a resolvable ceremony record. requireCurrentRoot additionally
// demands that the decision was issued by the root generation that is current
// now (gates); an accepted WorkPlan instead remains governed by its own
// unrevoked, unsuperseded root generation.
func (r Repository) validateOwnerDecisionAuthority(ctx context.Context, request contracts.AuthorityRequest, decision contracts.AuthorityDecision, now time.Time, requireCurrentRoot bool) error {
	scope, err := contracts.InstallationGovernanceScope(r.InstallationDigest)
	if err != nil {
		return fmt.Errorf("protected authority requires the installation identity: %w", err)
	}
	owner, err := contracts.InstallationOwnerPrincipal(r.InstallationDigest)
	if err != nil {
		return err
	}
	if request.RequestedScope != scope || decision.GrantedScope != scope {
		return errors.New("protected decision is not scoped to this installation's governance authority")
	}
	if decision.DecidedBy != owner {
		return errors.New("protected decision was not made by this installation's owner")
	}
	validator := r.AuthorityGeneration
	if validator == nil {
		validator = r
	}
	if err := validator.ValidateAuthorityGeneration(ctx, decision, now); err != nil {
		return fmt.Errorf("validate decision authority generation: %w", err)
	}
	// I12: the issuing generation must be positively live. Absence of an
	// invalidation record is not, by itself, evidence that it still governs.
	if err := r.requireLive(ctx, state.AuthorityGenerationLiveNamespace, decision.AuthorityRef, decision.AuthorityVersion, decision.AuthorityGenerationDigest, now); err != nil {
		return fmt.Errorf("issuing authority generation has no live record: %w", err)
	}
	if requireCurrentRoot {
		root, err := r.LoadCurrentInstallationRoot(ctx, r.InstallationDigest, now)
		if err != nil {
			return fmt.Errorf("load current installation root: %w", err)
		}
		if decision.AuthorityRef != root.Ref || decision.AuthorityVersion != root.Version || decision.AuthorityGenerationDigest != root.Digest {
			return errors.New("protected decision was not issued by the current installation root")
		}
	}
	return r.verifyDecisionCeremony(ctx, request, decision, now)
}

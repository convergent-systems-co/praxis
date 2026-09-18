package lifecycle

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

const (
	authorityDecisionNamespace               = "authority_decision"
	authorityRevocationNamespace             = "authority_revocation"
	authorityGenerationNamespace             = "authority_generation"
	authorityGenerationInvalidationNamespace = "authority_generation_invalidation"
)

// AuthoritySource is the narrow existing durable authority boundary used by
// lifecycle transitions. Implementations must apply current expiry,
// revocation, and generation-lineage rules when loading/validating records.
type AuthoritySource interface {
	LoadAuthorityRequest(context.Context, string, string, time.Time) (contracts.AuthorityRequest, error)
	LoadAuthorityDecision(context.Context, string, string, time.Time) (contracts.AuthorityDecision, error)
	LoadAuthorityGeneration(context.Context, string, string, time.Time) (contracts.AuthorityGeneration, error)
	ValidateAuthorityGeneration(context.Context, contracts.AuthorityDecision, time.Time) error
}

// DurableAuthorityValidator binds an accepted lifecycle step to the exact
// durable request, decision, and issuing generation. It derives no authority.
type DurableAuthorityValidator struct {
	Source             AuthoritySource
	InstallationDigest string
	Now                func() time.Time
}

// TransactionalAuthorityGuard binds the immutable secure-blob records that
// backed the plaintext authority validation and rechecks their operational
// freshness through the driver's caller-owned transaction. It deliberately
// does not decrypt authority content inside internal/state.
type TransactionalAuthorityGuard struct {
	Store                  *state.Store
	Decision               contracts.AuthorityDecision
	DecisionRecordDigest   string
	GenerationRecordDigest string
	Now                    func() time.Time
}

// NewTransactionalAuthorityGuard captures the exact durable decision and
// generation records whose plaintext was validated by
// DurableAuthorityValidator. Apply later rejects substitution, expiry,
// revocation, or generation invalidation inside its own transaction.
func NewTransactionalAuthorityGuard(ctx context.Context, store *state.Store, decision contracts.AuthorityDecision, now time.Time) (*TransactionalAuthorityGuard, error) {
	if store == nil || now.IsZero() {
		return nil, errors.New("transactional authority guard requires store and time")
	}
	decisionRecord, err := store.GetSecureBlob(ctx, authorityDecisionNamespace, decision.RequestID, decision.RequestVersion, now)
	if err != nil {
		return nil, fmt.Errorf("bind authority decision record: %w", err)
	}
	generationRecord, err := store.GetSecureBlob(ctx, authorityGenerationNamespace, decision.AuthorityRef, decision.AuthorityVersion, now)
	if err != nil {
		return nil, fmt.Errorf("bind authority generation record: %w", err)
	}
	return &TransactionalAuthorityGuard{Store: store, Decision: decision, DecisionRecordDigest: decisionRecord.ObjectDigest, GenerationRecordDigest: generationRecord.ObjectDigest}, nil
}

func (g *TransactionalAuthorityGuard) RevalidateInTx(ctx context.Context, tx *sql.Tx) error {
	if g == nil || g.Store == nil || tx == nil {
		return errors.New("transactional authority guard is incomplete")
	}
	now := time.Now().UTC()
	if g.Now != nil {
		now = g.Now().UTC()
	}
	if g.Decision.ExpiresAt != nil && !now.Before(*g.Decision.ExpiresAt) {
		return errors.New("repair authority decision expired")
	}
	decisionRecord, err := g.Store.GetSecureBlobInTx(ctx, tx, authorityDecisionNamespace, g.Decision.RequestID, g.Decision.RequestVersion, now)
	if err != nil {
		return fmt.Errorf("repair authority decision is absent or expired: %w", err)
	}
	if decisionRecord.ObjectDigest != g.DecisionRecordDigest {
		return errors.New("repair authority decision record was substituted")
	}
	revoked, err := g.Store.IsSecureBlobRevokedInTx(ctx, tx, authorityRevocationNamespace, g.Decision.RequestID, g.Decision.RequestVersion)
	if err != nil {
		return err
	}
	if revoked {
		return errors.New("repair authority decision is revoked")
	}
	generationRecord, err := g.Store.GetSecureBlobInTx(ctx, tx, authorityGenerationNamespace, g.Decision.AuthorityRef, g.Decision.AuthorityVersion, now)
	if err != nil {
		return fmt.Errorf("repair authority generation is absent or expired: %w", err)
	}
	if generationRecord.ObjectDigest != g.GenerationRecordDigest {
		return errors.New("repair authority generation record was substituted")
	}
	invalidated, err := g.Store.IsSecureBlobRevokedInTx(ctx, tx, authorityGenerationInvalidationNamespace, g.Decision.AuthorityRef, g.Decision.AuthorityVersion)
	if err != nil {
		return err
	}
	if invalidated {
		return errors.New("repair authority generation is revoked or superseded")
	}
	return nil
}

func (v DurableAuthorityValidator) ValidateLifecycleAuthority(ctx context.Context, plan contracts.LifecyclePlan, step contracts.LifecycleTransitionStep) (contracts.AuthorityDecision, error) {
	if v.Source == nil {
		return contracts.AuthorityDecision{}, errors.New("durable lifecycle authority source is required")
	}
	req := step.Authority
	if !req.Required || plan.PlanID == "" || plan.PlanVersion == "" || plan.Digest == "" || step.ID == "" {
		return contracts.AuthorityDecision{}, errors.New("lifecycle authority subject does not bind the exact plan step")
	}
	now := time.Now().UTC()
	if v.Now != nil {
		now = v.Now().UTC()
	}
	request, err := v.Source.LoadAuthorityRequest(ctx, req.RequestRef, req.RequestVersion, now)
	if err != nil {
		return contracts.AuthorityDecision{}, fmt.Errorf("load lifecycle authority request: %w", err)
	}
	if err := contracts.ValidateSHA256Digest(v.InstallationDigest); err != nil || request.InstallationDigest != v.InstallationDigest {
		return contracts.AuthorityDecision{}, errors.New("lifecycle repair authority is not bound to the opened installation bootstrap")
	}
	requestDigest, err := request.Digest()
	if err != nil || requestDigest != req.RequestDigest || request.RequestedAuthority != req.Operation || request.RequestedScope != req.Scope {
		return contracts.AuthorityDecision{}, errors.New("lifecycle authority request binding mismatch")
	}
	expectedPrincipal, err := contracts.InstallationOwnerPrincipal(request.InstallationDigest)
	if err != nil {
		return contracts.AuthorityDecision{}, fmt.Errorf("derive lifecycle installation owner: %w", err)
	}
	expectedScope, err := contracts.InstallationGovernanceScope(request.InstallationDigest)
	if err != nil {
		return contracts.AuthorityDecision{}, fmt.Errorf("derive lifecycle governance scope: %w", err)
	}
	if request.RequestedScope != expectedScope {
		return contracts.AuthorityDecision{}, errors.New("lifecycle authority request is outside the canonical installation-governance scope")
	}
	decision, err := v.Source.LoadAuthorityDecision(ctx, req.RequestRef, req.RequestVersion, now)
	if err != nil {
		return contracts.AuthorityDecision{}, fmt.Errorf("load lifecycle authority decision: %w", err)
	}
	decisionDigest, err := decision.Digest()
	if err != nil || decisionDigest != req.DecisionDigest || decision.DecisionRef != req.DecisionRef || decision.DecisionVersion != req.DecisionVersion || decision.Outcome != contracts.AuthorityApprove || decision.GrantedScope != req.Scope || decision.AuthorityRef != req.AuthorityRef || decision.AuthorityVersion != req.AuthorityVersion || decision.AuthorityGenerationDigest != req.AuthorityGenerationDigest {
		return contracts.AuthorityDecision{}, errors.New("lifecycle authority decision binding mismatch")
	}
	if err := v.Source.ValidateAuthorityGeneration(ctx, decision, now); err != nil {
		return contracts.AuthorityDecision{}, fmt.Errorf("validate lifecycle authority generation: %w", err)
	}
	generation, err := v.Source.LoadAuthorityGeneration(ctx, decision.AuthorityRef, decision.AuthorityVersion, now)
	if err != nil {
		return contracts.AuthorityDecision{}, fmt.Errorf("load lifecycle authority generation: %w", err)
	}
	if generation.Principal != expectedPrincipal || generation.Scope != expectedScope || generation.Ref != expectedScope || generation.ProvenanceDigest != request.InstallationDigest || generation.ParentRef != "" || generation.ParentVersion != "" || generation.ParentDigest != "" || generation.DelegatedBy != (contracts.PrincipalRef{}) {
		return contracts.AuthorityDecision{}, errors.New("lifecycle repair authority is not the canonical installation root generation")
	}
	possessesOperation := false
	for _, authority := range generation.Authorities {
		if authority == req.Operation {
			possessesOperation = true
			break
		}
	}
	if !possessesOperation {
		return contracts.AuthorityDecision{}, errors.New("installation root generation does not possess the requested repair authority")
	}
	return decision, nil
}

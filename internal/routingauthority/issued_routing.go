package routingauthority

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/internal/goalstore"
	"github.com/convergent-systems-co/praxis/internal/inference"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type IssuedTargetContribution struct {
	Authority contracts.TargetAuthority `json:"authority"`
	Target    contracts.ExecutionTarget `json:"target"`
}
type IssuedSurfaceEligibility struct {
	RequestID     string                        `json:"request_id"`
	SurfaceID     string                        `json:"surface_id"`
	SurfaceDigest string                        `json:"surface_digest"`
	Evidence      inference.EligibilityEvidence `json:"evidence"`
}
type issuanceLoader interface {
	LoadRoutingIssuance(context.Context, contracts.RoutingIssuanceRef, time.Time) (contracts.RoutingIssuance, error)
}
type IssuedRoutingAuthority struct{ repository issuanceLoader }

type IssuedRouteLedger struct {
	ledger    *inference.RouteLedger
	authority *IssuedRoutingAuthority
}

func NewIssuedRouteLedger(store eventstore.Store, authority *IssuedRoutingAuthority) (*IssuedRouteLedger, error) {
	if authority == nil {
		return nil, errors.New("issued route ledger requires core-issued authority")
	}
	ledger, err := inference.NewRouteLedger(store)
	if err != nil {
		return nil, err
	}
	return &IssuedRouteLedger{ledger: ledger, authority: authority}, nil
}

func (l *IssuedRouteLedger) RecordUnifiedRoute(ctx context.Context, record inference.UnifiedRouteRecord) error {
	if len(record.TargetIssuances) == 0 || len(record.EligibilityIssuances) == 0 {
		return errors.New("authoritative route record requires immutable issuance identities")
	}
	surfaces := make([]inference.ExecutorSurface, len(record.Decision.Evaluations))
	for i, evaluation := range record.Decision.Evaluations {
		surfaces[i] = evaluation.Surface
	}
	recomputed, err := selectIssuedExecutorSurfaceAt(ctx, record.Decision.Request, surfaces, l.authority, record.TargetIssuances, record.EligibilityIssuances, record.Decision.DecidedAt, time.Now().UTC())
	if err != nil {
		return err
	}
	want, _ := json.Marshal(recomputed)
	got, _ := json.Marshal(record.Decision)
	if !bytes.Equal(want, got) {
		return errors.New("route decision does not match current issued authority")
	}
	return l.ledger.RecordUnifiedRoute(ctx, record)
}

func (l *IssuedRouteLedger) UnifiedRoutes(ctx context.Context, subject string) ([]inference.UnifiedRouteRecord, error) {
	records, err := l.ledger.UnifiedRoutes(ctx, subject)
	if err != nil {
		return nil, err
	}
	for _, record := range records {
		surfaces := make([]inference.ExecutorSurface, len(record.Decision.Evaluations))
		for i, evaluation := range record.Decision.Evaluations {
			surfaces[i] = evaluation.Surface
		}
		recomputed, err := selectIssuedExecutorSurfaceAt(ctx, record.Decision.Request, surfaces, l.authority, record.TargetIssuances, record.EligibilityIssuances, record.Decision.DecidedAt, time.Now().UTC())
		if err != nil {
			return nil, err
		}
		want, _ := json.Marshal(recomputed)
		got, _ := json.Marshal(record.Decision)
		if !bytes.Equal(want, got) {
			return nil, errors.New("persisted route decision no longer verifies against issued authority")
		}
	}
	return records, nil
}

func NewIssuedRoutingAuthority(repository *goalstore.Repository) (*IssuedRoutingAuthority, error) {
	if repository == nil || repository.BootstrapDigest == "" {
		return nil, errors.New("issued routing authority requires bootstrap-bound protected repository")
	}
	return &IssuedRoutingAuthority{repository: repository}, nil
}

func (a *IssuedRoutingAuthority) MergeExecutionTargets(ctx context.Context, refs []contracts.RoutingIssuanceRef) (contracts.EffectiveExecutionTarget, error) {
	return a.mergeExecutionTargetsAt(ctx, refs, time.Now().UTC())
}

func (a *IssuedRoutingAuthority) mergeExecutionTargetsAt(ctx context.Context, refs []contracts.RoutingIssuanceRef, now time.Time) (contracts.EffectiveExecutionTarget, error) {
	contributions := make([]contracts.ExecutionTargetContribution, 0, len(refs))
	for _, ref := range refs {
		issued, err := a.repository.LoadRoutingIssuance(ctx, ref, now)
		if err != nil {
			return contracts.EffectiveExecutionTarget{}, err
		}
		if issued.Kind != contracts.RoutingTargetContribution || issued.Authority != contracts.AuthorityRoutingTargetContributionIssue {
			return contracts.EffectiveExecutionTarget{}, errors.New("issuance is not a target-contribution approval")
		}
		var payload IssuedTargetContribution
		if err := json.Unmarshal(issued.Payload, &payload); err != nil {
			return contracts.EffectiveExecutionTarget{}, err
		}
		targetBytes, _ := json.Marshal(payload.Target)
		targetSum := sha256.Sum256(targetBytes)
		targetDigest := "sha256:" + hex.EncodeToString(targetSum[:])
		wantScope, scopeErr := contracts.RoutingTargetContributionScope(payload.Target.Scope, payload.Target.Version, targetDigest)
		if scopeErr != nil || issued.Scope != wantScope {
			return contracts.EffectiveExecutionTarget{}, errors.New("issued target scope mismatch")
		}
		contributions = append(contributions, contracts.ExecutionTargetContribution{Authority: payload.Authority, SourceRef: issued.ID, SourceDigest: issued.PayloadDigest, Target: payload.Target})
	}
	return contracts.MergeExecutionTargets(contributions)
}

func SelectExecutorSurface(ctx context.Context, request inference.SurfaceRouteRequest, surfaces []inference.ExecutorSurface, authority *IssuedRoutingAuthority, targetRefs, eligibilityRefs []contracts.RoutingIssuanceRef) (inference.SurfaceRoutingDecision, error) {
	now := time.Now().UTC()
	return selectIssuedExecutorSurfaceAt(ctx, request, surfaces, authority, targetRefs, eligibilityRefs, now, now)
}

func selectIssuedExecutorSurfaceAt(ctx context.Context, request inference.SurfaceRouteRequest, surfaces []inference.ExecutorSurface, authority *IssuedRoutingAuthority, targetRefs, eligibilityRefs []contracts.RoutingIssuanceRef, decidedAt, validationAt time.Time) (inference.SurfaceRoutingDecision, error) {
	if authority == nil {
		return inference.SurfaceRoutingDecision{}, errors.New("surface routing requires core-issued authority")
	}
	target, err := authority.mergeExecutionTargetsAt(ctx, targetRefs, validationAt)
	if err != nil {
		return inference.SurfaceRoutingDecision{}, err
	}
	want, _ := json.Marshal(target)
	got, _ := json.Marshal(request.Target)
	if !bytes.Equal(want, got) {
		return inference.SurfaceRoutingDecision{}, errors.New("surface request target is not reconstructed from issued contributions")
	}
	evidence := []inference.EligibilityEvidence{}
	for _, ref := range eligibilityRefs {
		issued, err := authority.repository.LoadRoutingIssuance(ctx, ref, validationAt)
		if err != nil {
			return inference.SurfaceRoutingDecision{}, err
		}
		if issued.Kind != contracts.RoutingSurfaceEligibility || issued.Authority != contracts.AuthorityRoutingSurfaceEligibilityIssue {
			return inference.SurfaceRoutingDecision{}, errors.New("issuance is not scoped surface eligibility")
		}
		var payload IssuedSurfaceEligibility
		if err := json.Unmarshal(issued.Payload, &payload); err != nil {
			return inference.SurfaceRoutingDecision{}, err
		}
		if payload.RequestID != request.ID || payload.Evidence.SurfaceID != payload.SurfaceID || payload.Evidence.SurfaceDigest != payload.SurfaceDigest || payload.Evidence.AuthorityGenerationRef != issued.GenerationRef || payload.Evidence.AuthorityGenerationVersion != issued.GenerationVersion || payload.Evidence.AuthorityGenerationDigest != issued.GenerationDigest || payload.Evidence.AuthorityID != issued.IssuedBy.ID {
			return inference.SurfaceRoutingDecision{}, errors.New("issued eligibility payload lineage mismatch")
		}
		wantScope, scopeErr := contracts.RoutingSurfaceEligibilityScope(request.ID, "1", request.ID, payload.SurfaceDigest)
		if scopeErr != nil || issued.Scope != wantScope {
			return inference.SurfaceRoutingDecision{}, errors.New("issued eligibility canonical scope mismatch")
		}
		evidence = append(evidence, payload.Evidence)
	}
	return inference.RecomputeIssuedSurfaceDecision(ctx, request, surfaces, evidence, decidedAt)
}

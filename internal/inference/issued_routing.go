package inference

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/internal/goalstore"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type IssuedTargetContribution struct {
	Authority contracts.TargetAuthority `json:"authority"`
	Target    contracts.ExecutionTarget `json:"target"`
}
type IssuedSurfaceEligibility struct {
	RequestID     string              `json:"request_id"`
	SurfaceID     string              `json:"surface_id"`
	SurfaceDigest string              `json:"surface_digest"`
	Evidence      EligibilityEvidence `json:"evidence"`
}
type IssuedRoutingAuthority struct{ repository *goalstore.Repository }

type IssuedRouteLedger struct {
	ledger    *RouteLedger
	authority *IssuedRoutingAuthority
}

func NewIssuedRouteLedger(store eventstore.Store, authority *IssuedRoutingAuthority) (*IssuedRouteLedger, error) {
	if authority == nil {
		return nil, errors.New("issued route ledger requires core-issued authority")
	}
	ledger, err := NewRouteLedger(store)
	if err != nil {
		return nil, err
	}
	return &IssuedRouteLedger{ledger: ledger, authority: authority}, nil
}

func (l *IssuedRouteLedger) RecordUnifiedRoute(ctx context.Context, record UnifiedRouteRecord) error {
	if len(record.TargetIssuances) == 0 || len(record.EligibilityIssuances) == 0 {
		return errors.New("authoritative route record requires immutable issuance identities")
	}
	surfaces := make([]ExecutorSurface, len(record.Decision.Evaluations))
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

func (l *IssuedRouteLedger) UnifiedRoutes(ctx context.Context, subject string) ([]UnifiedRouteRecord, error) {
	records, err := l.ledger.UnifiedRoutes(ctx, subject)
	if err != nil {
		return nil, err
	}
	for _, record := range records {
		surfaces := make([]ExecutorSurface, len(record.Decision.Evaluations))
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
	if repository == nil || repository.InstallationDigest == "" {
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
		if payload.Target.Scope != issued.Scope {
			return contracts.EffectiveExecutionTarget{}, errors.New("issued target scope mismatch")
		}
		contributions = append(contributions, contracts.ExecutionTargetContribution{Authority: payload.Authority, SourceRef: issued.ID, SourceDigest: issued.PayloadDigest, Target: payload.Target})
	}
	return contracts.MergeExecutionTargets(contributions)
}

type issuedEligibilityMap map[string]EligibilityEvidence

func (m issuedEligibilityMap) EvaluateRouteEligibility(_ context.Context, _ RouteRequest, candidate RouteCandidate) (EligibilityEvidence, error) {
	value, ok := m[candidate.SurfaceID]
	if !ok {
		return EligibilityEvidence{}, errors.New("surface lacks issued eligibility")
	}
	return value, nil
}

func SelectExecutorSurface(ctx context.Context, request SurfaceRouteRequest, surfaces []ExecutorSurface, authority *IssuedRoutingAuthority, targetRefs, eligibilityRefs []contracts.RoutingIssuanceRef) (SurfaceRoutingDecision, error) {
	now := time.Now().UTC()
	return selectIssuedExecutorSurfaceAt(ctx, request, surfaces, authority, targetRefs, eligibilityRefs, now, now)
}

func selectIssuedExecutorSurfaceAt(ctx context.Context, request SurfaceRouteRequest, surfaces []ExecutorSurface, authority *IssuedRoutingAuthority, targetRefs, eligibilityRefs []contracts.RoutingIssuanceRef, decidedAt, validationAt time.Time) (SurfaceRoutingDecision, error) {
	if authority == nil {
		return SurfaceRoutingDecision{}, errors.New("surface routing requires core-issued authority")
	}
	target, err := authority.mergeExecutionTargetsAt(ctx, targetRefs, validationAt)
	if err != nil {
		return SurfaceRoutingDecision{}, err
	}
	want, _ := json.Marshal(target)
	got, _ := json.Marshal(request.Target)
	if !bytes.Equal(want, got) {
		return SurfaceRoutingDecision{}, errors.New("surface request target is not reconstructed from issued contributions")
	}
	evidence := issuedEligibilityMap{}
	for _, ref := range eligibilityRefs {
		issued, err := authority.repository.LoadRoutingIssuance(ctx, ref, validationAt)
		if err != nil {
			return SurfaceRoutingDecision{}, err
		}
		if issued.Kind != contracts.RoutingSurfaceEligibility || issued.Authority != contracts.AuthorityRoutingSurfaceEligibilityIssue || issued.Scope != request.ID {
			return SurfaceRoutingDecision{}, errors.New("issuance is not scoped surface eligibility")
		}
		var payload IssuedSurfaceEligibility
		if err := json.Unmarshal(issued.Payload, &payload); err != nil {
			return SurfaceRoutingDecision{}, err
		}
		if payload.RequestID != request.ID || payload.Evidence.SurfaceID != payload.SurfaceID || payload.Evidence.SurfaceDigest != payload.SurfaceDigest || payload.Evidence.AuthorityGenerationRef != issued.GenerationRef || payload.Evidence.AuthorityGenerationVersion != issued.GenerationVersion || payload.Evidence.AuthorityGenerationDigest != issued.GenerationDigest || payload.Evidence.AuthorityID != issued.IssuedBy.ID {
			return SurfaceRoutingDecision{}, errors.New("issued eligibility payload lineage mismatch")
		}
		if _, duplicate := evidence[payload.SurfaceID]; duplicate {
			return SurfaceRoutingDecision{}, errors.New("duplicate surface eligibility issuance")
		}
		evidence[payload.SurfaceID] = payload.Evidence
	}
	composer, _ := newSurfaceEligibilityComposer(evidence)
	return selectExecutorSurfaceAt(ctx, request, surfaces, composer, decidedAt)
}

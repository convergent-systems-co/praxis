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

type ReadinessRequest struct {
	Request              inference.SurfaceRouteRequest
	Surfaces             []inference.ExecutorSurface
	TargetIssuances      []contracts.RoutingIssuanceRef
	EligibilityIssuances []contracts.RoutingIssuanceRef
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

// RecordReadiness reconstructs and persists one authoritative v2 route without
// dispatching the selected surface. The API intentionally accepts no executor
// or effect callback: a readiness decision is evidence, not execution authority.
func (l *IssuedRouteLedger) RecordReadiness(ctx context.Context, request ReadinessRequest) (inference.UnifiedRouteRecord, error) {
	if l == nil || l.authority == nil || l.ledger == nil {
		return inference.UnifiedRouteRecord{}, errors.New("issued route readiness requires a core-issued durable ledger")
	}
	decision, err := SelectExecutorSurface(ctx, request.Request, request.Surfaces, l.authority, request.TargetIssuances, request.EligibilityIssuances)
	if err != nil {
		return inference.UnifiedRouteRecord{}, err
	}
	record, err := inference.FreezeUnifiedRouteRecord(inference.UnifiedRouteRecord{
		Decision:             decision,
		TargetIssuances:      request.TargetIssuances,
		EligibilityIssuances: request.EligibilityIssuances,
	})
	if err != nil {
		return inference.UnifiedRouteRecord{}, err
	}
	if err := l.RecordUnifiedRoute(ctx, record); err != nil {
		return inference.UnifiedRouteRecord{}, err
	}
	replayed, err := l.UnifiedRoutes(ctx, request.Request.RouteRequest.SubjectAgentID)
	if err != nil {
		return inference.UnifiedRouteRecord{}, err
	}
	matches := 0
	for _, candidate := range replayed {
		if candidate.ID == record.ID && candidate.Decision.Request.ID == request.Request.ID {
			matches++
		}
	}
	if matches != 1 {
		return inference.UnifiedRouteRecord{}, errors.New("persisted readiness route did not replay exactly once")
	}
	return record, nil
}

// PrepareDispatch reconstructs a selected surface from durable issued-route
// evidence and binds it to the active agent work. Separate dispatch authority
// is still required before an executor may be invoked.
func (l *IssuedRouteLedger) PrepareDispatch(ctx context.Context, binding inference.DispatchBinding) (inference.DispatchCandidate, error) {
	if l == nil || l.authority == nil || l.ledger == nil {
		return inference.DispatchCandidate{}, errors.New("issued dispatch preparation requires a core-issued durable ledger")
	}
	if binding.RequestID == "" || binding.SubjectAgentID == "" || binding.AgentGeneration == "" || binding.RunID == "" || binding.GraphID == "" || binding.GraphVersion == "" || binding.NodeID == "" || binding.GoalRef == "" {
		return inference.DispatchCandidate{}, errors.New("issued dispatch preparation requires exact request, agent, run, graph, node, and goal binding")
	}
	records, err := l.UnifiedRoutes(ctx, binding.SubjectAgentID)
	if err != nil {
		return inference.DispatchCandidate{}, err
	}
	var matched *inference.UnifiedRouteRecord
	for index := range records {
		if records[index].Decision.Request.ID != binding.RequestID {
			continue
		}
		if matched != nil {
			return inference.DispatchCandidate{}, errors.New("issued dispatch request has multiple authoritative routes")
		}
		matched = &records[index]
	}
	if matched == nil {
		return inference.DispatchCandidate{}, errors.New("issued dispatch request has no authoritative route")
	}
	request := matched.Decision.Request
	if request.RouteRequest.SubjectAgentID != binding.SubjectAgentID || request.AgentGeneration != binding.AgentGeneration || request.RouteRequest.RunID != binding.RunID || request.GraphID != binding.GraphID || request.GraphVersion != binding.GraphVersion || request.NodeID != binding.NodeID || request.GoalRef != binding.GoalRef {
		return inference.DispatchCandidate{}, errors.New("issued dispatch route does not bind the active agent work")
	}
	if matched.Decision.Outcome != inference.SurfaceSelected || matched.Decision.SelectedSurfaceID == "" {
		return inference.DispatchCandidate{}, errors.New("issued dispatch route has no selected surface")
	}
	var selected *inference.ExecutorSurface
	for index := range matched.Decision.Evaluations {
		if matched.Decision.Evaluations[index].Surface.ID == matched.Decision.SelectedSurfaceID {
			if selected != nil {
				return inference.DispatchCandidate{}, errors.New("issued dispatch route selects multiple surfaces")
			}
			selected = &matched.Decision.Evaluations[index].Surface
		}
	}
	if selected == nil {
		return inference.DispatchCandidate{}, errors.New("issued dispatch selected surface is absent")
	}
	return inference.DispatchCandidate{RouteRecordID: matched.ID, RequestID: request.ID, SurfaceID: selected.ID, ExecutorID: selected.ExecutorID, ProviderID: selected.ProviderID}, nil
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

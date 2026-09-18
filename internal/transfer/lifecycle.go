package transfer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

const (
	requestEvent     = "knowledge_transfer.requested"
	artifactEvent    = "knowledge_transfer.generalized"
	evaluationEvent  = "knowledge_transfer.evaluated"
	publicationEvent = "knowledge_transfer.published"
	adoptionEvent    = "knowledge_transfer.adopted"
)

// Processor belongs to a package. Core binds and verifies its outputs without
// interpreting the package's artifact contents or sanitization vocabulary.
type Processor interface {
	GeneralizeAndSanitize(context.Context, Request, TransferPolicy) (Artifact, error)
	Evaluate(context.Context, Artifact, TransferPolicy) (Evaluation, error)
}

type Authorizer interface {
	AuthorizeTransfer(context.Context, string, contracts.PrincipalRef, string, string) error
}

type Aggregate struct {
	Request     Request     `json:"request"`
	Artifact    Artifact    `json:"artifact"`
	Evaluation  Evaluation  `json:"evaluation"`
	Publication Publication `json:"publication"`
	Adoptions   []Adoption  `json:"adoptions"`
	Version     int64       `json:"version"`
}

type Lifecycle struct {
	store      eventstore.Store
	authorizer Authorizer
}

// PublishedArtifact is an in-process capability minted only after replay has
// verified accepted evaluation and distinct-authority publication. Package
// construction accepts this capability rather than caller-authored privacy
// booleans or raw memory payloads.
type PublishedArtifact struct {
	aggregate Aggregate
	sealed    bool
}

func (p PublishedArtifact) Validate() error {
	if !p.sealed {
		return errors.New("published artifact must be minted by the transfer lifecycle")
	}
	return validatePublishedAggregate(p.aggregate)
}

func (p PublishedArtifact) Artifact() Artifact {
	artifact := p.aggregate.Artifact
	artifact.SourceRefs = append([]SourceReference(nil), artifact.SourceRefs...)
	artifact.SanitizationEvidenceRefs = append([]string(nil), artifact.SanitizationEvidenceRefs...)
	return artifact
}
func (p PublishedArtifact) Evaluation() Evaluation {
	evaluation := p.aggregate.Evaluation
	evaluation.IndependentRoots = append([]string(nil), evaluation.IndependentRoots...)
	evaluation.Invariants = append([]InvariantResult(nil), evaluation.Invariants...)
	return evaluation
}
func (p PublishedArtifact) Publication() Publication { return p.aggregate.Publication }
func (p PublishedArtifact) Policy() TransferPolicy {
	policy := p.aggregate.Request.Policy
	policy.RequiredPrivacyControls = append([]string(nil), policy.RequiredPrivacyControls...)
	return policy
}

func (l *Lifecycle) ResolvePublished(ctx context.Context, requestID string) (PublishedArtifact, error) {
	aggregate, err := l.Inspect(ctx, requestID)
	if err != nil {
		return PublishedArtifact{}, err
	}
	if err := validatePublishedAggregate(aggregate); err != nil {
		return PublishedArtifact{}, err
	}
	return PublishedArtifact{aggregate: aggregate, sealed: true}, nil
}

func NewLifecycle(store eventstore.Store, authorizer Authorizer) (*Lifecycle, error) {
	if store == nil || authorizer == nil {
		return nil, errors.New("transfer event store and deterministic authorizer are required")
	}
	return &Lifecycle{store: store, authorizer: authorizer}, nil
}

func aggregateID(requestID string) string { return "knowledge-transfer:" + requestID }

func (l *Lifecycle) Propose(ctx context.Context, request Request, policy TransferPolicy, processor Processor) (Aggregate, error) {
	if err := VerifyRequest(request); err != nil {
		return Aggregate{}, err
	}
	if err := VerifyPolicy(policy); err != nil {
		return Aggregate{}, err
	}
	if processor == nil {
		return Aggregate{}, errors.New("package transfer processor is required")
	}
	if request.Policy.ID != policy.ID || request.SourceScope != policy.SourceScope || request.TargetScope != policy.TargetScope {
		return Aggregate{}, errors.New("transfer request does not match frozen package policy")
	}
	artifact, err := processor.GeneralizeAndSanitize(ctx, request, policy)
	if err != nil {
		return Aggregate{}, err
	}
	artifact, err = freezeArtifact(artifact, request, policy)
	if err != nil {
		return Aggregate{}, err
	}
	evaluation, err := processor.Evaluate(ctx, artifact, policy)
	if err != nil {
		return Aggregate{}, err
	}
	evaluation, err = freezeEvaluation(evaluation, artifact, policy)
	if err != nil {
		return Aggregate{}, err
	}
	events, err := encodeInitialEvents(request, artifact, evaluation)
	if err != nil {
		return Aggregate{}, err
	}
	if _, err := l.store.Append(ctx, aggregateID(request.ID), 0, events); err != nil {
		return Aggregate{}, err
	}
	return l.Inspect(ctx, request.ID)
}

func (l *Lifecycle) Publish(ctx context.Context, requestID string, authority contracts.PrincipalRef, authorityRef string, at time.Time) (Publication, error) {
	state, err := l.Inspect(ctx, requestID)
	if err != nil {
		return Publication{}, err
	}
	if state.Publication.ID != "" {
		return Publication{}, errors.New("transfer artifact is already published")
	}
	if !state.Evaluation.Accepted {
		return Publication{}, errors.New("rejected transfer candidate cannot be published")
	}
	if err := authority.Validate(); err != nil {
		return Publication{}, err
	}
	if authority.ID == state.Request.Proposer.ID {
		return Publication{}, errors.New("transfer candidate cannot publish itself")
	}
	if authorityRef == "" || at.IsZero() {
		return Publication{}, errors.New("publication authority evidence and time are required")
	}
	if err := l.authorizer.AuthorizeTransfer(ctx, "publish", authority, state.Artifact.Scope, authorityRef); err != nil {
		return Publication{}, fmt.Errorf("authorize transfer publication: %w", err)
	}
	publication := Publication{Version: transferPublicationVersions.CurrentVersion(), ArtifactID: state.Artifact.ID, EvaluationID: state.Evaluation.ID, Scope: state.Artifact.Scope, Authority: authority, AuthorityRef: authorityRef, PublishedAt: at.UTC()}
	publication.ID = digest("transfer-publication", publication)
	payload, _ := json.Marshal(publication)
	event := eventstore.Event{ID: "event:" + publication.ID, AggregateType: "knowledge_transfer", Type: publicationEvent, Version: transferPublicationVersions.CurrentVersion(), Actor: authority, CommandID: "publish:" + publication.ID, CorrelationID: requestID, CausationID: state.Evaluation.ID, Trust: contracts.TrustPolicy, Payload: payload, CreatedAt: at.UTC()}
	if _, err := l.store.Append(ctx, aggregateID(requestID), state.Version, []eventstore.Event{event}); err != nil {
		return Publication{}, err
	}
	return publication, nil
}

func (l *Lifecycle) Adopt(ctx context.Context, requestID, targetAgentID, targetGenerationID, receivingRecordID string, authority contracts.PrincipalRef, authorityRef string, at time.Time) (Adoption, error) {
	state, err := l.Inspect(ctx, requestID)
	if err != nil {
		return Adoption{}, err
	}
	if state.Publication.ID == "" {
		return Adoption{}, errors.New("unpublished transfer artifact cannot be adopted")
	}
	if targetAgentID == "" || targetGenerationID == "" || receivingRecordID == "" || authorityRef == "" || at.IsZero() {
		return Adoption{}, errors.New("adoption requires target, receiving record, authority evidence, and time")
	}
	if err := authority.Validate(); err != nil {
		return Adoption{}, err
	}
	if authority.ID == state.Request.Proposer.ID || authority.ID == targetAgentID {
		return Adoption{}, errors.New("candidate or receiving agent cannot authorize its own adoption")
	}
	if err := l.authorizer.AuthorizeTransfer(ctx, "adopt", authority, targetAgentID, authorityRef); err != nil {
		return Adoption{}, fmt.Errorf("authorize transfer adoption: %w", err)
	}
	for _, existing := range state.Adoptions {
		if existing.TargetAgentID == targetAgentID && existing.TargetGenerationID == targetGenerationID {
			return Adoption{}, errors.New("artifact already adopted into target generation")
		}
	}
	sourceAgents, sourceGenerations, sourceMemories := []string{}, []string{}, []string{}
	for _, source := range state.Artifact.SourceRefs {
		sourceAgents = append(sourceAgents, source.AgentID)
		sourceGenerations = append(sourceGenerations, source.GenerationID)
		sourceMemories = append(sourceMemories, source.MemoryID)
	}
	sourceAgents, sourceGenerations, sourceMemories = canonicalStrings(sourceAgents), canonicalStrings(sourceGenerations), canonicalStrings(sourceMemories)
	adoption := Adoption{Version: transferAdoptionVersions.CurrentVersion(), PublicationID: state.Publication.ID, ArtifactID: state.Artifact.ID, TargetAgentID: targetAgentID, TargetGenerationID: targetGenerationID, ReceivingRecordID: receivingRecordID, SourceAgentIDs: sourceAgents, SourceGenerationIDs: sourceGenerations, SourceMemoryIDs: sourceMemories, TransferMechanismRef: state.Request.Policy.ID, Trust: contracts.TrustDerived, Authority: authority, AuthorityRef: authorityRef, AdoptedAt: at.UTC()}
	adoption.ID = digest("transfer-adoption", adoption)
	payload, _ := json.Marshal(adoption)
	event := eventstore.Event{ID: "event:" + adoption.ID, AggregateType: "knowledge_transfer", Type: adoptionEvent, Version: transferAdoptionVersions.CurrentVersion(), Actor: authority, CommandID: "adopt:" + adoption.ID, CorrelationID: requestID, CausationID: state.Publication.ID, Trust: contracts.TrustDerived, Payload: payload, CreatedAt: at.UTC()}
	if _, err := l.store.Append(ctx, aggregateID(requestID), state.Version, []eventstore.Event{event}); err != nil {
		return Adoption{}, err
	}
	return adoption, nil
}

func (l *Lifecycle) Inspect(ctx context.Context, requestID string) (Aggregate, error) {
	if l == nil || l.store == nil || requestID == "" {
		return Aggregate{}, errors.New("transfer lifecycle and request identity are required")
	}
	events, err := l.store.LoadAggregate(ctx, aggregateID(requestID), 0)
	if err != nil {
		return Aggregate{}, err
	}
	if len(events) == 0 {
		return Aggregate{}, errors.New("transfer request not found")
	}
	return replay(requestID, events)
}

func encodeInitialEvents(request Request, artifact Artifact, evaluation Evaluation) ([]eventstore.Event, error) {
	requestPayload, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	artifactPayload, err := json.Marshal(artifact)
	if err != nil {
		return nil, err
	}
	evaluationPayload, err := json.Marshal(evaluation)
	if err != nil {
		return nil, err
	}
	actor := request.Proposer
	return []eventstore.Event{
		{ID: "event:" + request.ID, AggregateType: "knowledge_transfer", Type: requestEvent, Version: transferRequestVersions.CurrentVersion(), Actor: actor, CommandID: "propose:" + request.ID, CorrelationID: request.ID, Trust: contracts.TrustObserved, Payload: requestPayload, CreatedAt: request.RequestedAt.UTC()},
		{ID: "event:" + artifact.ID, AggregateType: "knowledge_transfer", Type: artifactEvent, Version: transferArtifactVersions.CurrentVersion(), Actor: actor, CommandID: "generalize:" + artifact.ID, CorrelationID: request.ID, CausationID: request.ID, Trust: contracts.TrustDerived, Payload: artifactPayload, CreatedAt: artifact.CreatedAt.UTC()},
		{ID: "event:" + evaluation.ID, AggregateType: "knowledge_transfer", Type: evaluationEvent, Version: transferEvaluationVersions.CurrentVersion(), Actor: actor, CommandID: "evaluate:" + evaluation.ID, CorrelationID: request.ID, CausationID: artifact.ID, Trust: contracts.TrustDerived, Payload: evaluationPayload, CreatedAt: evaluation.EvaluatedAt.UTC()},
	}, nil
}

func validatePublishedAggregate(state Aggregate) error {
	if err := VerifyRequest(state.Request); err != nil {
		return err
	}
	artifactID := state.Artifact.ID
	artifact, err := freezeArtifact(state.Artifact, state.Request, state.Request.Policy)
	if err != nil || artifact.ID != artifactID {
		return errors.New("published transfer artifact digest mismatch")
	}
	evaluationID := state.Evaluation.ID
	evaluation, err := freezeEvaluation(state.Evaluation, state.Artifact, state.Request.Policy)
	if err != nil || evaluation.ID != evaluationID || !state.Evaluation.Accepted {
		return errors.New("transfer artifact lacks accepted evaluation")
	}
	publication := state.Publication
	if publication.ID == "" || publication.ArtifactID != state.Artifact.ID || publication.EvaluationID != state.Evaluation.ID || publication.Scope != state.Artifact.Scope || publication.AuthorityRef == "" || publication.PublishedAt.IsZero() || publication.Authority.ID == state.Request.Proposer.ID || digest("transfer-publication", withoutPublicationID(publication)) != publication.ID {
		return errors.New("transfer artifact lacks valid distinct-authority publication")
	}
	return nil
}

func freezeArtifact(artifact Artifact, request Request, policy TransferPolicy) (Artifact, error) {
	artifact.Version, artifact.ID = transferArtifactVersions.CurrentVersion(), ""
	artifact.SourceRefs = append([]SourceReference(nil), artifact.SourceRefs...)
	sort.Slice(artifact.SourceRefs, func(i, j int) bool {
		if artifact.SourceRefs[i].AgentID != artifact.SourceRefs[j].AgentID {
			return artifact.SourceRefs[i].AgentID < artifact.SourceRefs[j].AgentID
		}
		return artifact.SourceRefs[i].MemoryID < artifact.SourceRefs[j].MemoryID
	})
	artifact.SanitizationEvidenceRefs = canonicalStrings(artifact.SanitizationEvidenceRefs)
	if artifact.RequestID != request.ID || artifact.Kind != request.ArtifactKind || artifact.Scope != request.TargetScope || artifact.GeneralizerID != policy.GeneralizerID || artifact.GeneralizerVersion != policy.GeneralizerVersion || artifact.SanitizerID != policy.SanitizerID || artifact.SanitizerVersion != policy.SanitizerVersion || artifact.ContentRef == "" || artifact.ContentDigest == "" || artifact.CreatedAt.IsZero() {
		return Artifact{}, errors.New("generalized artifact is not bound to request and package transforms")
	}
	if !equalSources(artifact.SourceRefs, request.Sources) {
		return Artifact{}, errors.New("generalized artifact must retain exact source provenance")
	}
	for _, source := range request.Sources {
		if artifact.ContentDigest == source.ContentDigest || artifact.ContentRef == source.ContentDigest {
			return Artifact{}, errors.New("raw source content cannot be the cross-agent transfer unit")
		}
	}
	if !containsAll(artifact.SanitizationEvidenceRefs, policy.RequiredPrivacyControls) {
		return Artifact{}, errors.New("generalized artifact lacks required sanitization evidence")
	}
	artifact.ID = digest("transfer-artifact", artifact)
	return artifact, nil
}

func freezeEvaluation(evaluation Evaluation, artifact Artifact, policy TransferPolicy) (Evaluation, error) {
	evaluation.Version, evaluation.ID = transferEvaluationVersions.CurrentVersion(), ""
	evaluation.IndependentRoots = canonicalStrings(evaluation.IndependentRoots)
	sort.Slice(evaluation.Invariants, func(i, j int) bool {
		if evaluation.Invariants[i].Class != evaluation.Invariants[j].Class {
			return evaluation.Invariants[i].Class < evaluation.Invariants[j].Class
		}
		return evaluation.Invariants[i].ControlID < evaluation.Invariants[j].ControlID
	})
	if evaluation.ArtifactID != artifact.ID || evaluation.EvaluatorID != policy.EvaluatorID || evaluation.EvaluatorVersion != policy.EvaluatorVersion || evaluation.EvaluatedAt.IsZero() {
		return Evaluation{}, errors.New("transfer evaluation is not bound to artifact and package evaluator")
	}
	if len(evaluation.IndependentRoots) < policy.MinIndependentRoots {
		evaluation.Accepted = false
	}
	sourceRoots := map[string]bool{}
	for _, source := range artifact.SourceRefs {
		sourceRoots[source.CausationRoot] = true
	}
	for _, root := range evaluation.IndependentRoots {
		if !sourceRoots[root] {
			return Evaluation{}, errors.New("transfer evaluation cites an unavailable causation root")
		}
	}
	privacySeen := map[string]bool{}
	for _, result := range evaluation.Invariants {
		if result.Class != "privacy" && result.Class != "security" && result.Class != "policy" {
			return Evaluation{}, errors.New("unknown transfer invariant class")
		}
		if result.ControlID == "" || result.EvidenceRef == "" {
			return Evaluation{}, errors.New("transfer invariant requires control and evidence")
		}
		if !result.Passed {
			evaluation.Accepted = false
		}
		if result.Class == "privacy" && result.Passed {
			privacySeen[result.ControlID] = true
		}
	}
	for _, control := range policy.RequiredPrivacyControls {
		if !privacySeen[control] {
			evaluation.Accepted = false
		}
	}
	evaluation.ID = digest("transfer-evaluation", evaluation)
	return evaluation, nil
}

func replay(requestID string, events []eventstore.Event) (Aggregate, error) {
	var state Aggregate
	for index, event := range events {
		if event.AggregateType != "knowledge_transfer" || event.CorrelationID != requestID {
			return Aggregate{}, errors.New("transfer event aggregate metadata mismatch")
		}
		switch event.Type {
		case requestEvent:
			if index != 0 {
				return Aggregate{}, errors.New("transfer request must be first event")
			}
			payload, _, err := transferRequestVersions.Canonicalize(event.Version, event.Payload)
			if err != nil {
				return Aggregate{}, err
			}
			if err := json.Unmarshal(payload, &state.Request); err != nil {
				return Aggregate{}, err
			}
			if err := VerifyRequest(state.Request); err != nil {
				return Aggregate{}, err
			}
			if state.Request.ID != requestID || event.ID != "event:"+requestID || event.Actor != state.Request.Proposer || event.Trust != contracts.TrustObserved {
				return Aggregate{}, errors.New("transfer request event binding mismatch")
			}
		case artifactEvent:
			payload, _, err := transferArtifactVersions.Canonicalize(event.Version, event.Payload)
			if err != nil {
				return Aggregate{}, err
			}
			if err := json.Unmarshal(payload, &state.Artifact); err != nil {
				return Aggregate{}, err
			}
			artifactID := state.Artifact.ID
			verified, verifyErr := freezeArtifact(state.Artifact, state.Request, state.Request.Policy)
			if verifyErr != nil || verified.ID != artifactID || event.ID != "event:"+artifactID || event.CausationID != state.Request.ID || event.Actor != state.Request.Proposer || event.Trust != contracts.TrustDerived {
				return Aggregate{}, errors.New("transfer artifact event binding mismatch")
			}
		case evaluationEvent:
			payload, _, err := transferEvaluationVersions.Canonicalize(event.Version, event.Payload)
			if err != nil {
				return Aggregate{}, err
			}
			if err := json.Unmarshal(payload, &state.Evaluation); err != nil {
				return Aggregate{}, err
			}
			evaluationID := state.Evaluation.ID
			verified, verifyErr := freezeEvaluation(state.Evaluation, state.Artifact, state.Request.Policy)
			if verifyErr != nil || verified.ID != evaluationID || event.ID != "event:"+evaluationID || event.CausationID != state.Artifact.ID || event.Actor != state.Request.Proposer || event.Trust != contracts.TrustDerived {
				return Aggregate{}, errors.New("transfer evaluation event binding mismatch")
			}
		case publicationEvent:
			payload, _, err := transferPublicationVersions.Canonicalize(event.Version, event.Payload)
			if err != nil {
				return Aggregate{}, err
			}
			if state.Publication.ID != "" || json.Unmarshal(payload, &state.Publication) != nil {
				return Aggregate{}, errors.New("invalid transfer publication event")
			}
			if state.Publication.Version != transferPublicationVersions.CurrentVersion() || state.Publication.Scope != state.Artifact.Scope || state.Publication.AuthorityRef == "" || state.Publication.PublishedAt.IsZero() || state.Publication.ArtifactID != state.Artifact.ID || state.Publication.EvaluationID != state.Evaluation.ID || !state.Evaluation.Accepted || event.ID != "event:"+state.Publication.ID || event.Actor != state.Publication.Authority || event.CausationID != state.Evaluation.ID || event.Trust != contracts.TrustPolicy || digest("transfer-publication", withoutPublicationID(state.Publication)) != state.Publication.ID {
				return Aggregate{}, errors.New("transfer publication event binding mismatch")
			}
		case adoptionEvent:
			var adoption Adoption
			payload, _, err := transferAdoptionVersions.Canonicalize(event.Version, event.Payload)
			if err != nil {
				return Aggregate{}, err
			}
			if err := json.Unmarshal(payload, &adoption); err != nil {
				return Aggregate{}, err
			}
			expectedAgents, expectedGenerations, expectedMemories := sourceIdentities(state.Artifact.SourceRefs)
			if state.Publication.ID == "" || adoption.Version != transferAdoptionVersions.CurrentVersion() || adoption.TargetAgentID == "" || adoption.TargetGenerationID == "" || adoption.ReceivingRecordID == "" || adoption.AuthorityRef == "" || adoption.AdoptedAt.IsZero() || !equalStrings(adoption.SourceAgentIDs, expectedAgents) || !equalStrings(adoption.SourceGenerationIDs, expectedGenerations) || !equalStrings(adoption.SourceMemoryIDs, expectedMemories) || adoption.PublicationID != state.Publication.ID || adoption.ArtifactID != state.Artifact.ID || adoption.TransferMechanismRef != state.Request.Policy.ID || adoption.Trust != contracts.TrustDerived || event.ID != "event:"+adoption.ID || event.Actor != adoption.Authority || event.CausationID != state.Publication.ID || event.Trust != contracts.TrustDerived || digest("transfer-adoption", withoutAdoptionID(adoption)) != adoption.ID {
				return Aggregate{}, errors.New("transfer adoption event binding mismatch")
			}
			state.Adoptions = append(state.Adoptions, adoption)
		default:
			return Aggregate{}, fmt.Errorf("unknown transfer event %s", event.Type)
		}
		state.Version = event.AggregateVersion
	}
	return state, nil
}

func withoutArtifactID(value Artifact) Artifact          { value.ID = ""; return value }
func withoutEvaluationID(value Evaluation) Evaluation    { value.ID = ""; return value }
func withoutPublicationID(value Publication) Publication { value.ID = ""; return value }
func withoutAdoptionID(value Adoption) Adoption          { value.ID = ""; return value }

func equalSources(a, b []SourceReference) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
func containsAll(actual, required []string) bool {
	set := map[string]bool{}
	for _, value := range actual {
		set[value] = true
	}
	for _, value := range required {
		if !set[value] {
			return false
		}
	}
	return true
}

func sourceIdentities(sources []SourceReference) ([]string, []string, []string) {
	agents, generations, memories := []string{}, []string{}, []string{}
	for _, source := range sources {
		agents = append(agents, source.AgentID)
		generations = append(generations, source.GenerationID)
		memories = append(memories, source.MemoryID)
	}
	return canonicalStrings(agents), canonicalStrings(generations), canonicalStrings(memories)
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

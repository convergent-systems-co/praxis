package goalstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/packages/goals"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

const baselineNamespace = "goal_baseline"
const sessionNamespace = "goal_session"
const workPlanProposalNamespace = "work_plan_proposal"
const workPlanAcceptanceNamespace = "work_plan_acceptance"
const workPlanReviewNamespace = "work_plan_review"
const authorityRequestNamespace = "authority_request"
const authorityDecisionNamespace = "authority_decision"
const authorityRevocationNamespace = "authority_revocation"
const authorityGenerationNamespace = "authority_generation"
const authorityGenerationInvalidationNamespace = "authority_generation_invalidation"
const authorityModelMigrationNamespace = "authority_model_migration"
const routingIssuanceNamespace = "routing_issuance"
const providerWorkspaceNamespace = "provider_workspace"

var ErrAuthorityDecisionRevoked = errors.New("authority decision is revoked")

type Repository struct {
	Store               *state.Store
	Crypto              praxiscrypto.EnvelopeService
	KeyRef              string
	Profile             contracts.CryptoProfile
	Sensitivity         state.Sensitivity
	AuthorityGeneration AuthorityGenerationValidator
	BootstrapDigest     string
}

// AuthorityGenerationValidator is the cross-registry authority boundary.
// GoalStore does not infer principal or policy validity from an opaque digest.
type AuthorityGenerationValidator interface {
	ValidateAuthorityGeneration(context.Context, contracts.AuthorityDecision, time.Time) error
}

func (r Repository) Save(ctx context.Context, baseline goals.GoalBaseline, createdAt time.Time, expiresAt *time.Time) (goals.GoalBaseline, error) {
	if r.Store == nil {
		return goals.GoalBaseline{}, errors.New("goal baseline store is required")
	}
	if r.KeyRef == "" {
		return goals.GoalBaseline{}, errors.New("goal baseline key reference is required")
	}
	if err := r.Profile.Validate(); err != nil {
		return goals.GoalBaseline{}, err
	}
	if err := r.Sensitivity.Validate(); err != nil {
		return goals.GoalBaseline{}, err
	}
	if err := baseline.Validate(); err != nil {
		return goals.GoalBaseline{}, err
	}
	digest, err := baseline.ComputeDigest()
	if err != nil {
		return goals.GoalBaseline{}, err
	}
	if baseline.Digest != "" && baseline.Digest != digest {
		return goals.GoalBaseline{}, goals.ErrBaselineDigestMismatch
	}
	baseline.Digest = digest
	payload, err := json.Marshal(baseline)
	if err != nil {
		return goals.GoalBaseline{}, fmt.Errorf("encode Goal Baseline: %w", err)
	}
	record, err := r.goalBaselineRecord(ctx, baseline, payload, digest, createdAt, expiresAt)
	if err != nil {
		return goals.GoalBaseline{}, err
	}
	if err := r.Store.PutSecureBlob(ctx, record); err != nil {
		return goals.GoalBaseline{}, fmt.Errorf("persist Goal Baseline: %w", err)
	}
	return baseline, nil
}

func (r Repository) goalBaselineRecord(ctx context.Context, baseline goals.GoalBaseline, payload []byte, digest string, createdAt time.Time, expiresAt *time.Time) (state.SecureBlobRecord, error) {
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	aad := state.SecureBlobAAD(baselineNamespace, baseline.ID, baseline.Version, digest)
	envelope, err := r.Crypto.Seal(ctx, r.KeyRef, r.Profile, payload, aad)
	if err != nil {
		return state.SecureBlobRecord{}, fmt.Errorf("encrypt Goal Baseline: %w", err)
	}
	return state.SecureBlobRecord{Namespace: baselineNamespace, ObjectID: baseline.ID, ObjectVersion: baseline.Version, ObjectDigest: digest, Sensitivity: r.Sensitivity, CryptoProfile: r.Profile, Envelope: envelope, CreatedAt: createdAt, ExpiresAt: expiresAt}, nil
}

func (r Repository) Load(ctx context.Context, id, version string, now time.Time) (goals.GoalBaseline, error) {
	if r.Store == nil {
		return goals.GoalBaseline{}, errors.New("goal baseline store is required")
	}
	record, err := r.Store.GetSecureBlob(ctx, baselineNamespace, id, version, now)
	if err != nil {
		return goals.GoalBaseline{}, err
	}
	aad := state.SecureBlobAAD(baselineNamespace, id, version, record.ObjectDigest)
	payload, err := r.Crypto.Open(ctx, record.Envelope, aad)
	if err != nil {
		return goals.GoalBaseline{}, fmt.Errorf("decrypt Goal Baseline: %w", err)
	}
	var baseline goals.GoalBaseline
	if err := json.Unmarshal(payload, &baseline); err != nil {
		return goals.GoalBaseline{}, fmt.Errorf("decode Goal Baseline: %w", err)
	}
	if baseline.ID != id || baseline.Version != version || baseline.Digest != record.ObjectDigest {
		return goals.GoalBaseline{}, errors.New("Goal Baseline identity/digest differs from secure record")
	}
	if err := baseline.VerifyDigest(); err != nil {
		return goals.GoalBaseline{}, err
	}
	return baseline, nil
}

// AttachAcceptedWorkPlan creates a successor immutable baseline from a
// previously persisted acceptance. The source baseline, proposal, acceptance,
// and plan are all reloaded and checked before the successor is persisted.
// This operation is the authority-bearing attachment transition; it does not
// mutate the predecessor or create an implicit active-baseline pointer.
func (r Repository) AttachAcceptedWorkPlan(ctx context.Context, sourceID, sourceVersion, sourceDigest, acceptanceRef, acceptanceVersion, successorVersion string, createdAt time.Time, expiresAt *time.Time) (goals.GoalBaseline, error) {
	source, err := r.Load(ctx, sourceID, sourceVersion, time.Now().UTC())
	if err != nil {
		return goals.GoalBaseline{}, fmt.Errorf("load source Goal Baseline: %w", err)
	}
	if source.Digest != sourceDigest {
		return goals.GoalBaseline{}, goals.ErrBaselineDigestMismatch
	}
	if source.WorkPlan != nil {
		return goals.GoalBaseline{}, errors.New("source Goal Baseline already has an accepted WorkPlan")
	}
	if successorVersion == "" || successorVersion == source.Version {
		return goals.GoalBaseline{}, errors.New("successor Goal Baseline version must be distinct")
	}
	plan, err := r.LoadAcceptedWorkPlan(ctx, acceptanceRef, acceptanceVersion, time.Now().UTC())
	if err != nil {
		return goals.GoalBaseline{}, fmt.Errorf("load accepted WorkPlan: %w", err)
	}
	if plan.BaselineDigest != source.Digest {
		return goals.GoalBaseline{}, fmt.Errorf("accepted WorkPlan source baseline differs: %w", goals.ErrBaselineDigestMismatch)
	}
	if plan.AuthorityRequestID != "" {
		if plan.AuthorityRequestVersion == "" || plan.AuthorityDecisionRef == "" || plan.AuthorityDecisionVersion == "" {
			return goals.GoalBaseline{}, errors.New("accepted WorkPlan authority lineage is incomplete")
		}
		if _, err := r.LoadAuthorityDecision(ctx, plan.AuthorityRequestID, plan.AuthorityRequestVersion, time.Now().UTC()); err != nil {
			return goals.GoalBaseline{}, fmt.Errorf("accepted WorkPlan authority is no longer effective: %w", err)
		}
	}
	successor := source
	successor.Version = successorVersion
	successor.Digest = ""
	successor.PredecessorDigest = source.Digest
	successor.WorkPlan = &plan
	if err := successor.Validate(); err != nil {
		return goals.GoalBaseline{}, err
	}
	digest, err := successor.ComputeDigest()
	if err != nil {
		return goals.GoalBaseline{}, err
	}
	successor.Digest = digest
	payload, err := json.Marshal(successor)
	if err != nil {
		return goals.GoalBaseline{}, fmt.Errorf("encode successor Goal Baseline: %w", err)
	}
	record, err := r.goalBaselineRecord(ctx, successor, payload, digest, createdAt, expiresAt)
	if err != nil {
		return goals.GoalBaseline{}, err
	}
	if plan.AuthorityRequestID != "" {
		err = r.Store.PutSecureBlobUnlessRevoked(ctx, record, authorityRevocationNamespace, plan.AuthorityRequestID, plan.AuthorityRequestVersion, authorityGenerationInvalidationNamespace, plan.AuthorityRef, plan.AuthorityVersion, authorityGenerationNamespace, plan.AuthorityRef, plan.AuthorityVersion)
	} else {
		err = r.Store.PutSecureBlob(ctx, record)
	}
	if err != nil {
		return goals.GoalBaseline{}, fmt.Errorf("persist successor Goal Baseline: %w", err)
	}
	return successor, nil
}

// SaveSession persists an immutable interruption checkpoint. The caller owns
// checkpoint versioning; this prevents a retry or concurrent writer from
// silently replacing an earlier conversational state.
func (r Repository) SaveSession(ctx context.Context, session goals.Session, version string, createdAt time.Time, expiresAt *time.Time) error {
	if r.Store == nil {
		return errors.New("goal session store is required")
	}
	if r.KeyRef == "" {
		return errors.New("goal session key reference is required")
	}
	if version == "" {
		return errors.New("goal session checkpoint version is required")
	}
	if err := r.Profile.Validate(); err != nil {
		return err
	}
	if err := r.Sensitivity.Validate(); err != nil {
		return err
	}
	payload, err := session.Snapshot()
	if err != nil {
		return fmt.Errorf("snapshot Goal session: %w", err)
	}
	digest := sessionDigest(payload)
	aad := state.SecureBlobAAD(sessionNamespace, session.ID, version, digest)
	envelope, err := r.Crypto.Seal(ctx, r.KeyRef, r.Profile, payload, aad)
	if err != nil {
		return fmt.Errorf("encrypt Goal session: %w", err)
	}
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	record := state.SecureBlobRecord{Namespace: sessionNamespace, ObjectID: session.ID, ObjectVersion: version, ObjectDigest: digest, Sensitivity: r.Sensitivity, CryptoProfile: r.Profile, Envelope: envelope, CreatedAt: createdAt, ExpiresAt: expiresAt}
	if err := r.Store.PutSecureBlob(ctx, record); err != nil {
		return fmt.Errorf("persist Goal session: %w", err)
	}
	return nil
}

// LoadSession verifies the secure record and the canonical snapshot before
// making an interrupted Goals conversation available for resumption.
func (r Repository) LoadSession(ctx context.Context, id, version string, now time.Time) (goals.Session, error) {
	if r.Store == nil {
		return goals.Session{}, errors.New("goal session store is required")
	}
	record, err := r.Store.GetSecureBlob(ctx, sessionNamespace, id, version, now)
	if err != nil {
		return goals.Session{}, err
	}
	aad := state.SecureBlobAAD(sessionNamespace, id, version, record.ObjectDigest)
	payload, err := r.Crypto.Open(ctx, record.Envelope, aad)
	if err != nil {
		return goals.Session{}, fmt.Errorf("decrypt Goal session: %w", err)
	}
	if sessionDigest(payload) != record.ObjectDigest {
		return goals.Session{}, errors.New("Goal session snapshot digest mismatch")
	}
	session, err := goals.RestoreSession(payload)
	if err != nil {
		return goals.Session{}, fmt.Errorf("restore Goal session: %w", err)
	}
	if session.ID != id {
		return goals.Session{}, errors.New("Goal session identity differs from secure record")
	}
	return session, nil
}

// SaveWorkPlanProposal persists advisory decomposition without granting it
// execution authority. The proposal is immutable and must be reloaded from
// this store before an acceptance can be recorded.
func (r Repository) SaveWorkPlanProposal(ctx context.Context, proposal contracts.WorkPlanProposal, version string, createdAt time.Time, expiresAt *time.Time) (string, error) {
	if err := r.validateWorkPlanStore(); err != nil {
		return "", err
	}
	if version == "" {
		return "", errors.New("work plan proposal version is required")
	}
	digest, err := proposal.Digest()
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(proposal)
	if err != nil {
		return "", fmt.Errorf("encode WorkPlan proposal: %w", err)
	}
	if err := r.putWorkPlanBlob(ctx, workPlanProposalNamespace, proposal.ID, version, payload, createdAt, expiresAt); err != nil {
		return "", fmt.Errorf("persist WorkPlan proposal: %w", err)
	}
	return digest, nil
}

func (r Repository) LoadWorkPlanProposal(ctx context.Context, id, version string, now time.Time) (contracts.WorkPlanProposal, error) {
	payload, record, err := r.loadWorkPlanBlob(ctx, workPlanProposalNamespace, id, version, now)
	if err != nil {
		return contracts.WorkPlanProposal{}, err
	}
	var proposal contracts.WorkPlanProposal
	if err := json.Unmarshal(payload, &proposal); err != nil {
		return contracts.WorkPlanProposal{}, fmt.Errorf("decode WorkPlan proposal: %w", err)
	}
	if proposal.ID != id || proposal.Validate() != nil {
		return contracts.WorkPlanProposal{}, errors.New("WorkPlan proposal identity or contract is invalid")
	}
	digest, err := proposal.Digest()
	if err != nil {
		return contracts.WorkPlanProposal{}, err
	}
	if payloadDigest(payload) != record.ObjectDigest || digest == "" {
		return contracts.WorkPlanProposal{}, errors.New("WorkPlan proposal digest mismatch")
	}
	return proposal, nil
}

type acceptedWorkPlanRecord struct {
	Proposal contracts.WorkPlanProposal       `json:"proposal"`
	Review   contracts.WorkPlanProposalReview `json:"review"`
	Decision contracts.WorkPlanAcceptance     `json:"decision"`
	Plan     contracts.WorkPlan               `json:"plan"`
}

type workPlanReviewRecord struct {
	Proposal contracts.WorkPlanProposal       `json:"proposal"`
	Review   contracts.WorkPlanProposalReview `json:"review"`
}

// SaveWorkPlanReview persists independent advisory review evidence only after
// reloading the exact immutable proposal. A review cannot attach or activate
// the proposal.
func (r Repository) SaveWorkPlanReview(ctx context.Context, proposalID, proposalVersion string, review contracts.WorkPlanProposalReview, recordVersion string, createdAt time.Time, expiresAt *time.Time) error {
	if err := r.validateWorkPlanStore(); err != nil {
		return err
	}
	proposal, err := r.LoadWorkPlanProposal(ctx, proposalID, proposalVersion, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("load proposal for review: %w", err)
	}
	if err := review.Validate(proposal); err != nil {
		return err
	}
	if recordVersion == "" {
		return errors.New("work plan review version is required")
	}
	payload, err := json.Marshal(workPlanReviewRecord{Proposal: proposal, Review: review})
	if err != nil {
		return fmt.Errorf("encode WorkPlan review: %w", err)
	}
	if err := r.putWorkPlanBlob(ctx, workPlanReviewNamespace, review.ReviewRef, recordVersion, payload, createdAt, expiresAt); err != nil {
		return fmt.Errorf("persist WorkPlan review: %w", err)
	}
	return nil
}

func (r Repository) LoadWorkPlanReview(ctx context.Context, reviewRef, version string, now time.Time) (contracts.WorkPlanProposalReview, error) {
	payload, record, err := r.loadWorkPlanBlob(ctx, workPlanReviewNamespace, reviewRef, version, now)
	if err != nil {
		return contracts.WorkPlanProposalReview{}, err
	}
	var stored workPlanReviewRecord
	if err := json.Unmarshal(payload, &stored); err != nil {
		return contracts.WorkPlanProposalReview{}, fmt.Errorf("decode WorkPlan review: %w", err)
	}
	if stored.Review.ReviewRef != reviewRef || payloadDigest(payload) != record.ObjectDigest {
		return contracts.WorkPlanProposalReview{}, errors.New("WorkPlan review identity or record digest mismatch")
	}
	if err := stored.Review.Validate(stored.Proposal); err != nil {
		return contracts.WorkPlanProposalReview{}, fmt.Errorf("validate WorkPlan review: %w", err)
	}
	return stored.Review, nil
}

// SaveAcceptedWorkPlan is the durable acceptance boundary. It requires the
// proposal to have been persisted first, revalidates the exact proposal and
// decision, and stores the acceptance evidence and resulting plan together.
// It deliberately does not mutate a Goal Baseline; attaching a plan belongs
// to a separate successor-baseline operation.
func (r Repository) SaveAcceptedWorkPlan(ctx context.Context, proposalID, proposalVersion string, accepted contracts.WorkPlan, decision contracts.WorkPlanAcceptance, recordVersion string, createdAt time.Time, expiresAt *time.Time) (contracts.WorkPlan, error) {
	if err := r.validateWorkPlanStore(); err != nil {
		return contracts.WorkPlan{}, err
	}
	proposal, err := r.LoadWorkPlanProposal(ctx, proposalID, proposalVersion, time.Now().UTC())
	if err != nil {
		return contracts.WorkPlan{}, fmt.Errorf("load proposal for acceptance: %w", err)
	}
	review, err := r.LoadWorkPlanReview(ctx, decision.ReviewRef, decision.ReviewVersion, time.Now().UTC())
	if err != nil {
		return contracts.WorkPlan{}, fmt.Errorf("load review for acceptance: %w", err)
	}
	proposalDigest, err := proposal.Digest()
	if err != nil || review.ProposalDigest != proposalDigest || review.BaselineDigest != proposal.BaselineDigest || review.ReviewDigest != decision.ReviewDigest || review.Status != contracts.ReviewAcceptableForAuthority {
		return contracts.WorkPlan{}, fmt.Errorf("%w: acceptance requires an exact acceptable independent review", contracts.ErrUnacceptedWorkPlan)
	}
	plan, err := contracts.AcceptWorkPlan(proposal, accepted, decision)
	if err != nil {
		return contracts.WorkPlan{}, err
	}
	if recordVersion == "" {
		return contracts.WorkPlan{}, errors.New("work plan acceptance version is required")
	}
	record := acceptedWorkPlanRecord{Proposal: proposal, Review: review, Decision: decision, Plan: plan}
	payload, err := json.Marshal(record)
	if err != nil {
		return contracts.WorkPlan{}, fmt.Errorf("encode accepted WorkPlan: %w", err)
	}
	if err := r.putWorkPlanBlob(ctx, workPlanAcceptanceNamespace, decision.AcceptanceRef, recordVersion, payload, createdAt, expiresAt); err != nil {
		return contracts.WorkPlan{}, fmt.Errorf("persist accepted WorkPlan: %w", err)
	}
	return plan, nil
}

// SaveAcceptedWorkPlanFromAuthorityDecision is the authority-bearing bridge
// from a durable generic decision to the WorkPlan acceptance record. It
// reloads every bound record and derives acceptance authority from the exact
// approved request; callers cannot substitute a conversational approval or
// widen the decision scope. Successor-baseline attachment remains separate.
func (r Repository) SaveAcceptedWorkPlanFromAuthorityDecision(ctx context.Context, requestID, requestVersion string, accepted contracts.WorkPlan, acceptanceRef, acceptanceVersion string, createdAt time.Time, expiresAt *time.Time) (contracts.WorkPlan, error) {
	if acceptanceRef == "" || acceptanceVersion == "" {
		return contracts.WorkPlan{}, errors.New("authority-backed acceptance identity is required")
	}
	request, err := r.LoadAuthorityRequest(ctx, requestID, requestVersion, time.Now().UTC())
	if err != nil {
		return contracts.WorkPlan{}, fmt.Errorf("load authority request for acceptance: %w", err)
	}
	authorityDecision, err := r.LoadAuthorityDecision(ctx, requestID, requestVersion, time.Now().UTC())
	if err != nil {
		return contracts.WorkPlan{}, fmt.Errorf("load authority decision for acceptance: %w", err)
	}
	if authorityDecision.Outcome != contracts.AuthorityApprove {
		return contracts.WorkPlan{}, fmt.Errorf("authority decision outcome %q cannot authorize WorkPlan acceptance", authorityDecision.Outcome)
	}
	validator := r.AuthorityGeneration
	if validator == nil {
		validator = r
	}
	if authorityDecision.AuthorityRef == "" || authorityDecision.AuthorityVersion == "" || authorityDecision.AuthorityGenerationDigest == "" {
		return contracts.WorkPlan{}, errors.New("authority generation validation is required for cross-registry acceptance")
	}
	if err := validator.ValidateAuthorityGeneration(ctx, authorityDecision, time.Now().UTC()); err != nil {
		return contracts.WorkPlan{}, fmt.Errorf("validate authority generation: %w", err)
	}
	proposal, err := r.LoadWorkPlanProposal(ctx, request.ProposalID, request.ProposalVersion, time.Now().UTC())
	if err != nil {
		return contracts.WorkPlan{}, fmt.Errorf("load proposal for authority-backed acceptance: %w", err)
	}
	proposalDigest, err := proposal.Digest()
	if err != nil {
		return contracts.WorkPlan{}, err
	}
	if proposal.ID != request.ProposalID || proposal.GoalID != request.BaselineID || proposal.GoalVersion != request.BaselineVersion || proposal.BaselineDigest != request.BaselineDigest || proposalDigest != request.ProposalDigest {
		return contracts.WorkPlan{}, fmt.Errorf("authority request proposal binding is stale or mismatched: %w", contracts.ErrUnacceptedWorkPlan)
	}
	review, err := r.LoadWorkPlanReview(ctx, request.ReviewRef, request.ReviewVersion, time.Now().UTC())
	if err != nil {
		return contracts.WorkPlan{}, fmt.Errorf("load review for authority-backed acceptance: %w", err)
	}
	if review.ProposalDigest != request.ProposalDigest || review.BaselineDigest != request.BaselineDigest || review.ReviewDigest != request.ReviewDigest || review.Status != contracts.ReviewAcceptableForAuthority {
		return contracts.WorkPlan{}, fmt.Errorf("authority request review binding is stale or unacceptable: %w", contracts.ErrUnacceptedWorkPlan)
	}
	mode := "policy"
	if authorityDecision.DecidedBy.Kind == "human" {
		mode = "human"
	}
	acceptance := contracts.WorkPlanAcceptance{
		ProposalDigest: proposalDigest, BaselineDigest: request.BaselineDigest,
		AuthorityRef: authorityDecision.AuthorityRef, AuthorityDigest: authorityDecision.AuthorityDigest,
		AcceptanceRef: acceptanceRef, AcceptanceDigest: authorityAcceptanceDigest(request, authorityDecision, acceptanceRef, acceptanceVersion),
		AcceptedBy: authorityDecision.DecidedBy, AuthorityScope: authorityDecision.GrantedScope,
		AuthorityRequestID: request.ID, AuthorityRequestVersion: request.Version,
		AuthorityDecisionRef: authorityDecision.DecisionRef, AuthorityDecisionVersion: authorityDecision.DecisionVersion, AuthorityVersion: authorityDecision.AuthorityVersion, AuthorityGenerationDigest: authorityDecision.AuthorityGenerationDigest,
		ReviewRef: request.ReviewRef, ReviewVersion: request.ReviewVersion, ReviewDigest: request.ReviewDigest, Mode: mode,
	}
	if existing, loadErr := r.LoadAcceptedWorkPlan(ctx, acceptanceRef, acceptanceVersion, time.Now().UTC()); loadErr == nil {
		candidate, candidateErr := contracts.AcceptWorkPlan(proposal, accepted, acceptance)
		if candidateErr != nil {
			return contracts.WorkPlan{}, candidateErr
		}
		candidatePayload, _ := json.Marshal(candidate)
		existingPayload, _ := json.Marshal(existing)
		if bytes.Equal(candidatePayload, existingPayload) {
			return existing, nil
		}
		return contracts.WorkPlan{}, errors.New("conflicting authority-backed acceptance already exists")
	} else if !errors.Is(loadErr, state.ErrSecureBlobNotFound) && !errors.Is(loadErr, state.ErrSecureBlobExpired) {
		return contracts.WorkPlan{}, fmt.Errorf("check existing authority-backed acceptance: %w", loadErr)
	}
	plan, err := contracts.AcceptWorkPlan(proposal, accepted, acceptance)
	if err != nil {
		return contracts.WorkPlan{}, err
	}
	payload, err := json.Marshal(acceptedWorkPlanRecord{Proposal: proposal, Review: review, Decision: acceptance, Plan: plan})
	if err != nil {
		return contracts.WorkPlan{}, fmt.Errorf("encode authority-backed acceptance: %w", err)
	}
	if err := r.putWorkPlanBlobUnlessRevoked(ctx, workPlanAcceptanceNamespace, acceptanceRef, acceptanceVersion, payload, createdAt, expiresAt, request.ID, request.Version, authorityDecision.AuthorityRef, authorityDecision.AuthorityVersion, authorityGenerationNamespace, authorityDecision.AuthorityRef, authorityDecision.AuthorityVersion); err != nil {
		if errors.Is(err, state.ErrAuthorityRevoked) {
			return contracts.WorkPlan{}, ErrAuthorityDecisionRevoked
		}
		return contracts.WorkPlan{}, fmt.Errorf("persist authority-backed acceptance: %w", err)
	}
	return plan, nil
}

func (r Repository) LoadAcceptedWorkPlan(ctx context.Context, acceptanceRef, version string, now time.Time) (contracts.WorkPlan, error) {
	payload, record, err := r.loadWorkPlanBlob(ctx, workPlanAcceptanceNamespace, acceptanceRef, version, now)
	if err != nil {
		return contracts.WorkPlan{}, err
	}
	var stored acceptedWorkPlanRecord
	if err := json.Unmarshal(payload, &stored); err != nil {
		return contracts.WorkPlan{}, fmt.Errorf("decode accepted WorkPlan: %w", err)
	}
	if stored.Decision.AcceptanceRef != acceptanceRef || payloadDigest(payload) != record.ObjectDigest {
		return contracts.WorkPlan{}, errors.New("accepted WorkPlan identity or record digest mismatch")
	}
	if err := stored.Review.Validate(stored.Proposal); err != nil || stored.Review.ReviewDigest != stored.Decision.ReviewDigest || stored.Review.Status != contracts.ReviewAcceptableForAuthority {
		return contracts.WorkPlan{}, fmt.Errorf("%w: persisted acceptance review is not valid", contracts.ErrUnacceptedWorkPlan)
	}
	plan, err := contracts.AcceptWorkPlan(stored.Proposal, stored.Plan, stored.Decision)
	if err != nil {
		return contracts.WorkPlan{}, fmt.Errorf("validate accepted WorkPlan: %w", err)
	}
	return plan, nil
}

// SaveAuthorityRevocation appends an immutable revocation record for one exact
// authority decision. It never rewrites the original decision; effective
// authority is determined by consulting this record at authority boundaries.
func (r Repository) SaveAuthorityRevocation(ctx context.Context, requestID, requestVersion string, revocation contracts.AuthorityRevocation, createdAt time.Time, expiresAt *time.Time) error {
	if err := r.validateWorkPlanStore(); err != nil {
		return err
	}
	if expiresAt != nil {
		return errors.New("authority revocation cannot expire")
	}
	if revocation.EffectiveAt.After(time.Now().UTC()) {
		return errors.New("authority revocation cannot become effective in the future")
	}
	decision, err := r.LoadAuthorityDecisionEvidence(ctx, requestID, requestVersion, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("load authority decision for revocation: %w", err)
	}
	if err := revocation.Validate(decision); err != nil {
		return err
	}
	payload, err := json.Marshal(authorityRevocationRecord{Decision: decision, Revocation: revocation})
	if err != nil {
		return fmt.Errorf("encode authority revocation: %w", err)
	}
	lockNamespace, lockID, lockVersion := authorityRequestNamespace, requestID, requestVersion
	if decision.AuthorityRef != "" && decision.AuthorityVersion != "" {
		lockNamespace, lockID, lockVersion = authorityGenerationNamespace, decision.AuthorityRef, decision.AuthorityVersion
	}
	if err := r.putWorkPlanBlobWithLock(ctx, authorityRevocationNamespace, requestID, requestVersion, payload, createdAt, expiresAt, lockNamespace, lockID, lockVersion); err != nil {
		return fmt.Errorf("persist authority revocation: %w", err)
	}
	return nil
}

func (r Repository) LoadAuthorityRevocation(ctx context.Context, requestID, requestVersion string, now time.Time) (contracts.AuthorityRevocation, error) {
	payload, record, err := r.loadWorkPlanBlob(ctx, authorityRevocationNamespace, requestID, requestVersion, now)
	if err != nil {
		return contracts.AuthorityRevocation{}, err
	}
	var stored authorityRevocationRecord
	if err := json.Unmarshal(payload, &stored); err != nil {
		return contracts.AuthorityRevocation{}, fmt.Errorf("decode authority revocation: %w", err)
	}
	if stored.Revocation.RequestID != requestID || stored.Revocation.RequestVersion != requestVersion || payloadDigest(payload) != record.ObjectDigest {
		return contracts.AuthorityRevocation{}, errors.New("authority revocation identity or digest mismatch")
	}
	if err := stored.Revocation.Validate(stored.Decision); err != nil {
		return contracts.AuthorityRevocation{}, fmt.Errorf("validate authority revocation: %w", err)
	}
	if !now.IsZero() && !now.Before(stored.Revocation.EffectiveAt) {
		return stored.Revocation, nil
	}
	return contracts.AuthorityRevocation{}, state.ErrSecureBlobNotFound
}

func authorityAcceptanceDigest(request contracts.AuthorityRequest, decision contracts.AuthorityDecision, acceptanceRef, acceptanceVersion string) string {
	payload, _ := json.Marshal(struct {
		RequestDigest, DecisionRef, DecisionVersion, AcceptanceRef, AcceptanceVersion string
	}{requestDigestOrEmpty(request), decision.DecisionRef, decision.DecisionVersion, acceptanceRef, acceptanceVersion})
	return payloadDigest(payload)
}

func requestDigestOrEmpty(request contracts.AuthorityRequest) string {
	digest, _ := request.Digest()
	return digest
}

// SaveAuthorityRequest records a pending governance question without granting
// any authority. Its immutable identity/version is the deduplication key.
func (r Repository) SaveAuthorityRequest(ctx context.Context, request contracts.AuthorityRequest, createdAt time.Time, expiresAt *time.Time) (string, error) {
	if err := r.validateWorkPlanStore(); err != nil {
		return "", err
	}
	digest, err := request.Digest()
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("encode authority request: %w", err)
	}
	if err := r.putWorkPlanBlob(ctx, authorityRequestNamespace, request.ID, request.Version, payload, createdAt, expiresAt); err != nil {
		return "", fmt.Errorf("persist authority request: %w", err)
	}
	return digest, nil
}

func (r Repository) LoadAuthorityRequest(ctx context.Context, id, version string, now time.Time) (contracts.AuthorityRequest, error) {
	payload, record, err := r.loadWorkPlanBlob(ctx, authorityRequestNamespace, id, version, now)
	if err != nil {
		return contracts.AuthorityRequest{}, err
	}
	var request contracts.AuthorityRequest
	if err := json.Unmarshal(payload, &request); err != nil {
		return contracts.AuthorityRequest{}, fmt.Errorf("decode authority request: %w", err)
	}
	_, err = request.Digest()
	if err != nil || request.ID != id || request.Version != version || payloadDigest(payload) != record.ObjectDigest {
		return contracts.AuthorityRequest{}, errors.New("authority request identity or digest mismatch")
	}
	return request, nil
}

// PendingAuthorityRequests returns only pending requests bound to one exact
// Goal generation. Ordering is stable and request identity is preserved.
func (r Repository) PendingAuthorityRequests(ctx context.Context, goalID, goalVersion string, now time.Time) ([]contracts.AuthorityRequest, error) {
	if err := r.validateWorkPlanStore(); err != nil {
		return nil, err
	}
	records, err := r.Store.ListSecureBlobs(ctx, authorityRequestNamespace, now)
	if err != nil {
		return nil, err
	}
	var pending []contracts.AuthorityRequest
	for _, record := range records {
		aad := state.SecureBlobAAD(record.Namespace, record.ObjectID, record.ObjectVersion, record.ObjectDigest)
		payload, err := r.Crypto.Open(ctx, record.Envelope, aad)
		if err != nil {
			return nil, fmt.Errorf("decrypt authority request: %w", err)
		}
		var request contracts.AuthorityRequest
		if err := json.Unmarshal(payload, &request); err != nil {
			return nil, fmt.Errorf("decode authority request: %w", err)
		}
		if request.ID != record.ObjectID || request.Version != record.ObjectVersion || request.BaselineID != goalID || request.BaselineVersion != goalVersion {
			continue
		}
		if request.Status == contracts.AuthorityRequestPending {
			_, decisionErr := r.Store.GetSecureBlob(ctx, authorityDecisionNamespace, request.ID, request.Version, now)
			if decisionErr == nil {
				continue
			}
			if !errors.Is(decisionErr, state.ErrSecureBlobNotFound) && !errors.Is(decisionErr, state.ErrSecureBlobExpired) {
				return nil, decisionErr
			}
			if _, err := request.Digest(); err != nil {
				return nil, err
			}
			pending = append(pending, request)
		}
	}
	return pending, nil
}

type authorityDecisionRecord struct {
	Request  contracts.AuthorityRequest  `json:"request"`
	Decision contracts.AuthorityDecision `json:"decision"`
}

type authorityRevocationRecord struct {
	Decision   contracts.AuthorityDecision   `json:"decision"`
	Revocation contracts.AuthorityRevocation `json:"revocation"`
}

// SaveAuthorityDecision is the explicit structured decision boundary. The
// request is reloaded by immutable identity; the decision is stored under that
// same identity/version so conflicting second decisions cannot be recorded.
func (r Repository) SaveAuthorityDecision(ctx context.Context, requestID, requestVersion string, decision contracts.AuthorityDecision, createdAt time.Time, expiresAt *time.Time) error {
	if err := r.validateWorkPlanStore(); err != nil {
		return err
	}
	request, err := r.LoadAuthorityRequest(ctx, requestID, requestVersion, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("load authority request: %w", err)
	}
	if request.Status != contracts.AuthorityRequestPending {
		return errors.New("authority request is no longer pending")
	}
	if err := decision.Validate(request, time.Now().UTC()); err != nil {
		return err
	}
	if decision.AuthorityRef != "" || decision.AuthorityVersion != "" || decision.AuthorityGenerationDigest != "" {
		validator := r.AuthorityGeneration
		if validator == nil {
			validator = r
		}
		if err := validator.ValidateAuthorityGeneration(ctx, decision, time.Now().UTC()); err != nil {
			return fmt.Errorf("validate issuing authority generation: %w", err)
		}
	}
	if existing, err := r.LoadAuthorityDecision(ctx, requestID, requestVersion, time.Now().UTC()); err == nil {
		left, _ := json.Marshal(existing)
		right, _ := json.Marshal(decision)
		if bytes.Equal(left, right) {
			return nil
		}
		return errors.New("conflicting authority decision already exists")
	} else if !errors.Is(err, state.ErrSecureBlobNotFound) {
		return fmt.Errorf("check existing authority decision: %w", err)
	}
	payload, err := json.Marshal(authorityDecisionRecord{Request: request, Decision: decision})
	if err != nil {
		return fmt.Errorf("encode authority decision: %w", err)
	}
	var persistErr error
	if decision.AuthorityRef != "" && decision.AuthorityVersion != "" && decision.AuthorityGenerationDigest != "" {
		persistErr = r.putWorkPlanBlobUnlessRevoked(ctx, authorityDecisionNamespace, requestID, requestVersion, payload, createdAt, expiresAt, requestID, requestVersion, decision.AuthorityRef, decision.AuthorityVersion, authorityGenerationNamespace, decision.AuthorityRef, decision.AuthorityVersion)
	} else {
		persistErr = r.putWorkPlanBlob(ctx, authorityDecisionNamespace, requestID, requestVersion, payload, createdAt, expiresAt)
	}
	if persistErr != nil {
		return fmt.Errorf("persist authority decision: %w", persistErr)
	}
	return nil
}

func (r Repository) LoadAuthorityDecision(ctx context.Context, requestID, requestVersion string, now time.Time) (contracts.AuthorityDecision, error) {
	decision, err := r.LoadAuthorityDecisionEvidence(ctx, requestID, requestVersion, now)
	if err != nil {
		return contracts.AuthorityDecision{}, err
	}
	if _, err := r.LoadAuthorityRevocation(ctx, requestID, requestVersion, now); err == nil {
		return contracts.AuthorityDecision{}, ErrAuthorityDecisionRevoked
	} else if !errors.Is(err, state.ErrSecureBlobNotFound) && !errors.Is(err, state.ErrSecureBlobExpired) {
		return contracts.AuthorityDecision{}, fmt.Errorf("check authority revocation: %w", err)
	}
	return decision, nil
}

// LoadAuthorityDecisionEvidence returns the immutable original decision even
// after revocation, for audit. Callers seeking effective authority must use
// LoadAuthorityDecision, which applies expiry and revocation.
func (r Repository) LoadAuthorityDecisionEvidence(ctx context.Context, requestID, requestVersion string, now time.Time) (contracts.AuthorityDecision, error) {
	payload, record, err := r.loadWorkPlanBlob(ctx, authorityDecisionNamespace, requestID, requestVersion, now)
	if err != nil {
		return contracts.AuthorityDecision{}, err
	}
	var stored authorityDecisionRecord
	if err := json.Unmarshal(payload, &stored); err != nil {
		return contracts.AuthorityDecision{}, fmt.Errorf("decode authority decision: %w", err)
	}
	if stored.Request.ID != requestID || stored.Request.Version != requestVersion || payloadDigest(payload) != record.ObjectDigest {
		return contracts.AuthorityDecision{}, errors.New("authority decision identity or digest mismatch")
	}
	if err := stored.Decision.Validate(stored.Request, now); err != nil {
		return contracts.AuthorityDecision{}, fmt.Errorf("validate authority decision: %w", err)
	}
	return stored.Decision, nil
}

func (r Repository) validateWorkPlanStore() error {
	if r.Store == nil {
		return errors.New("work plan store is required")
	}
	if r.KeyRef == "" {
		return errors.New("work plan key reference is required")
	}
	if err := r.Profile.Validate(); err != nil {
		return err
	}
	return r.Sensitivity.Validate()
}

func (r Repository) SaveAuthorityGeneration(ctx context.Context, generation contracts.AuthorityGeneration, createdAt time.Time, expiresAt *time.Time) error {
	if err := generation.Validate(); err != nil {
		return err
	}
	payload, err := json.Marshal(generation)
	if err != nil {
		return fmt.Errorf("encode authority generation: %w", err)
	}
	return r.putWorkPlanBlob(ctx, authorityGenerationNamespace, generation.Ref, generation.Version, payload, createdAt, expiresAt)
}

func (r Repository) SaveAuthorityModelMigration(ctx context.Context, requestID, requestVersion string, migration contracts.AuthorityModelMigration, target contracts.AuthorityGeneration) error {
	if err := r.validateWorkPlanStore(); err != nil {
		return err
	}
	if r.BootstrapDigest == "" || migration.BootstrapDigest != r.BootstrapDigest {
		return errors.New("authority-model migration does not match protected bootstrap identity")
	}
	now := time.Now().UTC()
	source, err := r.LoadAuthorityGeneration(ctx, migration.SourceRef, migration.SourceVersion, now)
	if err != nil {
		return fmt.Errorf("load migration source: %w", err)
	}
	request, err := r.LoadAuthorityRequest(ctx, requestID, requestVersion, now)
	if err != nil {
		return err
	}
	decision, err := r.LoadAuthorityDecision(ctx, requestID, requestVersion, now)
	if err != nil {
		return err
	}
	if err := contracts.ValidateAuthorityModelMigrationApproval(migration, source, target, request, decision, now); err != nil {
		return err
	}
	if _, _, err := r.loadWorkPlanBlob(ctx, authorityGenerationInvalidationNamespace, source.Ref, source.Version, now); err == nil {
		return errors.New("migration source is revoked or superseded")
	} else if !errors.Is(err, state.ErrSecureBlobNotFound) && !errors.Is(err, state.ErrSecureBlobExpired) {
		return err
	}
	if existing, loadErr := r.LoadAuthorityModelMigration(ctx, source.Ref, source.Version, now); loadErr == nil {
		left, _ := json.Marshal(existing)
		right, _ := json.Marshal(migration)
		if bytes.Equal(left, right) {
			return nil
		}
		return errors.New("authority-model source already has a conflicting successor")
	} else if !errors.Is(loadErr, state.ErrSecureBlobNotFound) {
		return loadErr
	}
	migrationPayload, _ := json.Marshal(migration)
	targetPayload, _ := json.Marshal(target)
	records := make([]state.SecureBlobRecord, 0, 2)
	for _, item := range []struct {
		ns, id, version string
		payload         []byte
	}{{authorityModelMigrationNamespace, source.Ref, source.Version, migrationPayload}, {authorityGenerationNamespace, target.Ref, target.Version, targetPayload}} {
		digest := payloadDigest(item.payload)
		aad := state.SecureBlobAAD(item.ns, item.id, item.version, digest)
		envelope, sealErr := r.Crypto.Seal(ctx, r.KeyRef, r.Profile, item.payload, aad)
		if sealErr != nil {
			return sealErr
		}
		records = append(records, state.SecureBlobRecord{Namespace: item.ns, ObjectID: item.id, ObjectVersion: item.version, ObjectDigest: digest, Sensitivity: r.Sensitivity, CryptoProfile: r.Profile, Envelope: envelope, CreatedAt: migration.EffectiveAt})
	}
	return r.Store.PutSecureBlobsUnlessRevoked(ctx, records, authorityRevocationNamespace, requestID, requestVersion, authorityGenerationInvalidationNamespace, source.Ref, source.Version, authorityGenerationNamespace, source.Ref, source.Version)
}

func (r Repository) LoadAuthorityModelMigration(ctx context.Context, sourceRef, sourceVersion string, now time.Time) (contracts.AuthorityModelMigration, error) {
	payload, record, err := r.loadWorkPlanBlob(ctx, authorityModelMigrationNamespace, sourceRef, sourceVersion, now)
	if err != nil {
		return contracts.AuthorityModelMigration{}, err
	}
	var migration contracts.AuthorityModelMigration
	if err := json.Unmarshal(payload, &migration); err != nil {
		return migration, err
	}
	if payloadDigest(payload) != record.ObjectDigest || migration.SourceRef != sourceRef || migration.SourceVersion != sourceVersion {
		return migration, errors.New("authority-model migration identity mismatch")
	}
	source, err := r.LoadAuthorityGeneration(ctx, sourceRef, sourceVersion, now)
	if err != nil {
		return migration, err
	}
	target, err := r.LoadAuthorityGeneration(ctx, migration.TargetRef, migration.TargetVersion, now)
	if err != nil {
		return migration, err
	}
	if err := contracts.VerifyAuthorityModelMigration(migration, source, target); err != nil {
		return migration, err
	}
	return migration, nil
}

func (r Repository) SaveRoutingIssuance(ctx context.Context, issuance contracts.RoutingIssuance) (contracts.RoutingIssuance, error) {
	if r.BootstrapDigest == "" {
		return contracts.RoutingIssuance{}, errors.New("routing issuance requires protected bootstrap identity")
	}
	now := time.Now().UTC()
	issuance.EffectiveAt = now
	issuance.ID = ""
	frozen, err := contracts.FreezeRoutingIssuance(issuance)
	if err != nil {
		return contracts.RoutingIssuance{}, err
	}
	request, err := r.LoadAuthorityRequest(ctx, frozen.RequestID, frozen.RequestVersion, now)
	if err != nil {
		return contracts.RoutingIssuance{}, err
	}
	decision, err := r.LoadAuthorityDecision(ctx, frozen.RequestID, frozen.RequestVersion, now)
	if err != nil {
		return contracts.RoutingIssuance{}, err
	}
	if decision.Outcome != contracts.AuthorityApprove || request.RequestedAuthority != frozen.Authority || request.RequestedScope != frozen.Scope || request.ProposalDigest != frozen.PayloadDigest || decision.DecisionRef != frozen.DecisionRef || decision.DecisionVersion != frozen.DecisionVersion || decision.AuthorityRef != frozen.GenerationRef || decision.AuthorityVersion != frozen.GenerationVersion || decision.AuthorityGenerationDigest != frozen.GenerationDigest || decision.DecidedBy != frozen.IssuedBy {
		return contracts.RoutingIssuance{}, errors.New("routing issuance does not bind its exact approved authority decision")
	}
	generation, err := r.ValidateAuthorityGenerationLineage(ctx, frozen.GenerationRef, frozen.GenerationVersion, frozen.GenerationDigest, r.BootstrapDigest, now)
	if err != nil {
		return contracts.RoutingIssuance{}, err
	}
	if generation.AuthorityModelVersion != contracts.AuthorityModelV2Version || !containsString(generation.Authorities, frozen.Authority) {
		return contracts.RoutingIssuance{}, errors.New("routing issuance generation lacks v2 requested authority")
	}
	if frozen.ExpiresAt == nil || (decision.ExpiresAt != nil && frozen.ExpiresAt.After(*decision.ExpiresAt)) || (generation.ExpiresAt != nil && frozen.ExpiresAt.After(*generation.ExpiresAt)) {
		return contracts.RoutingIssuance{}, errors.New("routing issuance expiry exceeds authority")
	}
	payload, _ := json.Marshal(frozen)
	if err := r.putWorkPlanBlobUnlessRevoked(ctx, routingIssuanceNamespace, frozen.ID, frozen.Version, payload, now, frozen.ExpiresAt, frozen.RequestID, frozen.RequestVersion, frozen.GenerationRef, frozen.GenerationVersion, authorityGenerationNamespace, frozen.GenerationRef, frozen.GenerationVersion); err != nil {
		return contracts.RoutingIssuance{}, err
	}
	return frozen, nil
}

func (r Repository) LoadRoutingIssuance(ctx context.Context, ref contracts.RoutingIssuanceRef, now time.Time) (contracts.RoutingIssuance, error) {
	if r.BootstrapDigest == "" {
		return contracts.RoutingIssuance{}, errors.New("routing issuance requires protected bootstrap identity")
	}
	payload, record, err := r.loadWorkPlanBlob(ctx, routingIssuanceNamespace, ref.ID, ref.Version, now)
	if err != nil {
		return contracts.RoutingIssuance{}, err
	}
	var issuance contracts.RoutingIssuance
	if err := json.Unmarshal(payload, &issuance); err != nil {
		return issuance, err
	}
	if payloadDigest(payload) != record.ObjectDigest || issuance.ID != ref.ID || issuance.Version != ref.Version || contracts.VerifyRoutingIssuance(issuance) != nil {
		return issuance, errors.New("routing issuance identity or payload mismatch")
	}
	request, err := r.LoadAuthorityRequest(ctx, issuance.RequestID, issuance.RequestVersion, now)
	if err != nil {
		return issuance, err
	}
	decision, err := r.LoadAuthorityDecision(ctx, issuance.RequestID, issuance.RequestVersion, now)
	if err != nil {
		return issuance, err
	}
	if decision.Outcome != contracts.AuthorityApprove || request.RequestedAuthority != issuance.Authority || request.RequestedScope != issuance.Scope || request.ProposalDigest != issuance.PayloadDigest || decision.DecisionRef != issuance.DecisionRef || decision.DecisionVersion != issuance.DecisionVersion {
		return issuance, errors.New("routing issuance decision binding is invalid")
	}
	if _, err := r.ValidateAuthorityGenerationLineage(ctx, issuance.GenerationRef, issuance.GenerationVersion, issuance.GenerationDigest, r.BootstrapDigest, now); err != nil {
		return issuance, err
	}
	return issuance, nil
}

// SaveDelegatedAuthorityGeneration is the generic authority-owned child
// transition. The policy registry is mandatory: no lexical or implicit
// containment is permitted.
func (r Repository) SaveDelegatedAuthorityGeneration(ctx context.Context, requestID, requestVersion string, decision contracts.AuthorityDecision, policy contracts.DelegationContainmentPolicy, createdAt time.Time) (contracts.AuthorityGeneration, error) {
	if policy == nil {
		return contracts.AuthorityGeneration{}, errors.New("delegation containment policy is required")
	}
	request, err := r.LoadAuthorityRequest(ctx, requestID, requestVersion, createdAt)
	if err != nil {
		return contracts.AuthorityGeneration{}, fmt.Errorf("load delegation request: %w", err)
	}
	if request.RequestedAuthority != contracts.AuthorityDelegateCapability || request.Delegation == nil {
		return contracts.AuthorityGeneration{}, errors.New("request is not a delegation request")
	}
	if err := decision.Validate(request, createdAt); err != nil {
		return contracts.AuthorityGeneration{}, fmt.Errorf("validate delegation decision: %w", err)
	}
	if decision.Outcome != contracts.AuthorityApprove || decision.Delegation == nil {
		return contracts.AuthorityGeneration{}, errors.New("delegation requires an approved exact delegation decision")
	}
	delegation := request.Delegation
	parent, err := r.LoadAuthorityGeneration(ctx, delegation.ParentRef, delegation.ParentVersion, createdAt)
	if err != nil {
		return contracts.AuthorityGeneration{}, fmt.Errorf("load delegation parent: %w", err)
	}
	if parent.Digest != delegation.ParentDigest || decision.AuthorityRef != parent.Ref || decision.AuthorityVersion != parent.Version || decision.AuthorityGenerationDigest != parent.Digest || decision.DecidedBy != parent.Principal || decision.GrantedScope != parent.Scope {
		return contracts.AuthorityGeneration{}, errors.New("delegation decision does not bind the exact parent generation")
	}
	if !containsString(parent.Capabilities, contracts.AuthorityDelegateCapability) {
		return contracts.AuthorityGeneration{}, errors.New("parent generation lacks authority.delegate")
	}
	if err := policy.ContainDelegation(parent, *delegation, createdAt); err != nil {
		return contracts.AuthorityGeneration{}, fmt.Errorf("delegation containment denied: %w", err)
	}
	delegationDigest, err := request.Digest()
	if err != nil {
		return contracts.AuthorityGeneration{}, err
	}
	child := contracts.AuthorityGeneration{Ref: "authority-delegation:" + request.ID, Version: "1", Principal: delegation.DelegatedPrincipal, Scope: delegation.RequestedScope, Authorities: []string{delegation.RequestedAuthority}, EffectiveAt: createdAt.UTC(), ExpiresAt: &delegation.ExpiresAt, ParentRef: parent.Ref, ParentVersion: parent.Version, ParentDigest: parent.Digest, DelegatedBy: decision.DecidedBy, DelegationRef: request.ID + "/" + request.Version, DelegationDigest: delegationDigest, PolicyRef: delegation.PolicyRef, PolicyVersion: delegation.PolicyVersion, PolicyDigest: delegation.PolicyDigest, AuthorityModel: parent.AuthorityModel, AuthorityModelVersion: parent.AuthorityModelVersion, AuthorityModelDigest: parent.AuthorityModelDigest, State: contracts.AuthorityGenerationActive, ProvenanceRef: "authority-decision:" + decision.DecisionRef + ":" + decision.DecisionVersion, ProvenanceDigest: decision.AuthorityDigest}
	child.Digest, err = child.ComputeDigest()
	if err != nil {
		return contracts.AuthorityGeneration{}, fmt.Errorf("derive delegated generation digest: %w", err)
	}
	payload, err := json.Marshal(child)
	if err != nil {
		return contracts.AuthorityGeneration{}, fmt.Errorf("encode delegated generation: %w", err)
	}
	if err := r.putWorkPlanBlobWithLock(ctx, authorityGenerationNamespace, child.Ref, child.Version, payload, createdAt, &delegation.ExpiresAt, authorityRequestNamespace, request.ID, request.Version); err != nil {
		return contracts.AuthorityGeneration{}, fmt.Errorf("persist delegated generation: %w", err)
	}
	return child, nil
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func (r Repository) LoadAuthorityGeneration(ctx context.Context, ref, version string, now time.Time) (contracts.AuthorityGeneration, error) {
	payload, record, err := r.loadWorkPlanBlob(ctx, authorityGenerationNamespace, ref, version, now)
	if err != nil {
		return contracts.AuthorityGeneration{}, err
	}
	var generation contracts.AuthorityGeneration
	if err := json.Unmarshal(payload, &generation); err != nil {
		return contracts.AuthorityGeneration{}, fmt.Errorf("decode authority generation: %w", err)
	}
	if generation.Ref != ref || generation.Version != version || payloadDigest(payload) != record.ObjectDigest {
		return contracts.AuthorityGeneration{}, errors.New("authority generation identity or digest mismatch")
	}
	if err := generation.Validate(); err != nil {
		return contracts.AuthorityGeneration{}, err
	}
	if err := generation.VerifyDigest(); err != nil {
		return contracts.AuthorityGeneration{}, err
	}
	return generation, nil
}

// SaveProviderWorkspace persists one immutable workspace lifecycle snapshot.
// The workspace path is metadata only; the encrypted record is the authority
// used to recover ownership after restart.
func (r Repository) SaveProviderWorkspace(ctx context.Context, record contracts.ProviderWorkspaceRecord, createdAt time.Time, expiresAt *time.Time) (string, error) {
	if err := r.validateWorkPlanStore(); err != nil {
		return "", err
	}
	if err := record.Validate(); err != nil {
		return "", err
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return "", fmt.Errorf("encode provider workspace: %w", err)
	}
	digest, err := record.Digest()
	if err != nil {
		return "", err
	}
	if err := r.putWorkPlanBlob(ctx, providerWorkspaceNamespace, record.WorkspaceID, record.Version, payload, createdAt, expiresAt); err != nil {
		return "", fmt.Errorf("persist provider workspace: %w", err)
	}
	return digest, nil
}

func (r Repository) LoadProviderWorkspace(ctx context.Context, workspaceID, version string, now time.Time) (contracts.ProviderWorkspaceRecord, error) {
	payload, record, err := r.loadWorkPlanBlob(ctx, providerWorkspaceNamespace, workspaceID, version, now)
	if err != nil {
		return contracts.ProviderWorkspaceRecord{}, err
	}
	var workspace contracts.ProviderWorkspaceRecord
	if err := json.Unmarshal(payload, &workspace); err != nil {
		return contracts.ProviderWorkspaceRecord{}, fmt.Errorf("decode provider workspace: %w", err)
	}
	if workspace.WorkspaceID != workspaceID || workspace.Version != version || payloadDigest(payload) != record.ObjectDigest {
		return contracts.ProviderWorkspaceRecord{}, errors.New("provider workspace identity or digest mismatch")
	}
	if err := workspace.Validate(); err != nil {
		return contracts.ProviderWorkspaceRecord{}, err
	}
	return workspace, nil
}

// ListProviderWorkspaces returns all validated lifecycle snapshots in stable
// identity/version order. Callers must derive the latest state explicitly;
// no mutable current-workspace pointer is authoritative.
func (r Repository) ListProviderWorkspaces(ctx context.Context, now time.Time) ([]contracts.ProviderWorkspaceRecord, error) {
	if err := r.validateWorkPlanStore(); err != nil {
		return nil, err
	}
	records, err := r.Store.ListSecureBlobs(ctx, providerWorkspaceNamespace, now)
	if err != nil {
		return nil, err
	}
	workspaces := make([]contracts.ProviderWorkspaceRecord, 0, len(records))
	for _, secure := range records {
		workspace, err := r.LoadProviderWorkspace(ctx, secure.ObjectID, secure.ObjectVersion, now)
		if err != nil {
			return nil, err
		}
		workspaces = append(workspaces, workspace)
	}
	sort.Slice(workspaces, func(i, j int) bool {
		if workspaces[i].WorkspaceID != workspaces[j].WorkspaceID {
			return workspaces[i].WorkspaceID < workspaces[j].WorkspaceID
		}
		left, _ := strconv.Atoi(workspaces[i].Version)
		right, _ := strconv.Atoi(workspaces[j].Version)
		return left < right
	})
	return workspaces, nil
}

// ListAuthorityGenerations returns the encrypted, durable governance
// generations in stable identity order. It is used by explicit root
// enrollment to reject a second installation root rather than silently
// creating a competing principal.
func (r Repository) ListAuthorityGenerations(ctx context.Context, now time.Time) ([]contracts.AuthorityGeneration, error) {
	if err := r.validateWorkPlanStore(); err != nil {
		return nil, err
	}
	records, err := r.Store.ListSecureBlobs(ctx, authorityGenerationNamespace, now)
	if err != nil {
		return nil, err
	}
	generations := make([]contracts.AuthorityGeneration, 0, len(records))
	for _, record := range records {
		payload, err := r.Crypto.Open(ctx, record.Envelope, state.SecureBlobAAD(record.Namespace, record.ObjectID, record.ObjectVersion, record.ObjectDigest))
		if err != nil {
			return nil, fmt.Errorf("decrypt authority generation %s/%s: %w", record.ObjectID, record.ObjectVersion, err)
		}
		var generation contracts.AuthorityGeneration
		if err := json.Unmarshal(payload, &generation); err != nil {
			return nil, fmt.Errorf("decode authority generation %s/%s: %w", record.ObjectID, record.ObjectVersion, err)
		}
		if generation.Ref != record.ObjectID || generation.Version != record.ObjectVersion || payloadDigest(payload) != record.ObjectDigest {
			return nil, errors.New("authority generation identity or digest mismatch")
		}
		if err := generation.Validate(); err != nil {
			return nil, err
		}
		if err := generation.VerifyDigest(); err != nil {
			return nil, err
		}
		generations = append(generations, generation)
	}
	return generations, nil
}

func (r Repository) SaveAuthorityGenerationInvalidation(ctx context.Context, invalidation contracts.AuthorityGenerationInvalidation, createdAt time.Time, expiresAt *time.Time) error {
	if expiresAt != nil || invalidation.EffectiveAt.After(time.Now().UTC()) {
		return errors.New("authority generation invalidation must be immediate and non-expiring")
	}
	generation, err := r.LoadAuthorityGeneration(ctx, invalidation.Ref, invalidation.Version, time.Now().UTC())
	if err != nil {
		return err
	}
	if err := invalidation.Validate(generation); err != nil {
		return err
	}
	payload, err := json.Marshal(invalidation)
	if err != nil {
		return fmt.Errorf("encode authority generation invalidation: %w", err)
	}
	return r.putWorkPlanBlobWithLock(ctx, authorityGenerationInvalidationNamespace, invalidation.Ref, invalidation.Version, payload, createdAt, expiresAt, authorityGenerationNamespace, invalidation.Ref, invalidation.Version)
}

func (r Repository) ValidateAuthorityGeneration(ctx context.Context, decision contracts.AuthorityDecision, now time.Time) error {
	if decision.AuthorityRef == "" || decision.AuthorityVersion == "" || decision.AuthorityGenerationDigest == "" {
		return errors.New("authority decision lacks exact generation binding")
	}
	generation, err := r.LoadAuthorityGeneration(ctx, decision.AuthorityRef, decision.AuthorityVersion, now)
	if err != nil {
		return err
	}
	if generation.Digest != decision.AuthorityGenerationDigest || generation.Principal != decision.DecidedBy || generation.Scope != decision.GrantedScope {
		return errors.New("authority decision does not match current authority generation")
	}
	request, err := r.LoadAuthorityRequest(ctx, decision.RequestID, decision.RequestVersion, now)
	if err != nil {
		return fmt.Errorf("load authority request for generation validation: %w", err)
	}
	if request.RequestedAuthority == contracts.AuthorityDelegateCapability {
		if !containsString(generation.Capabilities, contracts.AuthorityDelegateCapability) {
			return errors.New("authority generation lacks authority.delegate")
		}
	} else if !containsString(generation.Authorities, request.RequestedAuthority) {
		return errors.New("authority generation lacks requested authority")
	}
	if _, _, err := r.loadWorkPlanBlob(ctx, authorityGenerationInvalidationNamespace, generation.Ref, generation.Version, now); err == nil {
		return errors.New("authority generation is revoked or superseded")
	} else if !errors.Is(err, state.ErrSecureBlobNotFound) && !errors.Is(err, state.ErrSecureBlobExpired) {
		return err
	}
	return nil
}

func (r Repository) ValidateAuthorityGenerationLineage(ctx context.Context, ref, version, digest, bootstrapDigest string, now time.Time) (contracts.AuthorityGeneration, error) {
	seen := map[string]bool{}
	var walk func(string, string, string) (contracts.AuthorityGeneration, error)
	walk = func(currentRef, currentVersion, currentDigest string) (contracts.AuthorityGeneration, error) {
		key := currentRef + "@" + currentVersion
		if seen[key] {
			return contracts.AuthorityGeneration{}, errors.New("authority generation lineage contains a cycle")
		}
		seen[key] = true
		generation, err := r.LoadAuthorityGeneration(ctx, currentRef, currentVersion, now)
		if err != nil {
			return generation, err
		}
		if generation.Digest != currentDigest || generation.EffectiveAt.After(now) || (generation.ExpiresAt != nil && !now.Before(*generation.ExpiresAt)) {
			return generation, errors.New("authority generation is mismatched or inactive")
		}
		if _, _, err := r.loadWorkPlanBlob(ctx, authorityGenerationInvalidationNamespace, generation.Ref, generation.Version, now); err == nil {
			return generation, errors.New("authority generation is revoked or superseded")
		} else if !errors.Is(err, state.ErrSecureBlobNotFound) && !errors.Is(err, state.ErrSecureBlobExpired) {
			return generation, err
		}
		if generation.ParentRef == "" {
			scope, scopeErr := contracts.InstallationGovernanceScope(bootstrapDigest)
			owner, ownerErr := contracts.InstallationOwnerPrincipal(bootstrapDigest)
			if scopeErr != nil || ownerErr != nil || generation.Ref != scope || generation.Scope != scope || generation.Principal != owner || generation.Capabilities == nil || !containsString(generation.Capabilities, contracts.AuthorityDelegateCapability) {
				return generation, errors.New("authority lineage does not terminate at the installation root")
			}
			if generation.AuthorityModelVersion == contracts.AuthorityModelV2Version {
				migration, err := r.LoadAuthorityModelMigration(ctx, generation.Ref, "1", now)
				if err != nil || migration.TargetDigest != generation.Digest {
					return generation, errors.New("v2 root lacks exact governed migration")
				}
			}
			return generation, nil
		}
		parent, err := walk(generation.ParentRef, generation.ParentVersion, generation.ParentDigest)
		if err != nil {
			return generation, err
		}
		requestID, requestVersion, ok := strings.Cut(generation.DelegationRef, "/")
		if !ok {
			return generation, errors.New("delegated generation request lineage is malformed")
		}
		request, err := r.LoadAuthorityRequest(ctx, requestID, requestVersion, now)
		if err != nil || request.Delegation == nil {
			return generation, errors.New("delegated generation request is unavailable")
		}
		decision, err := r.LoadAuthorityDecision(ctx, requestID, requestVersion, now)
		if err != nil {
			return generation, err
		}
		if decision.AuthorityRef != parent.Ref || decision.AuthorityVersion != parent.Version || decision.AuthorityGenerationDigest != parent.Digest || decision.DecidedBy != parent.Principal || generation.DelegatedBy != parent.Principal || generation.AuthorityModelVersion != parent.AuthorityModelVersion || generation.AuthorityModelDigest != parent.AuthorityModelDigest {
			return generation, errors.New("delegated generation does not bind its parent decision")
		}
		if err := contracts.ValidateBuiltinDelegation(parent, *request.Delegation, now); err != nil {
			return generation, err
		}
		if request.Delegation.DelegatedPrincipal != generation.Principal || request.Delegation.RequestedAuthority != firstAuthority(generation.Authorities) || request.Delegation.RequestedScope != generation.Scope {
			return generation, errors.New("delegated generation exceeds its exact request")
		}
		return generation, nil
	}
	return walk(ref, version, digest)
}

func firstAuthority(values []string) string {
	if len(values) == 1 {
		return values[0]
	}
	return ""
}

func (r Repository) putWorkPlanBlob(ctx context.Context, namespace, objectID, version string, payload []byte, createdAt time.Time, expiresAt *time.Time) error {
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	digest := payloadDigest(payload)
	aad := state.SecureBlobAAD(namespace, objectID, version, digest)
	envelope, err := r.Crypto.Seal(ctx, r.KeyRef, r.Profile, payload, aad)
	if err != nil {
		return fmt.Errorf("encrypt WorkPlan record: %w", err)
	}
	return r.Store.PutSecureBlob(ctx, state.SecureBlobRecord{Namespace: namespace, ObjectID: objectID, ObjectVersion: version, ObjectDigest: digest, Sensitivity: r.Sensitivity, CryptoProfile: r.Profile, Envelope: envelope, CreatedAt: createdAt, ExpiresAt: expiresAt})
}

func (r Repository) putWorkPlanBlobUnlessRevoked(ctx context.Context, namespace, objectID, version string, payload []byte, createdAt time.Time, expiresAt *time.Time, requestID, requestVersion, additionalID, additionalVersion, lockNamespace, lockID, lockVersion string) error {
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	digest := payloadDigest(payload)
	aad := state.SecureBlobAAD(namespace, objectID, version, digest)
	envelope, err := r.Crypto.Seal(ctx, r.KeyRef, r.Profile, payload, aad)
	if err != nil {
		return fmt.Errorf("encrypt authority-bound WorkPlan record: %w", err)
	}
	record := state.SecureBlobRecord{Namespace: namespace, ObjectID: objectID, ObjectVersion: version, ObjectDigest: digest, Sensitivity: r.Sensitivity, CryptoProfile: r.Profile, Envelope: envelope, CreatedAt: createdAt, ExpiresAt: expiresAt}
	return r.Store.PutSecureBlobUnlessRevoked(ctx, record, authorityRevocationNamespace, requestID, requestVersion, authorityGenerationInvalidationNamespace, additionalID, additionalVersion, lockNamespace, lockID, lockVersion)
}

func (r Repository) putWorkPlanBlobWithLock(ctx context.Context, namespace, objectID, version string, payload []byte, createdAt time.Time, expiresAt *time.Time, lockNamespace, lockID, lockVersion string) error {
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	digest := payloadDigest(payload)
	aad := state.SecureBlobAAD(namespace, objectID, version, digest)
	envelope, err := r.Crypto.Seal(ctx, r.KeyRef, r.Profile, payload, aad)
	if err != nil {
		return fmt.Errorf("encrypt locked WorkPlan record: %w", err)
	}
	record := state.SecureBlobRecord{Namespace: namespace, ObjectID: objectID, ObjectVersion: version, ObjectDigest: digest, Sensitivity: r.Sensitivity, CryptoProfile: r.Profile, Envelope: envelope, CreatedAt: createdAt, ExpiresAt: expiresAt}
	return r.Store.PutSecureBlobWithLock(ctx, record, lockNamespace, lockID, lockVersion)
}

func (r Repository) loadWorkPlanBlob(ctx context.Context, namespace, objectID, version string, now time.Time) ([]byte, state.SecureBlobRecord, error) {
	if err := r.validateWorkPlanStore(); err != nil {
		return nil, state.SecureBlobRecord{}, err
	}
	record, err := r.Store.GetSecureBlob(ctx, namespace, objectID, version, now)
	if err != nil {
		return nil, state.SecureBlobRecord{}, err
	}
	aad := state.SecureBlobAAD(namespace, objectID, version, record.ObjectDigest)
	payload, err := r.Crypto.Open(ctx, record.Envelope, aad)
	if err != nil {
		return nil, state.SecureBlobRecord{}, fmt.Errorf("decrypt WorkPlan record: %w", err)
	}
	return payload, record, nil
}

func sessionDigest(payload []byte) string {
	return payloadDigest(payload)
}

func payloadDigest(payload []byte) string {
	digest := sha256.Sum256(payload)
	return fmt.Sprintf("sha256:%x", digest[:])
}

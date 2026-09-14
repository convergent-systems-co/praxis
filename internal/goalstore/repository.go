package goalstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
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

var ErrAuthorityDecisionRevoked = errors.New("authority decision is revoked")

type Repository struct {
	Store               *state.Store
	Crypto              praxiscrypto.EnvelopeService
	KeyRef              string
	Profile             contracts.CryptoProfile
	Sensitivity         state.Sensitivity
	AuthorityGeneration AuthorityGenerationValidator
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
		err = r.Store.PutSecureBlobUnlessRevoked(ctx, record, authorityRevocationNamespace, plan.AuthorityRequestID, plan.AuthorityRequestVersion, baselineNamespace, sourceID, sourceVersion)
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
	if authorityDecision.AuthorityRef == "" || authorityDecision.AuthorityVersion == "" || r.AuthorityGeneration == nil {
		return contracts.WorkPlan{}, errors.New("authority generation validation is required for cross-registry acceptance")
	}
	if err := r.AuthorityGeneration.ValidateAuthorityGeneration(ctx, authorityDecision, time.Now().UTC()); err != nil {
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
		AuthorityDecisionRef: authorityDecision.DecisionRef, AuthorityDecisionVersion: authorityDecision.DecisionVersion,
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
	if err := r.putWorkPlanBlobUnlessRevoked(ctx, workPlanAcceptanceNamespace, acceptanceRef, acceptanceVersion, payload, createdAt, expiresAt, request.ID, request.Version, authorityRequestNamespace, request.ID, request.Version); err != nil {
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
	if err := r.putWorkPlanBlobWithLock(ctx, authorityRevocationNamespace, requestID, requestVersion, payload, createdAt, expiresAt, authorityRequestNamespace, requestID, requestVersion); err != nil {
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
	if err := r.putWorkPlanBlob(ctx, authorityDecisionNamespace, requestID, requestVersion, payload, createdAt, expiresAt); err != nil {
		return fmt.Errorf("persist authority decision: %w", err)
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

func (r Repository) putWorkPlanBlobUnlessRevoked(ctx context.Context, namespace, objectID, version string, payload []byte, createdAt time.Time, expiresAt *time.Time, requestID, requestVersion, lockNamespace, lockID, lockVersion string) error {
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
	return r.Store.PutSecureBlobUnlessRevoked(ctx, record, authorityRevocationNamespace, requestID, requestVersion, lockNamespace, lockID, lockVersion)
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

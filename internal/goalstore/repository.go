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
	"github.com/convergent-systems-co/praxis/internal/faa"
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
const routingIssuanceNamespace = "routing_issuance"
const providerWorkspaceNamespace = "provider_workspace"

var ErrAuthorityDecisionRevoked = errors.New("authority decision is revoked")

type Repository struct {
	Store       *state.Store
	Crypto      praxiscrypto.EnvelopeService
	KeyRef      string
	Profile     contracts.CryptoProfile
	Sensitivity state.Sensitivity
	// InstallationDigest is the digest of the protected bootstrap record
	// opened by the production repository constructor. Lifecycle repair uses
	// it to bind authority to this installation rather than to a merely
	// self-consistent root identity supplied by durable request data.
	InstallationDigest  string
	AuthorityGeneration AuthorityGenerationValidator
	// SafetyActivation is the fail-closed activation predicate enforced at
	// every persistence boundary whose correctness depends on the pre-v4
	// safety kernel. Leaving it nil refuses such mutations; it never means
	// "not required".
	SafetyActivation SafetyActivationVerifier
	// FAA is the Forward Authority Anchor: forward-only governance freshness
	// state outside the mutable store (I13, I14). Production constructors always
	// set it and refuse to build a governed repository without one; nil means an
	// unanchored repository, which exists only so unit tests of unrelated store
	// behaviour need not stand up an anchor.
	FAA faa.Anchor
	// faaLagAttempts and faaLagDelay tune how long a reader waits out the
	// one-write lag between the anchor and the store (zero means the defaults).
	// They exist so exhaustive adversarial tests do not spend their time waiting.
	faaLagAttempts int
	faaLagDelay    time.Duration
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
	// Generic baseline persistence (Save, Import, Finalize, establish) is
	// baseline-only. A safety-bearing WorkPlan enters a generation solely
	// through AttachAcceptedWorkPlan, which reloads the durable acceptance,
	// re-verifies current authority and activation, and is revocation-fenced.
	if baseline.WorkPlan != nil {
		if baseline.WorkPlan.Safety != nil {
			return goals.GoalBaseline{}, ErrSafetyPlanRequiresAttachment
		}
		// I9: whether a plan is safety-bearing must not depend on the plan
		// carrying the safety binding. A Goal that durable, authenticated
		// state classifies as safety-bearing can never receive a plan through
		// this generic surface (Import, Finalize and establish included), so
		// stripping or omitting the binding cannot downgrade it.
		if kernel, classified, err := r.GoalSafetyKernel(ctx, baseline.ID); err != nil {
			return goals.GoalBaseline{}, err
		} else if classified {
			return goals.GoalBaseline{}, fmt.Errorf("%w: Goal %s (kernel %s); a plan enters only through the authenticated accepted-plan attachment transaction", ErrSafetyDowngrade, baseline.ID, kernel)
		}
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
	if err := contracts.UnmarshalExactJSON(payload, &baseline, false); err != nil {
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
	plan, acceptedProposal, err := r.loadAcceptedWorkPlanRecord(ctx, acceptanceRef, acceptanceVersion, time.Now().UTC())
	if err != nil {
		return goals.GoalBaseline{}, fmt.Errorf("load accepted WorkPlan: %w", err)
	}
	if plan.BaselineDigest != source.Digest {
		return goals.GoalBaseline{}, fmt.Errorf("accepted WorkPlan source baseline differs: %w", goals.ErrBaselineDigestMismatch)
	}
	// The baseline digest does not cover Goal identity, so an acceptance
	// granted for one Goal must not attach to a different Goal that happens to
	// have identical content.
	if acceptedProposal.GoalID != source.ID || acceptedProposal.GoalVersion != source.Version {
		return goals.GoalBaseline{}, fmt.Errorf("accepted WorkPlan was accepted for Goal %s/%s, not %s/%s", acceptedProposal.GoalID, acceptedProposal.GoalVersion, source.ID, source.Version)
	}
	if err := r.requireSafetyActivation(ctx, plan.Safety); err != nil {
		return goals.GoalBaseline{}, err
	}
	if err := r.requireSafetyConsistent(ctx, source.ID, plan.Safety); err != nil {
		return goals.GoalBaseline{}, err
	}
	if err := r.markGoalSafetyBearing(ctx, source.ID, plan.Safety, createdAt); err != nil {
		return goals.GoalBaseline{}, err
	}
	if plan.Safety != nil && plan.AuthorityRequestID == "" {
		return goals.GoalBaseline{}, errors.New("safety-bearing WorkPlan requires authority-backed acceptance; legacy acceptance cannot attach")
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
	if err := r.verifyPlanAuthorityLineage(ctx, plan, time.Now().UTC()); err != nil {
		return goals.GoalBaseline{}, fmt.Errorf("accepted WorkPlan authority is not currently effective: %w", err)
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
	if err := r.requireSafetyActivation(ctx, proposal.Safety); err != nil {
		return "", err
	}
	// I9: a legacy proposal cannot be introduced for a Goal that is already
	// classified safety-bearing, and a safety-bearing proposal classifies its
	// Goal durably before the proposal itself exists.
	if err := r.requireSafetyConsistent(ctx, proposal.GoalID, proposal.Safety); err != nil {
		return "", err
	}
	if err := r.markGoalSafetyBearing(ctx, proposal.GoalID, proposal.Safety, createdAt); err != nil {
		return "", err
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
	if err := r.requireSafetyActivation(ctx, proposal.Safety); err != nil {
		return err
	}
	if err := r.requireSafetyConsistent(ctx, proposal.GoalID, proposal.Safety); err != nil {
		return err
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
	if proposal.Safety != nil {
		return contracts.WorkPlan{}, errors.New("legacy acceptance cannot admit a safety-bearing WorkPlan; use the authority-backed acceptance bridge")
	}
	if err := r.requireSafetyConsistent(ctx, proposal.GoalID, proposal.Safety); err != nil {
		return contracts.WorkPlan{}, err
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
	if err := r.requireSafetyActivation(ctx, proposal.Safety); err != nil {
		return contracts.WorkPlan{}, err
	}
	if err := r.requireSafetyConsistent(ctx, proposal.GoalID, proposal.Safety); err != nil {
		return contracts.WorkPlan{}, err
	}
	if proposal.Safety != nil {
		if request.CeremonyProfile != "interactive-os-owner-v1" || request.ActivationManifestDigest != proposal.Safety.ActivationManifestDigest {
			return contracts.WorkPlan{}, errors.New("safety-bearing WorkPlan acceptance requires exact interactive ceremony and activation binding")
		}
		if err := r.validateOwnerDecisionAuthority(ctx, request, authorityDecision, time.Now().UTC(), false); err != nil {
			return contracts.WorkPlan{}, fmt.Errorf("safety-bearing WorkPlan acceptance lacks authentic owner authority: %w", err)
		}
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
	plan, _, err := r.loadAcceptedWorkPlanRecord(ctx, acceptanceRef, version, now)
	return plan, err
}

// loadAcceptedWorkPlanRecord additionally returns the exact proposal the
// acceptance was granted for, so attachment can bind Goal identity.
func (r Repository) loadAcceptedWorkPlanRecord(ctx context.Context, acceptanceRef, version string, now time.Time) (contracts.WorkPlan, contracts.WorkPlanProposal, error) {
	payload, record, err := r.loadWorkPlanBlob(ctx, workPlanAcceptanceNamespace, acceptanceRef, version, now)
	if err != nil {
		return contracts.WorkPlan{}, contracts.WorkPlanProposal{}, err
	}
	var stored acceptedWorkPlanRecord
	if err := json.Unmarshal(payload, &stored); err != nil {
		return contracts.WorkPlan{}, contracts.WorkPlanProposal{}, fmt.Errorf("decode accepted WorkPlan: %w", err)
	}
	if stored.Decision.AcceptanceRef != acceptanceRef || payloadDigest(payload) != record.ObjectDigest {
		return contracts.WorkPlan{}, contracts.WorkPlanProposal{}, errors.New("accepted WorkPlan identity or record digest mismatch")
	}
	if err := stored.Review.Validate(stored.Proposal); err != nil || stored.Review.ReviewDigest != stored.Decision.ReviewDigest || stored.Review.Status != contracts.ReviewAcceptableForAuthority {
		return contracts.WorkPlan{}, contracts.WorkPlanProposal{}, fmt.Errorf("%w: persisted acceptance review is not valid", contracts.ErrUnacceptedWorkPlan)
	}
	plan, err := contracts.AcceptWorkPlan(stored.Proposal, stored.Plan, stored.Decision)
	if err != nil {
		return contracts.WorkPlan{}, contracts.WorkPlanProposal{}, fmt.Errorf("validate accepted WorkPlan: %w", err)
	}
	return plan, stored.Proposal, nil
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
	// I13: the retirement is anchored FIRST, as a fact in the chain the forward
	// authority anchor pins, so neither deleting the revocation row nor replaying
	// the earlier liveness row nor restoring an earlier store can make the
	// decision current again. The liveness row is then retired (I12) and the
	// revocation written; a crash between leaves the decision retired by fact.
	decisionDigest, err := decision.Digest()
	if err != nil {
		return err
	}
	if err := r.anchorDecisionRetired(ctx, requestID, requestVersion, decisionDigest, "revoked", createdAt); err != nil {
		return err
	}
	if err := r.retireLive(ctx, state.AuthorityDecisionLiveNamespace, requestID, requestVersion); err != nil {
		return err
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
	if request.RequestedAuthority == "goal.gate.decide" {
		scope, scopeErr := contracts.InstallationGovernanceScope(r.InstallationDigest)
		if scopeErr != nil || request.RequestedScope != scope {
			return "", errors.New("authority gate request is not scoped to this installation's governance authority")
		}
	}
	binding, err := r.safetyBindingForRequest(ctx, request)
	if err != nil {
		return "", err
	}
	if err := r.requireSafetyActivation(ctx, binding); err != nil {
		return "", err
	}
	if binding != nil && (request.CeremonyProfile != "interactive-os-owner-v1" || request.ActivationManifestDigest != binding.ActivationManifestDigest) {
		return "", errors.New("protected authority request lacks exact interactive ceremony and activation binding")
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
	_, err = request.DigestAt(now)
	if err != nil || request.ID != id || request.Version != version || payloadDigest(payload) != record.ObjectDigest {
		return contracts.AuthorityRequest{}, errors.New("authority request identity or digest mismatch")
	}
	return request, nil
}

// LoadAuthorityRequestByDigest resolves the immutable request identity emitted
// by a canonical producer. It never reconstructs a request from caller fields.
func (r Repository) LoadAuthorityRequestByDigest(ctx context.Context, wanted string, now time.Time) (contracts.AuthorityRequest, error) {
	if err := contracts.ValidateSHA256Digest(wanted); err != nil {
		return contracts.AuthorityRequest{}, err
	}
	records, err := r.Store.ListSecureBlobs(ctx, authorityRequestNamespace, now)
	if err != nil {
		return contracts.AuthorityRequest{}, err
	}
	for _, record := range records {
		payload, err := r.Crypto.Open(ctx, record.Envelope, state.SecureBlobAAD(record.Namespace, record.ObjectID, record.ObjectVersion, record.ObjectDigest))
		if err != nil {
			return contracts.AuthorityRequest{}, err
		}
		var request contracts.AuthorityRequest
		if err := json.Unmarshal(payload, &request); err != nil {
			return contracts.AuthorityRequest{}, err
		}
		digest, err := request.Digest()
		if err != nil || digest != wanted || request.ID != record.ObjectID || request.Version != record.ObjectVersion || payloadDigest(payload) != record.ObjectDigest {
			continue
		}
		return request, nil
	}
	return contracts.AuthorityRequest{}, state.ErrSecureBlobNotFound
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
	binding, err := r.safetyBindingForRequest(ctx, request)
	if err != nil {
		return err
	}
	if err := r.requireSafetyActivation(ctx, binding); err != nil {
		return err
	}
	if request.RequestedAuthority == "goal.gate.decide" {
		if err := r.validateGateDecisionAuthority(ctx, request, decision, time.Now().UTC()); err != nil {
			return err
		}
	} else if err := r.verifyDecisionCeremony(ctx, request, decision, time.Now().UTC()); err != nil {
		return err
	}
	if request.RequestedAuthority == contracts.GovernedPackageDeploy && request.Delegation == nil {
		if err := r.validatePackageDeploymentDecision(ctx, request, decision, time.Now().UTC()); err != nil {
			return err
		}
	}
	if request.RequestedAuthority == contracts.GovernedInstallationRepairStorageSchema || request.RequestedAuthority == contracts.GovernedInstallationRepairRuntimeState {
		if request.Repair == nil || decision.AuthorityRef != request.Repair.RootRef || decision.AuthorityVersion != request.Repair.RootVersion || decision.AuthorityGenerationDigest != request.Repair.RootDigest || decision.DecidedBy.Kind != "human" || decision.AuthorityDigest != request.Repair.SuccessionDecisionDigest {
			return errors.New("installation-repair decision lacks exact successor-root lineage")
		}
	}
	if existing, err := r.LoadAuthorityDecisionEvidence(ctx, requestID, requestVersion, time.Now().UTC()); err == nil {
		if _, revokedErr := r.LoadAuthorityRevocation(ctx, requestID, requestVersion, time.Now().UTC()); revokedErr == nil {
			return fmt.Errorf("check existing authority decision: %w", ErrAuthorityDecisionRevoked)
		}
		left, _ := json.Marshal(existing)
		right, _ := json.Marshal(decision)
		if bytes.Equal(left, right) {
			// An exact replay never recreates a liveness record: it is written
			// atomically with the decision, so its absence means the decision
			// was retired or the governing state was lost, and neither may be
			// undone by resubmitting the same bytes.
			digest, digestErr := decision.Digest()
			if digestErr != nil {
				return digestErr
			}
			if err := r.requireLive(ctx, state.AuthorityDecisionLiveNamespace, requestID, requestVersion, digest, time.Now().UTC()); err != nil {
				return fmt.Errorf("%w: %w", ErrAuthorityDecisionNotLive, err)
			}
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
	decisionDigest, err := decision.Digest()
	if err != nil {
		return err
	}
	stamp, err := r.admissionStamp(ctx)
	if err != nil {
		return err
	}
	liveRecord, err := state.SealedLivenessRecord(ctx, r.Crypto, r.KeyRef, r.Profile, r.Sensitivity, state.AuthorityDecisionLiveNamespace, requestID, requestVersion, decisionDigest, createdAt, stamp)
	if err != nil {
		return fmt.Errorf("seal decision liveness: %w", err)
	}
	if decision.AuthorityRef != "" && decision.AuthorityVersion != "" && decision.AuthorityGenerationDigest != "" {
		persistErr = r.putWorkPlanBlobUnlessRevoked(ctx, authorityDecisionNamespace, requestID, requestVersion, payload, createdAt, expiresAt, requestID, requestVersion, decision.AuthorityRef, decision.AuthorityVersion, authorityGenerationNamespace, decision.AuthorityRef, decision.AuthorityVersion, liveRecord)
	} else {
		// Without an issuing generation there is no lock source; the decision
		// and its liveness record still commit together.
		persistErr = r.putWorkPlanBlobsAtomically(ctx, authorityDecisionNamespace, requestID, requestVersion, payload, createdAt, expiresAt, liveRecord)
	}
	if persistErr != nil {
		return fmt.Errorf("persist authority decision: %w", persistErr)
	}
	return nil
}

func (r Repository) validatePackageDeploymentDecision(ctx context.Context, request contracts.AuthorityRequest, decision contracts.AuthorityDecision, now time.Time) error {
	if request.Intent == nil || request.IntentDigest == "" || request.VerificationEvidenceDigest == "" || request.InstallationDigest == "" || request.ClosureDigest == "" || decision.OperationalAuthorityRef == "" || decision.OperationalAuthorityVersion == "" || decision.OperationalAuthorityGenerationDigest == "" {
		return errors.New("package-deploy decision lacks exact intent, evidence, or operational authority lineage")
	}
	intentDigest, err := request.Intent.Digest()
	if err != nil || intentDigest != request.IntentDigest || request.Intent.Operation != contracts.GovernedPackageDeploy || request.Intent.Actor != contracts.PackageManagerPrincipal() {
		return errors.New("package-deploy decision intent mismatch")
	}
	evidence, err := r.Store.LoadVerificationEvidence(ctx, request.VerificationEvidenceDigest)
	if err != nil || evidence.InstallationDigest != request.InstallationDigest || evidence.ClosureDigest != request.ClosureDigest {
		return errors.New("package-deploy decision evidence mismatch")
	}
	operational, err := r.LoadAuthorityGeneration(ctx, decision.OperationalAuthorityRef, decision.OperationalAuthorityVersion, now)
	if err != nil {
		return err
	}
	if operational.Digest != decision.OperationalAuthorityGenerationDigest || operational.Principal != contracts.PackageManagerPrincipal() || operational.DelegationProfile != contracts.DelegationProfilePackageDeploy || operational.AuthorityModelVersion != contracts.AuthorityModelDeploymentVersion || operational.AuthorityModelDigest != contracts.AuthorityModelDeploymentDigest() || operational.ParentDigest != request.InstallationDigest || !containsAuthority(operational.Authorities, contracts.GovernedPackageDeploy) {
		return errors.New("package-deploy decision does not bind exact operational authority")
	}
	// The operational generation must be current, with its whole lineage (N17
	// equivalent path): it was compared field by field but never asked whether it
	// was still in force.
	if err := r.requireCurrentLineage(ctx, operational, now); err != nil {
		return fmt.Errorf("package-deploy operational authority is not current: %w", err)
	}
	return nil
}

func (r Repository) LoadAuthorityDecision(ctx context.Context, requestID, requestVersion string, now time.Time) (contracts.AuthorityDecision, error) {
	// I13: a store that is not current against the forward authority anchor
	// yields no effective decision, whatever else it contains.
	if _, err := r.governanceSnapshot(ctx); err != nil {
		return contracts.AuthorityDecision{}, err
	}
	stored, err := r.loadAuthorityDecisionRecord(ctx, requestID, requestVersion, now)
	if err != nil {
		return contracts.AuthorityDecision{}, err
	}
	decision := stored.Decision
	if _, err := r.LoadAuthorityRevocation(ctx, requestID, requestVersion, now); err == nil {
		return contracts.AuthorityDecision{}, ErrAuthorityDecisionRevoked
	} else if !errors.Is(err, state.ErrSecureBlobNotFound) && !errors.Is(err, state.ErrSecureBlobExpired) {
		return contracts.AuthorityDecision{}, fmt.Errorf("check authority revocation: %w", err)
	}
	// I12: a decision grants nothing unless its authenticated liveness record
	// is present. Absence of a revocation record is never, on its own,
	// evidence that the decision is still in force: revocation retired the
	// liveness record, so deleting the revocation row cannot restore authority,
	// and deleting the liveness record refuses.
	digest, digestErr := decision.Digest()
	if digestErr != nil {
		return contracts.AuthorityDecision{}, digestErr
	}
	if err := r.requireLive(ctx, state.AuthorityDecisionLiveNamespace, requestID, requestVersion, digest, now); err != nil {
		return contracts.AuthorityDecision{}, fmt.Errorf("%w: %w", ErrAuthorityDecisionNotLive, err)
	}
	return decision, nil
}

// LoadAuthorityDecisionEvidence returns the immutable original decision even
// after revocation, for audit. Callers seeking effective authority must use
// LoadAuthorityDecision, which applies expiry and revocation.
func (r Repository) LoadAuthorityDecisionEvidence(ctx context.Context, requestID, requestVersion string, now time.Time) (contracts.AuthorityDecision, error) {
	stored, err := r.loadAuthorityDecisionRecord(ctx, requestID, requestVersion, now)
	if err != nil {
		return contracts.AuthorityDecision{}, err
	}
	return stored.Decision, nil
}

func (r Repository) loadAuthorityDecisionRecord(ctx context.Context, requestID, requestVersion string, now time.Time) (authorityDecisionRecord, error) {
	payload, record, err := r.loadWorkPlanBlob(ctx, authorityDecisionNamespace, requestID, requestVersion, now)
	if err != nil {
		return authorityDecisionRecord{}, err
	}
	var stored authorityDecisionRecord
	if err := json.Unmarshal(payload, &stored); err != nil {
		return authorityDecisionRecord{}, fmt.Errorf("decode authority decision: %w", err)
	}
	if stored.Request.ID != requestID || stored.Request.Version != requestVersion || payloadDigest(payload) != record.ObjectDigest {
		return authorityDecisionRecord{}, errors.New("authority decision identity or digest mismatch")
	}
	if err := stored.Decision.Validate(stored.Request, now); err != nil {
		return authorityDecisionRecord{}, fmt.Errorf("validate authority decision: %w", err)
	}
	return stored, nil
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
	// A fresh installation's first generation creates its anchor; an existing
	// store without one is refused (a governed re-anchor is required).
	if err := r.InitializeGovernanceAnchor(ctx); err != nil {
		return err
	}
	stamp, err := r.admissionStamp(ctx)
	if err != nil {
		return err
	}
	// I12: the generation's liveness record commits in the same transaction.
	return r.Store.PutAuthorityGeneration(ctx, state.AuthorityGenerationWrite{
		Generation: generation, Crypto: r.Crypto, KeyRef: r.KeyRef,
		Profile: r.Profile, Sensitivity: r.Sensitivity, CreatedAt: createdAt, ExpiresAt: expiresAt,
		MarkLive: true, AdmittedAtSeq: stamp,
	})
}

func (r Repository) SaveRoutingIssuance(ctx context.Context, issuance contracts.RoutingIssuance) (contracts.RoutingIssuance, error) {
	if r.InstallationDigest == "" {
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
	generation, err := r.ValidateAuthorityGenerationLineage(ctx, frozen.GenerationRef, frozen.GenerationVersion, frozen.GenerationDigest, r.InstallationDigest, now)
	if err != nil {
		return contracts.RoutingIssuance{}, err
	}
	// ADR-092: routing issuance requires the adopted v6 model to be active and
	// a v6-labelled delegated child whose closed profile carries exactly the
	// issued authority. The installation root itself never issues routes.
	model, err := r.LoadAuthorityModelState(ctx, now)
	if err != nil {
		return contracts.RoutingIssuance{}, err
	}
	if model.ActiveVersion != contracts.AuthorityModelRoutingVersion || model.ActiveDigest != contracts.AuthorityModelRoutingDigest() {
		return contracts.RoutingIssuance{}, errors.New("routing issuance requires adopted authority model v6")
	}
	if generation.ParentRef == "" || generation.AuthorityModelVersion != contracts.AuthorityModelRoutingVersion || generation.AuthorityModelDigest != contracts.AuthorityModelRoutingDigest() || contracts.RoutingAuthorityForProfile(generation.DelegationProfile) != frozen.Authority || !containsString(generation.Authorities, frozen.Authority) {
		return contracts.RoutingIssuance{}, errors.New("routing issuance generation lacks v6 requested authority")
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
	if r.InstallationDigest == "" {
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
	if _, err := r.ValidateAuthorityGenerationLineage(ctx, issuance.GenerationRef, issuance.GenerationVersion, issuance.GenerationDigest, r.InstallationDigest, now); err != nil {
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
	if request.CeremonyProfile != "" {
		return contracts.AuthorityGeneration{}, errors.New("a ceremony-protected request cannot be decided through the delegation path")
	}
	if (request.RequestedAuthority != contracts.AuthorityDelegateCapability && request.RequestedAuthority != contracts.GovernedPackagePublish && request.RequestedAuthority != contracts.GovernedPackageDeploy) || request.Delegation == nil {
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
	// A retired or superseded parent cannot mint a child: the immutable record
	// says what was enrolled, not that it is still in force (N17).
	if err := r.requireCurrentGeneration(ctx, parent, createdAt); err != nil {
		return contracts.AuthorityGeneration{}, fmt.Errorf("delegation parent is not current: %w", err)
	}
	if parent.Digest != delegation.ParentDigest || decision.AuthorityRef != parent.Ref || decision.AuthorityVersion != parent.Version || decision.AuthorityGenerationDigest != parent.Digest || decision.DecidedBy != parent.Principal {
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
	child := contracts.AuthorityGeneration{Ref: "authority-delegation:" + request.ID, Version: "1", Principal: delegation.DelegatedPrincipal, Scope: delegation.RequestedScope, Authorities: []string{delegation.RequestedAuthority}, DelegationProfile: delegation.Profile, SubjectKind: delegation.SubjectKind, SubjectID: delegation.SubjectID, SubjectVersion: delegation.SubjectVersion, SubjectDigest: delegation.SubjectDigest, SubjectKeyDigest: delegation.SubjectKeyDigest, EffectiveAt: createdAt.UTC(), ExpiresAt: &delegation.ExpiresAt, ParentRef: parent.Ref, ParentVersion: parent.Version, ParentDigest: parent.Digest, DelegatedBy: decision.DecidedBy, DelegationRef: request.ID + "/" + request.Version, DelegationDigest: delegationDigest, PolicyRef: delegation.PolicyRef, PolicyVersion: delegation.PolicyVersion, PolicyDigest: delegation.PolicyDigest, AuthorityModel: contracts.AuthorityModelID, AuthorityModelVersion: delegation.PolicyVersion, AuthorityModelDigest: delegation.PolicyDigest, State: contracts.AuthorityGenerationActive, ProvenanceRef: "authority-decision:" + decision.DecisionRef + ":" + decision.DecisionVersion, ProvenanceDigest: decision.AuthorityDigest}
	child.Digest, err = child.ComputeDigest()
	if err != nil {
		return contracts.AuthorityGeneration{}, fmt.Errorf("derive delegated generation digest: %w", err)
	}
	childStamp, err := r.admissionStamp(ctx)
	if err != nil {
		return contracts.AuthorityGeneration{}, err
	}
	if err := r.Store.PutAuthorityGeneration(ctx, state.AuthorityGenerationWrite{
		Generation: child, Crypto: r.Crypto, KeyRef: r.KeyRef, Profile: r.Profile,
		Sensitivity: r.Sensitivity, CreatedAt: createdAt, ExpiresAt: &delegation.ExpiresAt, MarkLive: true, AdmittedAtSeq: childStamp,
		LockNamespace: authorityRequestNamespace, LockID: request.ID, LockVersion: request.Version,
	}); err != nil {
		return contracts.AuthorityGeneration{}, fmt.Errorf("persist delegated generation: %w", err)
	}
	return child, nil
}

// SaveAuthorityDecisionAndDelegatedAuthorityGeneration commits the decision
// and its exact bounded child as one fenced durable transition. A confirmed
// delegation must never expose only one half of its authoritative lineage.
func (r Repository) SaveAuthorityDecisionAndDelegatedAuthorityGeneration(ctx context.Context, requestID, requestVersion string, decision contracts.AuthorityDecision, policy contracts.DelegationContainmentPolicy, createdAt time.Time) (contracts.AuthorityGeneration, error) {
	if policy == nil {
		return contracts.AuthorityGeneration{}, errors.New("delegation containment policy is required")
	}
	request, err := r.LoadAuthorityRequest(ctx, requestID, requestVersion, createdAt)
	if err != nil {
		return contracts.AuthorityGeneration{}, fmt.Errorf("load delegation request: %w", err)
	}
	if request.CeremonyProfile != "" {
		return contracts.AuthorityGeneration{}, errors.New("a ceremony-protected request cannot be decided through the delegation path")
	}
	if request.Status != contracts.AuthorityRequestPending || request.Delegation == nil {
		return contracts.AuthorityGeneration{}, errors.New("authority request is not pending delegation")
	}
	if err := decision.Validate(request, createdAt); err != nil {
		return contracts.AuthorityGeneration{}, err
	}
	delegation := request.Delegation
	parent, err := r.LoadAuthorityGeneration(ctx, delegation.ParentRef, delegation.ParentVersion, createdAt)
	if err != nil {
		return contracts.AuthorityGeneration{}, fmt.Errorf("load delegation parent: %w", err)
	}
	// A retired or superseded parent cannot mint a child: the immutable record
	// says what was enrolled, not that it is still in force (N17).
	if err := r.requireCurrentGeneration(ctx, parent, createdAt); err != nil {
		return contracts.AuthorityGeneration{}, fmt.Errorf("delegation parent is not current: %w", err)
	}
	if parent.Digest != delegation.ParentDigest || decision.AuthorityRef != parent.Ref || decision.AuthorityVersion != parent.Version || decision.AuthorityGenerationDigest != parent.Digest || decision.DecidedBy != parent.Principal {
		return contracts.AuthorityGeneration{}, errors.New("delegation decision does not bind the exact parent generation")
	}
	if err := policy.ContainDelegation(parent, *delegation, createdAt); err != nil {
		return contracts.AuthorityGeneration{}, fmt.Errorf("delegation containment denied: %w", err)
	}
	delegationDigest, err := request.Digest()
	if err != nil {
		return contracts.AuthorityGeneration{}, err
	}
	child := contracts.AuthorityGeneration{Ref: "authority-delegation:" + request.ID, Version: "1", Principal: delegation.DelegatedPrincipal, Scope: delegation.RequestedScope, Authorities: []string{delegation.RequestedAuthority}, DelegationProfile: delegation.Profile, SubjectKind: delegation.SubjectKind, SubjectID: delegation.SubjectID, SubjectVersion: delegation.SubjectVersion, SubjectDigest: delegation.SubjectDigest, SubjectKeyDigest: delegation.SubjectKeyDigest, EffectiveAt: createdAt.UTC(), ExpiresAt: &delegation.ExpiresAt, ParentRef: parent.Ref, ParentVersion: parent.Version, ParentDigest: parent.Digest, DelegatedBy: decision.DecidedBy, DelegationRef: request.ID + "/" + request.Version, DelegationDigest: delegationDigest, PolicyRef: delegation.PolicyRef, PolicyVersion: delegation.PolicyVersion, PolicyDigest: delegation.PolicyDigest, AuthorityModel: contracts.AuthorityModelID, AuthorityModelVersion: delegation.PolicyVersion, AuthorityModelDigest: delegation.PolicyDigest, State: contracts.AuthorityGenerationActive, ProvenanceRef: "authority-decision:" + decision.DecisionRef + ":" + decision.DecisionVersion, ProvenanceDigest: decision.AuthorityDigest}
	child.Digest, err = child.ComputeDigest()
	if err != nil {
		return contracts.AuthorityGeneration{}, fmt.Errorf("derive delegated generation digest: %w", err)
	}
	if existing, loadErr := r.LoadAuthorityDecisionEvidence(ctx, request.ID, request.Version, createdAt); loadErr == nil {
		left, _ := json.Marshal(existing)
		right, _ := json.Marshal(decision)
		if !bytes.Equal(left, right) {
			return contracts.AuthorityGeneration{}, errors.New("conflicting authority decision already exists")
		}
		current, generationErr := r.LoadAuthorityGeneration(ctx, child.Ref, child.Version, createdAt)
		if generationErr != nil || current.Digest != child.Digest {
			return contracts.AuthorityGeneration{}, errors.New("authority decision exists without its exact delegated generation")
		}
		return current, nil
	} else if !errors.Is(loadErr, state.ErrSecureBlobNotFound) && !errors.Is(loadErr, state.ErrSecureBlobExpired) {
		return contracts.AuthorityGeneration{}, fmt.Errorf("check existing authority decision: %w", loadErr)
	}
	decisionPayload, err := json.Marshal(authorityDecisionRecord{Request: request, Decision: decision})
	if err != nil {
		return contracts.AuthorityGeneration{}, fmt.Errorf("encode authority decision: %w", err)
	}
	decisionRecord, err := r.workPlanSecureRecord(ctx, authorityDecisionNamespace, request.ID, request.Version, decisionPayload, createdAt, &delegation.ExpiresAt)
	if err != nil {
		return contracts.AuthorityGeneration{}, err
	}
	decisionDigest, err := decision.Digest()
	if err != nil {
		return contracts.AuthorityGeneration{}, err
	}
	stamp, err := r.admissionStamp(ctx)
	if err != nil {
		return contracts.AuthorityGeneration{}, err
	}
	decisionLive, err := state.SealedLivenessRecord(ctx, r.Crypto, r.KeyRef, r.Profile, r.Sensitivity, state.AuthorityDecisionLiveNamespace, request.ID, request.Version, decisionDigest, createdAt, stamp)
	if err != nil {
		return contracts.AuthorityGeneration{}, err
	}
	if err := r.Store.PutAuthorityGeneration(ctx, state.AuthorityGenerationWrite{
		Generation: child, Crypto: r.Crypto, KeyRef: r.KeyRef, Profile: r.Profile,
		Sensitivity: r.Sensitivity, CreatedAt: createdAt, ExpiresAt: &delegation.ExpiresAt, MarkLive: true, AdmittedAtSeq: stamp,
		RelatedRecords: []state.SecureBlobRecord{decisionRecord, decisionLive},
		LockNamespace:  authorityRequestNamespace, LockID: request.ID, LockVersion: request.Version,
	}); err != nil {
		return contracts.AuthorityGeneration{}, fmt.Errorf("persist delegation decision and generation: %w", err)
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
	generations, _, _, err := r.listAuthorityGenerationsSnapshot(ctx, now)
	return generations, err
}

// listAuthorityGenerationsSnapshot returns every durable generation together
// with the set of generation identities that carry an invalidation record,
// both read from one statement so a concurrent succession commit can never
// be observed as "generation present, invalidation present, successor
// absent". Keys are ref + "@" + version. The generation liveness records
// (I12) are part of the same statement for the same reason: a succession
// retires the predecessor's and admits the successor's liveness record in the
// transaction that supersedes it, so a reader must never pair one snapshot of
// the generations with a later read of their liveness. live maps a generation
// to the digest its authenticated liveness record names.
func (r Repository) listAuthorityGenerationsSnapshot(ctx context.Context, now time.Time) ([]contracts.AuthorityGeneration, map[string]bool, map[string]livenessRecord, error) {
	if err := r.validateWorkPlanStore(); err != nil {
		return nil, nil, nil, err
	}
	all, err := r.Store.ListSecureBlobsInNamespaces(ctx, []string{authorityGenerationNamespace, authorityGenerationInvalidationNamespace, state.AuthorityGenerationLiveNamespace}, now)
	if err != nil {
		return nil, nil, nil, err
	}
	return r.decodeGenerationSnapshot(ctx, all)
}

// decodeGenerationSnapshot authenticates and decodes one statement's worth of
// generation, invalidation and liveness rows.
func (r Repository) decodeGenerationSnapshot(ctx context.Context, all []state.SecureBlobRecord) ([]contracts.AuthorityGeneration, map[string]bool, map[string]livenessRecord, error) {
	invalidated := map[string]bool{}
	live := map[string]livenessRecord{}
	records := make([]state.SecureBlobRecord, 0, len(all))
	for _, record := range all {
		switch record.Namespace {
		case authorityGenerationInvalidationNamespace:
			invalidated[record.ObjectID+"@"+record.ObjectVersion] = true
			continue
		case state.AuthorityGenerationLiveNamespace:
			payload, err := r.Crypto.Open(ctx, record.Envelope, state.SecureBlobAAD(record.Namespace, record.ObjectID, record.ObjectVersion, record.ObjectDigest))
			if err != nil {
				return nil, nil, nil, fmt.Errorf("decrypt generation liveness %s/%s: %w", record.ObjectID, record.ObjectVersion, err)
			}
			var stored livenessRecord
			if err := json.Unmarshal(payload, &stored); err != nil || stored.Namespace != record.Namespace || stored.ID != record.ObjectID || stored.Version != record.ObjectVersion || stored.Digest == "" {
				return nil, nil, nil, fmt.Errorf("generation liveness %s/%s is inconsistent", record.ObjectID, record.ObjectVersion)
			}
			live[record.ObjectID+"@"+record.ObjectVersion] = stored
			continue
		}
		records = append(records, record)
	}
	generations := make([]contracts.AuthorityGeneration, 0, len(records))
	for _, record := range records {
		payload, err := r.Crypto.Open(ctx, record.Envelope, state.SecureBlobAAD(record.Namespace, record.ObjectID, record.ObjectVersion, record.ObjectDigest))
		if err != nil {
			return nil, nil, nil, fmt.Errorf("decrypt authority generation %s/%s: %w", record.ObjectID, record.ObjectVersion, err)
		}
		var generation contracts.AuthorityGeneration
		if err := json.Unmarshal(payload, &generation); err != nil {
			return nil, nil, nil, fmt.Errorf("decode authority generation %s/%s: %w", record.ObjectID, record.ObjectVersion, err)
		}
		if generation.Ref != record.ObjectID || generation.Version != record.ObjectVersion || payloadDigest(payload) != record.ObjectDigest {
			return nil, nil, nil, errors.New("authority generation identity or digest mismatch")
		}
		if err := generation.Validate(); err != nil {
			return nil, nil, nil, err
		}
		if err := generation.VerifyDigest(); err != nil {
			return nil, nil, nil, err
		}
		generations = append(generations, generation)
	}
	return generations, invalidated, live, nil
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
	// I13: anchor the retirement first, then retire the liveness record (I12).
	if err := r.anchorGenerationRetired(ctx, generation.Ref, generation.Version, generation.Digest, invalidation.Kind, createdAt); err != nil {
		return err
	}
	if err := r.retireLive(ctx, state.AuthorityGenerationLiveNamespace, invalidation.Ref, invalidation.Version); err != nil {
		return err
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
	return r.requireCurrentGeneration(ctx, generation, now)
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

func (r Repository) workPlanSecureRecord(ctx context.Context, namespace, objectID, version string, payload []byte, createdAt time.Time, expiresAt *time.Time) (state.SecureBlobRecord, error) {
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	digest := payloadDigest(payload)
	aad := state.SecureBlobAAD(namespace, objectID, version, digest)
	envelope, err := r.Crypto.Seal(ctx, r.KeyRef, r.Profile, payload, aad)
	if err != nil {
		return state.SecureBlobRecord{}, fmt.Errorf("encrypt governed WorkPlan record: %w", err)
	}
	return state.SecureBlobRecord{Namespace: namespace, ObjectID: objectID, ObjectVersion: version, ObjectDigest: digest, Sensitivity: r.Sensitivity, CryptoProfile: r.Profile, Envelope: envelope, CreatedAt: createdAt, ExpiresAt: expiresAt}, nil
}

func (r Repository) putWorkPlanBlobUnlessRevoked(ctx context.Context, namespace, objectID, version string, payload []byte, createdAt time.Time, expiresAt *time.Time, requestID, requestVersion, additionalID, additionalVersion, lockNamespace, lockID, lockVersion string, live ...state.SecureBlobRecord) error {
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
	return r.Store.PutSecureBlobUnlessRevoked(ctx, record, authorityRevocationNamespace, requestID, requestVersion, authorityGenerationInvalidationNamespace, additionalID, additionalVersion, lockNamespace, lockID, lockVersion, live...)
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

// ValidateAuthorityGenerationLineage walks a generation's parent chain and
// proves every edge: each generation is exact, active, and not invalidated;
// each delegated edge binds its parent's decision and request and satisfies
// the closed built-in containment rule for its profile; and the chain
// terminates at the exact current canonical installation root resolved by
// LoadCurrentInstallationRoot. No model version or root label substitutes
// for that termination (ADR-092 supersedes the parallel v2 root migration).
func (r Repository) ValidateAuthorityGenerationLineage(ctx context.Context, ref, version, digest, bootstrapDigest string, now time.Time) (contracts.AuthorityGeneration, error) {
	root, err := r.LoadCurrentInstallationRoot(ctx, bootstrapDigest, now)
	if err != nil {
		return contracts.AuthorityGeneration{}, err
	}
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
		if err := r.requireLive(ctx, state.AuthorityGenerationLiveNamespace, generation.Ref, generation.Version, generation.Digest, now); err != nil {
			return generation, err
		}
		if generation.ParentRef == "" {
			if generation.Ref != root.Ref || generation.Version != root.Version || generation.Digest != root.Digest {
				return generation, errors.New("authority lineage does not terminate at the current installation root")
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
		if decision.AuthorityRef != parent.Ref || decision.AuthorityVersion != parent.Version || decision.AuthorityGenerationDigest != parent.Digest || decision.DecidedBy != parent.Principal || generation.DelegatedBy != parent.Principal {
			return generation, errors.New("delegated generation does not bind its parent decision")
		}
		if err := containBuiltinDelegation(parent, *request.Delegation, now); err != nil {
			return generation, err
		}
		if request.Delegation.DelegatedPrincipal != generation.Principal || request.Delegation.RequestedAuthority != firstAuthority(generation.Authorities) || request.Delegation.RequestedScope != generation.Scope || request.Delegation.Profile != generation.DelegationProfile {
			return generation, errors.New("delegated generation exceeds its exact request")
		}
		return generation, nil
	}
	return walk(ref, version, digest)
}

// containBuiltinDelegation dispatches the closed built-in containment rule
// by delegation profile. It has no fallback beyond the v1 WorkPlan rule.
func containBuiltinDelegation(parent contracts.AuthorityGeneration, request contracts.DelegationRequest, now time.Time) error {
	switch request.Profile {
	case contracts.DelegationProfilePackagePublish:
		return contracts.ValidateBuiltinPackagePublishDelegation(parent, request, now)
	case contracts.DelegationProfilePackageDeploy:
		return contracts.ValidateBuiltinPackageDeployDelegation(parent, request, now)
	case contracts.DelegationProfileRoutingTargetContribution, contracts.DelegationProfileRoutingSurfaceEligibility:
		return contracts.ValidateBuiltinRoutingDelegation(parent, request, now)
	}
	return contracts.ValidateBuiltinDelegation(parent, request, now)
}

func firstAuthority(values []string) string {
	if len(values) == 1 {
		return values[0]
	}
	return ""
}

// SaveReplanningSuccessor creates the successor immutable generation of a
// settled Goal generation for governed succession (ADR-099, #160): the same
// Goal contract, the predecessor digest, no WorkPlan (a successor WorkPlan
// is proposed, reviewed, accepted, and attached through the ordinary
// lifecycle), and evidence references binding the predecessor's completion
// state. The predecessor is never rewritten.
func (r Repository) SaveReplanningSuccessor(ctx context.Context, sourceID, sourceVersion, sourceDigest, successorVersion string, evidenceRefs []string, createdAt time.Time) (goals.GoalBaseline, error) {
	source, err := r.Load(ctx, sourceID, sourceVersion, createdAt)
	if err != nil {
		return goals.GoalBaseline{}, fmt.Errorf("load source Goal Baseline: %w", err)
	}
	if source.Digest != sourceDigest {
		return goals.GoalBaseline{}, goals.ErrBaselineDigestMismatch
	}
	if successorVersion == "" || successorVersion == source.Version {
		return goals.GoalBaseline{}, errors.New("successor Goal Baseline version must be distinct")
	}
	if _, err := r.Load(ctx, sourceID, successorVersion, createdAt); err == nil {
		return goals.GoalBaseline{}, fmt.Errorf("Goal generation %s/%s already exists", sourceID, successorVersion)
	}
	successor := source
	successor.Version = successorVersion
	successor.Digest = ""
	successor.PredecessorDigest = source.Digest
	successor.WorkPlan = nil
	successor.ImportSourceRef, successor.ImportSourceDigest = "", ""
	successor.EvidenceRefs = append(append([]string(nil), source.EvidenceRefs...), evidenceRefs...)
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
	record, err := r.goalBaselineRecord(ctx, successor, payload, digest, createdAt, nil)
	if err != nil {
		return goals.GoalBaseline{}, err
	}
	if err := r.Store.PutSecureBlob(ctx, record); err != nil {
		return goals.GoalBaseline{}, fmt.Errorf("persist successor Goal Baseline: %w", err)
	}
	return successor, nil
}

// putWorkPlanBlobsAtomically writes one sealed record together with related
// (liveness) records in a single transaction.
func (r Repository) putWorkPlanBlobsAtomically(ctx context.Context, namespace, objectID, version string, payload []byte, createdAt time.Time, expiresAt *time.Time, related ...state.SecureBlobRecord) error {
	record, err := r.workPlanSecureRecord(ctx, namespace, objectID, version, payload, createdAt, expiresAt)
	if err != nil {
		return err
	}
	return r.Store.PutSecureBlobsAtomically(ctx, append([]state.SecureBlobRecord{record}, related...))
}

package goalstore

import (
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

type Repository struct {
	Store       *state.Store
	Crypto      praxiscrypto.EnvelopeService
	KeyRef      string
	Profile     contracts.CryptoProfile
	Sensitivity state.Sensitivity
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
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	aad := state.SecureBlobAAD(baselineNamespace, baseline.ID, baseline.Version, digest)
	envelope, err := r.Crypto.Seal(ctx, r.KeyRef, r.Profile, payload, aad)
	if err != nil {
		return goals.GoalBaseline{}, fmt.Errorf("encrypt Goal Baseline: %w", err)
	}
	record := state.SecureBlobRecord{Namespace: baselineNamespace, ObjectID: baseline.ID, ObjectVersion: baseline.Version, ObjectDigest: digest, Sensitivity: r.Sensitivity, CryptoProfile: r.Profile, Envelope: envelope, CreatedAt: createdAt, ExpiresAt: expiresAt}
	if err := r.Store.PutSecureBlob(ctx, record); err != nil {
		return goals.GoalBaseline{}, fmt.Errorf("persist Goal Baseline: %w", err)
	}
	return baseline, nil
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
	Proposal contracts.WorkPlanProposal   `json:"proposal"`
	Decision contracts.WorkPlanAcceptance `json:"decision"`
	Plan     contracts.WorkPlan           `json:"plan"`
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
	plan, err := contracts.AcceptWorkPlan(proposal, accepted, decision)
	if err != nil {
		return contracts.WorkPlan{}, err
	}
	if recordVersion == "" {
		return contracts.WorkPlan{}, errors.New("work plan acceptance version is required")
	}
	record := acceptedWorkPlanRecord{Proposal: proposal, Decision: decision, Plan: plan}
	payload, err := json.Marshal(record)
	if err != nil {
		return contracts.WorkPlan{}, fmt.Errorf("encode accepted WorkPlan: %w", err)
	}
	if err := r.putWorkPlanBlob(ctx, workPlanAcceptanceNamespace, decision.AcceptanceRef, recordVersion, payload, createdAt, expiresAt); err != nil {
		return contracts.WorkPlan{}, fmt.Errorf("persist accepted WorkPlan: %w", err)
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
	plan, err := contracts.AcceptWorkPlan(stored.Proposal, stored.Plan, stored.Decision)
	if err != nil {
		return contracts.WorkPlan{}, fmt.Errorf("validate accepted WorkPlan: %w", err)
	}
	return plan, nil
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

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

func sessionDigest(payload []byte) string {
	digest := sha256.Sum256(payload)
	return fmt.Sprintf("sha256:%x", digest[:])
}

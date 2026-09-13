package goalstore

import (
	"context"
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

type Repository struct {
	Store       *state.Store
	Crypto      praxiscrypto.EnvelopeService
	KeyRef      string
	Profile     contracts.CryptoProfile
	Sensitivity state.Sensitivity
}

func (r Repository) Save(ctx context.Context, baseline goals.GoalBaseline, createdAt time.Time, expiresAt *time.Time) (goals.GoalBaseline, error) {
	if r.Store == nil { return goals.GoalBaseline{}, errors.New("goal baseline store is required") }
	if r.KeyRef == "" { return goals.GoalBaseline{}, errors.New("goal baseline key reference is required") }
	if err := r.Profile.Validate(); err != nil { return goals.GoalBaseline{}, err }
	if err := r.Sensitivity.Validate(); err != nil { return goals.GoalBaseline{}, err }
	if err := baseline.Validate(); err != nil { return goals.GoalBaseline{}, err }
	digest, err := baseline.ComputeDigest()
	if err != nil { return goals.GoalBaseline{}, err }
	if baseline.Digest != "" && baseline.Digest != digest { return goals.GoalBaseline{}, goals.ErrBaselineDigestMismatch }
	baseline.Digest = digest
	payload, err := json.Marshal(baseline)
	if err != nil { return goals.GoalBaseline{}, fmt.Errorf("encode Goal Baseline: %w", err) }
	if createdAt.IsZero() { createdAt = time.Now().UTC() }
	aad := state.SecureBlobAAD(baselineNamespace, baseline.ID, baseline.Version, digest)
	envelope, err := r.Crypto.Seal(ctx, r.KeyRef, r.Profile, payload, aad)
	if err != nil { return goals.GoalBaseline{}, fmt.Errorf("encrypt Goal Baseline: %w", err) }
	record := state.SecureBlobRecord{Namespace:baselineNamespace,ObjectID:baseline.ID,ObjectVersion:baseline.Version,ObjectDigest:digest,Sensitivity:r.Sensitivity,CryptoProfile:r.Profile,Envelope:envelope,CreatedAt:createdAt,ExpiresAt:expiresAt}
	if err := r.Store.PutSecureBlob(ctx, record); err != nil { return goals.GoalBaseline{}, fmt.Errorf("persist Goal Baseline: %w", err) }
	return baseline,nil
}

func (r Repository) Load(ctx context.Context, id, version string, now time.Time) (goals.GoalBaseline, error) {
	if r.Store == nil { return goals.GoalBaseline{}, errors.New("goal baseline store is required") }
	record, err := r.Store.GetSecureBlob(ctx, baselineNamespace, id, version, now)
	if err != nil { return goals.GoalBaseline{}, err }
	aad := state.SecureBlobAAD(baselineNamespace, id, version, record.ObjectDigest)
	payload, err := r.Crypto.Open(ctx, record.Envelope, aad)
	if err != nil { return goals.GoalBaseline{}, fmt.Errorf("decrypt Goal Baseline: %w", err) }
	var baseline goals.GoalBaseline
	if err := json.Unmarshal(payload, &baseline); err != nil { return goals.GoalBaseline{}, fmt.Errorf("decode Goal Baseline: %w", err) }
	if baseline.ID != id || baseline.Version != version || baseline.Digest != record.ObjectDigest {
		return goals.GoalBaseline{}, errors.New("Goal Baseline identity/digest differs from secure record")
	}
	if err := baseline.VerifyDigest(); err != nil { return goals.GoalBaseline{}, err }
	return baseline,nil
}

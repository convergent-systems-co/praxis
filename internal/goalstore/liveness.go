package goalstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/convergent-systems-co/praxis/internal/state"
)

// ErrLivenessMissing is returned when a governance fact that must be positively
// live has no matching authenticated liveness record: it was retired by a
// revocation/invalidation, or the governing state was lost or altered. Missing
// governance state is never read as "still granted" (I12).
var ErrLivenessMissing = errors.New("governance liveness record is missing or does not match")

// ErrAuthorityDecisionNotLive is the refusal for a protected decision without
// its live authorization record. It is deliberately also an
// ErrAuthorityDecisionRevoked: every caller that refuses a revoked decision
// refuses this, whether the decision was revoked or its liveness was lost.
var ErrAuthorityDecisionNotLive = fmt.Errorf("authority decision has no live authorization record (revoked, or governing state was lost): %w", ErrAuthorityDecisionRevoked)

type livenessRecord = state.LivenessRecord

// requireLive proves the sealed positive record exists, names exactly this
// digest, is not retired by an anchored governance fact and was not admitted
// before the latest governed re-anchor. Any other outcome is a refusal, and a
// store that is not current against the forward authority anchor is refused
// before the record is even considered (I13).
func (r Repository) requireLive(ctx context.Context, namespace, id, version, digest string, now time.Time) error {
	snap, err := r.governanceSnapshot(ctx)
	if err != nil {
		return err
	}
	stored, err := r.loadLiveness(ctx, namespace, id, version, now)
	if err != nil {
		return err
	}
	if stored.Namespace != namespace || stored.ID != id || stored.Version != version || stored.Digest != digest {
		return fmt.Errorf("%w: %s %s/%s names another digest", ErrLivenessMissing, namespace, id, version)
	}
	return r.currentAgainstFacts(snap, stored)
}

func (r Repository) currentAgainstFacts(snap governanceSnapshot, stored livenessRecord) error {
	if snap.retired(stored.Namespace, stored.ID, stored.Version, stored.Digest) {
		return fmt.Errorf("%w: %s %s/%s", ErrAuthorityRetired, retiredKind(stored.Namespace), stored.ID, stored.Version)
	}
	if snap.admissionVoid(stored.AdmittedAtSeq) {
		return fmt.Errorf("%w: %s %s/%s was admitted before the latest governed re-anchor and must be re-established", ErrLivenessMissing, retiredKind(stored.Namespace), stored.ID, stored.Version)
	}
	return nil
}

func (r Repository) loadLiveness(ctx context.Context, namespace, id, version string, now time.Time) (livenessRecord, error) {
	payload, _, err := r.loadWorkPlanBlob(ctx, namespace, id, version, now)
	if err != nil {
		if errors.Is(err, state.ErrSecureBlobNotFound) {
			return livenessRecord{}, fmt.Errorf("%w: %s %s/%s", ErrLivenessMissing, namespace, id, version)
		}
		return livenessRecord{}, fmt.Errorf("%w: %v", ErrLivenessMissing, err)
	}
	var stored livenessRecord
	if err := json.Unmarshal(payload, &stored); err != nil {
		return livenessRecord{}, fmt.Errorf("%w: %v", ErrLivenessMissing, err)
	}
	return stored, nil
}

// retireLive deletes the positive record. The retirement is anchored as a fact
// BEFORE this is called (write-ahead), so a crash leaves a retired-by-fact
// authority whose liveness row is merely still present, and a retry completes it.
func (r Repository) retireLive(ctx context.Context, namespace, id, version string) error {
	if err := r.validateWorkPlanStore(); err != nil {
		return err
	}
	return r.Store.DeleteLivenessRecord(ctx, namespace, id, version)
}

// requireLiveIdentity is requireLive for a caller that holds no digest: the
// sealed record must exist, name exactly this identity and be current.
func (r Repository) requireLiveIdentity(ctx context.Context, namespace, id, version string, now time.Time) error {
	snap, err := r.governanceSnapshot(ctx)
	if err != nil {
		return err
	}
	stored, err := r.loadLiveness(ctx, namespace, id, version, now)
	if err != nil {
		return err
	}
	if stored.Namespace != namespace || stored.ID != id || stored.Version != version || stored.Digest == "" {
		return fmt.Errorf("%w: %s %s/%s names another identity", ErrLivenessMissing, namespace, id, version)
	}
	return r.currentAgainstFacts(snap, stored)
}

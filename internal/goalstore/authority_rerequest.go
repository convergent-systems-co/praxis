package goalstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// SaveAuthorityReRequest is the generic durable transition for requesting
// fresh authority over an unchanged ActionIntent. It prepares exactly one
// deterministic successor request and delegates persistence to the existing
// immutable AuthorityRequest store. It does not decide, delegate, or execute.
func (r Repository) SaveAuthorityReRequest(ctx context.Context, prior contracts.AuthorityRequest, priorDecision contracts.AuthorityDecision, priorGeneration contracts.AuthorityGeneration, eligibility contracts.AuthorityReRequestEligibility) (contracts.AuthorityRequest, string, error) {
	if err := r.verifyHistoricalReRequestPredecessor(ctx, prior, priorDecision, priorGeneration, eligibility.Now.UTC()); err != nil {
		return contracts.AuthorityRequest{}, "", err
	}
	fresh, err := contracts.BuildAuthorityReRequest(prior, priorDecision, priorGeneration, eligibility)
	if err != nil {
		return contracts.AuthorityRequest{}, "", err
	}
	digest, err := fresh.DigestAt(eligibility.Now.UTC())
	if err != nil {
		return contracts.AuthorityRequest{}, "", err
	}
	expires := fresh.Delegation.ExpiresAt
	if _, err := r.SaveAuthorityRequest(ctx, fresh, eligibility.Now.UTC(), &expires); err == nil {
		return fresh, digest, nil
	}
	// A concurrent or repeated invocation may have won the immutable insert.
	// Read back the deterministic identity and converge only if bytes match.
	existing, loadErr := r.LoadAuthorityRequest(ctx, fresh.ID, fresh.Version, eligibility.Now.UTC())
	if loadErr == nil {
		left, _ := json.Marshal(existing)
		right, _ := json.Marshal(fresh)
		if string(left) == string(right) {
			return existing, digest, nil
		}
		return contracts.AuthorityRequest{}, "", errors.New("conflicting authority re-request already exists")
	}
	return contracts.AuthorityRequest{}, "", fmt.Errorf("persist authority re-request: %w", err)
}

// verifyHistoricalReRequestPredecessor prevents callers from manufacturing a
// predecessor tuple in memory. Every input must be byte-equivalent to the
// immutable encrypted records, including records whose operational expiry has
// passed.
func (r Repository) verifyHistoricalReRequestPredecessor(ctx context.Context, prior contracts.AuthorityRequest, decision contracts.AuthorityDecision, generation contracts.AuthorityGeneration, now time.Time) error {
	if now.IsZero() {
		return errors.New("re-request verification time is required")
	}
	request, requestRecord, err := r.loadHistoricalAuthorityRequest(ctx, prior.ID, prior.Version)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(request, prior) {
		return errors.New("re-request predecessor request differs from durable historical evidence")
	}
	storedDecision, _, err := r.loadHistoricalDecision(ctx, prior.ID, prior.Version, request, requestRecord.CreatedAt)
	if err != nil {
		return fmt.Errorf("load historical re-request decision: %w", err)
	}
	if !reflect.DeepEqual(storedDecision, decision) {
		return errors.New("re-request predecessor decision differs from durable historical evidence")
	}
	storedGeneration, err := r.loadHistoricalGeneration(ctx, generation.Ref, generation.Version, generation.Digest)
	if err != nil {
		return fmt.Errorf("load historical re-request generation: %w", err)
	}
	if !reflect.DeepEqual(storedGeneration, generation) {
		return errors.New("re-request predecessor generation differs from durable historical evidence")
	}
	if prior.Delegation == nil {
		return errors.New("re-request predecessor delegation is required")
	}
	if _, err := r.loadHistoricalGeneration(ctx, prior.Delegation.ParentRef, prior.Delegation.ParentVersion, prior.Delegation.ParentDigest); err != nil {
		return fmt.Errorf("load historical re-request parent generation: %w", err)
	}
	if record, err := r.Store.GetSecureBlobHistorical(ctx, authorityRevocationNamespace, prior.ID, prior.Version); err == nil {
		payload, openErr := r.Crypto.Open(ctx, record.Envelope, state.SecureBlobAAD(record.Namespace, record.ObjectID, record.ObjectVersion, record.ObjectDigest))
		if openErr != nil {
			return fmt.Errorf("open historical re-request revocation: %w", openErr)
		}
		var stored authorityRevocationRecord
		if err := json.Unmarshal(payload, &stored); err != nil {
			return fmt.Errorf("decode historical re-request revocation: %w", err)
		}
		if err := stored.Revocation.Validate(stored.Decision); err != nil {
			return fmt.Errorf("validate historical re-request revocation: %w", err)
		}
		if !now.Before(stored.Revocation.EffectiveAt) {
			return errors.New("revoked authority is not eligible for ordinary re-request")
		}
	} else if !errors.Is(err, state.ErrSecureBlobNotFound) && !errors.Is(err, state.ErrSecureBlobExpired) {
		return fmt.Errorf("check historical re-request revocation: %w", err)
	}
	// I12: a missing revocation record does not make a revoked decision merely
	// expired. Only a decision whose liveness record survives (it expired; it
	// was never retired) is eligible for an ordinary re-request.
	if err := r.requireLiveIdentity(ctx, state.AuthorityDecisionLiveNamespace, prior.ID, prior.Version, time.Unix(0, 0).UTC()); err != nil {
		return fmt.Errorf("re-request predecessor decision is not live: %w", err)
	}
	return nil
}

func (r Repository) loadHistoricalAuthorityRequest(ctx context.Context, id, version string) (contracts.AuthorityRequest, state.SecureBlobRecord, error) {
	record, err := r.Store.GetSecureBlobHistorical(ctx, authorityRequestNamespace, id, version)
	if err != nil {
		return contracts.AuthorityRequest{}, state.SecureBlobRecord{}, err
	}
	payload, err := r.Crypto.Open(ctx, record.Envelope, state.SecureBlobAAD(record.Namespace, record.ObjectID, record.ObjectVersion, record.ObjectDigest))
	if err != nil {
		return contracts.AuthorityRequest{}, state.SecureBlobRecord{}, err
	}
	var request contracts.AuthorityRequest
	if err := json.Unmarshal(payload, &request); err != nil {
		return contracts.AuthorityRequest{}, state.SecureBlobRecord{}, err
	}
	if request.ID != id || request.Version != version || payloadDigest(payload) != record.ObjectDigest {
		return contracts.AuthorityRequest{}, state.SecureBlobRecord{}, errors.New("historical authority request identity or digest mismatch")
	}
	if _, err := request.DigestAt(record.CreatedAt); err != nil {
		return contracts.AuthorityRequest{}, state.SecureBlobRecord{}, err
	}
	return request, record, nil
}

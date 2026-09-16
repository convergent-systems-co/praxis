package goalstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// LoadExpiredHistoricalAuthorityEvidence is the sole recovery-facing loader
// for an expired authority request. It never returns an executable authority.
func (r Repository) LoadExpiredHistoricalAuthorityEvidence(ctx context.Context, requestID, version, expectedDigest, installationDigest, executionID string, effectIDs []string, now time.Time) (contracts.ExpiredHistoricalAuthorityEvidence, error) {
	if err := r.validateWorkPlanStore(); err != nil {
		return contracts.ExpiredHistoricalAuthorityEvidence{}, err
	}
	if requestID == "" || version == "" || executionID == "" || len(effectIDs) == 0 {
		return contracts.ExpiredHistoricalAuthorityEvidence{}, errors.New("historical authority lookup identity is required")
	}
	if err := contracts.ValidateSHA256Digest(expectedDigest); err != nil {
		return contracts.ExpiredHistoricalAuthorityEvidence{}, err
	}
	if err := contracts.ValidateSHA256Digest(installationDigest); err != nil {
		return contracts.ExpiredHistoricalAuthorityEvidence{}, err
	}
	record, err := r.Store.GetSecureBlobHistorical(ctx, authorityRequestNamespace, requestID, version)
	if err != nil {
		return contracts.ExpiredHistoricalAuthorityEvidence{}, err
	}
	if record.ObjectDigest != expectedDigest || record.ExpiresAt == nil || now.Before(*record.ExpiresAt) {
		return contracts.ExpiredHistoricalAuthorityEvidence{}, errors.New("historical authority is not expired or exact")
	}
	payload, err := r.Crypto.Open(ctx, record.Envelope, state.SecureBlobAAD(record.Namespace, record.ObjectID, record.ObjectVersion, record.ObjectDigest))
	if err != nil {
		return contracts.ExpiredHistoricalAuthorityEvidence{}, fmt.Errorf("open historical authority request: %w", err)
	}
	var request contracts.AuthorityRequest
	if err := json.Unmarshal(payload, &request); err != nil {
		return contracts.ExpiredHistoricalAuthorityEvidence{}, fmt.Errorf("decode historical authority request: %w", err)
	}
	if request.ID != requestID || request.Version != version || request.InstallationDigest != installationDigest {
		return contracts.ExpiredHistoricalAuthorityEvidence{}, errors.New("historical authority request identity or installation mismatch")
	}
	requestDigest, err := request.DigestAt(record.CreatedAt)
	if err != nil || requestDigest != expectedDigest {
		return contracts.ExpiredHistoricalAuthorityEvidence{}, errors.New("historical authority request digest mismatch")
	}
	if request.Intent == nil {
		return contracts.ExpiredHistoricalAuthorityEvidence{}, errors.New("historical authority request has no intent")
	}
	intentDigest, err := request.Intent.Digest()
	if err != nil {
		return contracts.ExpiredHistoricalAuthorityEvidence{}, err
	}
	decision, decisionDigest, err := r.loadHistoricalDecision(ctx, requestID, version, request, record.CreatedAt)
	if err != nil {
		return contracts.ExpiredHistoricalAuthorityEvidence{}, err
	}
	gen, err := r.loadHistoricalGeneration(ctx, decision.AuthorityRef, decision.AuthorityVersion, decision.AuthorityGenerationDigest)
	if err != nil {
		return contracts.ExpiredHistoricalAuthorityEvidence{}, err
	}
	if gen.Principal != decision.DecidedBy || gen.Scope != decision.GrantedScope || gen.ExpiresAt == nil || now.Before(*gen.ExpiresAt) {
		return contracts.ExpiredHistoricalAuthorityEvidence{}, errors.New("historical authority generation is not exact and expired")
	}
	if request.Delegation == nil || request.Delegation.ParentDigest != gen.ParentDigest && gen.ParentDigest != "" {
		return contracts.ExpiredHistoricalAuthorityEvidence{}, errors.New("historical delegation lineage mismatch")
	}
	for _, id := range effectIDs {
		var effectPayload []byte
		var created string
		if err := r.Store.DB().QueryRowContext(ctx, `SELECT request_payload,created_at FROM effects WHERE effect_id=?`, id).Scan(&effectPayload, &created); err != nil {
			return contracts.ExpiredHistoricalAuthorityEvidence{}, fmt.Errorf("historical effect %s: %w", id, err)
		}
		at, err := time.Parse(time.RFC3339Nano, created)
		if err != nil || at.Before(gen.EffectiveAt) || !at.Before(*gen.ExpiresAt) {
			return contracts.ExpiredHistoricalAuthorityEvidence{}, errors.New("historical authority was not valid when effect occurred")
		}
		var effect struct {
			RequestID string                                `json:"request_id"`
			Authority contracts.PackagePublishAuthorization `json:"authority"`
		}
		if err := json.Unmarshal(effectPayload, &effect); err != nil || effect.RequestID != requestID || effect.Authority.Generation.Digest != gen.Digest || effect.Authority.Request.ID != requestID || effect.Authority.Request.Version != version {
			return contracts.ExpiredHistoricalAuthorityEvidence{}, errors.New("historical effect is not governed by exact authority")
		}
	}
	delegationDigest := ""
	if request.Delegation != nil {
		delegationDigest, _ = request.DigestAt(record.CreatedAt)
	}
	e := contracts.ExpiredHistoricalAuthorityEvidence{ObjectKind: "authority request", ObjectID: requestID, ObjectVersion: version, ObjectDigest: record.ObjectDigest, InstallationDigest: installationDigest, Principal: gen.Principal, RequestDigest: expectedDigest, IntentID: request.Intent.ID, IntentDigest: intentDigest, DecisionRef: decision.DecisionRef, DecisionVersion: decision.DecisionVersion, DecisionDigest: decisionDigest, DelegationRef: gen.DelegationRef, DelegationDigest: delegationDigest, GenerationRef: gen.Ref, GenerationVersion: gen.Version, GenerationDigest: gen.Digest, ExecutionID: executionID, EffectIDs: append([]string(nil), effectIDs...), EffectiveAt: gen.EffectiveAt, ExpiresAt: *gen.ExpiresAt, HistoricalValidityEstablished: true, Historical: true, NonExecutable: true, Status: contracts.HistoricalAuthorityExpired}
	e.Digest, err = e.ComputeDigest()
	if err != nil {
		return contracts.ExpiredHistoricalAuthorityEvidence{}, err
	}
	if err := e.Validate(); err != nil {
		return contracts.ExpiredHistoricalAuthorityEvidence{}, err
	}
	return e, nil
}

func (r Repository) loadHistoricalDecision(ctx context.Context, requestID, version string, request contracts.AuthorityRequest, at time.Time) (contracts.AuthorityDecision, string, error) {
	record, err := r.Store.GetSecureBlobHistorical(ctx, authorityDecisionNamespace, requestID, version)
	if err != nil {
		return contracts.AuthorityDecision{}, "", err
	}
	payload, err := r.Crypto.Open(ctx, record.Envelope, state.SecureBlobAAD(record.Namespace, record.ObjectID, record.ObjectVersion, record.ObjectDigest))
	if err != nil {
		return contracts.AuthorityDecision{}, "", err
	}
	var stored authorityDecisionRecord
	if err := json.Unmarshal(payload, &stored); err != nil {
		return contracts.AuthorityDecision{}, "", err
	}
	if stored.Request.ID != requestID || stored.Request.Version != version || stored.Request.IntentDigest != request.IntentDigest || payloadDigest(payload) != record.ObjectDigest {
		return contracts.AuthorityDecision{}, "", errors.New("historical decision request mismatch")
	}
	if err := stored.Decision.Validate(stored.Request, at); err != nil {
		return contracts.AuthorityDecision{}, "", fmt.Errorf("validate historical decision: %w", err)
	}
	dd, err := stored.Decision.Digest()
	if err != nil {
		return contracts.AuthorityDecision{}, "", err
	}
	return stored.Decision, dd, nil
}

func (r Repository) loadHistoricalGeneration(ctx context.Context, ref, version, expectedDigest string) (contracts.AuthorityGeneration, error) {
	record, err := r.Store.GetSecureBlobHistorical(ctx, authorityGenerationNamespace, ref, version)
	if err != nil {
		return contracts.AuthorityGeneration{}, err
	}
	if record.ObjectDigest != expectedDigest {
		return contracts.AuthorityGeneration{}, errors.New("historical generation digest mismatch")
	}
	payload, err := r.Crypto.Open(ctx, record.Envelope, state.SecureBlobAAD(record.Namespace, record.ObjectID, record.ObjectVersion, record.ObjectDigest))
	if err != nil {
		return contracts.AuthorityGeneration{}, err
	}
	var generation contracts.AuthorityGeneration
	if err := json.Unmarshal(payload, &generation); err != nil {
		return contracts.AuthorityGeneration{}, err
	}
	if generation.Ref != ref || generation.Version != version || payloadDigest(payload) != record.ObjectDigest || generation.Digest != expectedDigest {
		return contracts.AuthorityGeneration{}, errors.New("historical generation identity mismatch")
	}
	if err := generation.Validate(); err != nil {
		return contracts.AuthorityGeneration{}, err
	}
	if err := generation.VerifyDigest(); err != nil {
		return contracts.AuthorityGeneration{}, err
	}
	return generation, nil
}

package goalstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// historicalRecoveryStepPayload mirrors goalspublication.recoveryStepPayload.
// The production type is package-private, so this exact wire type preserves
// its default PascalCase field names without accepting alternate spellings.
type historicalRecoveryStepPayload struct {
	Version                string
	RequestID              string
	Step                   string
	Intent                 contracts.ActionIntent
	Authority              contracts.PackagePublishAuthorization
	PredecessorAbandonment string
}

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
	parent, err := r.loadHistoricalGeneration(ctx, decision.AuthorityRef, decision.AuthorityVersion, decision.AuthorityGenerationDigest)
	if err != nil {
		return contracts.ExpiredHistoricalAuthorityEvidence{}, err
	}
	if request.Delegation == nil {
		return contracts.ExpiredHistoricalAuthorityEvidence{}, errors.New("historical authority request has no delegation")
	}
	rootScope, _ := contracts.InstallationGovernanceScope(contracts.GoalsPublicationBootstrap)
	if parent.Ref != request.Delegation.ParentRef || parent.Version != request.Delegation.ParentVersion || parent.Digest != request.Delegation.ParentDigest || parent.Principal != decision.DecidedBy || parent.Scope != rootScope {
		return contracts.ExpiredHistoricalAuthorityEvidence{}, errors.New("historical parent authority lineage mismatch")
	}
	var childDigest string
	for _, id := range effectIDs {
		var body, actionIntentDigest []byte
		if err := r.Store.DB().QueryRowContext(ctx, `SELECT request_payload, action_intent_digest FROM effects WHERE effect_id=?`, id).Scan(&body, &actionIntentDigest); err != nil {
			return contracts.ExpiredHistoricalAuthorityEvidence{}, fmt.Errorf("historical effect %s: %w", id, err)
		}
		// recoveryStepPayload is the production wire contract. It intentionally
		// uses Go's canonical field names (RequestID, Step, Intent, Authority),
		// so this decoder must match that persisted representation exactly.
		var effect historicalRecoveryStepPayload
		if err := json.Unmarshal(body, &effect); err != nil {
			return contracts.ExpiredHistoricalAuthorityEvidence{}, errors.New("historical effect request lineage mismatch")
		}
		parts := strings.Split(id, ":")
		wantStep := parts[len(parts)-1]
		wantIntentDigest, _ := request.Intent.Digest()
		if effect.Version != "1" || effect.RequestID != requestID || effect.Step != wantStep || effect.Intent.ID != request.Intent.ID || effect.Intent.Version != request.Intent.Version || func() bool { d, _ := effect.Intent.Digest(); return d != wantIntentDigest }() || effect.Authority.Request.ID != request.ID || effect.Authority.Request.Version != request.Version || string(actionIntentDigest) != wantIntentDigest {
			return contracts.ExpiredHistoricalAuthorityEvidence{}, errors.New("historical effect request lineage mismatch")
		}
		if childDigest == "" {
			childDigest = effect.Authority.Generation.Digest
		} else if childDigest != effect.Authority.Generation.Digest {
			return contracts.ExpiredHistoricalAuthorityEvidence{}, errors.New("historical effects bind different authority generations")
		}
	}
	childRef := "authority-delegation:" + requestID
	child, err := r.loadHistoricalGeneration(ctx, childRef, "1", childDigest)
	if err != nil {
		return contracts.ExpiredHistoricalAuthorityEvidence{}, err
	}
	if request.Delegation == nil || child.Principal != request.Delegation.DelegatedPrincipal || child.Scope != request.Delegation.RequestedScope || child.Scope != decision.GrantedScope || child.ExpiresAt == nil || !child.ExpiresAt.Equal(request.Delegation.ExpiresAt) || child.DelegatedBy != decision.DecidedBy || child.ParentRef != parent.Ref || child.ParentVersion != parent.Version || child.ParentDigest != parent.Digest || child.Ref != childRef {
		return contracts.ExpiredHistoricalAuthorityEvidence{}, errors.New("historical delegated authority lineage mismatch")
	}
	if now.Before(*child.ExpiresAt) {
		return contracts.ExpiredHistoricalAuthorityEvidence{}, errors.New("historical delegated authority is not expired")
	}
	if request.Delegation.ParentDigest != parent.Digest {
		return contracts.ExpiredHistoricalAuthorityEvidence{}, errors.New("historical delegation lineage mismatch")
	}
	for _, id := range effectIDs {
		var effectPayload []byte
		var created string
		if err := r.Store.DB().QueryRowContext(ctx, `SELECT request_payload,created_at FROM effects WHERE effect_id=?`, id).Scan(&effectPayload, &created); err != nil {
			return contracts.ExpiredHistoricalAuthorityEvidence{}, fmt.Errorf("historical effect %s: %w", id, err)
		}
		at, err := time.Parse(time.RFC3339Nano, created)
		if err != nil || at.Before(child.EffectiveAt) || !at.Before(*child.ExpiresAt) {
			return contracts.ExpiredHistoricalAuthorityEvidence{}, errors.New("historical authority was not valid when effect occurred")
		}
		var effect historicalRecoveryStepPayload
		if err := json.Unmarshal(effectPayload, &effect); err != nil || effect.RequestID != requestID || effect.Authority.Generation.Digest != child.Digest || effect.Authority.Request.ID != requestID || effect.Authority.Request.Version != version {
			return contracts.ExpiredHistoricalAuthorityEvidence{}, errors.New("historical effect is not governed by exact authority")
		}
	}
	e := contracts.ExpiredHistoricalAuthorityEvidence{ObjectKind: "authority request", ObjectID: requestID, ObjectVersion: version, ObjectDigest: record.ObjectDigest, InstallationDigest: installationDigest, Principal: child.Principal, RequestDigest: expectedDigest, IntentID: request.Intent.ID, IntentDigest: intentDigest, DecisionRef: decision.DecisionRef, DecisionVersion: decision.DecisionVersion, DecisionDigest: decisionDigest, ParentRef: parent.Ref, ParentVersion: parent.Version, ParentDigest: parent.Digest, DelegationRef: child.DelegationRef, DelegationDigest: child.DelegationDigest, GenerationRef: child.Ref, GenerationVersion: child.Version, GenerationDigest: child.Digest, ExecutionID: executionID, EffectIDs: append([]string(nil), effectIDs...), EffectiveAt: child.EffectiveAt, ExpiresAt: *child.ExpiresAt, HistoricalValidityEstablished: true, Historical: true, NonExecutable: true, Status: contracts.HistoricalAuthorityExpired}
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
	payload, err := r.Crypto.Open(ctx, record.Envelope, state.SecureBlobAAD(record.Namespace, record.ObjectID, record.ObjectVersion, record.ObjectDigest))
	if err != nil {
		return contracts.AuthorityGeneration{}, err
	}
	var generation contracts.AuthorityGeneration
	if err := json.Unmarshal(payload, &generation); err != nil {
		return contracts.AuthorityGeneration{}, err
	}
	if generation.Ref != ref || generation.Version != version {
		return contracts.AuthorityGeneration{}, errors.New("historical generation identity mismatch")
	}
	if payloadDigest(payload) != record.ObjectDigest {
		return contracts.AuthorityGeneration{}, errors.New("historical generation secure-blob payload digest mismatch")
	}
	if err := generation.Validate(); err != nil {
		return contracts.AuthorityGeneration{}, err
	}
	if err := generation.VerifyDigest(); err != nil {
		return contracts.AuthorityGeneration{}, err
	}
	if generation.Digest != expectedDigest {
		return contracts.AuthorityGeneration{}, errors.New("historical generation digest mismatch")
	}
	return generation, nil
}

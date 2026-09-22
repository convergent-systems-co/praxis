package goalstore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/packages/goals"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

const completionSealNamespace = "goal_completion_seal"

// SealCompletion durably attests, under the installation storage key, the
// exact bytes of one controller-qualified unit or gate completion. The Goal-
// drive ledger is a plain event store that any writer of the database file can
// append to; a completion counts as evidence only if this authenticated seal
// exists for exactly its bytes (I11). The seal is the same trust root the
// frozen v4 plan already accepts for authority records: possession of the
// installation storage key. It is written by the controller after qualification
// and before the ledger append, and is refused for a Goal generation that is
// not the persisted safety-bearing generation.
func (r Repository) SealCompletion(ctx context.Context, baseline goals.GoalBaseline, payload []byte, now time.Time) (string, error) {
	if baseline.WorkPlan == nil || baseline.WorkPlan.Safety == nil {
		return "", errors.New("only a safety-bearing Goal generation seals completions")
	}
	if err := r.requireSafetyActivation(ctx, baseline.WorkPlan.Safety); err != nil {
		return "", err
	}
	if len(payload) == 0 {
		return "", errors.New("completion seal requires the exact completion bytes")
	}
	digest := payloadDigest(payload)
	id := completionSealID(baseline)
	if existing, _, err := r.loadWorkPlanBlob(ctx, completionSealNamespace, id, digest, now); err == nil {
		if payloadDigest(existing) == digest {
			return digest, nil
		}
		return "", errors.New("completion seal conflicts with an existing seal")
	} else if !errors.Is(err, state.ErrSecureBlobNotFound) {
		return "", err
	}
	if err := r.putWorkPlanBlob(ctx, completionSealNamespace, id, digest, payload, now, nil); err != nil {
		return "", fmt.Errorf("persist completion seal: %w", err)
	}
	return digest, nil
}

// LoadSealedCompletion returns the sealed bytes for a completion digest of the
// given Goal generation, or ErrCompletionUnauthenticated if none exists.
func (r Repository) LoadSealedCompletion(ctx context.Context, baseline goals.GoalBaseline, digest string, now time.Time) ([]byte, error) {
	payload, _, err := r.loadWorkPlanBlob(ctx, completionSealNamespace, completionSealID(baseline), digest, now)
	if err != nil {
		if errors.Is(err, state.ErrSecureBlobNotFound) {
			return nil, fmt.Errorf("%w: no authenticated seal for %s", contracts.ErrCompletionUnauthenticated, digest)
		}
		return nil, err
	}
	if payloadDigest(payload) != digest {
		return nil, fmt.Errorf("%w: seal bytes differ from %s", contracts.ErrCompletionUnauthenticated, digest)
	}
	return payload, nil
}

func completionSealID(baseline goals.GoalBaseline) string {
	return baseline.ID + "/" + baseline.Version
}

// VerifyGateCompletion re-resolves, from authenticated durable state, the exact
// lineage a gate completion claims: the deterministic request its qualified
// dossier implies, the recorded decision on that request, and the owner
// ceremony behind it. Two outcomes are distinct:
//
//   - the lineage does not resolve or does not match the completion's
//     evidence: contracts.ErrCompletionUnauthenticated (forgery or corruption);
//   - the lineage is authentic but the decision is not currently effective
//     (revoked, expired, or issued by a superseded root):
//     contracts.ErrGateAuthorityNotEffective. Historical truth is preserved;
//     the completion simply cannot authorize new work.
func (r Repository) VerifyGateCompletion(ctx context.Context, baseline goals.GoalBaseline, candidate contracts.WorkCandidate, artifacts []contracts.GovernedArtifactEvidence, requestDigest, decisionDigest string, now time.Time) error {
	request, err := r.prepareAuthorityGate(ctx, baseline, candidate, artifacts, now)
	if err != nil {
		return err
	}
	derived, err := request.Digest()
	if err != nil || derived != requestDigest {
		return fmt.Errorf("%w: gate completion does not cite the request its dossier implies", contracts.ErrCompletionUnauthenticated)
	}
	stored, err := r.LoadAuthorityRequest(ctx, request.ID, request.Version, now)
	if err != nil {
		if errors.Is(err, state.ErrSecureBlobNotFound) {
			return fmt.Errorf("%w: gate request was never durably recorded", contracts.ErrCompletionUnauthenticated)
		}
		return err
	}
	if storedDigest, digestErr := stored.Digest(); digestErr != nil || storedDigest != requestDigest {
		return fmt.Errorf("%w: recorded gate request differs from the completion's citation", contracts.ErrCompletionUnauthenticated)
	}
	evidence, err := r.LoadAuthorityDecisionEvidence(ctx, request.ID, request.Version, now)
	if err != nil {
		if errors.Is(err, state.ErrSecureBlobNotFound) {
			return fmt.Errorf("%w: gate decision was never durably recorded", contracts.ErrCompletionUnauthenticated)
		}
		return fmt.Errorf("%w: %v", contracts.ErrGateAuthorityNotEffective, err)
	}
	if got, digestErr := evidence.Digest(); digestErr != nil || got != decisionDigest {
		return fmt.Errorf("%w: recorded gate decision differs from the completion's citation", contracts.ErrCompletionUnauthenticated)
	}
	if err := r.verifyDecisionCeremony(ctx, stored, evidence, now); err != nil {
		return fmt.Errorf("%w: %v", contracts.ErrCompletionUnauthenticated, err)
	}
	// Authentic historical evidence. Whether it may authorize new work is the
	// current-authority question.
	decision, err := r.LoadAuthorityDecision(ctx, request.ID, request.Version, now)
	if err != nil {
		return fmt.Errorf("%w: %v", contracts.ErrGateAuthorityNotEffective, err)
	}
	if err := r.validateGateDecisionAuthority(ctx, stored, decision, now); err != nil {
		return fmt.Errorf("%w: %v", contracts.ErrGateAuthorityNotEffective, err)
	}
	if decision.Outcome != contracts.AuthorityApprove {
		return fmt.Errorf("%w: decision outcome is %s", contracts.ErrGateAuthorityNotEffective, decision.Outcome)
	}
	return nil
}

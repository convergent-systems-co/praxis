package goaldrive

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/convergent-systems-co/praxis/packages/goals"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// ErrSafetyDowngrade is returned when durable authenticated state classifies a
// Goal as safety-bearing but the generation being driven does not carry the
// safety binding (I9). A safety classification is never taken from the plan
// alone.
var ErrSafetyDowngrade = errors.New("Goal is classified safety-bearing but this generation carries no safety binding")

// ErrUnauthenticatedCompletion re-exports the contract sentinel for callers in
// this package's domain.
var ErrUnauthenticatedCompletion = contracts.ErrCompletionUnauthenticated

// EffectiveCompletions separates two questions that a bare ledger read
// conflates (I11):
//
//   - Effective: authentic completions that may currently authorize new work;
//   - Historical: authentic completions whose governing authority is no longer
//     effective (revoked, expired, superseded root), and units that hard-depend
//     on them. They remain true history and stay readable, but they do not
//     make anything eligible.
type EffectiveCompletions struct {
	Effective  []UnitCompletion
	Historical []UnitCompletion
}

// completionPayload is the canonical byte form a completion is sealed and
// compared under: the very bytes the ledger appends.
func completionPayload(completion UnitCompletion) ([]byte, error) {
	completion.CompletedAt = completion.CompletedAt.UTC()
	return json.Marshal(completion)
}

// safetyBearing decides whether a generation is under the safety kernel from
// authoritative state, not only from the plan's own claim (I9): a Goal that
// durable state classifies as safety-bearing whose generation carries no safety
// binding is refused rather than driven as a legacy plan.
func (c Controller) safetyBearing(ctx context.Context, baseline *goals.GoalBaseline) (bool, error) {
	return safetyBearingGoal(ctx, c.GoverningAuthority, baseline)
}

func safetyBearingGoal(ctx context.Context, governance PlanAuthorityVerifier, baseline *goals.GoalBaseline) (bool, error) {
	if baseline == nil {
		return false, nil
	}
	claimed := isSafetyBearing(baseline)
	if governance == nil {
		// Only the claim is available. The safety-bearing path then fails
		// closed on the missing verifier; a claim-free baseline cannot be
		// checked here and is only ever reached by non-production wiring.
		return claimed, nil
	}
	classified, err := governance.GoalSafetyClassified(ctx, baseline.ID)
	if err != nil {
		return false, fmt.Errorf("resolve Goal safety classification: %w", err)
	}
	if classified && !claimed {
		return false, fmt.Errorf("%w: %s/%s", ErrSafetyDowngrade, baseline.ID, baseline.Version)
	}
	return claimed, nil
}

// recordCompletion is the only controller path that appends a completion.
func (c Controller) recordCompletion(ctx context.Context, baseline *goals.GoalBaseline, completion UnitCompletion) error {
	safety, err := c.safetyBearing(ctx, baseline)
	if err != nil {
		return err
	}
	if !safety {
		completion.CompletedAt = completion.CompletedAt.UTC()
		return c.Ledger.RecordCompletion(ctx, completion)
	}
	return RecordSealedCompletion(ctx, c.Ledger, c.GoverningAuthority, baseline, completion)
}

// RecordSealedCompletion appends a completion for a safety-bearing generation.
// It seals the exact completion bytes under the installation storage key first,
// then appends to the ledger; a ledger row with no matching seal is refused at
// every consumption. Nothing else may append a safety-bearing completion and
// have it count.
func RecordSealedCompletion(ctx context.Context, ledger Ledger, governance PlanAuthorityVerifier, baseline *goals.GoalBaseline, completion UnitCompletion) error {
	completion.CompletedAt = completion.CompletedAt.UTC()
	if governance == nil {
		return fmt.Errorf("%w: verifier is not configured", ErrPlanAuthority)
	}
	payload, err := completionPayload(completion)
	if err != nil {
		return fmt.Errorf("encode completion: %w", err)
	}
	if _, err := governance.SealCompletion(ctx, *baseline, payload, time.Now().UTC()); err != nil {
		return fmt.Errorf("seal completion: %w", err)
	}
	return ledger.RecordCompletion(ctx, completion)
}

// AuthenticateCompletions is the single consumption boundary for ledger
// completions of a safety-bearing generation. A completion is trusted only if
// (1) it is structurally the evidence its kind requires, (2) its exact bytes
// carry an authenticated seal, and (3) for an authority gate, the request,
// decision, dossier and ceremony it cites re-resolve from authenticated state
// and the decision is still current authority. An unauthenticated completion is
// an error; an authentic completion whose authority is no longer effective is
// demoted to historical and never authorizes new work.
func AuthenticateCompletions(ctx context.Context, governance PlanAuthorityVerifier, baseline *goals.GoalBaseline, completions []UnitCompletion, now time.Time) (EffectiveCompletions, error) {
	if !isSafetyBearing(baseline) {
		return EffectiveCompletions{Effective: completions}, nil
	}
	if governance == nil {
		return EffectiveCompletions{}, fmt.Errorf("%w: verifier is not configured", ErrPlanAuthority)
	}
	plan := baseline.WorkPlan
	if err := VerifyPlanCompletions(plan, completions); err != nil {
		return EffectiveCompletions{}, err
	}
	units := make(map[string]contracts.WorkCandidate, len(plan.Candidates))
	for _, candidate := range plan.Candidates {
		units[candidate.ID] = candidate
	}
	var artifacts []contracts.GovernedArtifactEvidence
	for _, completion := range completions {
		if completion.GoalID != baseline.ID || completion.GoalVersion != baseline.Version {
			return EffectiveCompletions{}, fmt.Errorf("%w: completion of %q belongs to Goal %s/%s", ErrUnauthenticatedCompletion, completion.UnitID, completion.GoalID, completion.GoalVersion)
		}
		payload, err := completionPayload(completion)
		if err != nil {
			return EffectiveCompletions{}, err
		}
		digest := payloadDigest(payload)
		sealed, err := governance.LoadSealedCompletion(ctx, *baseline, digest, now)
		if err != nil {
			return EffectiveCompletions{}, fmt.Errorf("completion of %q: %w", completion.UnitID, err)
		}
		if !bytes.Equal(sealed, payload) {
			return EffectiveCompletions{}, fmt.Errorf("%w: sealed bytes for %q differ from the ledger record", ErrUnauthenticatedCompletion, completion.UnitID)
		}
		artifacts = append(artifacts, completion.GovernedArtifacts...)
	}
	stale := map[string]struct{}{}
	for _, completion := range completions {
		if !completion.AuthorityGate {
			continue
		}
		requestDigest, decisionDigest, err := gateCompletionCitations(completion)
		if err != nil {
			return EffectiveCompletions{}, err
		}
		err = governance.VerifyGateCompletion(ctx, *baseline, units[completion.UnitID], artifacts, requestDigest, decisionDigest, now)
		switch {
		case err == nil:
		case errors.Is(err, contracts.ErrGateAuthorityNotEffective):
			stale[completion.UnitID] = struct{}{}
		default:
			return EffectiveCompletions{}, fmt.Errorf("authority gate %q completion: %w", completion.UnitID, err)
		}
	}
	// A unit that completed under a gate whose decision is no longer effective
	// cannot authorize new work either: its hard prerequisites include a gate
	// that is not currently decided.
	if len(stale) > 0 {
		for changed := true; changed; {
			changed = false
			for _, relationship := range plan.Relationships {
				if relationship.Kind != contracts.RelationshipHardDependency {
					continue
				}
				if _, prerequisiteStale := stale[relationship.Prerequisite]; !prerequisiteStale {
					continue
				}
				if _, already := stale[relationship.Dependent]; !already {
					stale[relationship.Dependent] = struct{}{}
					changed = true
				}
			}
		}
	}
	var out EffectiveCompletions
	for _, completion := range completions {
		if _, isStale := stale[completion.UnitID]; isStale {
			out.Historical = append(out.Historical, completion)
			continue
		}
		out.Effective = append(out.Effective, completion)
	}
	return out, nil
}

// gateCompletionCitations extracts the exact request and decision digests a
// gate completion cites and requires its checkpoint field to name the decision.
func gateCompletionCitations(completion UnitCompletion) (string, string, error) {
	var request, decision string
	for _, item := range completion.Evidence {
		switch {
		case strings.HasPrefix(item, "authority-request:"):
			if request != "" {
				return "", "", fmt.Errorf("%w: gate completion cites more than one request", ErrUnauthenticatedCompletion)
			}
			request = strings.TrimPrefix(item, "authority-request:")
		case strings.HasPrefix(item, "authority-decision:"):
			if decision != "" {
				return "", "", fmt.Errorf("%w: gate completion cites more than one decision", ErrUnauthenticatedCompletion)
			}
			decision = strings.TrimPrefix(item, "authority-decision:")
		}
	}
	if request == "" || decision == "" || completion.EndHead != "authority:"+decision {
		return "", "", fmt.Errorf("%w: gate completion %q does not cite exactly one request and its decision", ErrUnauthenticatedCompletion, completion.UnitID)
	}
	return request, decision, nil
}

// LoadEffectiveCompletions is the consumption entry point for callers that
// hold a ledger and the governance store but not a Controller. It applies the
// same boundary the controller does.
func LoadEffectiveCompletions(ctx context.Context, ledger Ledger, governance PlanAuthorityVerifier, baseline *goals.GoalBaseline, goalID, goalVersion string) (EffectiveCompletions, error) {
	completions, err := ledger.LoadCompletions(ctx, goalID, goalVersion)
	if err != nil {
		return EffectiveCompletions{}, err
	}
	safety, err := safetyBearingGoal(ctx, governance, baseline)
	if err != nil {
		return EffectiveCompletions{}, err
	}
	if !safety {
		return EffectiveCompletions{Effective: completions}, nil
	}
	return AuthenticateCompletions(ctx, governance, baseline, completions, time.Now().UTC())
}

func (c Controller) effectiveCompletions(ctx context.Context, baseline *goals.GoalBaseline, goalID, goalVersion string) (EffectiveCompletions, error) {
	return LoadEffectiveCompletions(ctx, c.Ledger, c.GoverningAuthority, baseline, goalID, goalVersion)
}

func payloadDigest(payload []byte) string {
	sum := sha256.Sum256(payload)
	return fmt.Sprintf("sha256:%x", sum[:])
}

// VerifyCandidateCompletions is the consumption boundary for a Goal completion
// candidate (the evidence an owner is shown before settling a Goal). The
// candidate is a plaintext ledger record that embeds the unit completions; for a
// safety-bearing generation it is trusted only if every completion it names is
// authentic and currently effective and the set is exactly the authenticated
// one. Settlement is authority-bearing, so a forged or stale candidate must
// never be presented as "controller-verified".
func VerifyCandidateCompletions(ctx context.Context, ledger Ledger, governance PlanAuthorityVerifier, baseline *goals.GoalBaseline, candidate GoalCompletionCandidate) error {
	safety, err := safetyBearingGoal(ctx, governance, baseline)
	if err != nil {
		return err
	}
	if !safety {
		return nil
	}
	effective, err := LoadEffectiveCompletions(ctx, ledger, governance, baseline, baseline.ID, baseline.Version)
	if err != nil {
		return fmt.Errorf("authenticate completions behind the Goal completion candidate: %w", err)
	}
	if len(effective.Historical) != 0 {
		return fmt.Errorf("%w: %d completion(s) no longer have effective authority; the candidate cannot support settlement", contracts.ErrGateAuthorityNotEffective, len(effective.Historical))
	}
	authenticated := map[string]string{}
	for _, completion := range effective.Effective {
		payload, err := completionPayload(completion)
		if err != nil {
			return err
		}
		authenticated[completion.UnitID] = payloadDigest(payload)
	}
	if len(candidate.Units) != len(authenticated) {
		return fmt.Errorf("%w: the candidate names %d completions but %d are authenticated", ErrUnauthenticatedCompletion, len(candidate.Units), len(authenticated))
	}
	for _, completion := range candidate.Units {
		payload, err := completionPayload(completion)
		if err != nil {
			return err
		}
		if authenticated[completion.UnitID] != payloadDigest(payload) {
			return fmt.Errorf("%w: the candidate's record of %q is not the authenticated completion", ErrUnauthenticatedCompletion, completion.UnitID)
		}
	}
	return nil
}

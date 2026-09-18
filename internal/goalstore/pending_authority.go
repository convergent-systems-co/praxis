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

// AuthorityRequestDisposition is the truthful durable state of one authority
// request: pending, or decided with the exact recorded decision.
type AuthorityRequestDisposition struct {
	Request        contracts.AuthorityRequest   `json:"request"`
	RequestDigest  string                       `json:"request_digest"`
	Status         string                       `json:"status"`
	Decision       *contracts.AuthorityDecision `json:"decision,omitempty"`
	DecisionDigest string                       `json:"decision_digest,omitempty"`
}

// ListAuthorityRequests enumerates every durable authority request with its
// disposition. When goalID is set only requests bound to that exact Goal
// generation are returned. Nothing here selects or orders by recency: callers
// must name an exact request digest to act on one.
func (r Repository) ListAuthorityRequests(ctx context.Context, goalID, goalVersion string, now time.Time) ([]AuthorityRequestDisposition, error) {
	if err := r.validateWorkPlanStore(); err != nil {
		return nil, err
	}
	records, err := r.Store.ListSecureBlobs(ctx, authorityRequestNamespace, now)
	if err != nil {
		return nil, err
	}
	out := make([]AuthorityRequestDisposition, 0)
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
		if request.ID != record.ObjectID || request.Version != record.ObjectVersion {
			continue
		}
		if goalID != "" && (request.BaselineID != goalID || request.BaselineVersion != goalVersion) {
			continue
		}
		digest, err := request.Digest()
		if err != nil {
			return nil, err
		}
		item := AuthorityRequestDisposition{Request: request, RequestDigest: digest, Status: string(request.Status)}
		decision, decisionErr := r.LoadAuthorityDecision(ctx, request.ID, request.Version, now)
		if decisionErr == nil {
			decisionDigest, err := decision.Digest()
			if err != nil {
				return nil, err
			}
			item.Decision, item.DecisionDigest, item.Status = &decision, decisionDigest, "decided:"+string(decision.Outcome)
		} else if !errors.Is(decisionErr, state.ErrSecureBlobNotFound) && !errors.Is(decisionErr, state.ErrSecureBlobExpired) {
			return nil, decisionErr
		}
		out = append(out, item)
	}
	return out, nil
}

// decodeRecord opens and decodes one secure blob without validating its
// contract. Enumeration filters on the decoded identity first and validates
// only records that belong to the requested lineage, so an unrelated or
// historical record that no longer validates (for example a superseded
// dogfood lineage, #149) cannot abort inspection of another Goal.
func (r Repository) decodeRecord(ctx context.Context, record state.SecureBlobRecord, out any) error {
	aad := state.SecureBlobAAD(record.Namespace, record.ObjectID, record.ObjectVersion, record.ObjectDigest)
	payload, err := r.Crypto.Open(ctx, record.Envelope, aad)
	if err != nil {
		return fmt.Errorf("decrypt %s record %s: %w", record.Namespace, record.ObjectID, err)
	}
	if err := json.Unmarshal(payload, out); err != nil {
		return fmt.Errorf("decode %s record %s: %w", record.Namespace, record.ObjectID, err)
	}
	return nil
}

// ListWorkPlanProposals enumerates the durable proposals bound to one exact
// Goal generation, with their canonical digests.
func (r Repository) ListWorkPlanProposals(ctx context.Context, goalID, goalVersion string, now time.Time) ([]contracts.WorkPlanProposal, []string, error) {
	if err := r.validateWorkPlanStore(); err != nil {
		return nil, nil, err
	}
	records, err := r.Store.ListSecureBlobs(ctx, workPlanProposalNamespace, now)
	if err != nil {
		return nil, nil, err
	}
	var proposals []contracts.WorkPlanProposal
	var digests []string
	for _, record := range records {
		var raw contracts.WorkPlanProposal
		if err := r.decodeRecord(ctx, record, &raw); err != nil {
			return nil, nil, err
		}
		if raw.GoalID != goalID || raw.GoalVersion != goalVersion {
			continue
		}
		proposal, err := r.LoadWorkPlanProposal(ctx, record.ObjectID, record.ObjectVersion, now)
		if err != nil {
			return nil, nil, err
		}
		digest, err := proposal.Digest()
		if err != nil {
			return nil, nil, err
		}
		proposals, digests = append(proposals, proposal), append(digests, digest)
	}
	return proposals, digests, nil
}

// LoadWorkPlanProposalByDigest resolves a proposal by its exact canonical
// digest. The stored record version is returned alongside it.
func (r Repository) LoadWorkPlanProposalByDigest(ctx context.Context, wanted string, now time.Time) (contracts.WorkPlanProposal, string, error) {
	if wanted == "" {
		return contracts.WorkPlanProposal{}, "", errors.New("proposal digest is required")
	}
	records, err := r.Store.ListSecureBlobs(ctx, workPlanProposalNamespace, now)
	if err != nil {
		return contracts.WorkPlanProposal{}, "", err
	}
	for _, record := range records {
		var raw contracts.WorkPlanProposal
		if err := r.decodeRecord(ctx, record, &raw); err != nil {
			return contracts.WorkPlanProposal{}, "", err
		}
		digest, err := raw.Digest()
		if err != nil || digest != wanted {
			continue
		}
		proposal, err := r.LoadWorkPlanProposal(ctx, record.ObjectID, record.ObjectVersion, now)
		if err != nil {
			return contracts.WorkPlanProposal{}, "", err
		}
		return proposal, record.ObjectVersion, nil
	}
	return contracts.WorkPlanProposal{}, "", fmt.Errorf("WorkPlan proposal %s is not a durable record", wanted)
}

// ListWorkPlanReviews enumerates the durable reviews bound to one exact
// proposal digest, with their stored record versions.
func (r Repository) ListWorkPlanReviews(ctx context.Context, proposalDigest string, now time.Time) ([]contracts.WorkPlanProposalReview, []string, error) {
	if err := r.validateWorkPlanStore(); err != nil {
		return nil, nil, err
	}
	records, err := r.Store.ListSecureBlobs(ctx, workPlanReviewNamespace, now)
	if err != nil {
		return nil, nil, err
	}
	if proposalDigest == "" {
		return nil, nil, errors.New("review enumeration requires an exact proposal digest")
	}
	var reviews []contracts.WorkPlanProposalReview
	var versions []string
	for _, record := range records {
		var raw workPlanReviewRecord
		if err := r.decodeRecord(ctx, record, &raw); err != nil {
			return nil, nil, err
		}
		if raw.Review.ProposalDigest != proposalDigest {
			continue
		}
		review, err := r.LoadWorkPlanReview(ctx, record.ObjectID, record.ObjectVersion, now)
		if err != nil {
			return nil, nil, err
		}
		reviews, versions = append(reviews, review), append(versions, record.ObjectVersion)
	}
	return reviews, versions, nil
}

// LoadWorkPlanReviewByDigest resolves a review by its exact review digest and
// the proposal it reviewed. The stored record version is returned alongside.
func (r Repository) LoadWorkPlanReviewByDigest(ctx context.Context, proposalDigest, reviewDigest string, now time.Time) (contracts.WorkPlanProposalReview, string, error) {
	if reviewDigest == "" {
		return contracts.WorkPlanProposalReview{}, "", errors.New("review digest is required")
	}
	reviews, versions, err := r.ListWorkPlanReviews(ctx, proposalDigest, now)
	if err != nil {
		return contracts.WorkPlanProposalReview{}, "", err
	}
	for i, review := range reviews {
		if review.ReviewDigest == reviewDigest {
			return review, versions[i], nil
		}
	}
	return contracts.WorkPlanProposalReview{}, "", fmt.Errorf("WorkPlan review %s for proposal %s is not a durable record", reviewDigest, proposalDigest)
}

// ListAcceptedWorkPlans enumerates the durable authority-backed acceptances
// bound to one exact baseline digest, keyed by acceptance ref and version.
func (r Repository) ListAcceptedWorkPlans(ctx context.Context, baselineDigest string, now time.Time) (map[string]contracts.WorkPlan, error) {
	if err := r.validateWorkPlanStore(); err != nil {
		return nil, err
	}
	records, err := r.Store.ListSecureBlobs(ctx, workPlanAcceptanceNamespace, now)
	if err != nil {
		return nil, err
	}
	if baselineDigest == "" {
		return nil, errors.New("acceptance enumeration requires an exact baseline digest")
	}
	out := map[string]contracts.WorkPlan{}
	for _, record := range records {
		var raw acceptedWorkPlanRecord
		if err := r.decodeRecord(ctx, record, &raw); err != nil {
			return nil, err
		}
		if raw.Plan.BaselineDigest != baselineDigest {
			continue
		}
		plan, err := r.LoadAcceptedWorkPlan(ctx, record.ObjectID, record.ObjectVersion, now)
		if err != nil {
			return nil, err
		}
		out[record.ObjectID+"/"+record.ObjectVersion] = plan
	}
	return out, nil
}

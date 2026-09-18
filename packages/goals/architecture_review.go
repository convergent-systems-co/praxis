package goals

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/convergent-systems-co/praxis/internal/architecturereview"
)

var (
	ErrBaselineReviewEvidence   = errors.New("architecture review must bind to the candidate Goal Baseline")
	ErrBaselineReviewReceipt    = errors.New("invalid architecture review receipt")
	ErrUnresolvedBaselineReview = errors.New("architecture review requires an explicit resolution")
)

// BaselineReviewReceipt is durable evidence that an advisory inversion review
// evaluated one exact candidate baseline. It deliberately lives outside
// GoalBaseline so review output cannot alter the candidate's canonical digest.
type BaselineReviewReceipt struct {
	ID                 string                    `json:"id"`
	BaselineID         string                    `json:"baseline_id"`
	BaselineVersion    string                    `json:"baseline_version"`
	BaselineDigest     string                    `json:"baseline_digest"`
	Result             architecturereview.Result `json:"result"`
	ResolutionEvidence string                    `json:"resolution_evidence,omitempty"`
}

type canonicalBaselineReviewReceipt struct {
	BaselineID         string                    `json:"baseline_id"`
	BaselineVersion    string                    `json:"baseline_version"`
	BaselineDigest     string                    `json:"baseline_digest"`
	Result             architecturereview.Result `json:"result"`
	ResolutionEvidence string                    `json:"resolution_evidence,omitempty"`
}

// ReviewBaselineOwnership evaluates and records the advisory review against a
// fully formed, already-digested candidate. The receipt is content-addressed;
// it does not grant persistence, architecture, promotion, or execution authority.
func ReviewBaselineOwnership(b GoalBaseline, req architecturereview.Request) (BaselineReviewReceipt, error) {
	if err := verifyCandidateBaseline(b); err != nil {
		return BaselineReviewReceipt{}, err
	}
	wantID := fmt.Sprintf("baseline:%s@%s", b.ID, b.Version)
	bound := false
	for _, ref := range req.GoalEvidence {
		if ref.ID == wantID && ref.Digest == b.Digest {
			bound = true
			break
		}
	}
	if !bound {
		return BaselineReviewReceipt{}, ErrBaselineReviewEvidence
	}
	result, err := architecturereview.Review(req)
	if err != nil {
		return BaselineReviewReceipt{}, err
	}
	receipt := BaselineReviewReceipt{BaselineID: b.ID, BaselineVersion: b.Version, BaselineDigest: b.Digest, Result: result}
	return bindBaselineReviewReceipt(receipt)
}

// ResolveBaselineReview records explicit evidence for a review-required result
// and returns a new content-addressed receipt. It does not interpret that
// evidence as execution or promotion authority.
func ResolveBaselineReview(b GoalBaseline, receipt BaselineReviewReceipt, evidence string) (BaselineReviewReceipt, error) {
	if evidence == "" {
		return BaselineReviewReceipt{}, ErrUnresolvedBaselineReview
	}
	if err := VerifyBaselineReviewReceipt(b, receipt, false); err != nil {
		return BaselineReviewReceipt{}, err
	}
	if receipt.Result.Kind != architecturereview.ReviewRequired || receipt.ResolutionEvidence != "" {
		return BaselineReviewReceipt{}, ErrBaselineReviewReceipt
	}
	receipt.ID = ""
	receipt.ResolutionEvidence = evidence
	return bindBaselineReviewReceipt(receipt)
}

// VerifyBaselineReviewReceipt verifies receipt identity, semantic shape, exact
// candidate binding, and (when requested) resolution of review-required output.
func VerifyBaselineReviewReceipt(b GoalBaseline, receipt BaselineReviewReceipt, requireResolved bool) error {
	if err := verifyCandidateBaseline(b); err != nil {
		return err
	}
	if receipt.BaselineID != b.ID || receipt.BaselineVersion != b.Version || receipt.BaselineDigest != b.Digest {
		return ErrBaselineReviewEvidence
	}
	if err := validateReviewResult(receipt.Result); err != nil {
		return err
	}
	want, err := receiptDigest(receipt)
	if err != nil {
		return err
	}
	if receipt.ID == "" || receipt.ID != want {
		return ErrBaselineReviewReceipt
	}
	if receipt.Result.Kind != architecturereview.ReviewRequired && receipt.ResolutionEvidence != "" {
		return ErrBaselineReviewReceipt
	}
	if requireResolved && receipt.Result.Kind == architecturereview.ReviewRequired && receipt.ResolutionEvidence == "" {
		return ErrUnresolvedBaselineReview
	}
	return nil
}

func verifyCandidateBaseline(b GoalBaseline) error {
	if err := b.Validate(); err != nil {
		return err
	}
	if b.Digest == "" {
		return ErrBaselineReviewEvidence
	}
	return b.VerifyDigest()
}

func bindBaselineReviewReceipt(receipt BaselineReviewReceipt) (BaselineReviewReceipt, error) {
	if err := validateReviewResult(receipt.Result); err != nil {
		return BaselineReviewReceipt{}, err
	}
	id, err := receiptDigest(receipt)
	if err != nil {
		return BaselineReviewReceipt{}, err
	}
	receipt.ID = id
	return receipt, nil
}

func receiptDigest(receipt BaselineReviewReceipt) (string, error) {
	if receipt.BaselineID == "" || receipt.BaselineVersion == "" || receipt.BaselineDigest == "" {
		return "", ErrBaselineReviewReceipt
	}
	payload, err := json.Marshal(canonicalBaselineReviewReceipt{
		BaselineID: receipt.BaselineID, BaselineVersion: receipt.BaselineVersion,
		BaselineDigest: receipt.BaselineDigest, Result: receipt.Result,
		ResolutionEvidence: receipt.ResolutionEvidence,
	})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func validateReviewResult(result architecturereview.Result) error {
	if !result.AdvisoryOnly || len(result.Reasons) == 0 {
		return ErrBaselineReviewReceipt
	}
	switch result.Kind {
	case architecturereview.UniversalMechanism, architecturereview.DomainSpecific, architecturereview.ReviewRequired:
	default:
		return ErrBaselineReviewReceipt
	}
	seen := map[string]struct{}{}
	for _, ref := range result.Evidence {
		if ref.ID == "" || ref.Kind == "" || ref.Digest == "" {
			return ErrBaselineReviewReceipt
		}
		if _, exists := seen[ref.ID]; exists {
			return ErrBaselineReviewReceipt
		}
		seen[ref.ID] = struct{}{}
	}
	return nil
}

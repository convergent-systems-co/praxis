package learning

import (
	"errors"

	"github.com/convergent-systems-co/praxis/internal/architecturereview"
)

var ErrMissingBlindDerivation = errors.New("inversion review requires the blind candidate derivation digest")

func validateInversionReviewRecord(record InversionReviewRecord) error {
	if record.ID == "" || record.CandidateID == "" || record.BlindDerivationDigest == "" || !record.Review.AdvisoryOnly {
		return errors.New("invalid architecture inversion review record")
	}
	switch record.Review.Kind {
	case architecturereview.UniversalMechanism, architecturereview.DomainSpecific, architecturereview.ReviewRequired:
	default:
		return errors.New("architecture inversion review has unknown result kind")
	}
	if len(record.Review.Reasons) == 0 || len(record.Review.Evidence) == 0 {
		return errors.New("architecture inversion review result is incomplete")
	}
	seen := make(map[string]struct{}, len(record.Review.Evidence))
	for _, evidence := range record.Review.Evidence {
		if evidence.ID == "" || evidence.Kind == "" || evidence.Digest == "" {
			return errors.New("architecture inversion review has invalid evidence")
		}
		if _, duplicate := seen[evidence.ID]; duplicate {
			return errors.New("architecture inversion review has duplicate evidence identity")
		}
		seen[evidence.ID] = struct{}{}
	}
	payload := record
	payload.ID = ""
	digest, err := digestBehaviorValue(payload)
	if err != nil {
		return err
	}
	if record.ID != "sha256:"+digest {
		return errors.New("architecture inversion review content digest mismatch")
	}
	return nil
}

// InversionReviewRecord keeps an architecture review beside, rather than
// inside, blind candidate derivation. The digest is provenance only: this
// helper never promotes, activates, or rewrites a learning generation.
type InversionReviewRecord struct {
	ID                    string
	CandidateID           string
	BlindDerivationDigest string
	Review                architecturereview.Result
}

func freezeInversionReviewRecord(record InversionReviewRecord) (InversionReviewRecord, error) {
	record.ID = ""
	digest, err := digestBehaviorValue(record)
	if err != nil {
		return InversionReviewRecord{}, err
	}
	record.ID = "sha256:" + digest
	if err := validateInversionReviewRecord(record); err != nil {
		return InversionReviewRecord{}, err
	}
	return record, nil
}

func ReviewCandidateOwnership(candidateID, blindDerivationDigest string, req architecturereview.Request) (InversionReviewRecord, error) {
	if candidateID == "" || blindDerivationDigest == "" {
		return InversionReviewRecord{}, ErrMissingBlindDerivation
	}
	review, err := architecturereview.Review(req)
	if err != nil {
		return InversionReviewRecord{}, err
	}
	return freezeInversionReviewRecord(InversionReviewRecord{CandidateID: candidateID, BlindDerivationDigest: blindDerivationDigest, Review: review})
}

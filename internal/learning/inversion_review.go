package learning

import (
	"errors"

	"github.com/convergent-systems-co/praxis/internal/architecturereview"
)

var ErrMissingBlindDerivation = errors.New("inversion review requires the blind candidate derivation digest")

// InversionReviewRecord keeps an architecture review beside, rather than
// inside, blind candidate derivation. The digest is provenance only: this
// helper never promotes, activates, or rewrites a learning generation.
type InversionReviewRecord struct {
	CandidateID           string
	BlindDerivationDigest string
	Review                architecturereview.Result
}

func ReviewCandidateOwnership(candidateID, blindDerivationDigest string, req architecturereview.Request) (InversionReviewRecord, error) {
	if candidateID == "" || blindDerivationDigest == "" {
		return InversionReviewRecord{}, ErrMissingBlindDerivation
	}
	review, err := architecturereview.Review(req)
	if err != nil {
		return InversionReviewRecord{}, err
	}
	return InversionReviewRecord{CandidateID: candidateID, BlindDerivationDigest: blindDerivationDigest, Review: review}, nil
}

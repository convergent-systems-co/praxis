package goalstore

import (
	"context"
	"errors"
	"strings"
	"time"

	statepkg "github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func (r Repository) SaveGovernedAuthorityProposal(ctx context.Context, proposal contracts.GovernedAuthorityProposal, now time.Time) (string, error) {
	d, err := proposal.Digest()
	if err != nil { return "", err }
	if _, err := r.savePublisherGovernance(ctx, proposal.ID, proposal.Version, proposal, now, &proposal.ExpiresAt); err != nil { return "", err }
	return d, nil
}

func (r Repository) LoadGovernedAuthorityProposalByDigest(ctx context.Context, wanted string, now time.Time) (contracts.GovernedAuthorityProposal, error) {
	records, err := r.Store.ListSecureBlobs(ctx, publisherGovernanceNamespace, now)
	if err != nil { return contracts.GovernedAuthorityProposal{}, err }
	for _, record := range records {
		if !strings.HasPrefix(record.ObjectID, "package-manager-authority-proposal:") { continue }
		var p contracts.GovernedAuthorityProposal
		if err := r.loadPublisherGovernance(ctx, record.ObjectID, record.ObjectVersion, now, &p); err != nil { return contracts.GovernedAuthorityProposal{}, err }
		d, err := p.Digest(); if err != nil { return contracts.GovernedAuthorityProposal{}, err }
		if d == wanted { return p, nil }
	}
	return contracts.GovernedAuthorityProposal{}, statepkg.ErrSecureBlobNotFound
}

func (r Repository) SaveGovernedAuthorityReview(ctx context.Context, review contracts.GovernedAuthorityReview, now time.Time) (string, error) {
	proposal, err := r.LoadGovernedAuthorityProposalByDigest(ctx, review.ProposalDigest, now)
	if err != nil { return "", err }
	if proposal.ID != review.ProposalID || proposal.Version != review.ProposalVersion { return "", errors.New("review does not bind exact governed authority proposal") }
	d, err := review.Digest(); if err != nil { return "", err }
	if _, err := r.savePublisherGovernance(ctx, review.ID, review.Version, review, now, &proposal.ExpiresAt); err != nil { return "", err }
	return d, nil
}

func (r Repository) LoadGovernedAuthorityReviewByDigest(ctx context.Context, wanted string, now time.Time) (contracts.GovernedAuthorityReview, error) {
	records, err := r.Store.ListSecureBlobs(ctx, publisherGovernanceNamespace, now)
	if err != nil { return contracts.GovernedAuthorityReview{}, err }
	for _, record := range records {
		if !strings.HasPrefix(record.ObjectID, "package-manager-authority-review:") { continue }
		var review contracts.GovernedAuthorityReview
		if err := r.loadPublisherGovernance(ctx, record.ObjectID, record.ObjectVersion, now, &review); err != nil { return contracts.GovernedAuthorityReview{}, err }
		d, err := review.Digest(); if err != nil { return contracts.GovernedAuthorityReview{}, err }
		if d == wanted { return review, nil }
	}
	return contracts.GovernedAuthorityReview{}, statepkg.ErrSecureBlobNotFound
}

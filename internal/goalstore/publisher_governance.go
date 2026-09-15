package goalstore

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

const publisherGovernanceNamespace = "publisher_governance"

func (r Repository) savePublisherGovernance(ctx context.Context, id, version string, value any, created time.Time, expires *time.Time) (string, error) {
	if err := r.validateWorkPlanStore(); err != nil {
		return "", err
	}
	b, e := json.Marshal(value)
	if e != nil {
		return "", e
	}
	if err := r.putWorkPlanBlob(ctx, publisherGovernanceNamespace, id, version, b, created, expires); err != nil {
		return "", err
	}
	return "", nil
}
func (r Repository) loadPublisherGovernance(ctx context.Context, id, version string, now time.Time, value any) error {
	b, _, e := r.loadWorkPlanBlob(ctx, publisherGovernanceNamespace, id, version, now)
	if e != nil {
		return e
	}
	return json.Unmarshal(b, value)
}

func (r Repository) SaveAuthorityModelAdoption(ctx context.Context, adoption contracts.AuthorityModelAdoption, now time.Time) (string, error) {
	d, e := adoption.Digest()
	if e != nil {
		return "", e
	}
	if adoption.ToVersion != contracts.AuthorityModelSuccessorVersion || adoption.ToDigest != contracts.AuthorityModelSuccessorDigest() {
		return "", errors.New("adoption does not bind authority-model v2")
	}
	if _, e = r.savePublisherGovernance(ctx, adoption.ID, adoption.Version, adoption, now, nil); e != nil {
		return "", e
	}
	return d, nil
}
func (r Repository) SavePublisherAuthorityProposal(ctx context.Context, p contracts.PublisherAuthorityProposal, now time.Time) (string, error) {
	d, e := p.Digest()
	if e != nil {
		return "", e
	}
	if _, e = r.savePublisherGovernance(ctx, p.ID, p.Version, p, now, nil); e != nil {
		return "", e
	}
	return d, nil
}
func (r Repository) SavePublisherAuthorityReview(ctx context.Context, review contracts.PublisherAuthorityReview, now time.Time) (string, error) {
	p := contracts.PublisherAuthorityProposal{}
	if e := r.loadPublisherGovernance(ctx, review.ProposalID, review.ProposalVersion, now, &p); e != nil {
		return "", e
	}
	pd, e := p.Digest()
	if e != nil || pd != review.ProposalDigest {
		return "", errors.New("review does not bind exact proposal")
	}
	d, e := review.Digest()
	if e != nil {
		return "", e
	}
	if _, e = r.savePublisherGovernance(ctx, review.ID, review.Version, review, now, nil); e != nil {
		return "", e
	}
	return d, nil
}
func (r Repository) SavePublisherEnrollmentApproval(ctx context.Context, a contracts.PublisherEnrollmentApproval, now time.Time) (string, error) {
	d, e := a.Digest()
	if e != nil {
		return "", e
	}
	if _, e = r.savePublisherGovernance(ctx, a.ID, a.Version, a, now, nil); e != nil {
		return "", e
	}
	return d, nil
}

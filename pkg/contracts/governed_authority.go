package contracts

import (
	"errors"
	"time"
)

const (
	GovernedAuthorityProposalKind = "governed-authority-proposal"
	GovernedAuthorityReviewKind   = "governed-authority-review"
)

// GovernedAuthorityProposal is the profile-neutral durable proposal envelope.
// Closed profiles validate its semantic target separately.
type GovernedAuthorityProposal struct {
	ID, Version, Kind, BootstrapDigest, OwnerID, OwnerKind string
	AuthorityModel, AuthorityModelVersion, AuthorityModelDigest string
	Profile, Capability, PrincipalID, PrincipalKind, TargetKind, TargetIdentity, TargetVersion, TargetDigest, Scope, Reason string
	ParentRef, ParentVersion, ParentDigest string
	ExpiresAt, CreatedAt time.Time
}

func (p GovernedAuthorityProposal) Digest() (string, error) {
	if p.ID == "" || p.Version == "" || p.Kind != GovernedAuthorityProposalKind || p.BootstrapDigest == "" || p.OwnerID == "" || p.OwnerKind == "" || p.AuthorityModel != AuthorityModelID || p.AuthorityModelVersion == "" || p.AuthorityModelDigest == "" || p.Profile == "" || p.Capability == "" || p.PrincipalID == "" || p.PrincipalKind == "" || p.TargetKind == "" || p.TargetIdentity == "" || p.TargetVersion == "" || p.TargetDigest == "" || p.Scope == "" || p.ParentRef == "" || p.ParentVersion == "" || p.ParentDigest == "" || p.Reason == "" || p.CreatedAt.IsZero() || p.ExpiresAt.IsZero() || !p.ExpiresAt.After(p.CreatedAt) {
		return "", errors.New("governed authority proposal is incomplete")
	}
	return digestCanonical(p)
}

type GovernedAuthorityReview struct {
	ID, Version, Kind, ProposalID, ProposalVersion, ProposalDigest, ReviewedBy, ReviewedKind, Decision string
	ReviewedAt time.Time
}

func (r GovernedAuthorityReview) Digest() (string, error) {
	if r.ID == "" || r.Version == "" || r.Kind != GovernedAuthorityReviewKind || r.ProposalID == "" || r.ProposalVersion == "" || r.ProposalDigest == "" || r.ReviewedBy == "" || r.ReviewedKind == "" || r.Decision != "approve" || r.ReviewedAt.IsZero() {
		return "", errors.New("governed authority review is incomplete")
	}
	return digestCanonical(r)
}

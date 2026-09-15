package contracts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const (
	AuthorityModelAdoptionKind      = "authority-model-adoption"
	PublisherEnrollmentProposalKind = "publisher-enrollment-proposal"
	PublisherAuthorityProposalKind  = "publisher-authority-proposal"
	PublisherAuthorityReviewKind    = "publisher-authority-review"
)

type AuthorityModelState struct { Version, ActiveModel, ActiveVersion, ActiveDigest, AdoptionDigest, State string }

type AuthorityModelAdoption struct {
	ID, Version, FromModel, FromVersion, FromDigest, ToModel, ToVersion, ToDigest, RootRef, RootVersion, RootDigest, Reason string
	CreatedAt                                                                                                               time.Time
}
type PublisherEnrollmentApproval struct {
	ID, Version, Kind, GenerationTemplateDigest, PublisherPrincipal, PublicKeyDigest, Namespace, ApproverID, ApproverKind string
	IssuedAt                                                                                                              time.Time
}
type PublisherAuthorityProposal struct {
	ID, Version, Kind, PublisherGenerationDigest, PublisherPrincipal, Namespace, ParentRef, ParentVersion, ParentDigest, Reason string
	CreatedAt                                                                                                                   time.Time
}
type PublisherAuthorityReview struct {
	ID, Version, Kind, ProposalID, ProposalVersion, ProposalDigest, ReviewedBy, ReviewedKind, Decision string
	ReviewedAt                                                                                         time.Time
}

func digestCanonical(v any) (string, error) {
	b, e := json.Marshal(v)
	if e != nil {
		return "", e
	}
	s := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(s[:]), nil
}
func (a AuthorityModelAdoption) Digest() (string, error) {
	if a.ID == "" || a.Version == "" || a.FromModel == "" || a.FromVersion == "" || a.FromDigest == "" || a.ToModel == "" || a.ToVersion == "" || a.ToDigest == "" || a.RootRef == "" || a.RootVersion == "" || a.RootDigest == "" || a.CreatedAt.IsZero() {
		return "", errors.New("authority-model adoption identity is incomplete")
	}
	return digestCanonical(a)
}
func (p PublisherAuthorityProposal) Digest() (string, error) {
	if p.ID == "" || p.Version == "" || p.Kind != PublisherAuthorityProposalKind || p.PublisherGenerationDigest == "" || p.PublisherPrincipal == "" || p.Namespace == "" || p.ParentRef == "" || p.ParentVersion == "" || p.ParentDigest == "" || p.Reason == "" || p.CreatedAt.IsZero() {
		return "", errors.New("publisher authority proposal is incomplete")
	}
	return digestCanonical(p)
}
func (r PublisherAuthorityReview) Digest() (string, error) {
	if r.ID == "" || r.Version == "" || r.Kind != PublisherAuthorityReviewKind || r.ProposalID == "" || r.ProposalVersion == "" || r.ProposalDigest == "" || r.ReviewedBy == "" || r.ReviewedKind == "" || r.Decision != "approve" || r.ReviewedAt.IsZero() {
		return "", errors.New("publisher authority review is incomplete")
	}
	return digestCanonical(r)
}
func (p PublisherEnrollmentApproval) Digest() (string, error) {
	if p.ID == "" || p.Version == "" || p.Kind != "publisher-enrollment-approval" || p.GenerationTemplateDigest == "" || p.PublisherPrincipal == "" || p.PublicKeyDigest == "" || p.Namespace == "" || p.ApproverID == "" || p.ApproverKind == "" || p.IssuedAt.IsZero() {
		return "", errors.New("publisher enrollment approval is incomplete")
	}
	return digestCanonical(p)
}
func NamespaceFromPublishScope(scope string) (string, error) {
	const prefix = "package-namespace:"
	if !strings.HasPrefix(scope, prefix) || len(scope) == len(prefix) {
		return "", errors.New("invalid package namespace scope")
	}
	return strings.TrimPrefix(scope, prefix), nil
}

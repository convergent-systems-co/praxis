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

type AuthorityModelState struct{ Version, ActiveModel, ActiveVersion, ActiveDigest, AdoptionDigest, State string }

type PublisherEnrollmentPreview struct {
	ID, Version, BootstrapDigest, OwnerID, OwnerKind                  string
	AuthorityModel, AuthorityModelVersion, AuthorityModelDigest       string
	PublisherPrincipal, KeyID, Algorithm, KeyPurpose, PublicKeyDigest string
	Generation, Predecessor, Namespace, GenerationDigest              string
	GenerationRecord                                                  PublisherGeneration
	CreatedAt                                                         time.Time
}

func (p PublisherEnrollmentPreview) Digest() (string, error) {
	if p.ID == "" || p.Version == "" || p.BootstrapDigest == "" || p.OwnerID == "" || p.OwnerKind == "" || p.AuthorityModel == "" || p.AuthorityModelVersion == "" || p.AuthorityModelDigest == "" || p.PublisherPrincipal == "" || p.KeyID == "" || p.Algorithm == "" || p.KeyPurpose != "publisher-signing" || p.PublicKeyDigest == "" || p.Generation == "" || p.Namespace == "" || p.GenerationDigest == "" || p.CreatedAt.IsZero() {
		return "", errors.New("publisher enrollment preview is incomplete")
	}
	actual, err := p.GenerationRecord.Digest()
	if err != nil || actual != p.GenerationDigest {
		return "", errors.New("publisher enrollment preview generation mismatch")
	}
	return digestCanonical(p)
}

type AuthorityModelAdoption struct {
	ID, Version, FromModel, FromVersion, FromDigest, ToModel, ToVersion, ToDigest, RootRef, RootVersion, RootDigest, Reason string
	CreatedAt                                                                                                               time.Time
}
type PublisherEnrollmentApproval struct {
	ID, Version, Kind, PreviewDigest, BootstrapDigest, OwnerID, OwnerKind, AuthorityModel, AuthorityModelVersion, AuthorityModelDigest            string
	GenerationTemplateDigest, PublisherPrincipal, PublicKeyDigest, KeyID, Algorithm, Namespace, Generation, Predecessor, ApproverID, ApproverKind string
	GenerationRecord                                                                                                                              PublisherGeneration
	IssuedAt                                                                                                                                      time.Time
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
	if p.ID == "" || p.Version == "" || p.Kind != "publisher-enrollment-approval" || p.PreviewDigest == "" || p.BootstrapDigest == "" || p.OwnerID == "" || p.OwnerKind == "" || p.AuthorityModel == "" || p.AuthorityModelVersion == "" || p.AuthorityModelDigest == "" || p.GenerationTemplateDigest == "" || p.PublisherPrincipal == "" || p.PublicKeyDigest == "" || p.KeyID == "" || p.Algorithm == "" || p.Namespace == "" || p.Generation == "" || p.ApproverID == "" || p.ApproverKind == "" || p.IssuedAt.IsZero() {
		return "", errors.New("publisher enrollment approval is incomplete")
	}
	actual, err := p.GenerationRecord.Digest()
	if err != nil || actual != p.GenerationTemplateDigest {
		return "", errors.New("publisher enrollment approval generation mismatch")
	}
	return digestCanonical(p)
}
func (p PublisherEnrollmentApproval) DigestOrEmpty() string { d, _ := p.Digest(); return d }
func (p PublisherGeneration) DigestOrEmpty() string         { d, _ := p.Digest(); return d }
func (p PublisherEnrollmentApproval) ValidateForGeneration(g PublisherGeneration, actor PrincipalRef) error {
	if _, err := p.Digest(); err != nil {
		return err
	}
	gd, err := g.Digest()
	if err != nil {
		return err
	}
	if p.ID != "publisher-enrollment-approval:"+p.PreviewDigest || p.GenerationTemplateDigest != gd || p.GenerationRecord.DigestOrEmpty() != gd || p.PublisherPrincipal != g.Principal.ID || p.PublicKeyDigest != g.PublicKeyDigest || p.KeyID != g.KeyID || p.Algorithm != g.Algorithm || p.Namespace != g.PackageNamespace || p.Generation != g.Generation || p.Predecessor != g.Predecessor || p.ApproverID != actor.ID || p.ApproverKind != actor.Kind {
		return errors.New("publisher enrollment approval does not bind exact generation and owner")
	}
	return nil
}
func NamespaceFromPublishScope(scope string) (string, error) {
	const prefix = "package-namespace:"
	if !strings.HasPrefix(scope, prefix) || len(scope) == len(prefix) {
		return "", errors.New("invalid package namespace scope")
	}
	return strings.TrimPrefix(scope, prefix), nil
}

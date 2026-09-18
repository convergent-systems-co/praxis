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
	AuthorityModelAdoptionKind             = "authority-model-adoption"
	AuthorityModelAdoptionDecisionKind     = "authority-model-adoption-decision"
	AuthorityModelAdoptionSupersessionKind = "authority-model-adoption-supersession"
	PublisherEnrollmentProposalKind        = "publisher-enrollment-proposal"
	PublisherAuthorityProposalKind         = "publisher-authority-proposal"
	PublisherAuthorityReviewKind           = "publisher-authority-review"
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
type AuthorityModelAdoptionDecision struct {
	ID, Version, Kind, BootstrapDigest, OwnerID, OwnerKind                        string
	RootRef, RootVersion, RootDigest, AdoptionID, AdoptionVersion, AdoptionDigest string
	Decision, Confirmation, ProvenanceRef, ProvenanceDigest                       string
	DecidedAt                                                                     time.Time
}
type AuthorityModelAdoptionSupersession struct {
	ID, Version, Kind, BootstrapDigest, OwnerID, OwnerKind                        string
	RootRef, RootVersion, RootDigest, AdoptionID, AdoptionVersion, AdoptionDigest string
	Decision, Reason, Confirmation, ProvenanceRef, ProvenanceDigest               string
	DecidedAt                                                                     time.Time
}
type PublisherEnrollmentApproval struct {
	ID, Version, Kind, PreviewDigest, BootstrapDigest, OwnerID, OwnerKind, AuthorityModel, AuthorityModelVersion, AuthorityModelDigest            string
	GenerationTemplateDigest, PublisherPrincipal, PublicKeyDigest, KeyID, Algorithm, Namespace, Generation, Predecessor, ApproverID, ApproverKind string
	GenerationRecord                                                                                                                              PublisherGeneration
	IssuedAt                                                                                                                                      time.Time
}
type PublisherAuthorityProposal struct {
	ID, Version, Kind, BootstrapDigest, OwnerID, OwnerKind, AuthorityModel, AuthorityModelVersion, AuthorityModelDigest                          string
	PublisherGenerationDigest, PublisherPrincipal, PublicKeyDigest, Namespace, ParentRef, ParentVersion, ParentDigest, Capability, Scope, Reason string
	ExpiresAt, CreatedAt                                                                                                                         time.Time
}
type PublisherAuthorityReview struct {
	ID, Version, Kind, ProposalID, ProposalVersion, ProposalDigest, ReviewedBy, ReviewedKind, Decision, Namespace, PublisherGenerationDigest string
	ReviewedAt                                                                                                                               time.Time
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
func (d AuthorityModelAdoptionDecision) Digest() (string, error) {
	if d.ID == "" || d.Version == "" || d.Kind != AuthorityModelAdoptionDecisionKind || d.BootstrapDigest == "" || d.OwnerID == "" || d.OwnerKind == "" || d.RootRef == "" || d.RootVersion == "" || d.RootDigest == "" || d.AdoptionID == "" || d.AdoptionVersion == "" || d.AdoptionDigest == "" || d.Decision != "approve" || d.Confirmation != "ADOPT "+d.AdoptionDigest || d.ProvenanceRef == "" || d.ProvenanceDigest == "" || d.DecidedAt.IsZero() {
		return "", errors.New("authority-model adoption decision is incomplete")
	}
	return digestCanonical(d)
}
func (s AuthorityModelAdoptionSupersession) Digest() (string, error) {
	if s.ID == "" || s.Version == "" || s.Kind != AuthorityModelAdoptionSupersessionKind || s.BootstrapDigest == "" || s.OwnerID == "" || s.OwnerKind == "" || s.RootRef == "" || s.RootVersion == "" || s.RootDigest == "" || s.AdoptionID == "" || s.AdoptionVersion == "" || s.AdoptionDigest == "" || s.Decision != "abandon" || s.Confirmation != "ABANDON "+s.AdoptionDigest || s.Reason == "" || s.ProvenanceRef == "" || s.ProvenanceDigest == "" || s.DecidedAt.IsZero() {
		return "", errors.New("authority-model adoption supersession is incomplete")
	}
	return digestCanonical(s)
}
func (p PublisherAuthorityProposal) Digest() (string, error) {
	if p.ID == "" || p.Version == "" || p.Kind != PublisherAuthorityProposalKind || p.BootstrapDigest == "" || p.OwnerID == "" || p.OwnerKind == "" || p.AuthorityModel != AuthorityModelID || p.AuthorityModelVersion != AuthorityModelSuccessorVersion || p.AuthorityModelDigest != AuthorityModelSuccessorDigest() || p.PublisherGenerationDigest == "" || p.PublisherPrincipal != FirstPartyPublisherPrincipal || p.PublicKeyDigest == "" || p.Namespace == "" || p.ParentRef == "" || p.ParentVersion == "" || p.ParentDigest == "" || p.Capability != GovernedPackagePublish || p.Scope == "" || p.Reason == "" || p.CreatedAt.IsZero() || p.ExpiresAt.IsZero() || !p.ExpiresAt.After(p.CreatedAt) {
		return "", errors.New("publisher authority proposal is incomplete")
	}
	return digestCanonical(p)
}
func (r PublisherAuthorityReview) Digest() (string, error) {
	if r.ID == "" || r.Version == "" || r.Kind != PublisherAuthorityReviewKind || r.ProposalID == "" || r.ProposalVersion == "" || r.ProposalDigest == "" || r.ReviewedBy == "" || r.ReviewedKind == "" || r.Decision != "approve" || r.Namespace == "" || r.PublisherGenerationDigest == "" || r.ReviewedAt.IsZero() {
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

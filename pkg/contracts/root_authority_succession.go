package contracts

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"time"
)

const (
	RootAuthoritySuccessionProposalKind = "root-authority-succession-proposal"
	RootAuthoritySuccessionReviewKind   = "root-authority-succession-review"
	RootAuthoritySuccessionDecisionKind = "root-authority-succession-decision"
)

var InstallationRepairRootAuthorities = []string{
	GovernedInstallationRepairRuntimeState,
	GovernedInstallationRepairStorageSchema,
}

type RootAuthoritySuccessionProposal struct {
	ID               string              `json:"id"`
	Version          string              `json:"version"`
	Kind             string              `json:"kind"`
	BootstrapDigest  string              `json:"bootstrap_digest"`
	ProposedBy       PrincipalRef        `json:"proposed_by"`
	Predecessor      AuthorityGeneration `json:"predecessor"`
	Successor        AuthorityGeneration `json:"successor"`
	AddedAuthorities []string            `json:"added_authorities"`
	Reason           string              `json:"reason"`
	CreatedAt        time.Time           `json:"created_at"`
}

type RootAuthoritySuccessionReview struct {
	ID              string       `json:"id"`
	Version         string       `json:"version"`
	Kind            string       `json:"kind"`
	ProposalID      string       `json:"proposal_id"`
	ProposalVersion string       `json:"proposal_version"`
	ProposalDigest  string       `json:"proposal_digest"`
	ReviewedBy      PrincipalRef `json:"reviewed_by"`
	Decision        string       `json:"decision"`
	Confirmation    string       `json:"confirmation"`
	ReviewedAt      time.Time    `json:"reviewed_at"`
}

type RootAuthoritySuccessionDecision struct {
	ID                 string       `json:"id"`
	Version            string       `json:"version"`
	Kind               string       `json:"kind"`
	BootstrapDigest    string       `json:"bootstrap_digest"`
	ProposalID         string       `json:"proposal_id"`
	ProposalVersion    string       `json:"proposal_version"`
	ProposalDigest     string       `json:"proposal_digest"`
	ReviewID           string       `json:"review_id"`
	ReviewVersion      string       `json:"review_version"`
	ReviewDigest       string       `json:"review_digest"`
	PredecessorRef     string       `json:"predecessor_ref"`
	PredecessorVersion string       `json:"predecessor_version"`
	PredecessorDigest  string       `json:"predecessor_digest"`
	SuccessorRef       string       `json:"successor_ref"`
	SuccessorVersion   string       `json:"successor_version"`
	SuccessorDigest    string       `json:"successor_digest"`
	DecidedBy          PrincipalRef `json:"decided_by"`
	Decision           string       `json:"decision"`
	Confirmation       string       `json:"confirmation"`
	DecidedAt          time.Time    `json:"decided_at"`
}

type InstallationRepairAuthorityRequest struct {
	BootstrapDigest          string    `json:"bootstrap_digest"`
	RootRef                  string    `json:"root_ref"`
	RootVersion              string    `json:"root_version"`
	RootDigest               string    `json:"root_digest"`
	SuccessionDecisionDigest string    `json:"succession_decision_digest"`
	Operation                string    `json:"operation"`
	ExpiresAt                time.Time `json:"expires_at"`
}

func BuildRootAuthoritySuccession(predecessor AuthorityGeneration, bootstrapDigest string, now time.Time) (RootAuthoritySuccessionProposal, error) {
	if err := predecessor.VerifyDigest(); err != nil {
		return RootAuthoritySuccessionProposal{}, err
	}
	owner, err := InstallationOwnerPrincipal(bootstrapDigest)
	if err != nil {
		return RootAuthoritySuccessionProposal{}, err
	}
	scope, err := InstallationGovernanceScope(bootstrapDigest)
	if err != nil {
		return RootAuthoritySuccessionProposal{}, err
	}
	version, err := strconv.Atoi(predecessor.Version)
	if err != nil || version < 1 {
		return RootAuthoritySuccessionProposal{}, errors.New("root predecessor version is not numeric")
	}
	if predecessor.Principal != owner || predecessor.Ref != scope || predecessor.Scope != scope || predecessor.ProvenanceDigest != bootstrapDigest || predecessor.ParentRef != "" || predecessor.DelegatedBy != (PrincipalRef{}) {
		return RootAuthoritySuccessionProposal{}, errors.New("root predecessor is not the canonical installation root")
	}
	for _, authority := range InstallationRepairRootAuthorities {
		if slices.Contains(predecessor.Authorities, authority) {
			return RootAuthoritySuccessionProposal{}, errors.New("root predecessor already contains installation-repair authority")
		}
	}
	successor := predecessor
	successor.Version = strconv.Itoa(version + 1)
	successor.Digest = ""
	successor.EffectiveAt = now.UTC()
	successor.PredecessorRef = predecessor.Ref
	successor.PredecessorVersion = predecessor.Version
	successor.PredecessorDigest = predecessor.Digest
	successor.Authorities = append([]string(nil), predecessor.Authorities...)
	for _, authority := range InstallationRepairRootAuthorities {
		if !slices.Contains(successor.Authorities, authority) {
			successor.Authorities = append(successor.Authorities, authority)
		}
	}
	slices.Sort(successor.Authorities)
	successor.Authorities = slices.Compact(successor.Authorities)
	successor.Digest, err = successor.ComputeDigest()
	if err != nil {
		return RootAuthoritySuccessionProposal{}, err
	}
	proposal := RootAuthoritySuccessionProposal{
		ID: "root-authority-succession:" + predecessor.Digest, Version: "1", Kind: RootAuthoritySuccessionProposalKind,
		BootstrapDigest: bootstrapDigest, ProposedBy: owner, Predecessor: predecessor, Successor: successor,
		AddedAuthorities: append([]string(nil), InstallationRepairRootAuthorities...), Reason: "add ADR-088 installation-repair root authorities", CreatedAt: now.UTC(),
	}
	if _, err := proposal.Digest(); err != nil {
		return RootAuthoritySuccessionProposal{}, err
	}
	return proposal, nil
}

func (p RootAuthoritySuccessionProposal) Digest() (string, error) {
	if p.ID == "" || p.Version == "" || p.Kind != RootAuthoritySuccessionProposalKind || p.Reason == "" || p.CreatedAt.IsZero() {
		return "", errors.New("root-authority succession proposal is incomplete")
	}
	owner, err := InstallationOwnerPrincipal(p.BootstrapDigest)
	if err != nil || p.ProposedBy != owner {
		return "", errors.New("root-authority succession proposer is not the installation owner")
	}
	expected, err := BuildRootAuthoritySuccessionUnchecked(p.Predecessor, p.BootstrapDigest, p.CreatedAt)
	if err != nil || expected.Successor.Digest != p.Successor.Digest || !reflect.DeepEqual(expected.Successor, p.Successor) || !slices.Equal(p.AddedAuthorities, InstallationRepairRootAuthorities) {
		return "", errors.New("root-authority succession proposal does not bind the closed successor")
	}
	return digestCanonical(p)
}

// BuildRootAuthoritySuccessionUnchecked avoids recursive proposal validation.
func BuildRootAuthoritySuccessionUnchecked(predecessor AuthorityGeneration, bootstrapDigest string, now time.Time) (RootAuthoritySuccessionProposal, error) {
	if err := predecessor.VerifyDigest(); err != nil {
		return RootAuthoritySuccessionProposal{}, err
	}
	owner, err := InstallationOwnerPrincipal(bootstrapDigest)
	if err != nil {
		return RootAuthoritySuccessionProposal{}, err
	}
	scope, err := InstallationGovernanceScope(bootstrapDigest)
	if err != nil {
		return RootAuthoritySuccessionProposal{}, err
	}
	version, err := strconv.Atoi(predecessor.Version)
	if err != nil || predecessor.Principal != owner || predecessor.Ref != scope || predecessor.Scope != scope || predecessor.ProvenanceDigest != bootstrapDigest || predecessor.ParentRef != "" || predecessor.DelegatedBy != (PrincipalRef{}) {
		return RootAuthoritySuccessionProposal{}, errors.New("invalid canonical root predecessor")
	}
	for _, authority := range InstallationRepairRootAuthorities {
		if slices.Contains(predecessor.Authorities, authority) {
			return RootAuthoritySuccessionProposal{}, errors.New("root predecessor already contains installation-repair authority")
		}
	}
	successor := predecessor
	successor.Version, successor.Digest, successor.EffectiveAt = strconv.Itoa(version+1), "", now.UTC()
	successor.PredecessorRef, successor.PredecessorVersion, successor.PredecessorDigest = predecessor.Ref, predecessor.Version, predecessor.Digest
	successor.Authorities = append([]string(nil), predecessor.Authorities...)
	for _, a := range InstallationRepairRootAuthorities {
		if !slices.Contains(successor.Authorities, a) {
			successor.Authorities = append(successor.Authorities, a)
		}
	}
	slices.Sort(successor.Authorities)
	successor.Authorities = slices.Compact(successor.Authorities)
	successor.Digest, err = successor.ComputeDigest()
	return RootAuthoritySuccessionProposal{Successor: successor}, err
}

func (r RootAuthoritySuccessionReview) Digest() (string, error) {
	if r.ID == "" || r.Version == "" || r.Kind != RootAuthoritySuccessionReviewKind || r.ProposalID == "" || r.ProposalVersion == "" || r.ProposalDigest == "" || r.Decision != "approve" || r.Confirmation != "REVIEW-ROOT-SUCCESSOR "+r.ProposalDigest || r.ReviewedAt.IsZero() || r.ReviewedBy.Kind != "human" {
		return "", errors.New("root-authority succession review is incomplete or not human")
	}
	return digestCanonical(r)
}

func (d RootAuthoritySuccessionDecision) Digest() (string, error) {
	if d.ID == "" || d.Version == "" || d.Kind != RootAuthoritySuccessionDecisionKind || d.BootstrapDigest == "" || d.ProposalID == "" || d.ProposalVersion == "" || d.ProposalDigest == "" || d.ReviewID == "" || d.ReviewVersion == "" || d.ReviewDigest == "" || d.PredecessorRef == "" || d.PredecessorVersion == "" || d.PredecessorDigest == "" || d.SuccessorRef == "" || d.SuccessorVersion == "" || d.SuccessorDigest == "" || d.Decision != "approve" || d.DecidedAt.IsZero() || d.DecidedBy.Kind != "human" || d.Confirmation != "ACCEPT-ROOT-SUCCESSOR "+d.ProposalDigest+" "+d.ReviewDigest {
		return "", errors.New("root-authority succession decision is incomplete or not human")
	}
	owner, err := InstallationOwnerPrincipal(d.BootstrapDigest)
	if err != nil || d.DecidedBy != owner {
		return "", errors.New("root-authority succession decision is not installation-owner approval")
	}
	return digestCanonical(d)
}

func (r InstallationRepairAuthorityRequest) Validate(now time.Time) error {
	if err := ValidateSHA256Digest(r.BootstrapDigest); err != nil {
		return err
	}
	if r.RootRef == "" || r.RootVersion == "" || r.RootDigest == "" || r.SuccessionDecisionDigest == "" || r.ExpiresAt.IsZero() || !now.Before(r.ExpiresAt) {
		return errors.New("installation-repair request lineage or expiry is incomplete")
	}
	if r.Operation != GovernedInstallationRepairStorageSchema && r.Operation != GovernedInstallationRepairRuntimeState {
		return fmt.Errorf("unsupported installation-repair operation %q", r.Operation)
	}
	return nil
}

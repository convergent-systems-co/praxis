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

	// HistoricalRootModernizationProposalKind is the closed succession that
	// establishes the first current canonical installation root from a
	// historically valid schema-11 enrollment root (ADR-090). It reuses the
	// ADR-089 durable objects and atomic transition but derives a different
	// closed successor and is never interchangeable with repair succession.
	HistoricalRootModernizationProposalKind = "historical-root-modernization-proposal"

	// LegacyRootEnrollmentSchema is the last storage schema at which an
	// installation root could have been enrolled by the original
	// `praxis authority bootstrap --scope <least-scope>` boundary (commit
	// 5850f27), before DelegatedBy, Capabilities, the authority model, and
	// the installation-governance scope existed (commit ae6fd0f, still at
	// schema 11; migration 0012 followed). Such a root is recognisable by its
	// pre-delegation persisted representation.
	LegacyRootEnrollmentSchema = 11
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
	// HistoricalSchema is set only by historical-root modernization and
	// records the source schema whose root semantics the predecessor was
	// valid under. It is absent (zero) for ADR-089 repair succession.
	HistoricalSchema int `json:"historical_schema,omitempty"`
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
	if predecessor.PreDelegationForm() {
		return RootAuthoritySuccessionProposal{}, errors.New("root predecessor is a historical enrollment root; establish current authority through historical-root modernization first")
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
	successor.preDelegationForm = false
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
	if p.ID == "" || p.Version == "" || p.Reason == "" || p.CreatedAt.IsZero() {
		return "", errors.New("root-authority succession proposal is incomplete")
	}
	owner, err := InstallationOwnerPrincipal(p.BootstrapDigest)
	if err != nil || p.ProposedBy != owner {
		return "", errors.New("root-authority succession proposer is not the installation owner")
	}
	switch p.Kind {
	case RootAuthoritySuccessionProposalKind:
		expected, err := BuildRootAuthoritySuccessionUnchecked(p.Predecessor, p.BootstrapDigest, p.CreatedAt)
		if err != nil || p.HistoricalSchema != 0 || expected.Successor.Digest != p.Successor.Digest || !reflect.DeepEqual(expected.Successor, p.Successor) || !slices.Equal(p.AddedAuthorities, InstallationRepairRootAuthorities) {
			return "", errors.New("root-authority succession proposal does not bind the closed successor")
		}
	case HistoricalRootModernizationProposalKind:
		expected, err := buildHistoricalRootModernizationUnchecked(p.Predecessor, p.BootstrapDigest, p.CreatedAt)
		if err != nil || p.ID != expected.ID || p.HistoricalSchema != LegacyRootEnrollmentSchema || expected.Successor.Digest != p.Successor.Digest || !reflect.DeepEqual(expected.Successor, p.Successor) || len(p.AddedAuthorities) != 0 {
			return "", errors.New("historical-root modernization proposal does not bind the closed successor")
		}
	default:
		return "", fmt.Errorf("unknown root-authority succession proposal kind %q", p.Kind)
	}
	return digestCanonical(p)
}

// BuildHistoricalRootModernization derives the closed ADR-090 successor of a
// historically valid schema-11 enrollment root. The predecessor must be the
// exact pre-delegation persisted record bound to this installation: ref and
// principal derived from the bootstrap digest, provenance bound to the
// bootstrap record, no parent or delegation, an owner-declared least scope,
// and no repair authority. The successor is the first current canonical
// installation root: it advances the numeric version, binds the exact
// predecessor triple, adopts the installation-governance scope, the
// authority.delegate capability and built-in authority model v1, retains the
// predecessor's provenance and existing authorities, and adds nothing else.
// Migration preserved the predecessor as historical truth; this succession
// alone establishes current authority.
func BuildHistoricalRootModernization(predecessor AuthorityGeneration, bootstrapDigest string, now time.Time) (RootAuthoritySuccessionProposal, error) {
	proposal, err := buildHistoricalRootModernizationUnchecked(predecessor, bootstrapDigest, now)
	if err != nil {
		return RootAuthoritySuccessionProposal{}, err
	}
	if _, err := proposal.Digest(); err != nil {
		return RootAuthoritySuccessionProposal{}, err
	}
	return proposal, nil
}

func buildHistoricalRootModernizationUnchecked(predecessor AuthorityGeneration, bootstrapDigest string, now time.Time) (RootAuthoritySuccessionProposal, error) {
	if err := predecessor.VerifyDigest(); err != nil {
		return RootAuthoritySuccessionProposal{}, err
	}
	if !predecessor.PreDelegationForm() {
		return RootAuthoritySuccessionProposal{}, errors.New("historical-root modernization requires a pre-delegation enrollment root as predecessor")
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
	if predecessor.Principal != owner || predecessor.Ref != scope || predecessor.Scope == "" || predecessor.ProvenanceDigest != bootstrapDigest || predecessor.ParentRef != "" || predecessor.DelegatedBy != (PrincipalRef{}) || predecessor.PredecessorRef != "" || predecessor.State != AuthorityGenerationActive {
		return RootAuthoritySuccessionProposal{}, errors.New("historical root predecessor is not this installation's enrollment root")
	}
	for _, authority := range InstallationRepairRootAuthorities {
		if slices.Contains(predecessor.Authorities, authority) {
			return RootAuthoritySuccessionProposal{}, errors.New("historical root predecessor cannot carry installation-repair authority")
		}
	}
	successor := AuthorityGeneration{
		Ref: scope, Version: strconv.Itoa(version + 1), Principal: owner, Scope: scope,
		Capabilities:  []string{AuthorityDelegateCapability},
		ProvenanceRef: predecessor.ProvenanceRef, ProvenanceDigest: predecessor.ProvenanceDigest,
		State: AuthorityGenerationActive, EffectiveAt: now.UTC(),
		PredecessorRef: predecessor.Ref, PredecessorVersion: predecessor.Version, PredecessorDigest: predecessor.Digest,
		AuthorityModel: AuthorityModelID, AuthorityModelVersion: AuthorityModelVersion, AuthorityModelDigest: AuthorityModelDigest(),
		Authorities: append([]string(nil), predecessor.Authorities...),
	}
	for _, capability := range predecessor.Capabilities {
		if !slices.Contains(successor.Capabilities, capability) {
			successor.Capabilities = append(successor.Capabilities, capability)
		}
	}
	slices.Sort(successor.Capabilities)
	slices.Sort(successor.Authorities)
	successor.Authorities = slices.Compact(successor.Authorities)
	if len(successor.Authorities) == 0 {
		successor.Authorities = nil
	}
	successor.Digest, err = successor.ComputeDigest()
	if err != nil {
		return RootAuthoritySuccessionProposal{}, err
	}
	return RootAuthoritySuccessionProposal{
		ID: "historical-root-modernization:" + predecessor.Digest, Version: "1", Kind: HistoricalRootModernizationProposalKind,
		BootstrapDigest: bootstrapDigest, ProposedBy: owner, Predecessor: predecessor, Successor: successor,
		AddedAuthorities: nil, Reason: "establish current canonical installation root from historically valid schema-11 enrollment root", CreatedAt: now.UTC(),
		HistoricalSchema: LegacyRootEnrollmentSchema,
	}, nil
}

// IsHistoricalRootModernization reports whether the proposal is the ADR-090
// transition from a historical enrollment root to the first current root.
func (p RootAuthoritySuccessionProposal) IsHistoricalRootModernization() bool {
	return p.Kind == HistoricalRootModernizationProposalKind
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
	successor.preDelegationForm = false
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

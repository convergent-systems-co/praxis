package contracts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"
)

// OwnerCeremonyProfile is the only ceremony profile protected decisions accept.
const OwnerCeremonyProfile = "interactive-os-owner-v1"

// OwnerCeremonyEvidence is the durable record of one completed interactive
// owner ceremony. A protected AuthorityDecision does not carry a
// caller-selected digest asserting that a ceremony happened: its
// CeremonyEvidenceDigest must be this record's digest, and the record must
// resolve from durable state and match the decision field for field. Copying
// the owner, root, or request identifiers into a decision is therefore not
// proof; an unresolvable or mismatched ceremony record is.
type OwnerCeremonyEvidence struct {
	Profile             string       `json:"profile"`
	RequestDigest       string       `json:"request_digest"`
	Outcome             string       `json:"outcome"`
	SelectedAlternative string       `json:"selected_alternative,omitempty"`
	Owner               PrincipalRef `json:"owner"`
	RootRef             string       `json:"root_ref"`
	RootVersion         string       `json:"root_version"`
	RootDigest          string       `json:"root_digest"`
	AuthenticatedOSUser string       `json:"authenticated_os_user"`
	ConfirmationDigest  string       `json:"confirmation_digest"`
	ConfirmedAt         time.Time    `json:"confirmed_at"`
}

func (e OwnerCeremonyEvidence) Validate() error {
	if e.Profile != OwnerCeremonyProfile {
		return errors.New("ceremony evidence names an unsupported profile")
	}
	if err := ValidateSHA256Digest(e.RequestDigest); err != nil {
		return err
	}
	if err := ValidateSHA256Digest(e.ConfirmationDigest); err != nil {
		return err
	}
	if e.Outcome != string(AuthorityApprove) && e.Outcome != string(AuthorityReject) {
		return errors.New("ceremony evidence outcome is not approve or reject")
	}
	if e.Owner.Kind != "human" || e.Owner.Validate() != nil || e.RootRef == "" || e.RootVersion == "" || e.RootDigest == "" || e.AuthenticatedOSUser == "" || e.ConfirmedAt.IsZero() {
		return errors.New("ceremony evidence lacks the owner, root, authenticated user, or confirmation time")
	}
	return nil
}

func (e OwnerCeremonyEvidence) Digest() (string, error) {
	if err := e.Validate(); err != nil {
		return "", err
	}
	payload, err := json.Marshal(e)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// BindsDecision reports whether the ceremony is exactly the one that produced
// the decision: same request, outcome, alternative, owner, and root lineage.
func (e OwnerCeremonyEvidence) BindsDecision(d AuthorityDecision) bool {
	return e.RequestDigest == d.RequestDigest && e.Outcome == string(d.Outcome) && e.SelectedAlternative == d.SelectedAlternative && e.Owner == d.DecidedBy && e.RootRef == d.AuthorityRef && e.RootVersion == d.AuthorityVersion && e.RootDigest == d.AuthorityGenerationDigest
}

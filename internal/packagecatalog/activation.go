package packagecatalog

import (
	"encoding/json"
	"errors"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// NewActivationIntent binds local authority to one exact verified generation
// and its reviewed transitive capability/enforcement surface. Verification
// evidence informs this decision but cannot authorize it.
func NewActivationIntent(pkg VerifiedPackage, actor contracts.PrincipalRef) (contracts.ActionIntent, error) {
	if err := pkg.Validate(); err != nil {
		return contracts.ActionIntent{}, err
	}
	if err := actor.Validate(); err != nil {
		return contracts.ActionIntent{}, err
	}
	evidence := pkg.Evidence()
	capabilityDigest, err := digestStrings(evidence.EffectiveCapabilities)
	if err != nil {
		return contracts.ActionIntent{}, err
	}
	enforcementDigest, err := digestStrings(evidence.RequiredEnforcement)
	if err != nil {
		return contracts.ActionIntent{}, err
	}
	intent := contracts.ActionIntent{
		Version:   activationIntentVersions.CurrentVersion(),
		ID:        "package-activation:" + evidence.ID,
		Actor:     actor,
		Operation: "package.activate",
		Target:    pkg.Manifest().PackageID + "@" + pkg.Manifest().Version + "#" + pkg.Manifest().ContentDigest,
		Parameters: map[string]string{
			"verification_id":               evidence.ID,
			"manifest_digest":               evidence.ManifestDigest,
			"effective_capabilities_digest": capabilityDigest,
			"required_enforcement_digest":   enforcementDigest,
		},
		Scope:         "package:" + pkg.Manifest().PackageID,
		CryptoProfile: evidence.SignatureProfile,
	}
	return intent, intent.Validate()
}

type ActivationRequest struct {
	Package    VerifiedPackage
	Intent     contracts.ActionIntent
	ApprovalID string
}

func (r ActivationRequest) Validate() error {
	if r.ApprovalID == "" {
		return errors.New("package activation approval id is required")
	}
	expected, err := NewActivationIntent(r.Package, r.Intent.Actor)
	if err != nil {
		return err
	}
	want, err := expected.Digest()
	if err != nil {
		return err
	}
	got, err := r.Intent.Digest()
	if err != nil {
		return err
	}
	if got != want {
		return errors.New("package activation intent does not bind verified package evidence")
	}
	return nil
}

func digestStrings(values []string) (string, error) {
	body, err := json.Marshal(canonicalStrings(values))
	if err != nil {
		return "", err
	}
	return bytesDigest(body), nil
}

package packagecatalog

import "github.com/convergent-systems-co/praxis/pkg/contracts"

var (
	signatureEnvelopeVersions    = packageContract("package.signature_envelope", "v1")
	verificationEvidenceVersions = packageContract("package.verification_evidence", "v1")
	activationIntentVersions     = packageContract("package.activation_intent", "v1")
)

func packageContract(name, current string) *contracts.VersionRegistry {
	return contracts.MustVersionRegistry(contracts.ContractVersionPolicy{
		Contract: name, CurrentVersion: current,
		Versions: []contracts.ContractVersionDefinition{{Version: current, Disposition: contracts.VersionCurrent}},
	}, nil)
}

func SignatureEnvelopeCurrentVersion() string { return signatureEnvelopeVersions.CurrentVersion() }
func VerificationEvidenceCurrentVersion() string {
	return verificationEvidenceVersions.CurrentVersion()
}
func ActivationIntentCurrentVersion() string { return activationIntentVersions.CurrentVersion() }

func requirePackageContractVersion(registry *contracts.VersionRegistry, version string) error {
	_, _, err := registry.Canonicalize(version, nil)
	return err
}

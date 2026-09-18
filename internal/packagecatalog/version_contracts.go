package packagecatalog

import "github.com/convergent-systems-co/praxis/pkg/contracts"

var (
	packageVersionCatalog = contracts.NewVersionCatalog()

	manifestContractPolicy = contracts.ContractVersionPolicy{
		Contract: "package.manifest", CurrentVersion: "v1",
		Versions: []contracts.ContractVersionDefinition{{Version: "v1", Disposition: contracts.VersionCurrent}},
	}
	signatureEnvelopePolicy = contracts.ContractVersionPolicy{
		Contract: "package.signature_envelope", CurrentVersion: "v1",
		Versions: []contracts.ContractVersionDefinition{{Version: "v1", Disposition: contracts.VersionCurrent}},
	}
	verificationEvidencePolicy = contracts.ContractVersionPolicy{
		Contract: "package.verification_evidence", CurrentVersion: "v1",
		Versions: []contracts.ContractVersionDefinition{{Version: "v1", Disposition: contracts.VersionCurrent}},
	}
	activationIntentPolicy = contracts.ContractVersionPolicy{
		Contract: "package.activation_intent", CurrentVersion: "v1",
		Versions: []contracts.ContractVersionDefinition{{Version: "v1", Disposition: contracts.VersionCurrent}},
	}
	deploymentIntentPolicy = contracts.ContractVersionPolicy{
		Contract: "package.deployment_intent", CurrentVersion: "v1",
		Versions: []contracts.ContractVersionDefinition{{Version: "v1", Disposition: contracts.VersionCurrent}},
	}
	packageTransitionIntentPolicy = contracts.ContractVersionPolicy{
		Contract: "package.transition_intent", CurrentVersion: "v1",
		Versions: []contracts.ContractVersionDefinition{{Version: "v1", Disposition: contracts.VersionCurrent}},
	}
	packageRollbackIntentPolicy = contracts.ContractVersionPolicy{
		Contract: "package.rollback_intent", CurrentVersion: "v1",
		Versions: []contracts.ContractVersionDefinition{{Version: "v1", Disposition: contracts.VersionCurrent}},
	}
	catalogContributionEvidencePolicy = contracts.ContractVersionPolicy{
		Contract: "package.catalog_contribution_evidence", CurrentVersion: "v1",
		Versions: []contracts.ContractVersionDefinition{{Version: "v1", Disposition: contracts.VersionCurrent}},
	}

	manifestContractVersions        = packageVersionCatalog.MustRegister(manifestContractPolicy, nil)
	signatureEnvelopeVersions       = packageVersionCatalog.MustRegister(signatureEnvelopePolicy, nil)
	verificationEvidenceVersions    = packageVersionCatalog.MustRegister(verificationEvidencePolicy, nil)
	activationIntentVersions        = packageVersionCatalog.MustRegister(activationIntentPolicy, nil)
	deploymentIntentVersions        = packageVersionCatalog.MustRegister(deploymentIntentPolicy, nil)
	packageTransitionIntentVersions = packageVersionCatalog.MustRegister(packageTransitionIntentPolicy, nil)
	packageRollbackIntentVersions   = packageVersionCatalog.MustRegister(packageRollbackIntentPolicy, nil)
	catalogContributionVersions     = packageVersionCatalog.MustRegister(catalogContributionEvidencePolicy, nil)
)

func SignatureEnvelopeCurrentVersion() string { return signatureEnvelopeVersions.CurrentVersion() }
func ManifestContractCurrentVersion() string  { return manifestContractVersions.CurrentVersion() }
func VerificationEvidenceCurrentVersion() string {
	return verificationEvidenceVersions.CurrentVersion()
}
func ActivationIntentCurrentVersion() string { return activationIntentVersions.CurrentVersion() }
func DeploymentIntentCurrentVersion() string { return deploymentIntentVersions.CurrentVersion() }

func requirePackageContractVersion(registry *contracts.VersionRegistry, version string) error {
	_, _, err := registry.Canonicalize(version, nil)
	return err
}

package packagecatalog

import (
	"errors"
	"testing"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func TestPackageContractVersionsAreRegistryGovernedAndFailClosed(t *testing.T) {
	registries := map[string]*contracts.VersionRegistry{
		"manifest":     manifestContractVersions,
		"signature":    signatureEnvelopeVersions,
		"verification": verificationEvidenceVersions,
		"activation":   activationIntentVersions,
		"transition":   packageTransitionIntentVersions,
	}
	for name, registry := range registries {
		if _, _, err := registry.Canonicalize(registry.CurrentVersion(), []byte("canonical")); err != nil {
			t.Fatalf("%s current contract rejected: %v", name, err)
		}
		if _, _, err := registry.Canonicalize("future-unknown", nil); !errors.Is(err, contracts.ErrUnknownContractVersion) {
			t.Fatalf("%s unknown contract did not fail closed: %v", name, err)
		}
		definition, ok := registry.Definition(registry.CurrentVersion())
		if !ok || definition.Disposition != contracts.VersionCurrent || registry.PolicyDigest() == "" {
			t.Fatalf("%s compatibility is not represented by durable contract metadata", name)
		}
	}
}

func TestPackageContractFamilyHasOneCanonicalOwnerPerIdentity(t *testing.T) {
	for _, policy := range []contracts.ContractVersionPolicy{manifestContractPolicy, signatureEnvelopePolicy, verificationEvidencePolicy, activationIntentPolicy, packageTransitionIntentPolicy} {
		owned, ok := packageVersionCatalog.Lookup(policy.Contract)
		if !ok || owned.PolicyDigest() == "" || owned.CurrentVersion() != policy.CurrentVersion {
			t.Fatalf("package contract %s is not owned by the family catalog", policy.Contract)
		}
		if _, err := packageVersionCatalog.Register(policy, nil); !errors.Is(err, contracts.ErrContractPolicyAlreadyRegistered) {
			t.Fatalf("package contract %s accepted duplicate semantic ownership: %v", policy.Contract, err)
		}
	}
}

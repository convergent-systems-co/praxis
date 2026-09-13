package packagecatalog

import (
	"errors"
	"testing"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func TestPackageContractVersionsAreRegistryGovernedAndFailClosed(t *testing.T) {
	registries := map[string]*contracts.VersionRegistry{
		"signature":    signatureEnvelopeVersions,
		"verification": verificationEvidenceVersions,
		"activation":   activationIntentVersions,
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

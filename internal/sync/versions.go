package sync

import "github.com/convergent-systems-co/praxis/pkg/contracts"

func currentVersionPolicy(name string) *contracts.VersionRegistry {
	return contracts.MustVersionRegistry(contracts.ContractVersionPolicy{
		Contract:       name,
		CurrentVersion: "v1",
		Versions: []contracts.ContractVersionDefinition{
			{Version: "v1", Disposition: contracts.VersionCurrent},
		},
	}, nil)
}

var (
	portableRecordVersions   = currentVersionPolicy("praxis.portable-state.record")
	portableEnvelopeVersions = currentVersionPolicy("praxis.portable-state.envelope")
	portableConflictVersions = currentVersionPolicy("praxis.portable-state.conflict")
	portableEventVersions    = currentVersionPolicy("praxis.portable-state.event")
)

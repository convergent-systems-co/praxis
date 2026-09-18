package adaptation

import "github.com/convergent-systems-co/praxis/pkg/contracts"

var (
	observationEventContract = contracts.MustVersionRegistry(contracts.ContractVersionPolicy{
		Contract:       "adaptive.observation",
		CurrentVersion: "v2",
		Versions: []contracts.ContractVersionDefinition{
			{Version: "v2", Disposition: contracts.VersionCurrent},
			{Version: "v1", Disposition: contracts.VersionUnsupportedPreRelease, Rationale: "adaptive observation v1 was never released; durable support begins at v2"},
		},
	}, nil)
	profileFactEventContract = contracts.MustVersionRegistry(contracts.ContractVersionPolicy{
		Contract:       "adaptive.profile_fact",
		CurrentVersion: "v2",
		Versions: []contracts.ContractVersionDefinition{
			{Version: "v2", Disposition: contracts.VersionCurrent},
			{Version: "v1", Disposition: contracts.VersionUnsupportedPreRelease, Rationale: "adaptive profile fact v1 was never released; durable support begins at v2"},
		},
	}, nil)
	measurementEventContract = contracts.MustVersionRegistry(contracts.ContractVersionPolicy{
		Contract:       "adaptive.measurement",
		CurrentVersion: "v1",
		Versions:       []contracts.ContractVersionDefinition{{Version: "v1", Disposition: contracts.VersionCurrent}},
	}, nil)
	analysisEventContract = contracts.MustVersionRegistry(contracts.ContractVersionPolicy{
		Contract:       "adaptive.analysis",
		CurrentVersion: "v1",
		Versions:       []contracts.ContractVersionDefinition{{Version: "v1", Disposition: contracts.VersionCurrent}},
	}, nil)
	profileDerivationEventContract = contracts.MustVersionRegistry(contracts.ContractVersionPolicy{Contract: "adaptive.profile_derivation", CurrentVersion: "v1", Versions: []contracts.ContractVersionDefinition{{Version: "v1", Disposition: contracts.VersionCurrent}}}, nil)
	profileDivergenceEventContract = contracts.MustVersionRegistry(contracts.ContractVersionPolicy{Contract: "adaptive.profile_divergence", CurrentVersion: "v1", Versions: []contracts.ContractVersionDefinition{{Version: "v1", Disposition: contracts.VersionCurrent}}}, nil)
)

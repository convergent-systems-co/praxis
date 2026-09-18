package preference

import "github.com/convergent-systems-co/praxis/pkg/contracts"

var (
	preferenceContractVersions  = contracts.MustVersionRegistry(contracts.ContractVersionPolicy{Contract: "preference.contract", CurrentVersion: "v1", Versions: []contracts.ContractVersionDefinition{{Version: "v1", Disposition: contracts.VersionCurrent}}}, nil)
	preferenceRecordVersions    = contracts.MustVersionRegistry(contracts.ContractVersionPolicy{Contract: "preference.record", CurrentVersion: "v1", Versions: []contracts.ContractVersionDefinition{{Version: "v1", Disposition: contracts.VersionCurrent}}}, nil)
	preferenceEventVersions     = contracts.MustVersionRegistry(contracts.ContractVersionPolicy{Contract: "preference.recorded_event", CurrentVersion: "v1", Versions: []contracts.ContractVersionDefinition{{Version: "v1", Disposition: contracts.VersionCurrent}}}, nil)
	preferenceMigrationVersions = contracts.MustVersionRegistry(contracts.ContractVersionPolicy{Contract: "preference.migration", CurrentVersion: "v1", Versions: []contracts.ContractVersionDefinition{{Version: "v1", Disposition: contracts.VersionCurrent}}}, nil)
)

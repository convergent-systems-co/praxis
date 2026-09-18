package plugin

import "github.com/convergent-systems-co/praxis/pkg/contracts"

var (
	pluginVersionCatalog  = contracts.NewVersionCatalog()
	manifestVersionPolicy = contracts.ContractVersionPolicy{
		Contract:       "plugin.manifest",
		CurrentVersion: "v2",
		Versions: []contracts.ContractVersionDefinition{
			{Version: "v1", Disposition: contracts.VersionUnsupportedPreRelease, Rationale: "the pre-release schema did not bind package definition identity to separately verified executable content"},
			{Version: "v2", Disposition: contracts.VersionCurrent},
		},
	}
	manifestVersions = pluginVersionCatalog.MustRegister(manifestVersionPolicy, nil)
)

func ManifestContractCurrentVersion() string { return manifestVersions.CurrentVersion() }

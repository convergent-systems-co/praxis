package contracts

import "errors"

var (
	clientContractCatalog    = NewVersionCatalog()
	invocationContractPolicy = ContractVersionPolicy{
		Contract: "client.invocation", CurrentVersion: "v1",
		Versions: []ContractVersionDefinition{
			{Version: "v1", Disposition: VersionCurrent},
			{Version: "1", Disposition: VersionSupportedHistorical},
		},
	}
	invocationContractVersions = clientContractCatalog.MustRegister(invocationContractPolicy, nil)
)

func InvocationContractCurrentVersion() string { return invocationContractVersions.CurrentVersion() }

type InvocationOption struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Required    bool   `json:"required,omitempty"`
	Default     string `json:"default,omitempty"`
	Description string `json:"description,omitempty"`
}

type InvocationContract struct {
	Version                   string             `json:"version"`
	PackageID                 string             `json:"package_id"`
	PackageVersion            string             `json:"package_version"`
	GraphID                   string             `json:"graph_id"`
	GraphVersion              string             `json:"graph_version"`
	EntryPointID              string             `json:"entry_point_id"`
	Aliases                   []string           `json:"aliases"`
	Options                   []InvocationOption `json:"options,omitempty"`
	RequiredCapabilities      []string           `json:"required_capabilities,omitempty"`
	OptionalCapabilities      []string           `json:"optional_capabilities,omitempty"`
	RequiredEnforcement       []string           `json:"required_enforcement,omitempty"`
	RequireExclusiveMediation bool               `json:"require_exclusive_mediation,omitempty"`
}

func (c InvocationContract) Validate() error {
	if _, _, err := invocationContractVersions.Canonicalize(c.Version, nil); err != nil {
		return err
	}
	if c.PackageID == "" || c.PackageVersion == "" || c.GraphID == "" || c.GraphVersion == "" || c.EntryPointID == "" {
		return errors.New("invocation contract identity/version fields are required")
	}
	if len(c.Aliases) == 0 {
		return errors.New("at least one invocation alias is required")
	}
	aliases := map[string]struct{}{}
	for _, alias := range c.Aliases {
		if alias == "" {
			return errors.New("invocation alias cannot be empty")
		}
		if _, ok := aliases[alias]; ok {
			return errors.New("duplicate invocation alias")
		}
		aliases[alias] = struct{}{}
	}
	options := map[string]struct{}{}
	for _, option := range c.Options {
		if option.Name == "" || option.Type == "" {
			return errors.New("invocation option name/type are required")
		}
		if _, ok := options[option.Name]; ok {
			return errors.New("duplicate invocation option")
		}
		options[option.Name] = struct{}{}
	}
	return nil
}

package contracts

import "errors"

type InvocationOption struct {
	Name        string
	Type        string
	Required    bool
	Default     string
	Description string
}

type InvocationContract struct {
	Version                     string
	PackageID                   string
	PackageVersion              string
	GraphID                     string
	GraphVersion                string
	EntryPointID                string
	Aliases                     []string
	Options                     []InvocationOption
	RequiredCapabilities        []string
	OptionalCapabilities        []string
	RequiredEnforcement         []string
	RequireExclusiveMediation   bool
}

func (c InvocationContract) Validate() error {
	if c.Version == "" || c.PackageID == "" || c.PackageVersion == "" || c.GraphID == "" || c.GraphVersion == "" || c.EntryPointID == "" {
		return errors.New("invocation contract identity/version fields are required")
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

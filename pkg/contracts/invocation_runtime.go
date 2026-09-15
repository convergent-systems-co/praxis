package contracts

import (
	"errors"
	"fmt"
)

// ExecutableBinding identifies the package-owned implementation behind one
// invocation. Contract identity and implementation identity are intentionally
// separate so a contract digest cannot masquerade as executable provenance.
type ExecutableBinding struct {
	EntryPointID             string `json:"entry_point_id"`
	PluginID                 string `json:"plugin_id"`
	PluginVersion            string `json:"plugin_version"`
	PluginDefinitionDigest   string `json:"plugin_definition_digest"`
	ExecutableContentID      string `json:"executable_content_id"`
	ExecutableContentVersion string `json:"executable_content_version"`
	ExecutableDigest         string `json:"executable_digest"`
	RuntimeID                string `json:"runtime_id"`
	RuntimeVersion           string `json:"runtime_version"`
	RuntimeDigest            string `json:"runtime_digest"`
	ProtocolMin              string `json:"protocol_min"`
	ProtocolMax              string `json:"protocol_max"`
}

func (b ExecutableBinding) Validate() error {
	if b.EntryPointID == "" || b.PluginID == "" || b.PluginVersion == "" || b.ExecutableContentID == "" || b.ExecutableContentVersion == "" || b.RuntimeID == "" || b.RuntimeVersion == "" || b.ProtocolMin == "" || b.ProtocolMax == "" {
		return errors.New("executable binding identity and protocol are required")
	}
	for name, value := range map[string]string{
		"plugin definition digest": b.PluginDefinitionDigest,
		"executable digest":        b.ExecutableDigest,
		"runtime digest":           b.RuntimeDigest,
	} {
		if err := ValidateSHA256Digest(value); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

// InvocationRuntimeBinding is the durable bridge between one verified package
// generation, its exact invocation contract, and the runtime adapter that may
// execute it. The binding grants no authority.
type InvocationRuntimeBinding struct {
	PackageID      string             `json:"package_id"`
	PackageVersion string             `json:"package_version"`
	PackageDigest  string             `json:"package_digest"`
	EntryPointID   string             `json:"entry_point_id"`
	ContractDigest string             `json:"contract_digest"`
	RuntimeID      string             `json:"runtime_id"`
	RuntimeVersion string             `json:"runtime_version"`
	RuntimeDigest  string             `json:"runtime_digest"`
	Executable     *ExecutableBinding `json:"executable,omitempty"`
}

func (b InvocationRuntimeBinding) Validate() error {
	if b.PackageID == "" || b.PackageVersion == "" || b.EntryPointID == "" || b.RuntimeID == "" || b.RuntimeVersion == "" {
		return errors.New("invocation runtime binding identity is required")
	}
	for name, value := range map[string]string{
		"package digest": b.PackageDigest, "contract digest": b.ContractDigest, "runtime digest": b.RuntimeDigest,
	} {
		if err := ValidateSHA256Digest(value); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	if b.Executable != nil {
		if err := b.Executable.Validate(); err != nil {
			return fmt.Errorf("executable binding: %w", err)
		}
	}
	return nil
}

package plugin

import (
	"errors"
	"fmt"
)

type IsolationProperty string

const (
	IsolationFilesystem  IsolationProperty = "filesystem"
	IsolationNetwork     IsolationProperty = "network"
	IsolationProcess     IsolationProperty = "process"
	IsolationEnvironment IsolationProperty = "environment"
	IsolationIPC         IsolationProperty = "ipc"
	IsolationCredentials IsolationProperty = "credentials"
)

type RequestedPrivilege struct {
	Property IsolationProperty
	Scope    string
}

type Manifest struct {
	ContractVersion          string `json:"contract_version"`
	ID                       string `json:"id"`
	Version                  string `json:"version"`
	ProtocolMin              string `json:"protocol_min"`
	ProtocolMax              string `json:"protocol_max"`
	Entrypoint               string `json:"entrypoint"`
	ExecutableContentID      string `json:"executable_content_id"`
	ExecutableContentVersion string `json:"executable_content_version"`
	// Capabilities is the signed upper bound of capability identities this
	// executable generation may advertise. Runtime availability may be a
	// subset; actual authority still requires a separately governed lease.
	Capabilities        []string             `json:"capabilities,omitempty"`
	RequestedPrivileges []RequestedPrivilege `json:"requested_privileges,omitempty"`
	RequiredIsolation   []IsolationProperty  `json:"required_isolation,omitempty"`
	Publisher           string               `json:"publisher,omitempty"`
	ArtifactDigest      string               `json:"artifact_digest"`
}

func (m Manifest) Validate() error {
	if _, _, err := manifestVersions.Canonicalize(m.ContractVersion, nil); err != nil {
		return err
	}
	if m.ID == "" || m.Version == "" || m.ProtocolMin == "" || m.ProtocolMax == "" || m.Entrypoint == "" || m.ExecutableContentID == "" || m.ExecutableContentVersion == "" {
		return errors.New("plugin id, version, protocol range, entrypoint, and executable content identity are required")
	}
	if m.ArtifactDigest == "" {
		return errors.New("plugin artifact digest is required")
	}
	if err := (ProtocolRange{Min: m.ProtocolMin, Max: m.ProtocolMax}).Validate(); err != nil {
		return err
	}
	seen := map[string]struct{}{}
	for _, c := range m.Capabilities {
		if c == "" {
			return errors.New("plugin capability cannot be empty")
		}
		if _, ok := seen[c]; ok {
			return fmt.Errorf("duplicate plugin capability %q", c)
		}
		seen[c] = struct{}{}
	}
	return nil
}

type InstanceIdentity struct {
	InstanceID     string
	PluginID       string
	PluginVersion  string
	ArtifactDigest string
	RuntimeSession string
}

func (i InstanceIdentity) ValidateAgainst(m Manifest) error {
	if err := m.Validate(); err != nil {
		return err
	}
	if i.InstanceID == "" || i.RuntimeSession == "" {
		return errors.New("plugin instance id and runtime session are required")
	}
	if i.PluginID != m.ID || i.PluginVersion != m.Version || i.ArtifactDigest != m.ArtifactDigest {
		return errors.New("plugin instance identity does not match verified manifest")
	}
	return nil
}

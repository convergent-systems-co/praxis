package plugin

import (
	"errors"
	"fmt"
)

type IsolationProperty string

const (
	IsolationFilesystem IsolationProperty = "filesystem"
	IsolationNetwork    IsolationProperty = "network"
	IsolationProcess    IsolationProperty = "process"
	IsolationEnvironment IsolationProperty = "environment"
	IsolationIPC        IsolationProperty = "ipc"
	IsolationCredentials IsolationProperty = "credentials"
)

type RequestedPrivilege struct {
	Property IsolationProperty
	Scope    string
}

type Manifest struct {
	ID                    string
	Version               string
	ProtocolMin           string
	ProtocolMax           string
	Entrypoint             string
	Capabilities          []string
	RequestedPrivileges   []RequestedPrivilege
	RequiredIsolation     []IsolationProperty
	Publisher             string
	ArtifactDigest        string
}

func (m Manifest) Validate() error {
	if m.ID == "" || m.Version == "" || m.ProtocolMin == "" || m.ProtocolMax == "" || m.Entrypoint == "" {
		return errors.New("plugin id, version, protocol range, and entrypoint are required")
	}
	if m.ArtifactDigest == "" {
		return errors.New("plugin artifact digest is required")
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

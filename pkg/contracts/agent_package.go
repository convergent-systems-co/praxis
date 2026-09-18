package contracts

import (
	"errors"
	"fmt"
	"time"
)

var (
	agentContractCatalog         = NewVersionCatalog()
	packageAgentDefinitionPolicy = ContractVersionPolicy{
		Contract: "agent.package_definition", CurrentVersion: "v1",
		Versions: []ContractVersionDefinition{{Version: "v1", Disposition: VersionCurrent}},
	}
	packageAgentDefinitionVersions = agentContractCatalog.MustRegister(packageAgentDefinitionPolicy, nil)
	agentInstantiationPolicy       = ContractVersionPolicy{
		Contract: "agent.package_instantiation_intent", CurrentVersion: "v1",
		Versions: []ContractVersionDefinition{{Version: "v1", Disposition: VersionCurrent}},
	}
	agentInstantiationVersions = agentContractCatalog.MustRegister(agentInstantiationPolicy, nil)
)

type PackageGraphBinding struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

func (b PackageGraphBinding) Validate() error {
	if b.ID == "" || b.Version == "" {
		return errors.New("agent package graph id and version are required")
	}
	return nil
}

func (b PackageGraphBinding) Ref() string { return b.ID + "@" + b.Version }

// PackageAgentDefinition is immutable package data. It describes bootstrap
// inputs; it is never itself a local agent identity or authority grant.
type PackageAgentDefinition struct {
	Version       string                `json:"version"`
	Graphs        []PackageGraphBinding `json:"graphs"`
	PreferenceRef string                `json:"preference_ref,omitempty"`
}

type PackageAgentInstantiationRequest struct {
	PackageID         string       `json:"package_id"`
	PackageVersion    string       `json:"package_version"`
	PackageDigest     string       `json:"package_digest"`
	DefinitionID      string       `json:"definition_id"`
	DefinitionVersion string       `json:"definition_version"`
	AgentID           string       `json:"agent_id"`
	GenerationID      string       `json:"generation_id"`
	OwnerScope        string       `json:"owner_scope"`
	GovernanceRef     string       `json:"governance_ref"`
	Intent            ActionIntent `json:"intent"`
	ApprovalID        string       `json:"approval_id"`
}

type PackageAgentInstance struct {
	AgentID       string    `json:"agent_id"`
	GenerationID  string    `json:"generation_id"`
	OwnerScope    string    `json:"owner_scope"`
	GraphRefs     []string  `json:"graph_refs"`
	PreferenceRef string    `json:"preference_ref,omitempty"`
	GovernanceRef string    `json:"governance_ref"`
	CreatedAt     time.Time `json:"created_at"`
}

func PackageAgentDefinitionCurrentVersion() string {
	return packageAgentDefinitionVersions.CurrentVersion()
}
func AgentInstantiationIntentCurrentVersion() string {
	return agentInstantiationVersions.CurrentVersion()
}

func (d PackageAgentDefinition) Validate() error {
	if _, _, err := packageAgentDefinitionVersions.Canonicalize(d.Version, nil); err != nil {
		return err
	}
	if len(d.Graphs) == 0 {
		return errors.New("agent package definition requires at least one graph")
	}
	seen := map[string]bool{}
	for _, graph := range d.Graphs {
		if err := graph.Validate(); err != nil {
			return err
		}
		if seen[graph.Ref()] {
			return fmt.Errorf("duplicate agent package graph %q", graph.Ref())
		}
		seen[graph.Ref()] = true
	}
	return nil
}

func (r PackageAgentInstantiationRequest) ValidateShape() error {
	if r.PackageID == "" || r.PackageVersion == "" || r.PackageDigest == "" || r.DefinitionID == "" || r.DefinitionVersion == "" || r.AgentID == "" || r.GenerationID == "" || r.OwnerScope == "" || r.GovernanceRef == "" || r.ApprovalID == "" {
		return errors.New("complete package agent instantiation identity and authority reference are required")
	}
	return r.Intent.Validate()
}

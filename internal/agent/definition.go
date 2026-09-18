package agent

import (
	"errors"
	"time"

	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
)

type DefinitionBinding struct {
	PackageID      string
	PackageVersion string
	PackageDigest  string
	Definition     packagecatalog.ContentRef
	GraphRefs      []string
}

func (b DefinitionBinding) Validate() error {
	if b.PackageID == "" || b.PackageVersion == "" || b.PackageDigest == "" {
		return errors.New("agent definition package identity is required")
	}
	if b.Definition.Kind != packagecatalog.ContentAgentDefinition {
		return errors.New("content is not an agent definition")
	}
	if err := b.Definition.Validate(); err != nil {
		return err
	}
	if len(b.GraphRefs) == 0 {
		return errors.New("agent definition requires at least one graph reference")
	}
	return nil
}

// InstantiateDefinition creates a new local governed identity from an immutable
// package definition. The downloadable definition is a template, never the agent
// identity itself; each call requires a distinct local identity/generation ID.
func InstantiateDefinition(binding DefinitionBinding, agentID, generationID, ownerScope, governanceRef string, now time.Time) (Agent, Generation, error) {
	if err := binding.Validate(); err != nil {
		return Agent{}, Generation{}, err
	}
	if agentID == "" || generationID == "" || ownerScope == "" || governanceRef == "" || now.IsZero() {
		return Agent{}, Generation{}, errors.New("agent identity, generation, owner scope, governance, and creation time are required")
	}
	generation := Generation{
		ID: generationID, AgentID: agentID, Number: 1,
		GraphRefs: append([]string(nil), binding.GraphRefs...),
		CreationReason: "instantiated from " + binding.PackageID + "@" + binding.PackageVersion + ":" + binding.Definition.ID,
		GovernanceRef: governanceRef, CreatedAt: now.UTC(),
	}
	agent := Agent{ID: agentID, OwnerScope: ownerScope, CurrentGeneration: generationID, Lifecycle: AgentActive, CreatedAt: now.UTC()}
	if err := agent.Validate(); err != nil {
		return Agent{}, Generation{}, err
	}
	if err := generation.Validate(); err != nil {
		return Agent{}, Generation{}, err
	}
	return agent, generation, nil
}

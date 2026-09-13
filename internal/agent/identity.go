package agent

import (
	"errors"
	"fmt"
	"time"
)

type Lifecycle string

const (
	AgentActive   Lifecycle = "active"
	AgentDisabled Lifecycle = "disabled"
	AgentRetired  Lifecycle = "retired"
)

type Agent struct {
	ID                string
	OwnerScope        string
	CurrentGeneration string
	Lifecycle         Lifecycle
	CreatedAt         time.Time
}

type Generation struct {
	ID               string
	AgentID          string
	Number           uint64
	ParentGeneration string
	GraphRefs        []string
	LearningRefs     []string
	PreferenceRef    string
	CreationReason   string
	GovernanceRef    string
	CreatedAt        time.Time
}

func (a Agent) Validate() error {
	if a.ID == "" || a.OwnerScope == "" || a.CurrentGeneration == "" || a.CreatedAt.IsZero() {
		return errors.New("agent id, owner scope, current generation, and creation time are required")
	}
	switch a.Lifecycle {
	case AgentActive, AgentDisabled, AgentRetired:
		return nil
	default:
		return fmt.Errorf("unknown agent lifecycle %q", a.Lifecycle)
	}
}

func (g Generation) Validate() error {
	if g.ID == "" || g.AgentID == "" || g.Number == 0 || len(g.GraphRefs) == 0 || g.CreationReason == "" || g.GovernanceRef == "" || g.CreatedAt.IsZero() {
		return errors.New("generation identity, agent, number, graph refs, reason, governance ref, and creation time are required")
	}
	if g.Number == 1 && g.ParentGeneration != "" {
		return errors.New("first generation cannot have parent generation")
	}
	if g.Number > 1 && g.ParentGeneration == "" {
		return errors.New("later generation requires parent generation")
	}
	return nil
}

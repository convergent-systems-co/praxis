package agent

import (
	"testing"
	"time"
)

func TestLaterGenerationRequiresParent(t *testing.T) {
	g := Generation{ID: "g2", AgentID: "a1", Number: 2, GraphRefs: []string{"graph:v2"}, CreationReason: "promoted learning", GovernanceRef: "promotion:p1", CreatedAt: time.Now().UTC()}
	if err := g.Validate(); err == nil {
		t.Fatal("later generation without parent must fail")
	}
}

func TestFirstGenerationCannotClaimParent(t *testing.T) {
	g := Generation{ID: "g1", AgentID: "a1", Number: 1, ParentGeneration: "g0", GraphRefs: []string{"graph:v1"}, CreationReason: "created", GovernanceRef: "create:c1", CreatedAt: time.Now().UTC()}
	if err := g.Validate(); err == nil {
		t.Fatal("first generation cannot have parent")
	}
}

package trustboundary

import (
	"errors"

	"github.com/convergent-systems-co/praxis/internal/agent"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// Proposal is untrusted or derived content with explicit provenance. It is
// data that may be evaluated, never an authority-bearing command.
type Proposal struct {
	ID         string
	Content    string
	Provenance contracts.ProvenanceRef
	Trust      contracts.TrustClass
}

func NewProposal(id, content string, provenance contracts.ProvenanceRef) (Proposal, error) {
	if id == "" || content == "" {
		return Proposal{}, errors.New("proposal identity and content are required")
	}
	if err := provenance.Validate(); err != nil {
		return Proposal{}, err
	}
	if provenance.Trust == contracts.TrustPolicy || provenance.Trust == contracts.TrustUserConfirmed {
		return Proposal{}, errors.New("content cannot claim policy or human authority")
	}
	return Proposal{ID: id, Content: content, Provenance: provenance, Trust: provenance.Trust}, nil
}

// MemoryCandidate preserves the proposal's trust and provenance. It cannot
// create a confirmed fact, preference, or policy record through this boundary.
func (p Proposal) MemoryCandidate(agentID, scope string) (agent.MemoryRecord, error) {
	if agentID == "" || scope == "" {
		return agent.MemoryRecord{}, errors.New("memory candidate agent and scope are required")
	}
	record := agent.MemoryRecord{ID: "memory:" + p.ID, AgentID: agentID, Scope: scope, Type: agent.MemoryObservation, ContentRef: p.ID, Provenance: p.Provenance, Trust: p.Trust, SourceEvidence: []string{p.ID}, CreatedAt: p.Provenance.ObservedAt}
	if err := record.Validate(); err != nil {
		return agent.MemoryRecord{}, err
	}
	return record, nil
}

// AuthorizeAction is deliberately unavailable: content must pass a separate
// canonical command, policy, capability, and approval path.
func (p Proposal) AuthorizeAction() (contracts.ActionIntent, error) {
	return contracts.ActionIntent{}, errors.New("proposal content cannot authorize an action")
}

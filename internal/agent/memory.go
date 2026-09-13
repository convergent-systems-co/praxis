package agent

import (
	"errors"
	"sort"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type MemoryType string

const (
	MemoryObservation   MemoryType = "observation"
	MemoryFact          MemoryType = "fact"
	MemoryPreference    MemoryType = "preference"
	MemoryProcedure     MemoryType = "procedure"
	MemoryContext       MemoryType = "context"
	MemoryDecision      MemoryType = "decision"
	MemoryOutcome       MemoryType = "outcome"
	MemoryDerived       MemoryType = "derived"
)

type MemoryRecord struct {
	ID               string
	AgentID          string
	Scope            string
	Type             MemoryType
	ContentRef       string
	Provenance       contracts.ProvenanceRef
	Trust            contracts.TrustClass
	Sensitivity      string
	SourceEvidence   []string
	Confidence       float64
	CreatedAt        time.Time
	ValidUntil       *time.Time
	SupersededBy     string
}

func (m MemoryRecord) Validate() error {
	if m.ID == "" || m.AgentID == "" || m.Scope == "" || m.ContentRef == "" || m.CreatedAt.IsZero() {
		return errors.New("memory id, agent, scope, content reference, and creation time are required")
	}
	if err := m.Provenance.Validate(); err != nil {
		return err
	}
	if m.Confidence < 0 || m.Confidence > 1 {
		return errors.New("memory confidence must be in [0,1]")
	}
	return nil
}

func (m MemoryRecord) Active(now time.Time) bool {
	if m.SupersededBy != "" {
		return false
	}
	return m.ValidUntil == nil || now.Before(*m.ValidUntil)
}

// RankableMemory returns active records sorted by confidence, preserving trust
// as metadata. Ranking never upgrades a record's trust class.
func RankableMemory(records []MemoryRecord, now time.Time) []MemoryRecord {
	out := make([]MemoryRecord, 0, len(records))
	for _, record := range records {
		if record.Validate() == nil && record.Active(now) {
			out = append(out, record)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Confidence != out[j].Confidence {
			return out[i].Confidence > out[j].Confidence
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

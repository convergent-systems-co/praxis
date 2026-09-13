package agent

import (
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func memoryProvenance(trust contracts.TrustClass) contracts.ProvenanceRef {
	return contracts.ProvenanceRef{SourceType: "test", Trust: trust, ObservedAt: time.Now().UTC()}
}

func TestHighConfidenceUntrustedMemoryStaysUntrusted(t *testing.T) {
	now := time.Now().UTC()
	records := []MemoryRecord{
		{ID: "u", AgentID: "a", Scope: "project:p", Type: MemoryObservation, ContentRef: "content:u", Provenance: memoryProvenance(contracts.TrustUntrustedContent), Trust: contracts.TrustUntrustedContent, Confidence: 1, CreatedAt: now},
		{ID: "c", AgentID: "a", Scope: "project:p", Type: MemoryFact, ContentRef: "content:c", Provenance: memoryProvenance(contracts.TrustUserConfirmed), Trust: contracts.TrustUserConfirmed, Confidence: 0.8, CreatedAt: now},
	}
	ranked := RankableMemory(records, now)
	if len(ranked) != 2 || ranked[0].ID != "u" || ranked[1].ID != "c" {
		t.Fatalf("unexpected ranking: %+v", ranked)
	}
	if ranked[0].Trust != contracts.TrustUntrustedContent {
		t.Fatal("ranking must not upgrade trust")
	}
}

func TestSupersededMemoryIsNotActive(t *testing.T) {
	now := time.Now().UTC()
	m := MemoryRecord{ID: "m1", AgentID: "a", Scope: "user", Type: MemoryPreference, ContentRef: "c", Provenance: memoryProvenance(contracts.TrustUserConfirmed), Trust: contracts.TrustUserConfirmed, Confidence: 1, CreatedAt: now, SupersededBy: "m2"}
	if m.Active(now) {
		t.Fatal("superseded memory must not be active")
	}
}

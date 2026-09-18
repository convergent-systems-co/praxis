package agent

import (
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
)

func TestInstantiateDefinitionCreatesIndependentLocalAgents(t *testing.T) {
	binding := DefinitionBinding{
		PackageID: "praxis/research", PackageVersion: "1.0.0", PackageDigest: "sha256:pkg",
		Definition: packagecatalog.ContentRef{Kind: packagecatalog.ContentAgentDefinition, ID: "researcher", Version: "1", Digest: "sha256:def", Artifact: "agents/researcher.json"},
		GraphRefs:  []string{"praxis.research.default@1"},
	}
	now := time.Now().UTC()
	a, ga, err := InstantiateDefinition(binding, "agent-a", "gen-a-1", "user:a", "policy:default", now)
	if err != nil {
		t.Fatal(err)
	}
	b, gb, err := InstantiateDefinition(binding, "agent-b", "gen-b-1", "user:b", "policy:default", now)
	if err != nil {
		t.Fatal(err)
	}
	if a.ID == b.ID || ga.ID == gb.ID || ga.AgentID == gb.AgentID {
		t.Fatal("instances from one definition must have independent local identity")
	}
	if ga.CreationReason != gb.CreationReason {
		t.Fatal("both generations should retain the same immutable source definition provenance")
	}
}

func TestInstantiateDefinitionRejectsNonAgentContent(t *testing.T) {
	binding := DefinitionBinding{
		PackageID: "pkg", PackageVersion: "1", PackageDigest: "sha256:pkg",
		Definition: packagecatalog.ContentRef{Kind: packagecatalog.ContentGraph, ID: "x", Version: "1", Digest: "sha256:x", Artifact: "x.json"},
		GraphRefs:  []string{"g@1"},
	}
	if _, _, err := InstantiateDefinition(binding, "a", "g1", "user:a", "policy", time.Now().UTC()); err == nil {
		t.Fatal("non-agent content must be rejected")
	}
}

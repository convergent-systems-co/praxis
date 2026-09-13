package preference

import (
	"testing"
	"time"
)

func TestExplicitUserOutranksHighConfidenceLearned(t *testing.T) {
	now := time.Now().UTC()
	records := []Record{
		{ID: "learned", SlotID: "verbosity", Value: "long", Scope: "project:p", ScopeDepth: 3, Source: SourceLearned, Confidence: 1, UpdatedAt: now},
		{ID: "explicit", SlotID: "verbosity", Value: "short", Scope: "project:p", ScopeDepth: 3, Source: SourceExplicitUser, UpdatedAt: now.Add(-time.Hour)},
	}
	got, err := Resolve("verbosity", records, now)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "explicit" {
		t.Fatalf("explicit user preference must win, got %+v", got)
	}
}

func TestNarrowerScopeWinsWithinSameAuthority(t *testing.T) {
	now := time.Now().UTC()
	records := []Record{
		{ID: "global", SlotID: "review", Value: "normal", Scope: "user", ScopeDepth: 1, Source: SourceExplicitUser, UpdatedAt: now},
		{ID: "project", SlotID: "review", Value: "strict", Scope: "project:p", ScopeDepth: 3, Source: SourceExplicitUser, UpdatedAt: now.Add(-time.Hour)},
	}
	got, err := Resolve("review", records, now)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "project" {
		t.Fatalf("narrower scope must win, got %+v", got)
	}
}

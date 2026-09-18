package workspace

import "testing"

func TestContextPackRespectsAllBudgets(t *testing.T) {
	items := []Evidence{
		{ID: "a", Bytes: 100, TokenEstimate: 20, Score: 1.0},
		{ID: "b", Bytes: 100, TokenEstimate: 20, Score: 0.9},
		{ID: "c", Bytes: 100, TokenEstimate: 20, Score: 0.8},
	}
	pack, err := BuildContextPack(items, ContextBudget{MaxBytes: 200, MaxTokens: 40, MaxItems: 2})
	if err != nil { t.Fatal(err) }
	if len(pack.Items) != 2 || pack.BytesUsed != 200 || pack.TokensUsed != 40 || pack.TruncatedCount != 1 {
		t.Fatalf("unexpected pack: %+v", pack)
	}
}

func TestContextPackPrefersHigherScore(t *testing.T) {
	items := []Evidence{
		{ID: "low", Bytes: 10, TokenEstimate: 2, Score: 0.2},
		{ID: "high", Bytes: 10, TokenEstimate: 2, Score: 0.9},
	}
	pack, err := BuildContextPack(items, ContextBudget{MaxItems: 1})
	if err != nil { t.Fatal(err) }
	if len(pack.Items) != 1 || pack.Items[0].ID != "high" {
		t.Fatalf("expected high-ranked evidence, got %+v", pack.Items)
	}
}

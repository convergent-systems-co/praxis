package inference

import (
	"testing"
	"time"
)

func TestD0UsesNoExecutor(t *testing.T) {
	d, err := Route(Budget{Tier: D0}, nil)
	if err != nil || d.ExecutorID != "" || d.Tier != D0 {
		t.Fatalf("expected deterministic route, got %+v err=%v", d, err)
	}
}

func TestRouterPrefersFasterSufficientExecutor(t *testing.T) {
	candidates := []ExecutorEvidence{
		{ID: "slow-strong", SupportsTier: map[Tier]bool{D1: true}, Quality: 0.99, P50Latency: 12 * time.Second, P50TimeToFirstAction: 8 * time.Second, Available: true},
		{ID: "fast-sufficient", SupportsTier: map[Tier]bool{D1: true}, Quality: 0.93, P50Latency: 3 * time.Second, P50TimeToFirstAction: 1 * time.Second, Available: true},
	}
	d, err := Route(Budget{Tier: D1, MinQuality: 0.90, Deadline: 15 * time.Second, PreferTimeToFirstAction: true}, candidates)
	if err != nil { t.Fatal(err) }
	if d.ExecutorID != "fast-sufficient" { t.Fatalf("expected fast sufficient executor, got %+v", d) }
}

func TestRouterRejectsExecutorOutsideDeadline(t *testing.T) {
	_, err := Route(Budget{Tier: D2, MinQuality: 0.9, Deadline: time.Second}, []ExecutorEvidence{{ID: "slow", SupportsTier: map[Tier]bool{D2: true}, Quality: 1, P50Latency: 5 * time.Second, Available: true}})
	if err == nil { t.Fatal("executor outside deadline must be ineligible") }
}

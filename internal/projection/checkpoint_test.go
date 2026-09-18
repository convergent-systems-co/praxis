package projection

import "testing"

func TestSecurityReadRejectsEventualProjection(t *testing.T) {
	c := Checkpoint{Name: "dashboard", Version: "v1", Consistency: Eventual, LastSequence: 100}
	if err := c.RequireSequence(1); err == nil {
		t.Fatal("eventual projection must not authorize security-sensitive decisions")
	}
}

func TestStrongCheckpointRequiresFreshSequence(t *testing.T) {
	c := Checkpoint{Name: "authority", Version: "v1", Consistency: StrongCheckpointed, LastSequence: 9}
	if err := c.RequireSequence(10); err == nil {
		t.Fatal("stale projection must be rejected")
	}
	c.LastSequence = 10
	if err := c.RequireSequence(10); err != nil {
		t.Fatalf("fresh strong projection rejected: %v", err)
	}
}

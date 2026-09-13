package develop

import (
	"testing"

	"github.com/convergent-systems-co/praxis/internal/kernel"
)

func TestDevelopGraphIsValidAndBounded(t *testing.T) {
	g := Graph()
	if err := g.Validate(); err != nil { t.Fatalf("develop graph invalid: %v", err) }
	if g.MaxTransitions <= 0 { t.Fatal("develop repair cycle must be bounded") }
}

func TestDevelopFastPathSkipsPlan(t *testing.T) {
	g := Graph()
	to, err := kernel.ResolveTransition(g, "classify", "fast")
	if err != nil { t.Fatal(err) }
	if to != "prepare" { t.Fatalf("fast path should skip plan, got %s", to) }
}

func TestDevelopRepairReturnsToValidation(t *testing.T) {
	g := Graph()
	to, err := kernel.ResolveTransition(g, "repair", "done")
	if err != nil { t.Fatal(err) }
	if to != "validate" { t.Fatalf("repair should return to validation, got %s", to) }
}

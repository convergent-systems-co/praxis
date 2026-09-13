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

func TestDevelopFastPathSkipsGoalsAndPlan(t *testing.T) {
	g := Graph()
	to, err := kernel.ResolveTransition(g, "classify", "fast")
	if err != nil { t.Fatal(err) }
	if to != "prepare" { t.Fatalf("fast path should skip Goals and plan, got %s", to) }
}

func TestDevelopLocalPlanPathSkipsGoals(t *testing.T) {
	g := Graph()
	to, err := kernel.ResolveTransition(g, "classify", "plan")
	if err != nil { t.Fatal(err) }
	if to != "plan" { t.Fatalf("local plan should not repeat Goals baseline, got %s", to) }
}

func TestDevelopArchitectedPathComposesGoalsThenMaterializes(t *testing.T) {
	g := Graph()
	to, err := kernel.ResolveTransition(g, "classify", "goals")
	if err != nil { t.Fatal(err) }
	if to != "goals" { t.Fatalf("architected work should enter Goals subgraph, got %s", to) }
	to, err = kernel.ResolveTransition(g, "goals", "baseline")
	if err != nil { t.Fatal(err) }
	if to != "materialize" { t.Fatalf("Goal Baseline should be software-materialized, got %s", to) }
	to, err = kernel.ResolveTransition(g, "materialize", "ready")
	if err != nil { t.Fatal(err) }
	if to != "plan" { t.Fatalf("materialized baseline should feed local plan, got %s", to) }
}

func TestDevelopRepairReturnsToValidation(t *testing.T) {
	g := Graph()
	to, err := kernel.ResolveTransition(g, "repair", "done")
	if err != nil { t.Fatal(err) }
	if to != "validate" { t.Fatalf("repair should return to validation, got %s", to) }
}

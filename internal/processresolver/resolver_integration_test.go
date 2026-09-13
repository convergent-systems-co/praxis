package processresolver

import (
	"context"
	"testing"

	"github.com/convergent-systems-co/praxis/internal/kernel"
)

type fixedAdvisor struct{ proposal Proposal }

func (f fixedAdvisor) ProposeProcess(context.Context, GoalContext) (Proposal, error) {
	return f.proposal, nil
}

type successExecutor struct{}

func (successExecutor) ExecuteNode(_ context.Context, _ kernel.GraphDef, _ kernel.NodeDef, _ *kernel.RunExecution) (kernel.NodeResult, error) {
	return kernel.NodeResult{Outcome: "done", Evidence: []string{"executed"}}, nil
}

func executableGraph(id, version string) kernel.GraphDef {
	return kernel.GraphDef{
		ID: id, Version: version, EntryNode: "work", MaxTransitions: 2,
		Nodes:       []kernel.NodeDef{{ID: "work", Class: kernel.NodeDeterministic}, {ID: "complete", Class: kernel.NodeTerminal, TerminalState: kernel.RunSucceeded}},
		Transitions: []kernel.TransitionDef{{From: "work", Outcome: "done", To: "complete"}},
	}
}

func goal(id, class, profile, environment string) GoalContext {
	return GoalContext{GoalID: id, GoalClass: class, HumanContextProfile: profile, Environment: environment, Preferences: map[string]string{"style": "concise"}, AvailableCapabilities: []string{"read", "analyze"}}
}

func execute(t *testing.T, resolution Resolution) {
	t.Helper()
	if err := resolution.Verify(); err != nil {
		t.Fatal(err)
	}
	executor := kernel.NodeExecutor(successExecutor{})
	if resolution.Mode == ModeCompose {
		registry, err := kernel.NewGraphRegistry(resolution.Dependencies...)
		if err != nil {
			t.Fatal(err)
		}
		outcomes := map[string]kernel.SubgraphOutcomeContract{}
		for _, dependency := range resolution.Dependencies {
			outcomes[dependency.ID+"@"+dependency.Version] = kernel.SubgraphOutcomeContract{Succeeded: "succeeded", Failed: "failed", Cancelled: "cancelled"}
		}
		executor = kernel.ComposingExecutor{Delegate: successExecutor{}, Subgraphs: kernel.LocalSubgraphRuntime{Registry: registry, Executor: successExecutor{}}, Outcomes: outcomes}
	}
	run := &kernel.RunExecution{RunID: "run-" + string(resolution.Mode), State: kernel.RunQueued}
	if err := kernel.Run(context.Background(), resolution.Graph, run, executor); err != nil {
		t.Fatal(err)
	}
	if run.State != kernel.RunSucceeded {
		t.Fatalf("%s process did not execute: %s", resolution.Mode, run.State)
	}
}

func TestGoalProcessDiscoveryResolvesAndExecutesAllModes(t *testing.T) {
	exact := CatalogEntry{ID: "research-team", Graph: executableGraph("research", "3"), GoalClass: "research", HumanContextProfile: "team", Environment: "local", RequiredPreferences: map[string]string{"style": "concise"}, RequiredCapabilities: []string{"read"}, Adaptable: true}
	resolver := Resolver{Catalog: []CatalogEntry{exact}}
	selected, err := resolver.Resolve(context.Background(), Request{Goal: goal("g-select", "research", "team", "local")})
	if err != nil || selected.Mode != ModeSelect || selected.Graph.ID != exact.Graph.ID {
		t.Fatalf("exact selection failed: %#v %v", selected, err)
	}
	execute(t, selected)

	adapted, err := resolver.Resolve(context.Background(), Request{Goal: goal("g-adapt", "research", "solo", "remote")})
	if err != nil || adapted.Mode != ModeAdapt || adapted.Graph.ID == exact.Graph.ID || adapted.ContextBindings["environment"] != "remote" || !adapted.RequiresGovernance {
		t.Fatalf("context adaptation failed: %#v %v", adapted, err)
	}
	execute(t, adapted)

	collect := CatalogEntry{ID: "collect-v2", Graph: executableGraph("collect", "2"), GoalClass: "investigate", FragmentStage: "collect", RequiredCapabilities: []string{"read"}}
	analyze := CatalogEntry{ID: "analyze-v5", Graph: executableGraph("analyze", "5"), GoalClass: "investigate", FragmentStage: "analyze", RequiredCapabilities: []string{"analyze"}}
	composedResolver := Resolver{Catalog: []CatalogEntry{analyze, collect}}
	composedGoal := goal("g-compose", "investigate", "solo", "local")
	composedGoal.RequiredStages = []string{"collect", "analyze"}
	composed, err := composedResolver.Resolve(context.Background(), Request{Goal: composedGoal})
	if err != nil || composed.Mode != ModeCompose || len(composed.Dependencies) != 2 || composed.Dependencies[0].Version != "2" || composed.Dependencies[1].Version != "5" {
		t.Fatalf("exact-version composition failed: %#v %v", composed, err)
	}
	execute(t, composed)

	proposal := Proposal{Graph: executableGraph("advisory-novel", "9"), RequiredCapabilities: []string{"analyze"}, Rationale: "novel evidence synthesis"}
	novel := Resolver{Advisor: fixedAdvisor{proposal: proposal}, MinIndependentSuccesses: 2}
	candidateGoal := goal("g-candidate", "novel", "solo", "local")
	candidate, err := novel.Resolve(context.Background(), Request{Goal: candidateGoal, PriorOutcomes: []PriorOutcome{
		{EvidenceID: "run-1", CausationRoot: "human-a", Successful: true},
		{EvidenceID: "run-2", CausationRoot: "human-b", Successful: true},
	}})
	if err != nil || candidate.Mode != ModeCandidate || !candidate.RequiresGovernance || len(candidate.PriorEvidenceIDs) != 2 || candidate.Graph.ID == proposal.Graph.ID {
		t.Fatalf("candidate creation failed: %#v %v", candidate, err)
	}
	execute(t, candidate)

	oneOffGoal := goal("g-one-off", "novel", "solo", "local")
	oneOff, err := novel.Resolve(context.Background(), Request{Goal: oneOffGoal, AllowOneOff: true, MaxOneOffTransitions: 3})
	if err != nil || oneOff.Mode != ModeOneOff || oneOff.Reusable || oneOff.RequiresGovernance || oneOff.Graph.MaxTransitions > 3 {
		t.Fatalf("bounded one-off resolution failed: %#v %v", oneOff, err)
	}
	execute(t, oneOff)
}

func TestGoalProcessDiscoveryFailsClosedBeforeAdvisoryExecution(t *testing.T) {
	proposal := Proposal{Graph: executableGraph("network-proposal", "1"), RequiredCapabilities: []string{"network"}, PolicyTags: []string{"external-write"}}
	resolver := Resolver{Advisor: fixedAdvisor{proposal: proposal}}
	g := goal("g-denied", "novel", "solo", "local")
	g.DeniedPolicyTags = []string{"external-write"}
	if _, err := resolver.Resolve(context.Background(), Request{Goal: g, AllowOneOff: true, MaxOneOffTransitions: 3}); err == nil {
		t.Fatal("advisory proposal bypassed deterministic capability/policy eligibility")
	}
}

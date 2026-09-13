package state

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/convergent-systems-co/praxis/internal/kernel"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type allowContinuation struct{}

func (allowContinuation) AuthorizeContinuation(context.Context, contracts.PrincipalRef, kernel.RunExecution, kernel.ContinuationDecision) error {
	return nil
}

type allowContinuationResume struct{}

func (allowContinuationResume) AuthorizeRunControl(context.Context, contracts.PrincipalRef, kernel.RunExecution, kernel.RunControlOperation) error {
	return nil
}

type continuationExecutor struct{ evidence string }

func (e continuationExecutor) ExecuteNode(context.Context, kernel.GraphDef, kernel.NodeDef, *kernel.RunExecution) (kernel.NodeResult, error) {
	return kernel.NodeResult{Outcome: "complete", Evidence: []string{e.evidence}}, nil
}

func continuationGraph(id string) kernel.GraphDef {
	return kernel.GraphDef{
		ID: id, Version: "1", EntryNode: "work", MaxTransitions: 2,
		Nodes:       []kernel.NodeDef{{ID: "work", Class: kernel.NodeCapability}, {ID: "done", Class: kernel.NodeTerminal, TerminalState: kernel.RunSucceeded}},
		Transitions: []kernel.TransitionDef{{From: "work", Outcome: "complete", To: "done"}},
	}
}

func TestDomainNeutralResourceHandoffPreservesTwoDomainRunsAcrossSQLiteRestart(t *testing.T) {
	domains := []struct {
		name      string
		graphID   string
		agentID   string
		signal    string
		threshold int64
		observed  int64
	}{
		{name: "software-delivery-package", graphID: "praxis.package.software-delivery", agentID: "agent-delivery", signal: "workspace_context_ratio", threshold: 80, observed: 91},
		{name: "research-package", graphID: "praxis.package.research", agentID: "agent-research", signal: "source_window_ratio", threshold: 70, observed: 84},
	}
	for _, domain := range domains {
		t.Run(domain.name, func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "praxis.db")
			actor := contracts.PrincipalRef{ID: "runtime-governor", Kind: "service"}
			graph := continuationGraph(domain.graphID)
			run := &kernel.RunExecution{RunID: "run-" + domain.name, AgentID: domain.agentID, GraphID: graph.ID, GraphVersion: graph.Version, CurrentNode: graph.EntryNode, State: kernel.RunRunning, AttemptCounts: map[string]int{}}

			db, err := OpenSQLite(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			store := NewSQLiteEventStore(db)
			journal := &kernel.EventJournal{Store: store, Actor: actor, CommandID: "start-" + domain.name, CorrelationID: run.RunID}
			started := kernel.RunObservation{Kind: kernel.ObservationRunStarted, RunID: run.RunID, AgentID: run.AgentID, GraphID: run.GraphID, GraphVersion: run.GraphVersion, NodeID: run.CurrentNode, State: run.State}
			if err := journal.ObserveRun(ctx, started); err != nil {
				t.Fatal(err)
			}
			journal.CommandID = "handoff-" + domain.name
			profile := kernel.ResourceProfile{ID: "profile-" + domain.name, Version: "1", Rules: []kernel.ResourceRule{{ID: "domain-pressure", Signal: domain.signal, AtOrAbove: domain.threshold, Action: kernel.ContinuationHandoff}}}
			observation := kernel.ResourceObservation{ID: "pressure-" + domain.name, RunID: run.RunID, AgentID: run.AgentID, Signals: map[string]int64{domain.signal: domain.observed}, Evidence: []string{"meter:" + domain.signal}}
			decision, err := (kernel.ContinuationController{Authorizer: allowContinuation{}}).Apply(ctx, run, profile, observation, journal)
			if err != nil {
				t.Fatal(err)
			}
			if run.State != kernel.RunSuspended || run.PendingWait == nil || run.PendingWait.Kind != kernel.WaitHandoff || run.LastCheckpoint == nil {
				t.Fatalf("run did not enter durable handoff: %+v", run)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}

			reopened, err := OpenSQLite(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			reopenedStore := NewSQLiteEventStore(reopened)
			control := kernel.RunControl{Store: reopenedStore, Actor: actor, Authorizer: allowContinuationResume{}}
			status, err := control.Status(ctx, run.RunID)
			if err != nil {
				t.Fatal(err)
			}
			if status.Run.AgentID != domain.agentID || status.Run.GraphID != graph.ID || status.Run.PendingWait == nil || status.Run.PendingWait.Ref != decision.HandoffRef || status.Run.LastCheckpoint == nil {
				t.Fatalf("restart lost continuation identity: %+v", status.Run)
			}
			if len(status.Run.Evidence) != 1 || status.Run.Evidence[0] != "meter:"+domain.signal || len(status.Run.ContinuationHistory) != 1 {
				t.Fatalf("restart lost pressure/decision evidence: %+v", status.Run)
			}
			status, err = control.ResumeSignal(ctx, run.RunID, kernel.WaitHandoff, decision.HandoffRef, "resume-"+domain.name, run.RunID)
			if err != nil {
				t.Fatal(err)
			}
			journal = &kernel.EventJournal{Store: reopenedStore, Actor: actor, CommandID: "execute-" + domain.name, CorrelationID: run.RunID, ExpectedVersion: status.AggregateVersion}
			if err := kernel.RunObserved(ctx, graph, status.Run, continuationExecutor{evidence: "result:" + domain.name}, journal); err != nil {
				t.Fatal(err)
			}
			final, err := control.Status(ctx, run.RunID)
			if err != nil {
				t.Fatal(err)
			}
			if final.Run.State != kernel.RunSucceeded || final.Run.AgentID != domain.agentID || final.Run.RunID != run.RunID || len(final.Run.Evidence) != 2 {
				t.Fatalf("resumed execution lost identity/evidence: %s %+v", fmt.Sprint(final.Run.Evidence), final.Run)
			}
		})
	}
}

func TestResourceHandoffDenialDoesNotMutateRun(t *testing.T) {
	// Authorization is mandatory; the nil authorizer fails before state or
	// evidence changes and therefore cannot create an advisory-only handoff.
	run := &kernel.RunExecution{RunID: "run-denied", AgentID: "agent", GraphID: "graph", GraphVersion: "1", CurrentNode: "work", State: kernel.RunRunning}
	profile := kernel.ResourceProfile{ID: "p", Version: "1", Rules: []kernel.ResourceRule{{ID: "r", Signal: "opaque", AtOrAbove: 1, Action: kernel.ContinuationHandoff}}}
	observation := kernel.ResourceObservation{ID: "o", RunID: run.RunID, AgentID: run.AgentID, Signals: map[string]int64{"opaque": 2}, Evidence: []string{"meter:o"}}
	if _, err := (kernel.ContinuationController{}).Apply(context.Background(), run, profile, observation, &kernel.EventJournal{}); err == nil {
		t.Fatal("handoff without deterministic authorization succeeded")
	}
	if run.State != kernel.RunRunning || run.PendingWait != nil || len(run.Evidence) != 0 {
		t.Fatal("denied handoff mutated run")
	}
}

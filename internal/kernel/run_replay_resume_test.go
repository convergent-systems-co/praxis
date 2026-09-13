package kernel

import (
	"encoding/json"
	"testing"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
)

func TestReplayRunResumeClearsPersistedWait(t *testing.T) {
	started := RunObservation{Kind: ObservationRunStarted, RunID: "run-1", GraphID: "graph-1", GraphVersion: "1", NodeID: "work", State: RunRunning}
	suspended := RunObservation{Kind: ObservationRunStateChanged, RunID: "run-1", GraphID: "graph-1", GraphVersion: "1", NodeID: "work", State: RunSuspended, Wait: &Suspension{Kind: WaitApproval, Ref: "approval-1"}}
	resumed := RunObservation{Kind: ObservationRunResumed, RunID: "run-1", GraphID: "graph-1", GraphVersion: "1", NodeID: "work", State: RunRunning}
	observations := []RunObservation{started, suspended, resumed}
	events := make([]eventstore.Event, 0, len(observations))
	for i, observation := range observations {
		payload, err := json.Marshal(observation)
		if err != nil {
			t.Fatal(err)
		}
		events = append(events, eventstore.Event{ID: observation.RunID, AggregateID: observation.RunID, AggregateType: "run", AggregateVersion: int64(i + 1), Type: "run." + string(observation.Kind), Version: "1", Payload: payload})
	}
	run, version, err := ReplayRun(events)
	if err != nil {
		t.Fatal(err)
	}
	if version != 3 || run.State != RunRunning || run.CurrentNode != "work" {
		t.Fatalf("unexpected replayed resume: version=%d state=%s node=%s", version, run.State, run.CurrentNode)
	}
	if run.PendingWait != nil {
		t.Fatalf("resumed replay resurrected stale wait: %#v", run.PendingWait)
	}
}

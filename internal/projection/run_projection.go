package projection

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/internal/kernel"
)

// RunView is a disposable read model derived from authoritative run events.
type RunView struct {
	RunID           string              `json:"run_id"`
	GraphID         string              `json:"graph_id"`
	GraphVersion    string              `json:"graph_version"`
	CurrentNode     string              `json:"current_node"`
	State           kernel.RunState     `json:"state"`
	TransitionCount int                 `json:"transition_count"`
	EvidenceCount   int                 `json:"evidence_count"`
	FailedAttempts  int                 `json:"failed_attempts"`
	LastFailure     kernel.FailureClass `json:"last_failure,omitempty"`
	PendingWait     *kernel.Suspension  `json:"pending_wait,omitempty"`
	LastSequence    int64               `json:"last_sequence"`
	LastVersion     int64               `json:"last_version"`
}

type RunViews struct {
	mu    sync.RWMutex
	views map[string]RunView
}

func NewRunViews() *RunViews { return &RunViews{views: map[string]RunView{}} }

func (r *RunViews) Apply(_ context.Context, event eventstore.Event) error {
	if event.AggregateType != "run" || !strings.HasPrefix(event.Type, "run.") {
		return nil
	}
	var observation kernel.RunObservation
	if err := json.Unmarshal(event.Payload, &observation); err != nil {
		return fmt.Errorf("decode run projection event: %w", err)
	}
	if observation.RunID == "" || observation.RunID != event.AggregateID {
		return errors.New("run projection event identity mismatch")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	view := r.views[observation.RunID]
	if view.RunID == "" {
		view = RunView{RunID: observation.RunID, GraphID: observation.GraphID, GraphVersion: observation.GraphVersion}
	}
	if view.GraphID != observation.GraphID || view.GraphVersion != observation.GraphVersion {
		return errors.New("run projection graph identity changed")
	}
	if view.LastVersion != 0 && event.AggregateVersion != view.LastVersion+1 {
		return fmt.Errorf("run projection version gap: expected %d, got %d", view.LastVersion+1, event.AggregateVersion)
	}

	switch observation.Kind {
	case kernel.ObservationRunStarted, kernel.ObservationRunResumed:
		view.CurrentNode = observation.NodeID
		view.State = observation.State
		view.TransitionCount = observation.TransitionCount
	case kernel.ObservationRunStateChanged:
		view.CurrentNode = observation.NodeID
		view.State = observation.State
		view.TransitionCount = observation.TransitionCount
		if observation.Wait != nil {
			wait := *observation.Wait
			view.PendingWait = &wait
		} else if observation.State != kernel.RunSuspended {
			view.PendingWait = nil
		}
		if observation.FailureClass != "" {
			view.LastFailure = observation.FailureClass
		}
	case kernel.ObservationNodeAttemptFailed:
		view.FailedAttempts++
		view.LastFailure = observation.FailureClass
	case kernel.ObservationNodeCompleted:
		view.EvidenceCount += len(observation.Evidence)
	case kernel.ObservationTransitioned:
		view.CurrentNode = observation.ToNode
		view.State = observation.State
		view.TransitionCount = observation.TransitionCount
	case kernel.ObservationRunTerminal:
		view.CurrentNode = observation.NodeID
		view.State = observation.State
		view.TransitionCount = observation.TransitionCount
		view.PendingWait = nil
		if observation.FailureClass != "" {
			view.LastFailure = observation.FailureClass
		}
	default:
		return fmt.Errorf("unknown run observation kind %q", observation.Kind)
	}
	view.LastSequence = event.Sequence
	view.LastVersion = event.AggregateVersion
	r.views[view.RunID] = view
	return nil
}

func (r *RunViews) Get(runID string) (RunView, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	view, ok := r.views[runID]
	if ok && view.PendingWait != nil {
		copyWait := *view.PendingWait
		view.PendingWait = &copyWait
	}
	return view, ok
}

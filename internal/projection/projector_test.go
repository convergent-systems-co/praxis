package projection

import (
	"context"
	"errors"
	"testing"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
)

type collectingHandler struct {
	ids  []string
	fail string
}

func (h *collectingHandler) Apply(_ context.Context, e eventstore.Event) error {
	if e.ID == h.fail { return errors.New("projection failure") }
	h.ids = append(h.ids, e.ID)
	return nil
}

func TestProjectionCatchUpAdvancesCheckpoint(t *testing.T) {
	store := eventstore.NewMemoryStore()
	if _, err := store.Append(context.Background(), "run-1", 0, []eventstore.Event{fixtureProjectionEvent("e1"), fixtureProjectionEvent("e2")}); err != nil { t.Fatal(err) }
	h := &collectingHandler{}
	r := &Runner{Events: store, Handler: h, Checkpoint: Checkpoint{Name: "run-view", Version: "v1", Consistency: StrongCheckpointed}, BatchSize: 1}
	if err := r.CatchUp(context.Background()); err != nil { t.Fatal(err) }
	if r.Checkpoint.LastSequence != 2 || len(h.ids) != 2 { t.Fatalf("unexpected projection state: checkpoint=%+v ids=%v", r.Checkpoint, h.ids) }
}

func TestProjectionFailureDoesNotAdvancePastFailedEvent(t *testing.T) {
	store := eventstore.NewMemoryStore()
	if _, err := store.Append(context.Background(), "run-1", 0, []eventstore.Event{fixtureProjectionEvent("e1"), fixtureProjectionEvent("e2")}); err != nil { t.Fatal(err) }
	h := &collectingHandler{fail: "e2"}
	r := &Runner{Events: store, Handler: h, Checkpoint: Checkpoint{Name: "run-view", Version: "v1", Consistency: StrongCheckpointed}}
	if err := r.CatchUp(context.Background()); err == nil { t.Fatal("projection failure expected") }
	if r.Checkpoint.LastSequence != 1 { t.Fatalf("checkpoint must stop before failed event, got %d", r.Checkpoint.LastSequence) }
}

func fixtureProjectionEvent(id string) eventstore.Event {
	return eventstore.Event{ID: id, AggregateType: "run", Type: "run.transitioned", Version: "v1", CommandID: "cmd", CorrelationID: "corr"}
}

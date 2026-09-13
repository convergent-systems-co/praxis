package projection

import (
	"context"
	"errors"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
)

type Handler interface {
	Apply(ctx context.Context, event eventstore.Event) error
}

type Runner struct {
	Events     eventstore.Store
	Handler    Handler
	Checkpoint Checkpoint
	BatchSize  int
}

// CatchUp applies events monotonically and advances the checkpoint only after
// each event is applied successfully.
func (r *Runner) CatchUp(ctx context.Context) error {
	if r.Events == nil || r.Handler == nil {
		return errors.New("projection event store and handler are required")
	}
	if r.BatchSize <= 0 {
		r.BatchSize = 100
	}
	if err := r.Checkpoint.Validate(); err != nil {
		return err
	}
	for {
		events, err := r.Events.ReadFrom(ctx, r.Checkpoint.LastSequence, r.BatchSize)
		if err != nil {
			return err
		}
		if len(events) == 0 {
			return nil
		}
		for _, event := range events {
			if event.Sequence <= r.Checkpoint.LastSequence {
				return errors.New("event sequence did not advance monotonically")
			}
			if err := r.Handler.Apply(ctx, event); err != nil {
				return err
			}
			r.Checkpoint.LastSequence = event.Sequence
		}
		if len(events) < r.BatchSize {
			return nil
		}
	}
}

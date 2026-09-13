package projection

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
)

type Handler interface {
	Apply(ctx context.Context, event eventstore.Event) error
}

type CheckpointStore interface {
	LoadCheckpoint(ctx context.Context, name, version string) (Checkpoint, bool, error)
	SaveCheckpoint(ctx context.Context, checkpoint Checkpoint) error
}

type Runner struct {
	Events      eventstore.Store
	Handler     Handler
	Checkpoint Checkpoint
	Checkpoints CheckpointStore
	BatchSize   int
	Now         func() time.Time
}

// CatchUp applies events monotonically. If a persistent checkpoint store is
// configured, restart begins from the last committed sequence. Projection
// handlers MUST therefore be idempotent: a crash after Apply but before the
// checkpoint commit can cause one event to be replayed.
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
	if r.Checkpoints != nil {
		persisted, ok, err := r.Checkpoints.LoadCheckpoint(ctx, r.Checkpoint.Name, r.Checkpoint.Version)
		if err != nil {
			return fmt.Errorf("load projection checkpoint: %w", err)
		}
		if ok {
			if persisted.Consistency != r.Checkpoint.Consistency {
				return errors.New("persisted projection consistency class mismatch")
			}
			r.Checkpoint = persisted
		}
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
			r.Checkpoint.UpdatedAt = time.Now().UTC()
			if r.Now != nil {
				r.Checkpoint.UpdatedAt = r.Now().UTC()
			}
			if r.Checkpoints != nil {
				if err := r.Checkpoints.SaveCheckpoint(ctx, r.Checkpoint); err != nil {
					return fmt.Errorf("save projection checkpoint: %w", err)
				}
			}
		}
		if len(events) < r.BatchSize {
			return nil
		}
	}
}

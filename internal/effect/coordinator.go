package effect

import (
	"context"
	"errors"
	"fmt"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

var ErrPreconditionChanged = errors.New("effect precondition changed")

// AuthorizationSnapshot is the security material that was valid at command
// preflight. It must be revalidated immediately before an external effect.
type AuthorizationSnapshot struct {
	IntentDigest string
	ApprovalID   string
	LeaseID      string
}

// Revalidator performs deterministic commit-time checks against current
// authoritative state. It must not delegate the decision to an LLM.
type Revalidator interface {
	Revalidate(ctx context.Context, intent contracts.ActionIntent, auth AuthorizationSnapshot) error
}

// PreconditionChecker verifies target state has not materially changed since
// the ActionIntent was approved.
type PreconditionChecker interface {
	Check(ctx context.Context, intent contracts.ActionIntent) error
}

// Dispatcher is the mediated effect boundary. Dispatch success is only an
// observation and does not by itself define authoritative Praxis state.
type Dispatcher interface {
	Dispatch(ctx context.Context, intent contracts.ActionIntent, idempotencyKey string) (Result, error)
}

type Result struct {
	ObservedState string
	Evidence      []byte
}

type Coordinator struct {
	Revalidator Revalidator
	Preconditions PreconditionChecker
	Dispatcher Dispatcher
}

func (c Coordinator) Commit(ctx context.Context, intent contracts.ActionIntent, auth AuthorizationSnapshot, idempotencyKey string) (Result, error) {
	if c.Revalidator == nil || c.Preconditions == nil || c.Dispatcher == nil {
		return Result{}, errors.New("effect coordinator is not fully configured")
	}
	actualDigest, err := intent.Digest()
	if err != nil {
		return Result{}, fmt.Errorf("digest intent: %w", err)
	}
	if auth.IntentDigest == "" || auth.IntentDigest != actualDigest {
		return Result{}, errors.New("authorization snapshot does not bind current action intent")
	}
	if err := c.Revalidator.Revalidate(ctx, intent, auth); err != nil {
		return Result{}, fmt.Errorf("commit-time authorization failed: %w", err)
	}
	if err := c.Preconditions.Check(ctx, intent); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrPreconditionChanged, err)
	}
	result, err := c.Dispatcher.Dispatch(ctx, intent, idempotencyKey)
	if err != nil {
		return Result{}, fmt.Errorf("dispatch effect: %w", err)
	}
	return result, nil
}

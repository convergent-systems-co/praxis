package state

import (
	"errors"
	"fmt"
)

type EffectState string

const (
	EffectPending     EffectState = "pending"
	EffectDispatched  EffectState = "dispatched"
	EffectSucceeded   EffectState = "succeeded"
	EffectFailed      EffectState = "failed"
	EffectUnknown     EffectState = "unknown"
	EffectReconciling EffectState = "reconciling"
)

type RecoveryAction string

const (
	RecoveryNone       RecoveryAction = "none"
	RecoveryDispatch   RecoveryAction = "dispatch"
	RecoveryReconcile  RecoveryAction = "reconcile"
	RecoveryFailClosed RecoveryAction = "fail-closed"
)

type RecoverableEffect struct {
	ID              string
	State           EffectState
	IdempotencyKey  string
	ReconcileCapable bool
}

// ClassifyEffectRecovery decides what startup may do without guessing whether
// an external side effect happened.
func ClassifyEffectRecovery(e RecoverableEffect) (RecoveryAction, error) {
	if e.ID == "" {
		return RecoveryFailClosed, errors.New("effect id is required")
	}
	switch e.State {
	case EffectSucceeded, EffectFailed:
		return RecoveryNone, nil
	case EffectPending:
		return RecoveryDispatch, nil
	case EffectDispatched, EffectUnknown, EffectReconciling:
		if e.ReconcileCapable {
			return RecoveryReconcile, nil
		}
		if e.IdempotencyKey != "" {
			// Even with an idempotency key, the runtime should reconcile if possible;
			// absent reconciliation support it may safely re-dispatch only through a
			// later effect executor that verifies target idempotency semantics.
			return RecoveryDispatch, nil
		}
		return RecoveryFailClosed, fmt.Errorf("effect %s has ambiguous outcome and no reconciliation/idempotency support", e.ID)
	default:
		return RecoveryFailClosed, fmt.Errorf("unknown effect state %q", e.State)
	}
}

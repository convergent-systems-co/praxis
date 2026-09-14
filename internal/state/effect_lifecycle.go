package state

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// MarkEffectDispatched records that an external dispatch was attempted. It is
// separate from the external result and therefore remains safe across a crash.
func (s *Store) MarkEffectDispatched(ctx context.Context, effectID string, now time.Time) error {
	if s == nil || s.db == nil {
		return errors.New("state store is required")
	}
	if effectID == "" {
		return errors.New("effect identity is required")
	}
	res, err := s.db.ExecContext(ctx, `UPDATE effects SET state=?, attempts=attempts+1, updated_at=? WHERE effect_id=? AND state IN (?,?)`, string(EffectDispatched), now.UTC().Format(time.RFC3339Nano), effectID, string(EffectPending), string(EffectReconciling))
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return fmt.Errorf("effect %s is not dispatchable", effectID)
	}
	return nil
}

// MarkEffectOutcome durably records an observed external result. Unknown
// outcomes remain explicit and cannot be treated as success by omission.
func (s *Store) MarkEffectOutcome(ctx context.Context, effectID string, state EffectState, observed, reconciliation []byte, now time.Time) error {
	if s == nil || s.db == nil {
		return errors.New("state store is required")
	}
	if effectID == "" {
		return errors.New("effect identity is required")
	}
	if state != EffectSucceeded && state != EffectFailed && state != EffectUnknown && state != EffectReconciling {
		return errors.New("invalid effect outcome state")
	}
	res, err := s.db.ExecContext(ctx, `UPDATE effects SET state=?, observed_result=?, reconciliation_evidence=?, updated_at=? WHERE effect_id=? AND state IN (?,?,?,?)`, string(state), bytesOrNil(observed), bytesOrNil(reconciliation), now.UTC().Format(time.RFC3339Nano), effectID, string(EffectPending), string(EffectDispatched), string(EffectUnknown), string(EffectReconciling))
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return fmt.Errorf("effect %s has no valid outcome transition", effectID)
	}
	return nil
}

func (s *Store) RecoverableEffects(ctx context.Context) ([]RecoverableEffect, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("state store is required")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT effect_id,state,COALESCE(idempotency_key,'') FROM effects WHERE state IN (?,?,?) ORDER BY effect_id`, string(EffectPending), string(EffectDispatched), string(EffectUnknown))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RecoverableEffect
	for rows.Next() {
		var e RecoverableEffect
		var state string
		if err := rows.Scan(&e.ID, &state, &e.IdempotencyKey); err != nil {
			return nil, err
		}
		e.State = EffectState(state)
		e.IdempotencyVerified = e.IdempotencyKey != ""
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// CommitObservationResolution atomically appends the resolution command/event
// and closes one UNKNOWN observational effect. It preserves the original
// observed result and is deliberately narrower than general reconciliation.
func (s *Store) CommitObservationResolution(ctx context.Context, cmd CommandRecord, event EventRecord, effectID string, evidence []byte, now time.Time) error {
	if s == nil || s.db == nil || effectID == "" || len(evidence) == 0 {
		return errors.New("observation resolution inputs are required")
	}
	if cmd.ID == "" || event.ID == "" || event.CommandID != cmd.ID {
		return errors.New("observation resolution command/event identity is required")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := insertCommand(ctx, tx, cmd); err != nil {
		return err
	}
	if err := insertEvent(ctx, tx, event); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `UPDATE effects SET state=?, reconciliation_evidence=?, updated_at=? WHERE effect_id=? AND state=? AND attempts=1 AND observed_result IS NOT NULL`, string(EffectSucceeded), evidence, now.UTC().Format(time.RFC3339Nano), effectID, string(EffectUnknown))
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return fmt.Errorf("effect %s is not an unresolved observational effect", effectID)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE commands SET status='committed', completed_at=? WHERE command_id=?`, event.CreatedAt.UTC().Format(time.RFC3339Nano), cmd.ID); err != nil {
		return err
	}
	return tx.Commit()
}

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

// ReconcileEffect closes an unknown/reconciling outcome only with explicit
// evidence from the external authority. Empty evidence is rejected so a
// caller cannot manufacture certainty by selecting a terminal enum.
func (s *Store) ReconcileEffect(ctx context.Context, effectID string, terminal EffectState, evidence []byte, now time.Time) error {
	if len(evidence) == 0 {
		return errors.New("effect reconciliation evidence is required")
	}
	if terminal != EffectSucceeded && terminal != EffectFailed {
		return errors.New("effect reconciliation must produce succeeded or failed")
	}
	return s.MarkEffectOutcome(ctx, effectID, terminal, nil, evidence, now)
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

package state

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/convergent-systems-co/praxis/internal/plugin"
)

// LoadSupervisorSnapshots reconstructs only durable launch/failure state. It
// never reconstructs runtime advertisement or registry readiness.
func (s *Store) LoadSupervisorSnapshots(ctx context.Context) ([]plugin.SupervisorSnapshot, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("state store is required")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT snapshot_json FROM plugin_supervisor_snapshots ORDER BY instance_id`)
	if err != nil {
		return nil, fmt.Errorf("load plugin supervisor snapshots: %w", err)
	}
	defer rows.Close()
	out := make([]plugin.SupervisorSnapshot, 0)
	for rows.Next() {
		var body []byte
		if err := rows.Scan(&body); err != nil {
			return nil, err
		}
		var snapshot plugin.SupervisorSnapshot
		if err := json.Unmarshal(body, &snapshot); err != nil {
			return nil, fmt.Errorf("decode plugin supervisor snapshot: %w", err)
		}
		if snapshot.Provider.Identity.InstanceID == "" {
			return nil, errors.New("plugin supervisor snapshot has no instance identity")
		}
		out = append(out, snapshot)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) SaveSupervisorSnapshot(ctx context.Context, snapshot plugin.SupervisorSnapshot) error {
	if s == nil || s.db == nil {
		return errors.New("state store is required")
	}
	if snapshot.Provider.Identity.InstanceID == "" {
		return errors.New("plugin supervisor snapshot requires instance identity")
	}
	body, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("encode plugin supervisor snapshot: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO plugin_supervisor_snapshots(instance_id,snapshot_json,updated_at) VALUES(?,?,?) ON CONFLICT(instance_id) DO UPDATE SET snapshot_json=excluded.snapshot_json, updated_at=excluded.updated_at`, snapshot.Provider.Identity.InstanceID, body, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("save plugin supervisor snapshot: %w", err)
	}
	return nil
}

var _ interface {
	LoadSupervisorSnapshots(context.Context) ([]plugin.SupervisorSnapshot, error)
	SaveSupervisorSnapshot(context.Context, plugin.SupervisorSnapshot) error
} = (*Store)(nil)

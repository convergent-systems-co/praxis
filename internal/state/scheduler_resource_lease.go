package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/convergent-systems-co/praxis/internal/scheduler"
)

var ErrResourceUnavailable = errors.New("scheduler resource unavailable")

func (s *Store) DefineSchedulerResource(ctx context.Context, state scheduler.ResourceState) error {
	if s == nil || s.db == nil {
		return errors.New("state store is required")
	}
	if state.Key == "" || state.Capacity <= 0 {
		return errors.New("scheduler resource key and positive capacity are required")
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO scheduler_resources(resource_key,capacity,exclusive) VALUES(?,?,?) ON CONFLICT(resource_key) DO UPDATE SET capacity=excluded.capacity, exclusive=excluded.exclusive`, state.Key, state.Capacity, boolInt(state.Exclusive))
	return err
}

// AcquireSchedulerResourceLeases atomically acquires every ordered
// requirement, expiring stale leases before admission. No prefix is retained.
func (s *Store) AcquireSchedulerResourceLeases(ctx context.Context, sliceID, attemptID string, requirements []scheduler.ResourceRequirement, now time.Time, expiry *time.Time) ([]scheduler.ResourceLease, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("state store is required")
	}
	if sliceID == "" || attemptID == "" {
		return nil, errors.New("slice and attempt identity are required")
	}
	slice := scheduler.Slice{ID: sliceID, Resources: append([]scheduler.ResourceRequirement(nil), requirements...)}
	ordered, err := slice.OrderedRequirements()
	if err != nil {
		return nil, err
	}
	now = now.UTC()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	for _, req := range ordered {
		if _, err := tx.ExecContext(ctx, `UPDATE scheduler_resource_leases SET released_at=? WHERE released_at IS NULL AND expires_at IS NOT NULL AND expires_at<=? AND resource_key=?`, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), req.Key); err != nil {
			return nil, err
		}
	}
	for _, req := range ordered {
		var capacity int64
		var exclusive int
		if err := tx.QueryRowContext(ctx, `SELECT capacity,exclusive FROM scheduler_resources WHERE resource_key=?`, req.Key).Scan(&capacity, &exclusive); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, ErrResourceUnavailable
			}
			return nil, err
		}
		var allocated int64
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(capacity),0) FROM scheduler_resource_leases WHERE resource_key=? AND released_at IS NULL`, req.Key).Scan(&allocated); err != nil {
			return nil, err
		}
		available := (exclusive != 0 || req.Exclusive) && allocated == 0 && req.Capacity <= capacity || exclusive == 0 && !req.Exclusive && capacity-allocated >= req.Capacity
		if !available {
			return nil, ErrResourceUnavailable
		}
	}
	leases := make([]scheduler.ResourceLease, 0, len(ordered))
	for i, req := range ordered {
		id := fmt.Sprintf("%s:%s:%s:%d", sliceID, attemptID, req.Key, i)
		var expiryText any
		if expiry != nil {
			expiryText = expiry.UTC().Format(time.RFC3339Nano)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO scheduler_resource_leases(lease_id,slice_id,attempt_id,resource_key,capacity,acquired_at,expires_at) VALUES(?,?,?,?,?,?,?)`, id, sliceID, attemptID, req.Key, req.Capacity, now.Format(time.RFC3339Nano), expiryText); err != nil {
			return nil, err
		}
		lease := scheduler.ResourceLease{ID: id, SliceID: sliceID, AttemptID: attemptID, ResourceKey: req.Key, Capacity: req.Capacity, AcquiredAt: now}
		if expiry != nil {
			e := expiry.UTC()
			lease.ExpiresAt = &e
		}
		leases = append(leases, lease)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return leases, nil
}

func (s *Store) ReleaseSchedulerResourceLeases(ctx context.Context, leaseIDs []string, now time.Time) error {
	if s == nil || s.db == nil {
		return errors.New("state store is required")
	}
	ids := append([]string(nil), leaseIDs...)
	sort.Strings(ids)
	now = now.UTC()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range ids {
		if id == "" {
			return errors.New("lease identity is required")
		}
		if _, err := tx.ExecContext(ctx, `UPDATE scheduler_resource_leases SET released_at=? WHERE lease_id=? AND released_at IS NULL`, now.Format(time.RFC3339Nano), id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// RecoverSchedulerResourceLeases fences expired active leases and returns the
// recovered records for durable observability. Recovery is idempotent.
func (s *Store) RecoverSchedulerResourceLeases(ctx context.Context, now time.Time) ([]scheduler.ResourceLease, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("state store is required")
	}
	now = now.UTC()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT lease_id,slice_id,attempt_id,resource_key,capacity,acquired_at,expires_at FROM scheduler_resource_leases WHERE released_at IS NULL AND expires_at IS NOT NULL AND expires_at<=? ORDER BY lease_id`, now.Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	var recovered []scheduler.ResourceLease
	for rows.Next() {
		var lease scheduler.ResourceLease
		var acquired, expiry string
		if err := rows.Scan(&lease.ID, &lease.SliceID, &lease.AttemptID, &lease.ResourceKey, &lease.Capacity, &acquired, &expiry); err != nil {
			rows.Close()
			return nil, err
		}
		lease.AcquiredAt, err = time.Parse(time.RFC3339Nano, acquired)
		if err != nil {
			rows.Close()
			return nil, err
		}
		expired, err := time.Parse(time.RFC3339Nano, expiry)
		if err != nil {
			rows.Close()
			return nil, err
		}
		lease.ExpiresAt = &expired
		recovered = append(recovered, lease)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if len(recovered) == 0 {
		return recovered, tx.Commit()
	}
	if _, err := tx.ExecContext(ctx, `UPDATE scheduler_resource_leases SET released_at=? WHERE released_at IS NULL AND expires_at IS NOT NULL AND expires_at<=?`, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	released := now
	for i := range recovered {
		recovered[i].ReleasedAt = &released
	}
	return recovered, nil
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

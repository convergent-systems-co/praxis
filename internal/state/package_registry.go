package state

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

var reservedCoreCommands = map[string]struct{}{
	"discover": {}, "info": {}, "install": {}, "update": {}, "uninstall": {},
	"list": {}, "help": {}, "status": {}, "resume": {}, "cancel": {},
	"doctor": {}, "version": {},
}

func ReservedCoreCommands() []string {
	out := make([]string, 0, len(reservedCoreCommands))
	for k := range reservedCoreCommands {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func IsReservedCoreCommand(name string) bool {
	_, ok := reservedCoreCommands[name]
	return ok
}

type RegisteredInvocation struct {
	Contract       contracts.InvocationContract
	ContentDigest  string
	ContractDigest string
}

// ActivatePackage atomically installs/activates one immutable package generation
// and publishes its complete InvocationContract set. No alias becomes visible if
// any contract or collision check fails.
func (s *Store) ActivatePackage(ctx context.Context, manifest packagecatalog.Manifest, sourceKind, sourceRef string, now time.Time) error {
	if s == nil || s.db == nil {
		return errors.New("state store is required")
	}
	if err := manifest.Validate(); err != nil {
		return err
	}
	if sourceKind == "" || sourceRef == "" {
		return errors.New("package source kind and ref are required")
	}
	for _, inv := range manifest.Invocations {
		for _, alias := range inv.Aliases {
			if IsReservedCoreCommand(alias) {
				return fmt.Errorf("package cannot register reserved core command %q", alias)
			}
		}
	}

	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("marshal package manifest: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin package activation: %w", err)
	}
	defer tx.Rollback()

	for _, inv := range manifest.Invocations {
		for _, alias := range inv.Aliases {
			var existingPackage string
			err := tx.QueryRowContext(ctx, `SELECT ir.package_id FROM invocation_aliases ia JOIN invocation_registry ir ON ir.entry_point_id=ia.entry_point_id AND ir.package_version=ia.package_version AND ir.content_digest=ia.content_digest WHERE ia.alias=? AND ir.active=1`, alias).Scan(&existingPackage)
			if err == nil && existingPackage != manifest.PackageID {
				return fmt.Errorf("invocation alias %q already owned by active package %q", alias, existingPackage)
			}
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("check alias collision: %w", err)
			}
		}
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM invocation_aliases WHERE (entry_point_id,package_version,content_digest) IN (SELECT entry_point_id,package_version,content_digest FROM invocation_registry WHERE package_id=? AND active=1)`, manifest.PackageID); err != nil {
		return fmt.Errorf("remove previous invocation aliases: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE invocation_registry SET active=0 WHERE package_id=? AND active=1`, manifest.PackageID); err != nil {
		return fmt.Errorf("deactivate previous invocations: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE installed_packages SET state='installed' WHERE package_id=? AND state='active'`, manifest.PackageID); err != nil {
		return fmt.Errorf("deactivate previous package generation: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO installed_packages(package_id,package_version,content_digest,state,source_kind,source_ref,manifest_json,installed_at,activated_at) VALUES(?,?,?,?,?,?,?,?,?)
		ON CONFLICT(package_id,package_version,content_digest) DO UPDATE SET state=excluded.state,source_kind=excluded.source_kind,source_ref=excluded.source_ref,manifest_json=excluded.manifest_json,activated_at=excluded.activated_at`,
		manifest.PackageID, manifest.Version, manifest.ContentDigest, "active", sourceKind, sourceRef, manifestJSON, now.UTC().Format(time.RFC3339Nano), now.UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("persist package generation: %w", err)
	}

	for _, inv := range manifest.Invocations {
		body, err := json.Marshal(inv)
		if err != nil {
			return fmt.Errorf("marshal invocation contract: %w", err)
		}
		sum := sha256.Sum256(body)
		digest := "sha256:" + hex.EncodeToString(sum[:])
		if _, err := tx.ExecContext(ctx, `INSERT INTO invocation_registry(entry_point_id,package_id,package_version,content_digest,graph_id,graph_version,contract_json,contract_digest,active,registered_at) VALUES(?,?,?,?,?,?,?,?,1,?)
			ON CONFLICT(entry_point_id,package_version,content_digest) DO UPDATE SET package_id=excluded.package_id,graph_id=excluded.graph_id,graph_version=excluded.graph_version,contract_json=excluded.contract_json,contract_digest=excluded.contract_digest,active=1,registered_at=excluded.registered_at`,
			inv.EntryPointID, manifest.PackageID, manifest.Version, manifest.ContentDigest, inv.GraphID, inv.GraphVersion, body, digest, now.UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("register invocation %q: %w", inv.EntryPointID, err)
		}
		for _, alias := range inv.Aliases {
			if _, err := tx.ExecContext(ctx, `INSERT INTO invocation_aliases(alias,entry_point_id,package_version,content_digest) VALUES(?,?,?,?)`, alias, inv.EntryPointID, manifest.Version, manifest.ContentDigest); err != nil {
				return fmt.Errorf("register invocation alias %q: %w", alias, err)
			}
		}
	}
	return tx.Commit()
}

func (s *Store) DeactivatePackage(ctx context.Context, packageID string) error {
	if s == nil || s.db == nil || packageID == "" {
		return errors.New("state store and package id are required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM invocation_aliases WHERE (entry_point_id,package_version,content_digest) IN (SELECT entry_point_id,package_version,content_digest FROM invocation_registry WHERE package_id=? AND active=1)`, packageID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE invocation_registry SET active=0 WHERE package_id=?`, packageID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE installed_packages SET state='disabled' WHERE package_id=? AND state='active'`, packageID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) RemovePackage(ctx context.Context, packageID string) error {
	if err := s.DeactivatePackage(ctx, packageID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE installed_packages SET state='removed' WHERE package_id=?`, packageID)
	return err
}

func (s *Store) ActiveInvocations(ctx context.Context) ([]RegisteredInvocation, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT contract_json,content_digest,contract_digest FROM invocation_registry WHERE active=1 ORDER BY entry_point_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RegisteredInvocation
	for rows.Next() {
		var body []byte
		var item RegisteredInvocation
		if err := rows.Scan(&body, &item.ContentDigest, &item.ContractDigest); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(body, &item.Contract); err != nil {
			return nil, fmt.Errorf("decode invocation registry: %w", err)
		}
		if err := item.Contract.Validate(); err != nil {
			return nil, fmt.Errorf("invalid persisted invocation %q: %w", item.Contract.EntryPointID, err)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) ResolveInvocationAlias(ctx context.Context, alias string) (RegisteredInvocation, error) {
	var body []byte
	var item RegisteredInvocation
	err := s.db.QueryRowContext(ctx, `SELECT ir.contract_json,ir.content_digest,ir.contract_digest FROM invocation_aliases ia JOIN invocation_registry ir ON ir.entry_point_id=ia.entry_point_id AND ir.package_version=ia.package_version AND ir.content_digest=ia.content_digest WHERE ia.alias=? AND ir.active=1`, alias).Scan(&body, &item.ContentDigest, &item.ContractDigest)
	if err != nil {
		return RegisteredInvocation{}, err
	}
	if err := json.Unmarshal(body, &item.Contract); err != nil {
		return RegisteredInvocation{}, err
	}
	return item, item.Contract.Validate()
}

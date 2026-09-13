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

type RegisteredContent struct {
	PackageID      string
	PackageVersion string
	PackageDigest  string
	Content        packagecatalog.ContentRef
}

// ActivatePackage atomically consumes exact local authority, records immutable
// verification evidence, activates one package generation, and publishes its
// invocation and typed-content registrations.
func (s *Store) ActivatePackage(ctx context.Context, request packagecatalog.ActivationRequest, now time.Time) error {
	if s == nil || s.db == nil {
		return errors.New("state store is required")
	}
	if err := request.Validate(); err != nil {
		return err
	}
	manifest := request.Package.Manifest()
	verification := request.Package.Evidence()
	sourceKind, sourceRef := verification.SourceKind, verification.SourceRef
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
	intentDigest, err := request.Intent.Digest()
	if err != nil {
		return err
	}
	if err := consumePackageApproval(ctx, tx, request.ApprovalID, request.Intent.Actor, intentDigest, now); err != nil {
		return err
	}

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
	for _, content := range manifest.Contents {
		var existingPackage string
		err := tx.QueryRowContext(ctx, `SELECT package_id FROM package_contents WHERE kind=? AND content_id=? AND content_version=? AND active=1`, content.Kind, content.ID, content.Version).Scan(&existingPackage)
		if err == nil && existingPackage != manifest.PackageID {
			return fmt.Errorf("package content %s/%s@%s already active from package %q", content.Kind, content.ID, content.Version, existingPackage)
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("check package content collision: %w", err)
		}
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM invocation_aliases WHERE (entry_point_id,package_version,content_digest) IN (SELECT entry_point_id,package_version,content_digest FROM invocation_registry WHERE package_id=? AND active=1)`, manifest.PackageID); err != nil {
		return fmt.Errorf("remove previous invocation aliases: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE invocation_registry SET active=0 WHERE package_id=? AND active=1`, manifest.PackageID); err != nil {
		return fmt.Errorf("deactivate previous invocations: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE package_contents SET active=0 WHERE package_id=? AND active=1`, manifest.PackageID); err != nil {
		return fmt.Errorf("deactivate previous package contents: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE installed_packages SET state='installed' WHERE package_id=? AND state='active'`, manifest.PackageID); err != nil {
		return fmt.Errorf("deactivate previous package generation: %w", err)
	}

	stamp := now.UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `INSERT INTO installed_packages(package_id,package_version,content_digest,state,source_kind,source_ref,manifest_json,installed_at,activated_at) VALUES(?,?,?,?,?,?,?,?,?)
		ON CONFLICT(package_id,package_version,content_digest) DO UPDATE SET state=excluded.state,source_kind=excluded.source_kind,source_ref=excluded.source_ref,manifest_json=excluded.manifest_json,activated_at=excluded.activated_at`,
		manifest.PackageID, manifest.Version, manifest.ContentDigest, "active", sourceKind, sourceRef, manifestJSON, stamp, stamp); err != nil {
		return fmt.Errorf("persist package generation: %w", err)
	}
	verificationJSON, err := json.Marshal(verification)
	if err != nil {
		return fmt.Errorf("marshal package verification: %w", err)
	}
	signatureJSON, err := json.Marshal(request.Package.Signature())
	if err != nil {
		return fmt.Errorf("marshal package signature: %w", err)
	}
	intentJSON, err := json.Marshal(request.Intent)
	if err != nil {
		return fmt.Errorf("marshal package activation intent: %w", err)
	}
	activationSum := sha256.Sum256([]byte(verification.ID + "\x00" + request.ApprovalID))
	activationID := "package-activation:sha256:" + hex.EncodeToString(activationSum[:])
	if _, err := tx.ExecContext(ctx, `INSERT INTO package_activation_receipts(activation_id,package_id,package_version,content_digest,verification_id,verification_json,manifest_bytes,signature_json,activation_intent_json,activation_intent_digest,approval_id,authority_id,authority_kind,activated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		activationID, manifest.PackageID, manifest.Version, manifest.ContentDigest, verification.ID, verificationJSON, request.Package.ManifestBytes(), signatureJSON, intentJSON, intentDigest, request.ApprovalID, request.Intent.Actor.ID, request.Intent.Actor.Kind, stamp); err != nil {
		return fmt.Errorf("persist package activation receipt: %w", err)
	}

	for _, content := range manifest.Contents {
		if _, err := tx.ExecContext(ctx, `INSERT INTO package_contents(package_id,package_version,content_digest,kind,content_id,content_version,artifact_digest,artifact_ref,compatibility,active,registered_at) VALUES(?,?,?,?,?,?,?,?,?,1,?)
			ON CONFLICT(package_id,package_version,content_digest,kind,content_id,content_version) DO UPDATE SET artifact_digest=excluded.artifact_digest,artifact_ref=excluded.artifact_ref,compatibility=excluded.compatibility,active=1,registered_at=excluded.registered_at`,
			manifest.PackageID, manifest.Version, manifest.ContentDigest, content.Kind, content.ID, content.Version, content.Digest, content.Artifact, nullable(content.Compatibility), stamp); err != nil {
			return fmt.Errorf("register package content %s/%s@%s: %w", content.Kind, content.ID, content.Version, err)
		}
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
			inv.EntryPointID, manifest.PackageID, manifest.Version, manifest.ContentDigest, inv.GraphID, inv.GraphVersion, body, digest, stamp); err != nil {
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

func consumePackageApproval(ctx context.Context, tx *sql.Tx, id string, actor contracts.PrincipalRef, intentDigest string, now time.Time) error {
	var approverID, approverKind string
	var persistedDigest sql.NullString
	var issued string
	var expires, revoked sql.NullString
	var remaining, version int64
	if err := tx.QueryRowContext(ctx, `SELECT approver_id,approver_kind,intent_digest,issued_at,expires_at,revoked_at,remaining_uses,version FROM approvals WHERE approval_id=?`, id).Scan(&approverID, &approverKind, &persistedDigest, &issued, &expires, &revoked, &remaining, &version); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrApprovalUnavailable
		}
		return fmt.Errorf("load package activation approval: %w", err)
	}
	if approverID != actor.ID || approverKind != actor.Kind || !persistedDigest.Valid || persistedDigest.String != intentDigest || revoked.Valid || remaining < 1 {
		return ErrApprovalUnavailable
	}
	if _, err := time.Parse(time.RFC3339Nano, issued); err != nil {
		return fmt.Errorf("parse package approval issue time: %w", err)
	}
	if expires.Valid {
		expiresAt, err := time.Parse(time.RFC3339Nano, expires.String)
		if err != nil {
			return fmt.Errorf("parse package approval expiry: %w", err)
		}
		if !now.UTC().Before(expiresAt.UTC()) {
			return ErrApprovalUnavailable
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE approvals SET remaining_uses=remaining_uses-1,version=version+1 WHERE approval_id=? AND version=? AND remaining_uses>0 AND revoked_at IS NULL`, id, version)
	if err != nil {
		return fmt.Errorf("consume package activation approval: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil || changed != 1 {
		return ErrApprovalUnavailable
	}
	return nil
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
	if _, err := tx.ExecContext(ctx, `UPDATE package_contents SET active=0 WHERE package_id=?`, packageID); err != nil {
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

func (s *Store) ActiveContents(ctx context.Context, kind packagecatalog.ContentKind) ([]RegisteredContent, error) {
	if kind == "" {
		return nil, errors.New("content kind is required")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT package_id,package_version,content_digest,kind,content_id,content_version,artifact_digest,artifact_ref,COALESCE(compatibility,'') FROM package_contents WHERE active=1 AND kind=? ORDER BY content_id,content_version`, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RegisteredContent
	for rows.Next() {
		var item RegisteredContent
		if err := rows.Scan(&item.PackageID, &item.PackageVersion, &item.PackageDigest, &item.Content.Kind, &item.Content.ID, &item.Content.Version, &item.Content.Digest, &item.Content.Artifact, &item.Content.Compatibility); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) ResolveContent(ctx context.Context, kind packagecatalog.ContentKind, id, version string) (RegisteredContent, error) {
	if kind == "" || id == "" || version == "" {
		return RegisteredContent{}, errors.New("content kind, id, and version are required")
	}
	var item RegisteredContent
	err := s.db.QueryRowContext(ctx, `SELECT package_id,package_version,content_digest,kind,content_id,content_version,artifact_digest,artifact_ref,COALESCE(compatibility,'') FROM package_contents WHERE active=1 AND kind=? AND content_id=? AND content_version=?`, kind, id, version).Scan(&item.PackageID, &item.PackageVersion, &item.PackageDigest, &item.Content.Kind, &item.Content.ID, &item.Content.Version, &item.Content.Digest, &item.Content.Artifact, &item.Content.Compatibility)
	return item, err
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

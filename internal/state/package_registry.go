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
	"discover": {}, "info": {}, "install": {}, "update": {}, "rollback": {}, "disable": {}, "uninstall": {},
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

func digestPackageBytes(body []byte) string {
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}

type RegisteredInvocation struct {
	Contract       contracts.InvocationContract
	ContentDigest  string
	ContractDigest string
	RuntimeBinding contracts.InvocationRuntimeBinding
}

type RegisteredContent struct {
	PackageID      string
	PackageVersion string
	PackageDigest  string
	Content        packagecatalog.ContentRef
	ArtifactBytes  []byte
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
	if len(request.Package.Manifest().Dependencies) != 0 {
		return errors.New("package with dependencies requires closure-bound deployment")
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
	manifest := request.Package.Manifest()
	singleTarget := map[string]packagecatalog.PackageIdentity{manifest.PackageID: {PackageID: manifest.PackageID, Version: manifest.Version, ContentDigest: manifest.ContentDigest}}
	if err := validateActiveDependencyCompatibilityTx(ctx, tx, singleTarget, singleTarget); err != nil {
		return err
	}
	if err := activateVerifiedPackageTx(ctx, tx, request.Package, request.Intent, intentDigest, request.ApprovalID, now); err != nil {
		return err
	}
	return tx.Commit()
}

// DeployPackages activates an exact verified dependency closure in one
// transaction. One approval authorizes the closure-bound deployment intent;
// any collision or persistence failure rolls back every generation and the
// authority consumption.
func (s *Store) DeployPackages(ctx context.Context, request packagecatalog.DeploymentRequest, now time.Time) error {
	if s == nil || s.db == nil {
		return errors.New("state store is required")
	}
	if err := request.Validate(); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin package deployment: %w", err)
	}
	defer tx.Rollback()
	intentDigest, err := request.Intent.Digest()
	if err != nil {
		return err
	}
	if err := consumePackageApproval(ctx, tx, request.ApprovalID, request.Intent.Actor, intentDigest, now); err != nil {
		return err
	}
	targets := make(map[string]packagecatalog.PackageIdentity, len(request.Packages))
	for _, pkg := range request.Packages {
		manifest := pkg.Manifest()
		targets[manifest.PackageID] = packagecatalog.PackageIdentity{PackageID: manifest.PackageID, Version: manifest.Version, ContentDigest: manifest.ContentDigest}
	}
	if err := validateActiveDependencyCompatibilityTx(ctx, tx, targets, targets); err != nil {
		return err
	}
	for _, pkg := range request.Packages {
		if err := activateVerifiedPackageTx(ctx, tx, pkg, request.Intent, intentDigest, request.ApprovalID, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func validateActiveDependencyCompatibilityTx(ctx context.Context, tx *sql.Tx, targets map[string]packagecatalog.PackageIdentity, affected map[string]packagecatalog.PackageIdentity) error {
	rows, err := tx.QueryContext(ctx, `SELECT package_id,manifest_json FROM installed_packages WHERE state='active'`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var packageID string
		var manifestJSON []byte
		if err := rows.Scan(&packageID, &manifestJSON); err != nil {
			return err
		}
		if _, changing := affected[packageID]; changing {
			continue
		}
		var manifest packagecatalog.Manifest
		if err := json.Unmarshal(manifestJSON, &manifest); err != nil {
			return fmt.Errorf("decode active dependent %q: %w", packageID, err)
		}
		for _, dependency := range manifest.Dependencies {
			_, changing := affected[dependency.PackageID]
			target, retained := targets[dependency.PackageID]
			if changing && (!retained || dependency.Version != target.Version || dependency.Digest != target.ContentDigest) {
				return fmt.Errorf("package generation %s@%s is required by active package %q", dependency.PackageID, dependency.Version, packageID)
			}
		}
	}
	return rows.Err()
}

func activateVerifiedPackageTx(ctx context.Context, tx *sql.Tx, pkg packagecatalog.VerifiedPackage, intent contracts.ActionIntent, intentDigest, approvalID string, now time.Time) error {
	if err := validateVerifiedPluginContents(pkg); err != nil {
		return fmt.Errorf("validate package plugin contents: %w", err)
	}
	manifest := pkg.Manifest()
	verification := pkg.Evidence()
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
	signatureJSON, err := json.Marshal(pkg.Signature())
	if err != nil {
		return fmt.Errorf("marshal package signature: %w", err)
	}
	intentJSON, err := json.Marshal(intent)
	if err != nil {
		return fmt.Errorf("marshal package activation intent: %w", err)
	}
	activationSum := sha256.Sum256([]byte(verification.ID + "\x00" + approvalID))
	activationID := "package-activation:sha256:" + hex.EncodeToString(activationSum[:])
	if _, err := tx.ExecContext(ctx, `INSERT INTO package_activation_receipts(activation_id,package_id,package_version,content_digest,verification_id,verification_json,manifest_bytes,signature_json,activation_intent_json,activation_intent_digest,approval_id,authority_id,authority_kind,activated_at,artifact_bytes) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		activationID, manifest.PackageID, manifest.Version, manifest.ContentDigest, verification.ID, verificationJSON, pkg.ManifestBytes(), signatureJSON, intentJSON, intentDigest, approvalID, intent.Actor.ID, intent.Actor.Kind, stamp, pkg.ArtifactBytes()); err != nil {
		return fmt.Errorf("persist package activation receipt: %w", err)
	}

	for _, content := range manifest.Contents {
		artifactBytes, err := pkg.ContentBytes(content)
		if err != nil {
			return fmt.Errorf("load verified package content %s/%s@%s: %w", content.Kind, content.ID, content.Version, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO package_contents(package_id,package_version,content_digest,kind,content_id,content_version,artifact_digest,artifact_ref,compatibility,active,registered_at,artifact_bytes) VALUES(?,?,?,?,?,?,?,?,?,1,?,?)
			ON CONFLICT(package_id,package_version,content_digest,kind,content_id,content_version) DO UPDATE SET artifact_digest=excluded.artifact_digest,artifact_ref=excluded.artifact_ref,compatibility=excluded.compatibility,active=1,registered_at=excluded.registered_at,artifact_bytes=excluded.artifact_bytes`,
			manifest.PackageID, manifest.Version, manifest.ContentDigest, content.Kind, content.ID, content.Version, content.Digest, content.Artifact, nullable(content.Compatibility), stamp, artifactBytes); err != nil {
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
		runtimeDigest := digestPackageBytes(body)
		runtime := contracts.InvocationRuntimeBinding{
			PackageID: manifest.PackageID, PackageVersion: manifest.Version, PackageDigest: manifest.ContentDigest,
			EntryPointID: inv.EntryPointID, ContractDigest: digest, RuntimeID: "client-adapter:" + inv.PackageID + ":" + inv.EntryPointID,
			RuntimeVersion: inv.Version, RuntimeDigest: runtimeDigest,
		}
		if err := runtime.Validate(); err != nil {
			return fmt.Errorf("validate invocation runtime binding %q: %w", inv.EntryPointID, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO invocation_runtime_bindings(entry_point_id,package_version,content_digest,package_id,contract_digest,runtime_id,runtime_version,runtime_digest,registered_at) VALUES(?,?,?,?,?,?,?,?,?)`, runtime.EntryPointID, runtime.PackageVersion, runtime.PackageDigest, runtime.PackageID, runtime.ContractDigest, runtime.RuntimeID, runtime.RuntimeVersion, runtime.RuntimeDigest, stamp); err != nil {
			return fmt.Errorf("register invocation runtime binding %q: %w", inv.EntryPointID, err)
		}
		for _, alias := range inv.Aliases {
			if _, err := tx.ExecContext(ctx, `INSERT INTO invocation_aliases(alias,entry_point_id,package_version,content_digest) VALUES(?,?,?,?)`, alias, inv.EntryPointID, manifest.Version, manifest.ContentDigest); err != nil {
				return fmt.Errorf("register invocation alias %q: %w", alias, err)
			}
		}
	}
	return nil
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

func (s *Store) TransitionPackage(ctx context.Context, request packagecatalog.TransitionRequest, now time.Time) error {
	if s == nil || s.db == nil {
		return errors.New("state store is required")
	}
	if err := request.Validate(); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var persistedVersion, persistedDigest, state string
	if err := tx.QueryRowContext(ctx, `SELECT package_version,content_digest,state FROM installed_packages WHERE package_id=? AND state IN ('active','disabled') ORDER BY CASE state WHEN 'active' THEN 0 ELSE 1 END,activated_at DESC LIMIT 1`, request.Identity.PackageID).Scan(&persistedVersion, &persistedDigest, &state); err != nil {
		return err
	}
	if persistedVersion != request.Identity.Version || persistedDigest != request.Identity.ContentDigest {
		return errors.New("package transition target is not the selected installed generation")
	}
	if request.Operation == packagecatalog.TransitionDisable && state != "active" {
		return errors.New("only an active package can be disabled")
	}
	intentDigest, err := request.Intent.Digest()
	if err != nil {
		return err
	}
	if err := consumePackageApproval(ctx, tx, request.ApprovalID, request.Intent.Actor, intentDigest, now); err != nil {
		return err
	}
	packageID := request.Identity.PackageID
	if _, err := tx.ExecContext(ctx, `DELETE FROM invocation_aliases WHERE (entry_point_id,package_version,content_digest) IN (SELECT entry_point_id,package_version,content_digest FROM invocation_registry WHERE package_id=? AND active=1)`, packageID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE invocation_registry SET active=0 WHERE package_id=?`, packageID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE package_contents SET active=0 WHERE package_id=?`, packageID); err != nil {
		return err
	}
	nextState := "disabled"
	if request.Operation == packagecatalog.TransitionRemove {
		nextState = "removed"
	}
	if _, err := tx.ExecContext(ctx, `UPDATE installed_packages SET state=? WHERE package_id=? AND state<>'removed'`, nextState, packageID); err != nil {
		return err
	}
	intentJSON, err := json.Marshal(request.Intent)
	if err != nil {
		return err
	}
	transitionSum := sha256.Sum256([]byte(intentDigest + "\x00" + request.ApprovalID))
	transitionID := "package-transition:sha256:" + hex.EncodeToString(transitionSum[:])
	if _, err := tx.ExecContext(ctx, `INSERT INTO package_transition_receipts(transition_id,package_id,package_version,content_digest,operation,intent_json,intent_digest,approval_id,authority_id,authority_kind,transitioned_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, transitionID, packageID, request.Identity.Version, request.Identity.ContentDigest, request.Operation, intentJSON, intentDigest, request.ApprovalID, request.Intent.Actor.ID, request.Intent.Actor.Kind, now.UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ActiveInvocations(ctx context.Context) ([]RegisteredInvocation, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT ir.contract_json,ir.content_digest,ir.contract_digest,rb.package_id,rb.package_version,rb.content_digest,rb.entry_point_id,rb.contract_digest,rb.runtime_id,rb.runtime_version,rb.runtime_digest FROM invocation_registry ir JOIN invocation_runtime_bindings rb ON rb.entry_point_id=ir.entry_point_id AND rb.package_version=ir.package_version AND rb.content_digest=ir.content_digest WHERE ir.active=1 ORDER BY ir.entry_point_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RegisteredInvocation
	for rows.Next() {
		var body []byte
		var item RegisteredInvocation
		if err := rows.Scan(&body, &item.ContentDigest, &item.ContractDigest, &item.RuntimeBinding.PackageID, &item.RuntimeBinding.PackageVersion, &item.RuntimeBinding.PackageDigest, &item.RuntimeBinding.EntryPointID, &item.RuntimeBinding.ContractDigest, &item.RuntimeBinding.RuntimeID, &item.RuntimeBinding.RuntimeVersion, &item.RuntimeBinding.RuntimeDigest); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(body, &item.Contract); err != nil {
			return nil, fmt.Errorf("decode invocation registry: %w", err)
		}
		if digestPackageBytes(body) != item.ContractDigest {
			return nil, fmt.Errorf("invocation contract %q digest mismatch", item.Contract.EntryPointID)
		}
		if err := item.Contract.Validate(); err != nil {
			return nil, fmt.Errorf("invalid persisted invocation %q: %w", item.Contract.EntryPointID, err)
		}
		if item.RuntimeBinding.PackageID != item.Contract.PackageID || item.RuntimeBinding.PackageVersion != item.Contract.PackageVersion || item.RuntimeBinding.PackageDigest != item.ContentDigest || item.RuntimeBinding.EntryPointID != item.Contract.EntryPointID || item.RuntimeBinding.ContractDigest != item.ContractDigest || item.RuntimeBinding.RuntimeDigest != item.ContractDigest {
			return nil, fmt.Errorf("invocation runtime binding %q does not match active package generation", item.Contract.EntryPointID)
		}
		if err := item.RuntimeBinding.Validate(); err != nil {
			return nil, fmt.Errorf("invalid invocation runtime binding %q: %w", item.Contract.EntryPointID, err)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) ActiveContents(ctx context.Context, kind packagecatalog.ContentKind) ([]RegisteredContent, error) {
	if kind == "" {
		return nil, errors.New("content kind is required")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT package_id,package_version,content_digest,kind,content_id,content_version,artifact_digest,artifact_ref,COALESCE(compatibility,''),artifact_bytes FROM package_contents WHERE active=1 AND kind=? ORDER BY content_id,content_version`, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RegisteredContent
	for rows.Next() {
		var item RegisteredContent
		if err := rows.Scan(&item.PackageID, &item.PackageVersion, &item.PackageDigest, &item.Content.Kind, &item.Content.ID, &item.Content.Version, &item.Content.Digest, &item.Content.Artifact, &item.Content.Compatibility, &item.ArtifactBytes); err != nil {
			return nil, err
		}
		if digestPackageBytes(item.ArtifactBytes) != item.Content.Digest {
			return nil, fmt.Errorf("persisted package content %s/%s@%s digest mismatch", item.Content.Kind, item.Content.ID, item.Content.Version)
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
	err := s.db.QueryRowContext(ctx, `SELECT package_id,package_version,content_digest,kind,content_id,content_version,artifact_digest,artifact_ref,COALESCE(compatibility,''),artifact_bytes FROM package_contents WHERE active=1 AND kind=? AND content_id=? AND content_version=?`, kind, id, version).Scan(&item.PackageID, &item.PackageVersion, &item.PackageDigest, &item.Content.Kind, &item.Content.ID, &item.Content.Version, &item.Content.Digest, &item.Content.Artifact, &item.Content.Compatibility, &item.ArtifactBytes)
	if err != nil {
		return RegisteredContent{}, err
	}
	if digestPackageBytes(item.ArtifactBytes) != item.Content.Digest {
		return RegisteredContent{}, errors.New("persisted package content artifact digest mismatch")
	}
	return item, nil
}

func (s *Store) ResolveInvocationAlias(ctx context.Context, alias string) (RegisteredInvocation, error) {
	var body []byte
	var item RegisteredInvocation
	err := s.db.QueryRowContext(ctx, `SELECT ir.contract_json,ir.content_digest,ir.contract_digest,rb.package_id,rb.package_version,rb.content_digest,rb.entry_point_id,rb.contract_digest,rb.runtime_id,rb.runtime_version,rb.runtime_digest FROM invocation_aliases ia JOIN invocation_registry ir ON ir.entry_point_id=ia.entry_point_id AND ir.package_version=ia.package_version AND ir.content_digest=ia.content_digest JOIN invocation_runtime_bindings rb ON rb.entry_point_id=ir.entry_point_id AND rb.package_version=ir.package_version AND rb.content_digest=ir.content_digest WHERE ia.alias=? AND ir.active=1`, alias).Scan(&body, &item.ContentDigest, &item.ContractDigest, &item.RuntimeBinding.PackageID, &item.RuntimeBinding.PackageVersion, &item.RuntimeBinding.PackageDigest, &item.RuntimeBinding.EntryPointID, &item.RuntimeBinding.ContractDigest, &item.RuntimeBinding.RuntimeID, &item.RuntimeBinding.RuntimeVersion, &item.RuntimeBinding.RuntimeDigest)
	if err != nil {
		return RegisteredInvocation{}, err
	}
	if err := json.Unmarshal(body, &item.Contract); err != nil {
		return RegisteredInvocation{}, err
	}
	if digestPackageBytes(body) != item.ContractDigest {
		return RegisteredInvocation{}, errors.New("persisted invocation contract digest mismatch")
	}
	if err := item.Contract.Validate(); err != nil {
		return RegisteredInvocation{}, err
	}
	if item.RuntimeBinding.PackageID != item.Contract.PackageID || item.RuntimeBinding.PackageVersion != item.Contract.PackageVersion || item.RuntimeBinding.PackageDigest != item.ContentDigest || item.RuntimeBinding.EntryPointID != item.Contract.EntryPointID || item.RuntimeBinding.ContractDigest != item.ContractDigest || item.RuntimeBinding.RuntimeDigest != item.ContractDigest {
		return RegisteredInvocation{}, errors.New("invocation runtime binding does not match active package generation")
	}
	if err := item.RuntimeBinding.Validate(); err != nil {
		return RegisteredInvocation{}, err
	}
	return item, nil
}

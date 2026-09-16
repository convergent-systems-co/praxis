package state

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"time"

	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type packageQuery interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func (s *Store) PreparePackageRollback(ctx context.Context, packageID, approvalID string, actor contracts.PrincipalRef) (packagecatalog.RollbackRequest, error) {
	if s == nil || s.db == nil {
		return packagecatalog.RollbackRequest{}, errors.New("state store is required")
	}
	current, targets, err := deriveRollbackClosures(ctx, s.db, packageID)
	if err != nil {
		return packagecatalog.RollbackRequest{}, err
	}
	return packagecatalog.NewRollbackRequest(packageID, current, targets, actor, approvalID)
}

func (s *Store) RollbackPackage(ctx context.Context, request packagecatalog.RollbackRequest, now time.Time) error {
	if s == nil || s.db == nil || now.IsZero() {
		return errors.New("state store and rollback time are required")
	}
	if err := request.Validate(); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	current, targets, err := deriveRollbackClosures(ctx, tx, request.RootPackageID)
	if err != nil {
		return err
	}
	expected, err := packagecatalog.NewRollbackRequest(request.RootPackageID, current, targets, request.Intent.Actor, request.ApprovalID)
	if err != nil {
		return err
	}
	wantDigest, err := expected.Intent.Digest()
	if err != nil {
		return err
	}
	gotDigest, err := request.Intent.Digest()
	if err != nil || gotDigest != wantDigest || !reflect.DeepEqual(request.Current, expected.Current) || !reflect.DeepEqual(request.Targets, expected.Targets) {
		return errors.New("package rollback precondition or target closure changed after authorization")
	}
	if err := consumePackageApproval(ctx, tx, request.ApprovalID, request.Intent.Actor, gotDigest, "", now); err != nil {
		return err
	}
	affected := map[string]bool{}
	targetIdentities := map[string]packagecatalog.PackageIdentity{}
	for _, target := range targets {
		affected[target.PackageID] = true
		targetIdentities[target.PackageID] = target
	}
	affectedIdentities := map[string]packagecatalog.PackageIdentity{}
	for _, identity := range current {
		affectedIdentities[identity.PackageID] = identity
	}
	if err := validateActiveDependencyCompatibilityTx(ctx, tx, targetIdentities, affectedIdentities); err != nil {
		return err
	}
	for _, target := range targets {
		if err := validateRollbackTargetTx(ctx, tx, target, affected); err != nil {
			return err
		}
	}
	for _, currentIdentity := range current {
		if _, err := tx.ExecContext(ctx, `DELETE FROM invocation_aliases WHERE (entry_point_id,package_version,content_digest) IN (SELECT entry_point_id,package_version,content_digest FROM invocation_registry WHERE package_id=? AND active=1)`, currentIdentity.PackageID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE invocation_registry SET active=0 WHERE package_id=?`, currentIdentity.PackageID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE package_contents SET active=0 WHERE package_id=?`, currentIdentity.PackageID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE installed_packages SET state='rolled_back' WHERE package_id=? AND package_version=? AND content_digest=? AND state='active'`, currentIdentity.PackageID, currentIdentity.Version, currentIdentity.ContentDigest); err != nil {
			return err
		}
	}
	for _, target := range targets {
		if err := reactivatePackageGenerationTx(ctx, tx, target); err != nil {
			return err
		}
	}
	requestJSON, err := json.Marshal(request)
	if err != nil {
		return err
	}
	sum := sha256.Sum256([]byte(gotDigest + "\x00" + request.ApprovalID))
	rollbackID := "package-rollback:sha256:" + hex.EncodeToString(sum[:])
	if _, err := tx.ExecContext(ctx, `INSERT INTO package_rollback_receipts(rollback_id,root_package_id,request_json,intent_digest,approval_id,authority_id,authority_kind,rolled_back_at) VALUES(?,?,?,?,?,?,?,?)`, rollbackID, request.RootPackageID, requestJSON, gotDigest, request.ApprovalID, request.Intent.Actor.ID, request.Intent.Actor.Kind, now.UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	return tx.Commit()
}

func deriveRollbackClosures(ctx context.Context, q packageQuery, rootPackageID string) ([]packagecatalog.PackageIdentity, []packagecatalog.PackageIdentity, error) {
	if rootPackageID == "" {
		return nil, nil, errors.New("rollback root package id is required")
	}
	var activeRoot packagecatalog.PackageIdentity
	var activeRootManifest []byte
	activeRoot.PackageID = rootPackageID
	if err := q.QueryRowContext(ctx, `SELECT package_version,content_digest,manifest_json FROM installed_packages WHERE package_id=? AND state='active'`, rootPackageID).Scan(&activeRoot.Version, &activeRoot.ContentDigest, &activeRootManifest); err != nil {
		return nil, nil, fmt.Errorf("load active rollback root: %w", err)
	}
	var rootTarget packagecatalog.PackageIdentity
	rootTarget.PackageID = rootPackageID
	var rootManifestJSON []byte
	if err := q.QueryRowContext(ctx, `SELECT package_version,content_digest,manifest_json FROM installed_packages WHERE package_id=? AND state IN ('installed','rolled_back') ORDER BY activated_at DESC LIMIT 1`, rootPackageID).Scan(&rootTarget.Version, &rootTarget.ContentDigest, &rootManifestJSON); err != nil {
		return nil, nil, fmt.Errorf("load retained rollback root: %w", err)
	}
	targets := []packagecatalog.PackageIdentity{}
	visited, visiting := map[string]bool{}, map[string]bool{}
	var visit func(packagecatalog.PackageIdentity, []byte) error
	visit = func(identity packagecatalog.PackageIdentity, manifestJSON []byte) error {
		if visiting[identity.PackageID] {
			return fmt.Errorf("retained rollback dependency cycle at %q", identity.PackageID)
		}
		if visited[identity.PackageID] {
			return nil
		}
		visiting[identity.PackageID] = true
		var manifest packagecatalog.Manifest
		if err := json.Unmarshal(manifestJSON, &manifest); err != nil {
			return fmt.Errorf("decode retained package %q: %w", identity.PackageID, err)
		}
		if manifest.PackageID != identity.PackageID || manifest.Version != identity.Version || manifest.ContentDigest != identity.ContentDigest {
			return fmt.Errorf("retained package %q identity mismatch", identity.PackageID)
		}
		locks := append([]packagecatalog.Dependency(nil), manifest.Dependencies...)
		sort.Slice(locks, func(i, j int) bool { return locks[i].PackageID < locks[j].PackageID })
		for _, lock := range locks {
			dependency := packagecatalog.PackageIdentity{PackageID: lock.PackageID, Version: lock.Version, ContentDigest: lock.Digest}
			var dependencyManifest []byte
			if err := q.QueryRowContext(ctx, `SELECT manifest_json FROM installed_packages WHERE package_id=? AND package_version=? AND content_digest=? AND state<>'removed'`, dependency.PackageID, dependency.Version, dependency.ContentDigest).Scan(&dependencyManifest); err != nil {
				return fmt.Errorf("load retained rollback dependency %q: %w", lock.PackageID, err)
			}
			if err := visit(dependency, dependencyManifest); err != nil {
				return err
			}
		}
		visiting[identity.PackageID] = false
		visited[identity.PackageID] = true
		targets = append(targets, identity)
		return nil
	}
	if err := visit(rootTarget, rootManifestJSON); err != nil {
		return nil, nil, err
	}
	currentByID := map[string]packagecatalog.PackageIdentity{}
	var visitCurrent func(packagecatalog.PackageIdentity, []byte) error
	visitCurrent = func(identity packagecatalog.PackageIdentity, manifestJSON []byte) error {
		if _, exists := currentByID[identity.PackageID]; exists {
			return nil
		}
		var manifest packagecatalog.Manifest
		if err := json.Unmarshal(manifestJSON, &manifest); err != nil || manifest.PackageID != identity.PackageID || manifest.Version != identity.Version || manifest.ContentDigest != identity.ContentDigest {
			return fmt.Errorf("active rollback package %q identity mismatch", identity.PackageID)
		}
		currentByID[identity.PackageID] = identity
		for _, lock := range manifest.Dependencies {
			dependency := packagecatalog.PackageIdentity{PackageID: lock.PackageID}
			var dependencyManifest []byte
			if err := q.QueryRowContext(ctx, `SELECT package_version,content_digest,manifest_json FROM installed_packages WHERE package_id=? AND state='active'`, lock.PackageID).Scan(&dependency.Version, &dependency.ContentDigest, &dependencyManifest); err != nil {
				return fmt.Errorf("load active rollback dependency %q: %w", lock.PackageID, err)
			}
			if dependency.Version != lock.Version || dependency.ContentDigest != lock.Digest {
				return fmt.Errorf("active rollback dependency %q does not satisfy current lock", lock.PackageID)
			}
			if err := visitCurrent(dependency, dependencyManifest); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visitCurrent(activeRoot, activeRootManifest); err != nil {
		return nil, nil, err
	}
	for _, target := range targets {
		if _, exists := currentByID[target.PackageID]; exists {
			continue
		}
		identity := packagecatalog.PackageIdentity{PackageID: target.PackageID}
		if err := q.QueryRowContext(ctx, `SELECT package_version,content_digest FROM installed_packages WHERE package_id=? AND state='active'`, target.PackageID).Scan(&identity.Version, &identity.ContentDigest); err != nil {
			return nil, nil, fmt.Errorf("load current retained rollback dependency %q: %w", target.PackageID, err)
		}
		currentByID[identity.PackageID] = identity
	}
	current := make([]packagecatalog.PackageIdentity, 0, len(currentByID))
	for _, identity := range currentByID {
		current = append(current, identity)
	}
	sort.Slice(current, func(i, j int) bool { return current[i].PackageID < current[j].PackageID })
	return current, targets, nil
}

func validateRollbackTargetTx(ctx context.Context, tx *sql.Tx, target packagecatalog.PackageIdentity, affected map[string]bool) error {
	var manifestJSON, verificationJSON, manifestBytes, artifactBytes, signatureJSON []byte
	if err := tx.QueryRowContext(ctx, `SELECT ip.manifest_json,par.verification_json,par.manifest_bytes,par.artifact_bytes,par.signature_json FROM installed_packages ip JOIN package_activation_receipts par ON par.package_id=ip.package_id AND par.package_version=ip.package_version AND par.content_digest=ip.content_digest WHERE ip.package_id=? AND ip.package_version=? AND ip.content_digest=? AND ip.state<>'removed' ORDER BY par.activated_at DESC LIMIT 1`, target.PackageID, target.Version, target.ContentDigest).Scan(&manifestJSON, &verificationJSON, &manifestBytes, &artifactBytes, &signatureJSON); err != nil {
		return fmt.Errorf("load rollback target evidence %q: %w", target.PackageID, err)
	}
	var manifest packagecatalog.Manifest
	if err := json.Unmarshal(manifestJSON, &manifest); err != nil || manifest.PackageID != target.PackageID || manifest.Version != target.Version || manifest.ContentDigest != target.ContentDigest {
		return fmt.Errorf("rollback target %q manifest identity mismatch", target.PackageID)
	}
	var evidence packagecatalog.VerificationEvidence
	if err := json.Unmarshal(verificationJSON, &evidence); err != nil || packagecatalog.ValidateVerificationEvidence(evidence) != nil {
		return fmt.Errorf("rollback target %q verification evidence is invalid", target.PackageID)
	}
	if digestPackageBytes(manifestJSON) != evidence.ManifestDigest || digestPackageBytes(manifestBytes) != evidence.ManifestDigest || digestPackageBytes(artifactBytes) != evidence.ArtifactDigest || digestPackageBytes(signatureJSON) != evidence.SignatureEnvelopeDigest {
		return fmt.Errorf("rollback target %q retained bytes do not bind verification evidence", target.PackageID)
	}
	var signature packagecatalog.SignatureEnvelope
	if err := json.Unmarshal(signatureJSON, &signature); err != nil || signature.Validate() != nil {
		return fmt.Errorf("rollback target %q signature envelope is invalid", target.PackageID)
	}
	rows, err := tx.QueryContext(ctx, `SELECT alias,ir.package_id FROM invocation_aliases ia JOIN invocation_registry ir ON ir.entry_point_id=ia.entry_point_id AND ir.package_version=ia.package_version AND ir.content_digest=ia.content_digest WHERE ir.active=1`)
	if err != nil {
		return err
	}
	activeAliases := map[string]string{}
	for rows.Next() {
		var alias, packageID string
		if err := rows.Scan(&alias, &packageID); err != nil {
			return err
		}
		activeAliases[alias] = packageID
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	invocations, err := tx.QueryContext(ctx, `SELECT contract_json,contract_digest FROM invocation_registry WHERE package_id=? AND package_version=? AND content_digest=?`, target.PackageID, target.Version, target.ContentDigest)
	if err != nil {
		return err
	}
	for invocations.Next() {
		var body []byte
		var digest string
		if err := invocations.Scan(&body, &digest); err != nil {
			return err
		}
		var contract contracts.InvocationContract
		if err := json.Unmarshal(body, &contract); err != nil || contract.Validate() != nil || digestPackageBytes(body) != digest || contract.PackageID != target.PackageID || contract.PackageVersion != target.Version {
			return fmt.Errorf("rollback target %q invocation contract is invalid", target.PackageID)
		}
		for _, alias := range contract.Aliases {
			if owner := activeAliases[alias]; owner != "" && !affected[owner] {
				return fmt.Errorf("rollback alias %q conflicts with active package %q", alias, owner)
			}
		}
	}
	if err := invocations.Err(); err != nil {
		invocations.Close()
		return err
	}
	if err := invocations.Close(); err != nil {
		return err
	}
	contents, err := tx.QueryContext(ctx, `SELECT kind,content_id,content_version,artifact_digest,artifact_bytes FROM package_contents WHERE package_id=? AND package_version=? AND content_digest=?`, target.PackageID, target.Version, target.ContentDigest)
	if err != nil {
		return err
	}
	expectedContents := map[string]string{}
	for _, content := range manifest.Contents {
		expectedContents[string(content.Kind)+"\x00"+content.ID+"\x00"+content.Version] = content.Digest
	}
	actualContents := map[string]string{}
	type contentIdentity struct{ kind, id, version string }
	var contentIdentities []contentIdentity
	for contents.Next() {
		var kind, id, version, digest string
		var body []byte
		if err := contents.Scan(&kind, &id, &version, &digest, &body); err != nil {
			return err
		}
		if digestPackageBytes(body) != digest {
			contents.Close()
			return fmt.Errorf("rollback content %s/%s@%s digest mismatch", kind, id, version)
		}
		key := kind + "\x00" + id + "\x00" + version
		actualContents[key] = digest
		contentIdentities = append(contentIdentities, contentIdentity{kind: kind, id: id, version: version})
	}
	if err := contents.Err(); err != nil {
		contents.Close()
		return err
	}
	if err := contents.Close(); err != nil {
		return err
	}
	if !reflect.DeepEqual(actualContents, expectedContents) {
		return fmt.Errorf("rollback target %q content inventory differs from retained manifest", target.PackageID)
	}
	for _, content := range contentIdentities {
		var activeOwner string
		err := tx.QueryRowContext(ctx, `SELECT package_id FROM package_contents WHERE kind=? AND content_id=? AND content_version=? AND active=1 LIMIT 1`, content.kind, content.id, content.version).Scan(&activeOwner)
		if err == nil && !affected[activeOwner] {
			return fmt.Errorf("rollback content %s/%s@%s conflicts with active package %q", content.kind, content.id, content.version, activeOwner)
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	return nil
}

func reactivatePackageGenerationTx(ctx context.Context, tx *sql.Tx, target packagecatalog.PackageIdentity) error {
	if _, err := tx.ExecContext(ctx, `UPDATE installed_packages SET state='active' WHERE package_id=? AND package_version=? AND content_digest=?`, target.PackageID, target.Version, target.ContentDigest); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE package_contents SET active=1 WHERE package_id=? AND package_version=? AND content_digest=?`, target.PackageID, target.Version, target.ContentDigest); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE invocation_registry SET active=1 WHERE package_id=? AND package_version=? AND content_digest=?`, target.PackageID, target.Version, target.ContentDigest); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT contract_json,entry_point_id FROM invocation_registry WHERE package_id=? AND package_version=? AND content_digest=?`, target.PackageID, target.Version, target.ContentDigest)
	if err != nil {
		return err
	}
	type invocationAliases struct {
		entryPoint string
		aliases    []string
	}
	var registrations []invocationAliases
	for rows.Next() {
		var body []byte
		var entryPoint string
		if err := rows.Scan(&body, &entryPoint); err != nil {
			return err
		}
		var contract contracts.InvocationContract
		if err := json.Unmarshal(body, &contract); err != nil {
			rows.Close()
			return err
		}
		registrations = append(registrations, invocationAliases{entryPoint: entryPoint, aliases: append([]string(nil), contract.Aliases...)})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, registration := range registrations {
		for _, alias := range registration.aliases {
			if _, err := tx.ExecContext(ctx, `INSERT INTO invocation_aliases(alias,entry_point_id,package_version,content_digest) VALUES(?,?,?,?)`, alias, registration.entryPoint, target.Version, target.ContentDigest); err != nil {
				return err
			}
		}
	}
	return nil
}

package state

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type InstalledPackage struct {
	Manifest   packagecatalog.Manifest `json:"manifest"`
	State      string                  `json:"state"`
	SourceKind string                  `json:"source_kind"`
	SourceRef  string                  `json:"source_ref"`
}

type PackageActivationReceipt struct {
	ActivationID   string
	PackageID      string
	PackageVersion string
	ContentDigest  string
	Verification   packagecatalog.VerificationEvidence
	ManifestBytes  []byte
	ArtifactBytes  []byte
	Signature      packagecatalog.SignatureEnvelope
	Intent         contracts.ActionIntent
	IntentDigest   string
	ApprovalID     string
	Authority      contracts.PrincipalRef
	ActivatedAt    time.Time
}

type PackageTransitionReceipt struct {
	TransitionID   string
	Request        packagecatalog.TransitionRequest
	IntentDigest   string
	TransitionedAt time.Time
}

type PackageRollbackReceipt struct {
	RollbackID   string
	Request      packagecatalog.RollbackRequest
	IntentDigest string
	RolledBackAt time.Time
}

func (s *Store) PackageRollbackReceipts(ctx context.Context, packageID string) ([]PackageRollbackReceipt, error) {
	if s == nil || s.db == nil || packageID == "" {
		return nil, errors.New("state store and package id are required")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT rollback_id,request_json,intent_digest,authority_id,authority_kind,rolled_back_at FROM package_rollback_receipts WHERE root_package_id=? ORDER BY rolled_back_at,rollback_id`, packageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PackageRollbackReceipt
	for rows.Next() {
		var item PackageRollbackReceipt
		var requestJSON []byte
		var authority contracts.PrincipalRef
		var stamp string
		if err := rows.Scan(&item.RollbackID, &requestJSON, &item.IntentDigest, &authority.ID, &authority.Kind, &stamp); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(requestJSON, &item.Request); err != nil {
			return nil, fmt.Errorf("decode package rollback receipt: %w", err)
		}
		if err := item.Request.Validate(); err != nil {
			return nil, fmt.Errorf("invalid package rollback receipt: %w", err)
		}
		digest, err := item.Request.Intent.Digest()
		if err != nil || digest != item.IntentDigest || item.Request.Intent.Actor != authority {
			return nil, errors.New("package rollback receipt authority or intent digest mismatch")
		}
		item.RolledBackAt, err = time.Parse(time.RFC3339Nano, stamp)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) PackageTransitionReceipts(ctx context.Context, packageID string) ([]PackageTransitionReceipt, error) {
	if s == nil || s.db == nil || packageID == "" {
		return nil, errors.New("state store and package id are required")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT transition_id,package_version,content_digest,operation,intent_json,intent_digest,approval_id,authority_id,authority_kind,transitioned_at FROM package_transition_receipts WHERE package_id=? ORDER BY transitioned_at,transition_id`, packageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PackageTransitionReceipt
	for rows.Next() {
		var item PackageTransitionReceipt
		var operation packagecatalog.TransitionOperation
		var intentJSON []byte
		var authority contracts.PrincipalRef
		var stamp string
		item.Request.Identity.PackageID = packageID
		if err := rows.Scan(&item.TransitionID, &item.Request.Identity.Version, &item.Request.Identity.ContentDigest, &operation, &intentJSON, &item.IntentDigest, &item.Request.ApprovalID, &authority.ID, &authority.Kind, &stamp); err != nil {
			return nil, err
		}
		item.Request.Operation = operation
		if err := json.Unmarshal(intentJSON, &item.Request.Intent); err != nil {
			return nil, fmt.Errorf("decode package transition intent: %w", err)
		}
		if item.Request.Intent.Actor != authority {
			return nil, errors.New("package transition authority does not match intent actor")
		}
		if err := item.Request.Validate(); err != nil {
			return nil, fmt.Errorf("invalid package transition receipt: %w", err)
		}
		digest, err := item.Request.Intent.Digest()
		if err != nil || digest != item.IntentDigest {
			return nil, errors.New("package transition intent digest mismatch")
		}
		item.TransitionedAt, err = time.Parse(time.RFC3339Nano, stamp)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) PackageActivationReceipts(ctx context.Context, packageID string) ([]PackageActivationReceipt, error) {
	if s == nil || s.db == nil || packageID == "" {
		return nil, errors.New("state store and package id are required")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT activation_id,package_id,package_version,content_digest,verification_json,manifest_bytes,artifact_bytes,signature_json,activation_intent_json,activation_intent_digest,approval_id,authority_id,authority_kind,activated_at FROM package_activation_receipts WHERE package_id=? ORDER BY activated_at,activation_id`, packageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PackageActivationReceipt
	for rows.Next() {
		var item PackageActivationReceipt
		var verificationJSON, signatureJSON, intentJSON []byte
		var stamp string
		if err := rows.Scan(&item.ActivationID, &item.PackageID, &item.PackageVersion, &item.ContentDigest, &verificationJSON, &item.ManifestBytes, &item.ArtifactBytes, &signatureJSON, &intentJSON, &item.IntentDigest, &item.ApprovalID, &item.Authority.ID, &item.Authority.Kind, &stamp); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(verificationJSON, &item.Verification); err != nil {
			return nil, fmt.Errorf("decode package verification receipt: %w", err)
		}
		if err := packagecatalog.ValidateVerificationEvidence(item.Verification); err != nil {
			return nil, fmt.Errorf("invalid package verification receipt: %w", err)
		}
		if digestPackageBytes(item.ManifestBytes) != item.Verification.ManifestDigest || digestPackageBytes(item.ArtifactBytes) != item.Verification.ArtifactDigest {
			return nil, errors.New("package activation receipt bytes do not match verification evidence")
		}
		if err := json.Unmarshal(signatureJSON, &item.Signature); err != nil {
			return nil, fmt.Errorf("decode package signature receipt: %w", err)
		}
		if err := item.Signature.Validate(); err != nil {
			return nil, fmt.Errorf("invalid package signature receipt: %w", err)
		}
		if digestPackageBytes(signatureJSON) != item.Verification.SignatureEnvelopeDigest {
			return nil, errors.New("package activation signature bytes do not match verification evidence")
		}
		if err := json.Unmarshal(intentJSON, &item.Intent); err != nil {
			return nil, fmt.Errorf("decode package activation intent: %w", err)
		}
		if err := item.Intent.Validate(); err != nil {
			return nil, fmt.Errorf("invalid package activation intent: %w", err)
		}
		actualDigest, err := item.Intent.Digest()
		if err != nil || actualDigest != item.IntentDigest {
			return nil, errors.New("package activation intent digest mismatch")
		}
		if item.Intent.Actor != item.Authority {
			return nil, errors.New("package activation authority does not match persisted intent actor")
		}
		item.ActivatedAt, err = time.Parse(time.RFC3339Nano, stamp)
		if err != nil {
			return nil, fmt.Errorf("parse package activation time: %w", err)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) InstalledPackages(ctx context.Context) ([]InstalledPackage, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("state store is required")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT manifest_json,state,source_kind,source_ref FROM installed_packages WHERE state<>'removed' ORDER BY package_id,package_version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []InstalledPackage
	for rows.Next() {
		var body []byte
		var p InstalledPackage
		if err := rows.Scan(&body, &p.State, &p.SourceKind, &p.SourceRef); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(body, &p.Manifest); err != nil {
			return nil, fmt.Errorf("decode installed package: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) ActivePackage(ctx context.Context, packageID string) (InstalledPackage, error) {
	if s == nil || s.db == nil || packageID == "" {
		return InstalledPackage{}, errors.New("state store and package id are required")
	}
	var body []byte
	var p InstalledPackage
	if err := s.db.QueryRowContext(ctx, `SELECT manifest_json,state,source_kind,source_ref FROM installed_packages WHERE package_id=? AND state='active'`, packageID).Scan(&body, &p.State, &p.SourceKind, &p.SourceRef); err != nil {
		return InstalledPackage{}, err
	}
	if err := json.Unmarshal(body, &p.Manifest); err != nil {
		return InstalledPackage{}, fmt.Errorf("decode installed package: %w", err)
	}
	return p, nil
}

// SelectedPackage returns the generation currently governing package
// lifecycle operations. Disabled packages remain selectable for governed
// removal; removed and superseded installed generations do not.
func (s *Store) SelectedPackage(ctx context.Context, packageID string) (InstalledPackage, error) {
	if s == nil || s.db == nil || packageID == "" {
		return InstalledPackage{}, errors.New("state store and package id are required")
	}
	var body []byte
	var p InstalledPackage
	err := s.db.QueryRowContext(ctx, `SELECT manifest_json,state,source_kind,source_ref FROM installed_packages WHERE package_id=? AND state IN ('active','disabled') ORDER BY CASE state WHEN 'active' THEN 0 ELSE 1 END,activated_at DESC LIMIT 1`, packageID).Scan(&body, &p.State, &p.SourceKind, &p.SourceRef)
	if err != nil {
		return InstalledPackage{}, err
	}
	if err := json.Unmarshal(body, &p.Manifest); err != nil {
		return InstalledPackage{}, fmt.Errorf("decode selected installed package: %w", err)
	}
	return p, nil
}

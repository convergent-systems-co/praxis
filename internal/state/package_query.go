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
	Signature      packagecatalog.SignatureEnvelope
	Intent         contracts.ActionIntent
	IntentDigest   string
	ApprovalID     string
	Authority      contracts.PrincipalRef
	ActivatedAt    time.Time
}

func (s *Store) PackageActivationReceipts(ctx context.Context, packageID string) ([]PackageActivationReceipt, error) {
	if s == nil || s.db == nil || packageID == "" {
		return nil, errors.New("state store and package id are required")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT activation_id,package_id,package_version,content_digest,verification_json,manifest_bytes,signature_json,activation_intent_json,activation_intent_digest,approval_id,authority_id,authority_kind,activated_at FROM package_activation_receipts WHERE package_id=? ORDER BY activated_at,activation_id`, packageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PackageActivationReceipt
	for rows.Next() {
		var item PackageActivationReceipt
		var verificationJSON, signatureJSON, intentJSON []byte
		var stamp string
		if err := rows.Scan(&item.ActivationID, &item.PackageID, &item.PackageVersion, &item.ContentDigest, &verificationJSON, &item.ManifestBytes, &signatureJSON, &intentJSON, &item.IntentDigest, &item.ApprovalID, &item.Authority.ID, &item.Authority.Kind, &stamp); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(verificationJSON, &item.Verification); err != nil {
			return nil, fmt.Errorf("decode package verification receipt: %w", err)
		}
		if err := packagecatalog.ValidateVerificationEvidence(item.Verification); err != nil {
			return nil, fmt.Errorf("invalid package verification receipt: %w", err)
		}
		if err := json.Unmarshal(signatureJSON, &item.Signature); err != nil {
			return nil, fmt.Errorf("decode package signature receipt: %w", err)
		}
		if err := item.Signature.Validate(); err != nil {
			return nil, fmt.Errorf("invalid package signature receipt: %w", err)
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

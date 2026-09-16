package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// SaveVerificationEvidence freezes verifier output before governance. It is
// idempotent only for the same semantic evidence digest.
func (s *Store) SaveVerificationEvidence(ctx context.Context, record packagecatalog.VerificationEvidenceRecord) error {
	if s == nil || s.db == nil {
		return errors.New("state store is required")
	}
	if err := packagecatalog.ValidateVerificationEvidenceRecord(record); err != nil {
		return err
	}
	body, err := json.Marshal(record)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO package_verification_evidence(evidence_id,installation_digest,closure_digest,evidence_json,evidence_digest,verified_at) VALUES(?,?,?,?,?,?) ON CONFLICT(evidence_id) DO NOTHING`, record.ID, record.InstallationDigest, record.ClosureDigest, body, record.ID, record.VerifiedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("persist verification evidence: %w", err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		var existing []byte
		if err := s.db.QueryRowContext(ctx, `SELECT evidence_json FROM package_verification_evidence WHERE evidence_id=?`, record.ID).Scan(&existing); err != nil {
			return err
		}
		if string(existing) != string(body) {
			return errors.New("conflicting verification evidence identity")
		}
	}
	return nil
}

func (s *Store) LoadVerificationEvidence(ctx context.Context, id string) (packagecatalog.VerificationEvidenceRecord, error) {
	if s == nil || s.db == nil {
		return packagecatalog.VerificationEvidenceRecord{}, errors.New("state store is required")
	}
	var body []byte
	if err := s.db.QueryRowContext(ctx, `SELECT evidence_json FROM package_verification_evidence WHERE evidence_id=?`, id).Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return packagecatalog.VerificationEvidenceRecord{}, ErrApprovalUnavailable
		}
		return packagecatalog.VerificationEvidenceRecord{}, err
	}
	var record packagecatalog.VerificationEvidenceRecord
	if err := json.Unmarshal(body, &record); err != nil {
		return packagecatalog.VerificationEvidenceRecord{}, err
	}
	if record.ID != id {
		return packagecatalog.VerificationEvidenceRecord{}, errors.New("verification evidence storage identity mismatch")
	}
	if err := packagecatalog.ValidateVerificationEvidenceRecord(record); err != nil {
		return packagecatalog.VerificationEvidenceRecord{}, err
	}
	return record, nil
}

func (s *Store) PackageApprovalRequest(ctx context.Context, approvalID string) (string, string, error) {
	if s == nil || s.db == nil {
		return "", "", errors.New("state store is required")
	}
	var requestID, requestVersion string
	if err := s.db.QueryRowContext(ctx, `SELECT authority_request_id,authority_request_version FROM approvals WHERE approval_id=? AND approver_id=? AND approver_kind=?`, approvalID, contracts.PackageManagerPrincipal().ID, contracts.PackageManagerPrincipal().Kind).Scan(&requestID, &requestVersion); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", "", ErrApprovalUnavailable
		}
		return "", "", err
	}
	if requestID == "" || requestVersion == "" {
		return "", "", ErrApprovalUnavailable
	}
	return requestID, requestVersion, nil
}

package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// PersistPublisherSigningReceipt records public signing provenance and the
// envelope, never private key material. Repeating the same receipt is
// idempotent only when every bound identity is identical.
func (s *Store) PersistPublisherSigningReceipt(ctx context.Context, provenanceDigest, generationDigest, packageID, packageVersion string, envelope, provenance any, signedAt time.Time) error {
	if s == nil || provenanceDigest == "" || generationDigest == "" || packageID == "" || packageVersion == "" || signedAt.IsZero() {
		return errors.New("publisher signing receipt fields are required")
	}
	envelopeJSON, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	provenanceJSON, err := json.Marshal(provenance)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var existing []byte
	err = tx.QueryRowContext(ctx, `SELECT provenance_json FROM publisher_signing_receipts WHERE provenance_digest=?`, provenanceDigest).Scan(&existing)
	if err == nil {
		if string(existing) != string(provenanceJSON) {
			return errors.New("publisher signing provenance digest collision")
		}
		return tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var stateValue string
	if err := tx.QueryRowContext(ctx, `SELECT state FROM publisher_generations WHERE publisher_generation_digest=?`, generationDigest).Scan(&stateValue); err != nil {
		return err
	}
	if stateValue != "active" {
		return errors.New("publisher generation is not active")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO publisher_signing_receipts(provenance_digest,publisher_generation_digest,package_id,package_version,envelope_json,provenance_json,signed_at) VALUES(?,?,?,?,?,?,?)`, provenanceDigest, generationDigest, packageID, packageVersion, envelopeJSON, provenanceJSON, signedAt.UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) PublisherSigningReceipt(ctx context.Context, digest string) (map[string]any, error) {
	var generation, packageID, version, envelope, provenance, signedAt string
	if err := s.db.QueryRowContext(ctx, `SELECT publisher_generation_digest,package_id,package_version,envelope_json,provenance_json,signed_at FROM publisher_signing_receipts WHERE provenance_digest=?`, digest).Scan(&generation, &packageID, &version, &envelope, &provenance, &signedAt); err != nil {
		return nil, err
	}
	var envelopeValue, provenanceValue any
	if err := json.Unmarshal([]byte(envelope), &envelopeValue); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(provenance), &provenanceValue); err != nil {
		return nil, err
	}
	return map[string]any{"provenance_digest": digest, "publisher_generation_digest": generation, "package_id": packageID, "package_version": version, "envelope": envelopeValue, "provenance": provenanceValue, "signed_at": signedAt}, nil
}

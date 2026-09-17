package state

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

var (
	ErrSecureBlobNotFound = errors.New("secure blob not found")
	ErrSecureBlobExpired  = errors.New("secure blob expired")
	ErrAuthorityRevoked   = errors.New("authority revoked")
)

type Sensitivity string

const (
	SensitivityInternal     Sensitivity = "internal"
	SensitivityConfidential Sensitivity = "confidential"
	SensitivityRestricted   Sensitivity = "restricted"
)

func (s Sensitivity) Validate() error {
	switch s {
	case SensitivityInternal, SensitivityConfidential, SensitivityRestricted:
		return nil
	default:
		return fmt.Errorf("invalid sensitivity %q", s)
	}
}

type SecureBlobRecord struct {
	Namespace     string
	ObjectID      string
	ObjectVersion string
	ObjectDigest  string
	Sensitivity   Sensitivity
	CryptoProfile contracts.CryptoProfile
	Envelope      praxiscrypto.Envelope
	CreatedAt     time.Time
	ExpiresAt     *time.Time
}

func SecureBlobAAD(namespace, objectID, version, digest string) []byte {
	b, _ := json.Marshal(struct {
		Namespace string `json:"namespace"`
		ObjectID  string `json:"object_id"`
		Version   string `json:"version"`
		Digest    string `json:"digest"`
	}{namespace, objectID, version, digest})
	return b
}

func (r SecureBlobRecord) Validate() error {
	if r.Namespace == "" || r.ObjectID == "" || r.ObjectVersion == "" || r.ObjectDigest == "" {
		return errors.New("secure blob namespace, object id, version, and digest are required")
	}
	if err := r.Sensitivity.Validate(); err != nil {
		return err
	}
	if err := r.CryptoProfile.Validate(); err != nil {
		return err
	}
	if r.Envelope.Version == "" || len(r.Envelope.Ciphertext) == 0 {
		return errors.New("secure blob requires encrypted envelope content")
	}
	if r.Envelope.RequestedProfile != r.CryptoProfile {
		return errors.New("secure blob crypto profile differs from envelope request")
	}
	expectedAAD := SecureBlobAAD(r.Namespace, r.ObjectID, r.ObjectVersion, r.ObjectDigest)
	if !bytes.Equal(r.Envelope.AAD, expectedAAD) {
		return errors.New("secure blob envelope is not bound to record identity/digest")
	}
	if r.CreatedAt.IsZero() {
		return errors.New("secure blob creation time is required")
	}
	if r.ExpiresAt != nil && !r.ExpiresAt.After(r.CreatedAt) {
		return errors.New("secure blob expiry must be after creation")
	}
	return nil
}

// PutSecureBlob persists ciphertext-only immutable versioned state. There is no
// plaintext storage path in this table/API.
func (s *Store) PutSecureBlob(ctx context.Context, record SecureBlobRecord) error {
	if s == nil || s.db == nil {
		return errors.New("state store is required")
	}
	if err := record.Validate(); err != nil {
		return err
	}
	envelopeJSON, err := json.Marshal(record.Envelope)
	if err != nil {
		return fmt.Errorf("encode secure blob envelope: %w", err)
	}
	var expires any
	if record.ExpiresAt != nil {
		expires = record.ExpiresAt.UTC().Format(time.RFC3339Nano)
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO secure_blobs(namespace,object_id,object_version,object_digest,sensitivity,crypto_profile,envelope_json,created_at,expires_at) VALUES(?,?,?,?,?,?,?,?,?)`, record.Namespace, record.ObjectID, record.ObjectVersion, record.ObjectDigest, string(record.Sensitivity), string(record.CryptoProfile), envelopeJSON, record.CreatedAt.UTC().Format(time.RFC3339Nano), expires)
	if err != nil {
		return fmt.Errorf("insert secure blob: %w", err)
	}
	return nil
}

// PutSecureBlobWithLock serializes an immutable write with other authority
// transitions that lock the same existing source record. The lock is held in
// the database transaction, not process memory, and therefore survives
// restart and coordinates multiple processes using the same SQLite store.
func (s *Store) PutSecureBlobWithLock(ctx context.Context, record SecureBlobRecord, lockNamespace, lockID, lockVersion string) error {
	if s == nil || s.db == nil {
		return errors.New("state store is required")
	}
	if err := record.Validate(); err != nil {
		return err
	}
	if lockNamespace == "" || lockID == "" || lockVersion == "" {
		return errors.New("secure blob lock identity is required")
	}
	envelopeJSON, err := json.Marshal(record.Envelope)
	if err != nil {
		return fmt.Errorf("encode secure blob envelope: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin secure blob transition: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE secure_blobs SET object_digest=object_digest WHERE namespace=? AND object_id=? AND object_version=?`, lockNamespace, lockID, lockVersion); err != nil {
		return fmt.Errorf("lock secure blob source: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO secure_blobs(namespace,object_id,object_version,object_digest,sensitivity,crypto_profile,envelope_json,created_at,expires_at) VALUES(?,?,?,?,?,?,?,?,?)`, record.Namespace, record.ObjectID, record.ObjectVersion, record.ObjectDigest, string(record.Sensitivity), string(record.CryptoProfile), envelopeJSON, record.CreatedAt.UTC().Format(time.RFC3339Nano), nullableTime(record.ExpiresAt)); err != nil {
		return fmt.Errorf("insert locked secure blob: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit secure blob transition: %w", err)
	}
	return nil
}

func (s *Store) PutSecureBlobsWithLock(ctx context.Context, records []SecureBlobRecord, lockNamespace, lockID, lockVersion string) error {
	if s == nil || s.db == nil || len(records) == 0 {
		return errors.New("state store and secure blob records are required")
	}
	for _, record := range records {
		if err := record.Validate(); err != nil {
			return err
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin secure blob transition: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE secure_blobs SET object_digest=object_digest WHERE namespace=? AND object_id=? AND object_version=?`, lockNamespace, lockID, lockVersion)
	if err != nil {
		return fmt.Errorf("lock secure blob source: %w", err)
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return errors.New("secure blob lock source is missing")
	}
	for _, record := range records {
		envelopeJSON, err := json.Marshal(record.Envelope)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO secure_blobs(namespace,object_id,object_version,object_digest,sensitivity,crypto_profile,envelope_json,created_at,expires_at) VALUES(?,?,?,?,?,?,?,?,?)`, record.Namespace, record.ObjectID, record.ObjectVersion, record.ObjectDigest, string(record.Sensitivity), string(record.CryptoProfile), envelopeJSON, record.CreatedAt.UTC().Format(time.RFC3339Nano), nullableTime(record.ExpiresAt))
		if err != nil {
			return fmt.Errorf("insert locked secure blob set: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit secure blob transition: %w", err)
	}
	return nil
}

func (s *Store) PutSecureBlobsUnlessRevoked(ctx context.Context, records []SecureBlobRecord, revocationNamespace, requestID, requestVersion, generationInvalidationNamespace, generationID, generationVersion, lockNamespace, lockID, lockVersion string) error {
	if s == nil || s.db == nil || len(records) == 0 {
		return errors.New("state store and secure blob records are required")
	}
	for _, record := range records {
		if err := record.Validate(); err != nil {
			return err
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE secure_blobs SET object_digest=object_digest WHERE namespace=? AND object_id=? AND object_version=?`, lockNamespace, lockID, lockVersion)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return errors.New("secure blob lock source is missing")
	}
	for _, check := range []struct{ namespace, id, version string }{{revocationNamespace, requestID, requestVersion}, {generationInvalidationNamespace, generationID, generationVersion}} {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM secure_blobs WHERE namespace=? AND object_id=? AND object_version=?`, check.namespace, check.id, check.version).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return ErrAuthorityRevoked
		}
	}
	for _, record := range records {
		envelopeJSON, err := json.Marshal(record.Envelope)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO secure_blobs(namespace,object_id,object_version,object_digest,sensitivity,crypto_profile,envelope_json,created_at,expires_at) VALUES(?,?,?,?,?,?,?,?,?)`, record.Namespace, record.ObjectID, record.ObjectVersion, record.ObjectDigest, string(record.Sensitivity), string(record.CryptoProfile), envelopeJSON, record.CreatedAt.UTC().Format(time.RFC3339Nano), nullableTime(record.ExpiresAt)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// PutSecureBlobUnlessRevoked atomically locks the source authority record,
// checks the immutable revocation namespace, and writes the new record. A
// revoke and this operation therefore have one durable SQLite ordering.
func (s *Store) PutSecureBlobUnlessRevoked(ctx context.Context, record SecureBlobRecord, revocationNamespace, requestID, requestVersion, additionalRevocationNamespace, additionalID, additionalVersion, lockNamespace, lockID, lockVersion string) error {
	if s == nil || s.db == nil {
		return errors.New("state store is required")
	}
	if err := record.Validate(); err != nil {
		return err
	}
	if revocationNamespace == "" || requestID == "" || requestVersion == "" {
		return errors.New("authority revocation lookup identity is required")
	}
	if lockNamespace == "" || lockID == "" || lockVersion == "" {
		return errors.New("secure blob lock identity is required")
	}
	envelopeJSON, err := json.Marshal(record.Envelope)
	if err != nil {
		return fmt.Errorf("encode secure blob envelope: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin authority transition: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE secure_blobs SET object_digest=object_digest WHERE namespace=? AND object_id=? AND object_version=?`, lockNamespace, lockID, lockVersion); err != nil {
		return fmt.Errorf("lock authority source: %w", err)
	}
	for _, check := range []struct{ namespace, id, version string }{{revocationNamespace, requestID, requestVersion}, {additionalRevocationNamespace, additionalID, additionalVersion}} {
		namespace, id, version := check.namespace, check.id, check.version
		if namespace == "" {
			continue
		}
		var revoked int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM secure_blobs WHERE namespace=? AND object_id=? AND object_version=?`, namespace, id, version).Scan(&revoked); err != nil {
			return fmt.Errorf("check authority revocation: %w", err)
		}
		if revoked != 0 {
			return ErrAuthorityRevoked
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO secure_blobs(namespace,object_id,object_version,object_digest,sensitivity,crypto_profile,envelope_json,created_at,expires_at) VALUES(?,?,?,?,?,?,?,?,?)`, record.Namespace, record.ObjectID, record.ObjectVersion, record.ObjectDigest, string(record.Sensitivity), string(record.CryptoProfile), envelopeJSON, record.CreatedAt.UTC().Format(time.RFC3339Nano), nullableTime(record.ExpiresAt)); err != nil {
		return fmt.Errorf("insert authority-bound secure blob: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit authority transition: %w", err)
	}
	return nil
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func (s *Store) GetSecureBlob(ctx context.Context, namespace, objectID, version string, now time.Time) (SecureBlobRecord, error) {
	if s == nil || s.db == nil {
		return SecureBlobRecord{}, errors.New("state store is required")
	}
	if namespace == "" || objectID == "" || version == "" {
		return SecureBlobRecord{}, errors.New("secure blob lookup identity is required")
	}
	var r SecureBlobRecord
	var sensitivity, profile, created string
	var envelopeJSON []byte
	var expires sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT object_digest,sensitivity,crypto_profile,envelope_json,created_at,expires_at FROM secure_blobs WHERE namespace=? AND object_id=? AND object_version=?`, namespace, objectID, version).Scan(&r.ObjectDigest, &sensitivity, &profile, &envelopeJSON, &created, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return SecureBlobRecord{}, ErrSecureBlobNotFound
	}
	if err != nil {
		return SecureBlobRecord{}, fmt.Errorf("load secure blob: %w", err)
	}
	r.Namespace, r.ObjectID, r.ObjectVersion = namespace, objectID, version
	r.Sensitivity = Sensitivity(sensitivity)
	r.CryptoProfile = contracts.CryptoProfile(profile)
	if err := json.Unmarshal(envelopeJSON, &r.Envelope); err != nil {
		return SecureBlobRecord{}, fmt.Errorf("decode secure blob envelope: %w", err)
	}
	r.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return SecureBlobRecord{}, fmt.Errorf("parse secure blob creation time: %w", err)
	}
	if expires.Valid {
		t, err := time.Parse(time.RFC3339Nano, expires.String)
		if err != nil {
			return SecureBlobRecord{}, fmt.Errorf("parse secure blob expiry: %w", err)
		}
		r.ExpiresAt = &t
		if now.IsZero() {
			now = time.Now().UTC()
		}
		if !now.Before(t) {
			return SecureBlobRecord{}, ErrSecureBlobExpired
		}
	}
	if err := r.Validate(); err != nil {
		return SecureBlobRecord{}, fmt.Errorf("stored secure blob failed validation: %w", err)
	}
	return r, nil
}

// HasSecureBlobDigest checks immutable lineage without opening encrypted
// content. Import uses it to require a predecessor generation.
func (s *Store) HasSecureBlobDigest(ctx context.Context, namespace, objectID, digest string) (bool, error) {
	if s == nil || s.db == nil {
		return false, errors.New("state store is required")
	}
	if namespace == "" || objectID == "" || digest == "" {
		return false, errors.New("secure blob lineage identity is required")
	}
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM secure_blobs WHERE namespace=? AND object_id=? AND object_digest=?`, namespace, objectID, digest).Scan(&count); err != nil {
		return false, fmt.Errorf("check secure blob lineage: %w", err)
	}
	return count == 1, nil
}

// ListSecureBlobs returns immutable records for a namespace in stable identity
// order. Callers still decrypt and validate each record through their owning
// contract; this method exposes no plaintext.
func (s *Store) ListSecureBlobs(ctx context.Context, namespace string, now time.Time) ([]SecureBlobRecord, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("state store is required")
	}
	if namespace == "" {
		return nil, errors.New("secure blob namespace is required")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT object_id,object_version,object_digest,sensitivity,crypto_profile,envelope_json,created_at,expires_at FROM secure_blobs WHERE namespace=? ORDER BY object_id,object_version`, namespace)
	if err != nil {
		return nil, fmt.Errorf("list secure blobs: %w", err)
	}
	defer rows.Close()
	var records []SecureBlobRecord
	for rows.Next() {
		var r SecureBlobRecord
		var sensitivity, profile, created string
		var envelopeJSON []byte
		var expires sql.NullString
		if err := rows.Scan(&r.ObjectID, &r.ObjectVersion, &r.ObjectDigest, &sensitivity, &profile, &envelopeJSON, &created, &expires); err != nil {
			return nil, fmt.Errorf("scan secure blob: %w", err)
		}
		r.Namespace, r.Sensitivity, r.CryptoProfile = namespace, Sensitivity(sensitivity), contracts.CryptoProfile(profile)
		if err := json.Unmarshal(envelopeJSON, &r.Envelope); err != nil {
			return nil, fmt.Errorf("decode secure blob envelope: %w", err)
		}
		r.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
		if err != nil {
			return nil, fmt.Errorf("parse secure blob creation time: %w", err)
		}
		if expires.Valid {
			t, parseErr := time.Parse(time.RFC3339Nano, expires.String)
			if parseErr != nil {
				return nil, fmt.Errorf("parse secure blob expiry: %w", parseErr)
			}
			r.ExpiresAt = &t
		}
		if r.ExpiresAt != nil && (now.IsZero() || !now.Before(*r.ExpiresAt)) {
			continue
		}
		if err := r.Validate(); err != nil {
			return nil, fmt.Errorf("listed secure blob failed validation: %w", err)
		}
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

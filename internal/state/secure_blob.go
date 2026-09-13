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
	if err := r.Sensitivity.Validate(); err != nil { return err }
	if err := r.CryptoProfile.Validate(); err != nil { return err }
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
	if r.CreatedAt.IsZero() { return errors.New("secure blob creation time is required") }
	if r.ExpiresAt != nil && !r.ExpiresAt.After(r.CreatedAt) {
		return errors.New("secure blob expiry must be after creation")
	}
	return nil
}

// PutSecureBlob persists ciphertext-only immutable versioned state. There is no
// plaintext storage path in this table/API.
func (s *Store) PutSecureBlob(ctx context.Context, record SecureBlobRecord) error {
	if s == nil || s.db == nil { return errors.New("state store is required") }
	if err := record.Validate(); err != nil { return err }
	envelopeJSON, err := json.Marshal(record.Envelope)
	if err != nil { return fmt.Errorf("encode secure blob envelope: %w", err) }
	var expires any
	if record.ExpiresAt != nil { expires = record.ExpiresAt.UTC().Format(time.RFC3339Nano) }
	_, err = s.db.ExecContext(ctx, `INSERT INTO secure_blobs(namespace,object_id,object_version,object_digest,sensitivity,crypto_profile,envelope_json,created_at,expires_at) VALUES(?,?,?,?,?,?,?,?,?)`, record.Namespace, record.ObjectID, record.ObjectVersion, record.ObjectDigest, string(record.Sensitivity), string(record.CryptoProfile), envelopeJSON, record.CreatedAt.UTC().Format(time.RFC3339Nano), expires)
	if err != nil { return fmt.Errorf("insert secure blob: %w", err) }
	return nil
}

func (s *Store) GetSecureBlob(ctx context.Context, namespace, objectID, version string, now time.Time) (SecureBlobRecord, error) {
	if s == nil || s.db == nil { return SecureBlobRecord{}, errors.New("state store is required") }
	if namespace == "" || objectID == "" || version == "" { return SecureBlobRecord{}, errors.New("secure blob lookup identity is required") }
	var r SecureBlobRecord
	var sensitivity, profile, created string
	var envelopeJSON []byte
	var expires sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT object_digest,sensitivity,crypto_profile,envelope_json,created_at,expires_at FROM secure_blobs WHERE namespace=? AND object_id=? AND object_version=?`, namespace, objectID, version).Scan(&r.ObjectDigest, &sensitivity, &profile, &envelopeJSON, &created, &expires)
	if errors.Is(err, sql.ErrNoRows) { return SecureBlobRecord{}, ErrSecureBlobNotFound }
	if err != nil { return SecureBlobRecord{}, fmt.Errorf("load secure blob: %w", err) }
	r.Namespace, r.ObjectID, r.ObjectVersion = namespace, objectID, version
	r.Sensitivity = Sensitivity(sensitivity)
	r.CryptoProfile = contracts.CryptoProfile(profile)
	if err := json.Unmarshal(envelopeJSON, &r.Envelope); err != nil { return SecureBlobRecord{}, fmt.Errorf("decode secure blob envelope: %w", err) }
	r.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	if err != nil { return SecureBlobRecord{}, fmt.Errorf("parse secure blob creation time: %w", err) }
	if expires.Valid {
		t, err := time.Parse(time.RFC3339Nano, expires.String)
		if err != nil { return SecureBlobRecord{}, fmt.Errorf("parse secure blob expiry: %w", err) }
		r.ExpiresAt = &t
		if now.IsZero() { now = time.Now().UTC() }
		if !now.Before(t) { return SecureBlobRecord{}, ErrSecureBlobExpired }
	}
	if err := r.Validate(); err != nil { return SecureBlobRecord{}, fmt.Errorf("stored secure blob failed validation: %w", err) }
	return r, nil
}

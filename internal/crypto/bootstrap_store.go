package crypto

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var (
	ErrBootstrapRecordMissing = errors.New("bootstrap record is missing")
	ErrBootstrapRecordExists  = errors.New("bootstrap record already exists")
	ErrBootstrapRecordCorrupt = errors.New("bootstrap record is corrupt")
)

type persistedBootstrapRecord struct {
	Record       BootstrapRecord `json:"record"`
	RecordDigest string          `json:"record_digest"`
}

// SaveBootstrapRecord persists only non-secret binding metadata. It refuses to
// replace an existing file so an existing encrypted store cannot be silently
// rebound to newly generated key material.
func SaveBootstrapRecord(path string, record BootstrapRecord) error {
	if path == "" {
		return errors.New("bootstrap record path is required")
	}
	if err := record.Validate(); err != nil {
		return err
	}
	digest, err := record.Digest()
	if err != nil {
		return err
	}
	payload, err := json.MarshalIndent(persistedBootstrapRecord{Record: record, RecordDigest: digest}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode bootstrap record: %w", err)
	}
	if _, err := os.Stat(path); err == nil {
		return ErrBootstrapRecordExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect bootstrap record: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create bootstrap record directory: %w", err)
	}
	tmp, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return ErrBootstrapRecordExists
		}
		return fmt.Errorf("create bootstrap record temporary file: %w", err)
	}
	if _, err := tmp.Write(payload); err != nil {
		tmp.Close()
		return fmt.Errorf("write bootstrap record: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync bootstrap record: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close bootstrap record: %w", err)
	}
	return nil
}

func LoadBootstrapRecord(path string) (BootstrapRecord, error) {
	if path == "" {
		return BootstrapRecord{}, errors.New("bootstrap record path is required")
	}
	payload, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return BootstrapRecord{}, ErrBootstrapRecordMissing
	}
	if err != nil {
		return BootstrapRecord{}, fmt.Errorf("read bootstrap record: %w", err)
	}
	var persisted persistedBootstrapRecord
	if err := json.Unmarshal(payload, &persisted); err != nil {
		return BootstrapRecord{}, fmt.Errorf("%w: decode: %v", ErrBootstrapRecordCorrupt, err)
	}
	if err := persisted.Record.Validate(); err != nil {
		return BootstrapRecord{}, fmt.Errorf("%w: %v", ErrBootstrapRecordCorrupt, err)
	}
	digest, err := persisted.Record.Digest()
	if err != nil || persisted.RecordDigest != digest {
		return BootstrapRecord{}, ErrBootstrapRecordCorrupt
	}
	return persisted.Record, nil
}

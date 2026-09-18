package contracts

import (
	"encoding/hex"
	"errors"
	"strings"
)

// ValidateSHA256Digest accepts only canonical, content-addressed SHA-256
// values. Labels and human-readable placeholders are never authority.
func ValidateSHA256Digest(value string) error {
	if !strings.HasPrefix(value, "sha256:") {
		return errors.New("digest must use sha256: prefix")
	}
	encoded := strings.TrimPrefix(value, "sha256:")
	if len(encoded) != 64 {
		return errors.New("sha256 digest must contain exactly 64 hexadecimal characters")
	}
	if _, err := hex.DecodeString(encoded); err != nil {
		return errors.New("sha256 digest contains non-hexadecimal characters")
	}
	return nil
}

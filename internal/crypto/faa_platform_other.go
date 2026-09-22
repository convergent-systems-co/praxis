//go:build !darwin

package crypto

import (
	"errors"

	"github.com/convergent-systems-co/praxis/internal/faa"
)

// NewPlatformAnchor: no first-party Forward Authority Anchor backend exists for
// this platform, so governed operations that require one fail closed, exactly as
// governed bootstrap does (no first-party bootstrap backend).
func NewPlatformAnchor() (faa.Anchor, error) {
	return nil, errors.New("no first-party forward authority anchor backend is available for this platform")
}

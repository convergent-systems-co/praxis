//go:build darwin

package crypto

import "github.com/convergent-systems-co/praxis/internal/faa"

// NewPlatformAnchor returns the platform's Forward Authority Anchor backend.
func NewPlatformAnchor() (faa.Anchor, error) { return NewKeychainAnchor(), nil }

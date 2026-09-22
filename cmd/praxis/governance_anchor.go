package main

import (
	"fmt"

	"github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/internal/faa"
	"github.com/convergent-systems-co/praxis/internal/goalstore"
)

// governanceAnchorFactory yields the installation's Forward Authority Anchor.
// Production has exactly one implementation, the platform backend (the macOS
// Keychain on darwin). It is a variable only so tests can substitute a double;
// there is no environment variable, flag or file that selects another anchor.
var governanceAnchorFactory = func(getenv func(string) string) (faa.Anchor, error) {
	return crypto.NewPlatformAnchor()
}

// withGovernanceAnchor attaches the anchor to a governed repository. A
// governed repository without one is never returned: no backend, no governed
// operation (fail closed), exactly as governed bootstrap behaves without a
// first-party key backend.
func withGovernanceAnchor(repo goalstore.Repository, getenv func(string) string) (goalstore.Repository, error) {
	anchor, err := governanceAnchorFactory(getenv)
	if err != nil {
		return goalstore.Repository{}, fmt.Errorf("forward authority anchor: %w", err)
	}
	repo.FAA = anchor
	return repo, nil
}

package main

import (
	"github.com/convergent-systems-co/praxis/internal/faa"
	"github.com/convergent-systems-co/praxis/internal/faa/faatest"
)

// The production anchor is the platform Keychain. Tests use one process-wide
// in-memory double, keyed by installation digest, that survives the repeated
// "restarts" a single test performs. An installation's anchor is created, with
// its genesis fact, by the first generation saved through an anchored
// repository, exactly as enrollment does.
var (
	testGovernanceAnchor = faatest.NewMemory()
)

func init() {
	governanceAnchorFactory = func(func(string) string) (faa.Anchor, error) {
		return testGovernanceAnchor, nil
	}
}

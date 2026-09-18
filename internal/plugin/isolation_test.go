package plugin

import "testing"

func TestIsolationUnknownFailsClosed(t *testing.T) {
	profile := IsolationProfile{Properties: map[IsolationProperty]EnforcementState{IsolationFilesystem: Unknown}}
	if err := profile.Satisfies([]IsolationProperty{IsolationFilesystem}); err == nil {
		t.Fatal("unknown required isolation must fail closed")
	}
}

func TestIsolationRequiredPropertiesMustAllBeEnforced(t *testing.T) {
	profile := IsolationProfile{Properties: map[IsolationProperty]EnforcementState{IsolationFilesystem: Enforced, IsolationNetwork: Enforced}}
	if err := profile.Satisfies([]IsolationProperty{IsolationFilesystem, IsolationNetwork}); err != nil {
		t.Fatalf("enforced profile rejected: %v", err)
	}
}

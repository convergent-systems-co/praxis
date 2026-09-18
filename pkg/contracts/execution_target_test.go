package contracts

import (
	"errors"
	"testing"
)

func validExecutionTarget() ExecutionTarget {
	return ExecutionTarget{Version: "1", RequiredCapabilities: []string{"reasoning"}, PreferredProfiles: []string{"quality"}, AllowedFallbackProfiles: []string{"balanced"}, TransportPolicy: []TransportClass{TransportSubscriptionCLI, TransportLocal}, APIPolicy: APIPolicyForbid, SourceAuthority: "package:goals", Scope: "node:plan"}
}

func TestExecutionTargetValidatesProviderNeutralConstraints(t *testing.T) {
	if err := validExecutionTarget().Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestExecutionTargetRejectsUnknownAndConflictingConstraints(t *testing.T) {
	unknown := validExecutionTarget()
	unknown.Version = "2"
	if err := unknown.Validate(); !errors.Is(err, ErrInvalidExecutionTarget) {
		t.Fatalf("unknown version did not fail closed: %v", err)
	}
	conflict := validExecutionTarget()
	conflict.RequiredProfiles = []string{"codex-profile"}
	conflict.ProhibitedProfiles = []string{"codex-profile"}
	if err := conflict.Validate(); !errors.Is(err, ErrTargetConflict) {
		t.Fatalf("profile conflict did not fail closed: %v", err)
	}
	apiConflict := validExecutionTarget()
	apiConflict.APIPolicy = APIPolicyForbid
	apiConflict.TransportPolicy = []TransportClass{TransportMeteredAPI}
	if err := apiConflict.Validate(); !errors.Is(err, ErrTargetConflict) {
		t.Fatalf("API fallback conflict did not fail closed: %v", err)
	}
}

func TestExecutionTargetDoesNotTreatProviderNamesAsCoreIdentity(t *testing.T) {
	target := validExecutionTarget()
	if target.Scope == "codex" || target.SourceAuthority == "claude" {
		t.Fatal("provider names must not define target identity")
	}
	if err := target.Validate(); err != nil {
		t.Fatal(err)
	}
}

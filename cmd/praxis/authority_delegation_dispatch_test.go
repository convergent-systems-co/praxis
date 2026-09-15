package main

import (
	"strings"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func TestBuiltinDelegationPolicyDispatchesPackagePublishWithoutV1ControllerRule(t *testing.T) {
	request := contracts.DelegationRequest{
		Profile:            contracts.DelegationProfilePackagePublish,
		RequestedAuthority: contracts.GovernedPackagePublish,
		RequestedOperation: "sign",
		DelegatedPrincipal: contracts.PrincipalRef{ID: contracts.FirstPartyPublisherPrincipal, Kind: "publisher"},
		TargetKind:         "publisher-generation",
		SubjectKind:        "publisher",
		SubjectID:          contracts.FirstPartyPublisherPrincipal,
		TargetIdentity:     contracts.FirstPartyPublisherPrincipal,
		PolicyRef:          contracts.AuthorityModelID,
		PolicyVersion:      contracts.AuthorityModelSuccessorVersion,
		PolicyDigest:       contracts.AuthorityModelSuccessorDigest(),
		RequestedScope:     "package-namespace:praxis.package",
		TargetVersion:      "praxis.package",
		TargetDigest:       "sha256:" + strings.Repeat("a", 64),
		SubjectDigest:      "sha256:" + strings.Repeat("a", 64),
		SubjectKeyDigest:   "sha256:" + strings.Repeat("b", 64),
		TargetConstraints:  []string{"praxis.package"},
		ExpiresAt:          time.Now().UTC().Add(time.Hour),
	}
	err := (builtinDelegationPolicy{}).ContainDelegation(contracts.AuthorityGeneration{}, request, time.Now().UTC())
	if err == nil {
		t.Fatal("incomplete package-publish request must fail closed")
	}
	if strings.Contains(err.Error(), "v1 delegation") || strings.Contains(err.Error(), "controller principal") {
		t.Fatalf("package-publish request was routed through v1 validation: %v", err)
	}
}

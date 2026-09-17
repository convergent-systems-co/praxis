package contracts

import (
	"strings"
	"testing"
	"time"
)

func validBuiltinDelegation(t *testing.T) (AuthorityGeneration, DelegationRequest, time.Time) {
	t.Helper()
	now := time.Unix(1000, 0).UTC()
	parent := AuthorityGeneration{
		Ref: "installation-governance:sha256:" + strings.Repeat("a", 64), Version: "1", Digest: "sha256:" + strings.Repeat("b", 64),
		Principal: PrincipalRef{ID: "installation-owner:sha256:" + strings.Repeat("a", 64), Kind: "human"}, Scope: "installation-governance:sha256:" + strings.Repeat("a", 64),
		Capabilities: []string{AuthorityDelegateCapability}, AuthorityModel: AuthorityModelID, AuthorityModelVersion: AuthorityModelVersion, AuthorityModelDigest: AuthorityModelDigest(),
		ProvenanceRef: "bootstrap-record:sha256:" + strings.Repeat("a", 64) + ":os-user:test", ProvenanceDigest: "sha256:" + strings.Repeat("a", 64), State: AuthorityGenerationActive, EffectiveAt: now,
	}
	request := DelegationRequest{
		ParentRef: parent.Ref, ParentVersion: parent.Version, ParentDigest: parent.Digest,
		DelegatedPrincipal: PrincipalRef{ID: "controller:goals", Kind: "controller"}, TargetKind: "goals.workplan", TargetIdentity: "goal-1",
		TargetVersion: "1", TargetDigest: "sha256:" + strings.Repeat("c", 64), ProposalVersion: "1", ProposalDigest: "sha256:" + strings.Repeat("d", 64), ReviewVersion: "1", ReviewDigest: "sha256:" + strings.Repeat("e", 64),
		RequestedAuthority: GovernedWorkPlanAccept, RequestedOperation: "accept", RequestedScope: "goals-workplan/goal-1/1/sha256:" + strings.Repeat("c", 64) + "/1/sha256:" + strings.Repeat("d", 64) + "/1/sha256:" + strings.Repeat("e", 64),
		ExpiresAt: now.Add(time.Hour), Reason: "bounded qualification", PolicyRef: AuthorityModelID, PolicyVersion: AuthorityModelVersion, PolicyDigest: AuthorityModelDigest(),
	}
	return parent, request, now
}

func TestBuiltinDelegationAcceptsOnlyClosedV1Edge(t *testing.T) {
	parent, request, now := validBuiltinDelegation(t)
	if err := ValidateBuiltinDelegation(parent, request, now); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*DelegationRequest){
		"capability": func(r *DelegationRequest) { r.RequestedCapabilities = []string{"vcs.write"} },
		"operation":  func(r *DelegationRequest) { r.RequestedOperation = "execute" },
		"target":     func(r *DelegationRequest) { r.TargetIdentity = "goal-2" },
		"scope":      func(r *DelegationRequest) { r.RequestedScope = "goals-workplan/other" },
		"principal":  func(r *DelegationRequest) { r.DelegatedPrincipal = parent.Principal },
		"policy":     func(r *DelegationRequest) { r.PolicyDigest = "sha256:" + strings.Repeat("f", 64) },
		"expiry":     func(r *DelegationRequest) { r.ExpiresAt = now.Add(-time.Second) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			candidate := request
			mutate(&candidate)
			if err := ValidateBuiltinDelegation(parent, candidate, now); err == nil {
				t.Fatal("invalid delegation was accepted")
			}
		})
	}
}

func TestAuthorityModelV2IsDistinctAndDelegatesOnlyRoutingIssuance(t *testing.T) {
	parent, request, now := validBuiltinDelegation(t)
	if AuthorityModelV2Digest() == AuthorityModelDigest() {
		t.Fatal("v2 reused v1 digest")
	}
	parent.Version, parent.AuthorityModelVersion, parent.AuthorityModelDigest = "2", AuthorityModelV2Version, AuthorityModelV2Digest()
	request.ParentVersion = parent.Version
	request.PolicyVersion, request.PolicyDigest = AuthorityModelV2Version, AuthorityModelV2Digest()
	request.RequestedAuthority, request.RequestedOperation, request.TargetKind = AuthorityRoutingTargetContributionIssue, "issue", "routing.target-contribution"
	if err := ValidateBuiltinDelegation(parent, request, now); err != nil {
		t.Fatal(err)
	}
	request.RequestedAuthority = AuthorityDelegateCapability
	if err := ValidateBuiltinDelegation(parent, request, now); err == nil {
		t.Fatal("v2 routing child received authority.delegate")
	}
}

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

func TestBuiltinPackagePublishDelegationBindsExactPublisherAndNamespace(t *testing.T) {
	parent, _, now := validBuiltinDelegation(t)
	publisherDigest := "sha256:" + strings.Repeat("1", 64)
	keyDigest := "sha256:" + strings.Repeat("2", 64)
	scope, err := PackagePublishScope("praxis.package")
	if err != nil {
		t.Fatal(err)
	}
	request := DelegationRequest{
		Profile:   DelegationProfilePackagePublish,
		ParentRef: parent.Ref, ParentVersion: parent.Version, ParentDigest: parent.Digest,
		DelegatedPrincipal: PrincipalRef{ID: FirstPartyPublisherPrincipal, Kind: "publisher"},
		TargetKind:         "publisher-generation", TargetIdentity: FirstPartyPublisherPrincipal, TargetVersion: "praxis.package", TargetDigest: publisherDigest, TargetConstraints: []string{"praxis.package"},
		SubjectKind: "publisher", SubjectID: FirstPartyPublisherPrincipal, SubjectVersion: "1", SubjectDigest: publisherDigest, SubjectKeyDigest: keyDigest,
		RequestedAuthority: GovernedPackagePublish, RequestedOperation: "sign", RequestedScope: scope,
		ExpiresAt: now.Add(time.Hour), Reason: "first-party package signing", PolicyRef: AuthorityModelID, PolicyVersion: AuthorityModelSuccessorVersion, PolicyDigest: AuthorityModelSuccessorDigest(),
	}
	if err := ValidateBuiltinPackagePublishDelegation(parent, request, now); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*DelegationRequest){
		"publisher":  func(r *DelegationRequest) { r.SubjectDigest = "sha256:" + strings.Repeat("3", 64) },
		"namespace":  func(r *DelegationRequest) { r.TargetConstraints = []string{"praxis"} },
		"capability": func(r *DelegationRequest) { r.RequestedCapabilities = []string{"package.activate"} },
		"downgrade": func(r *DelegationRequest) {
			r.PolicyVersion = AuthorityModelVersion
			r.PolicyDigest = AuthorityModelDigest()
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := request
			mutate(&candidate)
			if err := ValidateBuiltinPackagePublishDelegation(parent, candidate, now); err == nil {
				t.Fatal("invalid package-publish delegation was accepted")
			}
		})
	}
}

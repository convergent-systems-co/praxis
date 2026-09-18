package goalstore

import (
	"context"
	"strings"
	"testing"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// TestSaveAuthorityGenerationRejectsEveryRepairDelegationMarker proves the
// production Repository path reaches the structurally mandatory typed state
// admission boundary for both forms of delegated/root ambiguity.
func TestSaveAuthorityGenerationRejectsEveryRepairDelegationMarker(t *testing.T) {
	repo, _ := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	now := time.Unix(1_800_000_000, 0).UTC()

	withParent := authorityGenerationFixture(now)
	withParent.Authorities = []string{contracts.GovernedInstallationRepairStorageSchema}
	withParent.ParentRef = "authority:parent"
	withParent.ParentVersion = "1"
	withParent.ParentDigest = "sha256:" + strings.Repeat("b", 64)
	withParent.DelegatedBy = contracts.PrincipalRef{ID: "principal:parent", Kind: "human"}
	withParent.DelegationRef = "delegation:1"
	withParent.DelegationDigest = "sha256:" + strings.Repeat("c", 64)
	withParent.PolicyRef = "policy:test"
	withParent.PolicyVersion = "1"
	withParent.PolicyDigest = "sha256:" + strings.Repeat("d", 64)
	withParent.Digest, _ = withParent.ComputeDigest()
	if err := repo.SaveAuthorityGeneration(context.Background(), withParent, now, nil); err == nil {
		t.Fatal("production persistence accepted repair authority with ParentRef")
	}

	withDelegatedBy := authorityGenerationFixture(now)
	withDelegatedBy.Authorities = []string{contracts.GovernedInstallationRepairRuntimeState}
	withDelegatedBy.DelegatedBy = contracts.PrincipalRef{ID: "principal:delegate", Kind: "human"}
	withDelegatedBy.Digest, _ = withDelegatedBy.ComputeDigest()
	if err := repo.SaveAuthorityGeneration(context.Background(), withDelegatedBy, now, nil); err == nil {
		t.Fatal("production persistence accepted repair authority with DelegatedBy but no ParentRef")
	}
}

// TestSaveAuthorityGenerationRejectsRootRepairAuthorityBypass proves repair
// roots can only be minted by the governed successor transition.
func TestSaveAuthorityGenerationRejectsRootRepairAuthorityBypass(t *testing.T) {
	repo, _ := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	now := time.Unix(1_800_000_000, 0).UTC()
	root := authorityGenerationFixture(now)
	root.Authorities = []string{contracts.GovernedInstallationRepairStorageSchema, contracts.GovernedInstallationRepairRuntimeState}
	digest, err := root.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	root.Digest = digest
	if err := repo.SaveAuthorityGeneration(context.Background(), root, now, nil); err == nil {
		t.Fatal("generic repository persistence accepted a repair-bearing root")
	}
}

// alwaysAllowPolicy is a deliberately permissive DelegationContainmentPolicy
// used only to prove that Repository's plaintext repair-authority guard
// blocks a delegated repair-authority generation even when the
// containment policy itself would allow it through — i.e. the guard is a
// real, independent second layer, not merely a restatement of whatever
// the containment policy already enforces.
type alwaysAllowPolicy struct{}

func (alwaysAllowPolicy) ContainDelegation(contracts.AuthorityGeneration, contracts.DelegationRequest, time.Time) error {
	return nil
}

func repairDelegationFixture(now time.Time, requestedAuthority string) (root contracts.AuthorityGeneration, request contracts.AuthorityRequest, decision contracts.AuthorityDecision) {
	root = contracts.AuthorityGeneration{Ref: "installation-governance:root", Version: "1", Principal: contracts.PrincipalRef{ID: "installation-owner:root", Kind: "human"}, Scope: "installation-governance:root", Capabilities: []string{contracts.AuthorityDelegateCapability}, ProvenanceRef: "bootstrap-record:test", ProvenanceDigest: "sha256:" + strings.Repeat("a", 64), State: contracts.AuthorityGenerationActive, EffectiveAt: now.Add(-time.Hour)}
	root.Digest, _ = root.ComputeDigest()

	expiresAt := now.Add(time.Hour)
	delegatedPrincipal := contracts.PrincipalRef{ID: "controller:repair", Kind: "controller"}
	delegation := contracts.DelegationRequest{
		Profile: "installation-repair-test", ParentRef: root.Ref, ParentVersion: root.Version, ParentDigest: root.Digest,
		DelegatedPrincipal: delegatedPrincipal, TargetKind: "lifecycle-plan", TargetIdentity: "plan:repair-1", TargetVersion: "1",
		TargetDigest: "sha256:" + strings.Repeat("b", 64), RequestedAuthority: requestedAuthority, RequestedOperation: "repair",
		RequestedScope: "installation-repair:root", ProposalVersion: "1", ProposalDigest: "sha256:" + strings.Repeat("c", 64),
		ReviewVersion: "1", ReviewDigest: "sha256:" + strings.Repeat("c", 64), ExpiresAt: expiresAt, Reason: "test",
		PolicyRef: contracts.AuthorityModelID, PolicyVersion: contracts.AuthorityModelVersion, PolicyDigest: contracts.AuthorityModelDigest(),
	}
	request = contracts.AuthorityRequest{
		ID: "authority-request:repair-1", Version: "1", RequestedAuthority: contracts.AuthorityDelegateCapability,
		RequestedScope: delegation.RequestedScope, Reason: "test", Status: contracts.AuthorityRequestPending, Delegation: &delegation,
	}
	requestDigest, _ := request.DigestAt(now)
	decision = contracts.AuthorityDecision{
		RequestID: request.ID, RequestVersion: request.Version, RequestDigest: requestDigest, DecisionRef: "authority-decision:repair-1",
		DecisionVersion: "1", DecidedBy: root.Principal, AuthorityRef: root.Ref, AuthorityVersion: root.Version, AuthorityGenerationDigest: root.Digest,
		GrantedScope: delegation.RequestedScope, Outcome: contracts.AuthorityApprove, AuthorityDigest: "sha256:" + strings.Repeat("d", 64),
		IssuedAt: now, Delegation: &delegation,
	}
	return root, request, decision
}

// TestSaveDelegatedAuthorityGenerationRejectsRepairAuthority proves the
// real production entry point refuses to persist a delegated generation
// naming an installation-repair authority, even against a permissive
// containment policy that would otherwise allow the delegation through —
// isolating PLAN-016 WU4's plaintext check as an independent closure, not
// a restatement of the containment policy.
func TestSaveDelegatedAuthorityGenerationRejectsRepairAuthority(t *testing.T) {
	repo, _ := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	now := time.Unix(1_800_000_000, 0).UTC()
	root, request, decision := repairDelegationFixture(now, contracts.GovernedInstallationRepairStorageSchema)

	if err := repo.SaveAuthorityGeneration(context.Background(), root, root.EffectiveAt, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SaveAuthorityRequest(context.Background(), request, now, nil); err != nil {
		t.Fatal(err)
	}

	if _, err := repo.SaveDelegatedAuthorityGeneration(context.Background(), request.ID, request.Version, decision, alwaysAllowPolicy{}, now); err == nil {
		t.Fatal("SaveDelegatedAuthorityGeneration accepted a delegated installation-repair authority")
	}
}

// TestSaveAuthorityDecisionAndDelegatedAuthorityGenerationRejectsRepairAuthority
// is the same proof against the batched decision+generation entry point.
func TestSaveAuthorityDecisionAndDelegatedAuthorityGenerationRejectsRepairAuthority(t *testing.T) {
	repo, _ := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	now := time.Unix(1_800_000_000, 0).UTC()
	root, request, decision := repairDelegationFixture(now, contracts.GovernedInstallationRepairRuntimeState)

	if err := repo.SaveAuthorityGeneration(context.Background(), root, root.EffectiveAt, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SaveAuthorityRequest(context.Background(), request, now, nil); err != nil {
		t.Fatal(err)
	}

	if _, err := repo.SaveAuthorityDecisionAndDelegatedAuthorityGeneration(context.Background(), request.ID, request.Version, decision, alwaysAllowPolicy{}, now); err == nil {
		t.Fatal("SaveAuthorityDecisionAndDelegatedAuthorityGeneration accepted a delegated installation-repair authority")
	}
}

// TestSaveDelegatedAuthorityGenerationAllowsNonRepairAuthority is a
// regression guard: an ordinary delegation unrelated to installation
// repair must still succeed through the same code path, proving the new
// check is scoped to exactly the repair-authority case.
func TestSaveDelegatedAuthorityGenerationAllowsNonRepairAuthority(t *testing.T) {
	repo, _ := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	now := time.Unix(1_800_000_000, 0).UTC()
	root, request, decision := repairDelegationFixture(now, contracts.GovernedPackagePublish)

	if err := repo.SaveAuthorityGeneration(context.Background(), root, root.EffectiveAt, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SaveAuthorityRequest(context.Background(), request, now, nil); err != nil {
		t.Fatal(err)
	}

	if _, err := repo.SaveDelegatedAuthorityGeneration(context.Background(), request.ID, request.Version, decision, alwaysAllowPolicy{}, now); err != nil {
		t.Fatalf("ordinary delegated authority was unexpectedly rejected: %v", err)
	}
}

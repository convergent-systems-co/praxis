package goalstore

import (
	"context"
	"strings"
	"testing"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// activateAuthorityModelRecordForTest positions an active model state under an
// explicit record id so a test can present several identities for the same
// version label (for example a forged v6 digest followed by the real v6).
func activateAuthorityModelRecordForTest(t *testing.T, repo Repository, id, version, digest string, now time.Time) {
	t.Helper()
	ctx := context.Background()
	tx, err := repo.Store.DB().BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := repo.insertGovernanceRecordTx(ctx, tx, id, "1", contracts.AuthorityModelState{Version: "1", ActiveModel: contracts.AuthorityModelID, ActiveVersion: version, ActiveDigest: digest, State: "committed"}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE authority_model_active SET object_namespace=?,object_id=?,object_version=?,updated_at=? WHERE singleton_id=?`, publisherGovernanceNamespace, id, "1", now.UTC().Format(time.RFC3339Nano), authorityModelActiveProjectionID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

// packageDeployDelegationFixture delegates the closed v3 package-deploy
// profile from the installation root exactly as the CLI request path does.
func packageDeployDelegationFixture(t *testing.T) (Repository, contracts.AuthorityGeneration, contracts.AuthorityGeneration, time.Time) {
	t.Helper()
	repo, _ := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	ctx := context.Background()
	now := time.Now().UTC()
	root := authorityGenerationFixture(now.Add(-time.Minute))
	repo.InstallationDigest = root.ProvenanceDigest
	if err := repo.SaveAuthorityGeneration(ctx, root, root.EffectiveAt, nil); err != nil {
		t.Fatal(err)
	}
	expires := now.Add(time.Hour)
	scope, err := contracts.PackageDeploymentScope(root.Digest)
	if err != nil {
		t.Fatal(err)
	}
	proposalDigest := "sha256:" + strings.Repeat("b", 64)
	reviewDigest := "sha256:" + strings.Repeat("c", 64)
	delegation := contracts.DelegationRequest{Profile: contracts.DelegationProfilePackageDeploy, ParentRef: root.Ref, ParentVersion: root.Version, ParentDigest: root.Digest, DelegatedPrincipal: contracts.PackageManagerPrincipal(), TargetKind: contracts.PackageManagerPrincipalKind, TargetIdentity: contracts.PackageManagerPrincipalID, TargetVersion: "1", TargetDigest: root.Digest, TargetConstraints: []string{root.Digest}, RequestedCapabilities: []string{}, RequestedOperations: []string{}, RequestedAuthority: contracts.GovernedPackageDeploy, RequestedOperation: "deploy", RequestedScope: scope, ProposalVersion: "1", ProposalDigest: proposalDigest, ReviewVersion: "1", ReviewDigest: reviewDigest, ExpiresAt: expires, Reason: "governed installation-local package deployment", PolicyRef: contracts.AuthorityModelID, PolicyVersion: contracts.AuthorityModelDeploymentVersion, PolicyDigest: contracts.AuthorityModelDeploymentDigest(), SubjectKind: contracts.PackageManagerPrincipalKind, SubjectID: contracts.PackageManagerPrincipalID, SubjectVersion: "1", SubjectDigest: root.Digest}
	request := contracts.AuthorityRequest{ID: "package-manager-authority-request:" + proposalDigest + ":" + reviewDigest, Version: "1", RequestedAuthority: contracts.GovernedPackageDeploy, RequestedScope: scope, Reason: delegation.Reason, Status: contracts.AuthorityRequestPending, Delegation: &delegation}
	requestDigest, err := repo.SaveAuthorityRequest(ctx, request, now, &expires)
	if err != nil {
		t.Fatal(err)
	}
	decision := contracts.AuthorityDecision{RequestID: request.ID, RequestVersion: "1", RequestDigest: requestDigest, DecisionRef: "authority-decision:" + request.ID, DecisionVersion: "1", DecidedBy: root.Principal, AuthorityRef: root.Ref, AuthorityVersion: root.Version, AuthorityGenerationDigest: root.Digest, GrantedScope: scope, Outcome: contracts.AuthorityApprove, AuthorityDigest: contracts.AuthorityModelDeploymentDigest(), IssuedAt: now, ExpiresAt: &expires, Delegation: &delegation}
	child, err := repo.SaveAuthorityDecisionAndDelegatedAuthorityGeneration(ctx, request.ID, "1", decision, builtinTestPolicy{}, now)
	if err != nil {
		t.Fatalf("package-deploy delegation failed: %v", err)
	}
	if child.AuthorityModelVersion != contracts.AuthorityModelDeploymentVersion || child.AuthorityModelDigest != contracts.AuthorityModelDeploymentDigest() {
		t.Fatalf("package-manager generation must be labelled v3: %#v", child)
	}
	return repo, root, child, now
}

// TestResolvePackageManagerAuthorityAdmitsModelsThatRetainV3 proves the
// installation-local package-manager principal resolves under every adopted
// model whose succession chain retains v3, and under nothing else.
func TestResolvePackageManagerAuthorityAdmitsModelsThatRetainV3(t *testing.T) {
	repo, root, child, now := packageDeployDelegationFixture(t)
	ctx := context.Background()
	if _, err := repo.ResolvePackageManagerAuthority(ctx, root.Digest, now); err == nil {
		t.Fatal("package-manager authority resolved without an adopted model")
	}
	steps := []struct {
		id, version, digest string
		admit               bool
	}{
		{"model-v1", contracts.AuthorityModelVersion, contracts.AuthorityModelDigest(), false},
		{"model-v2", contracts.AuthorityModelSuccessorVersion, contracts.AuthorityModelSuccessorDigest(), false},
		{"model-v3", contracts.AuthorityModelDeploymentVersion, contracts.AuthorityModelDeploymentDigest(), true},
		{"model-v4", contracts.AuthorityModelGoalsPublicationVersion, contracts.AuthorityModelGoalsPublicationDigest(), true},
		{"model-v5", contracts.AuthorityModelGoalsRecoveryVersion, contracts.AuthorityModelGoalsRecoveryDigest(), true},
		{"model-forged-v6", contracts.AuthorityModelRoutingVersion, contracts.AuthorityModelDeploymentDigest(), false},
		{"model-forged-v3", contracts.AuthorityModelDeploymentVersion, contracts.AuthorityModelRoutingDigest(), false},
		{"model-v6", contracts.AuthorityModelRoutingVersion, contracts.AuthorityModelRoutingDigest(), true},
	}
	for i, step := range steps {
		at := now.Add(time.Duration(i+1) * time.Millisecond)
		activateAuthorityModelRecordForTest(t, repo, "active-authority-model-"+step.id, step.version, step.digest, at)
		got, err := repo.ResolvePackageManagerAuthority(ctx, root.Digest, at)
		if step.admit {
			if err != nil {
				t.Fatalf("%s must admit package.deploy: %v", step.id, err)
			}
			if got.Digest != child.Digest || got.AuthorityModelVersion != contracts.AuthorityModelDeploymentVersion || got.AuthorityModelDigest != contracts.AuthorityModelDeploymentDigest() {
				t.Fatalf("%s resolved a different generation: %#v", step.id, got)
			}
			continue
		}
		if err == nil {
			t.Fatalf("%s must refuse package.deploy", step.id)
		}
	}
	// The resolved generation must bind the exact root; a foreign root digest
	// never resolves even under an admitting model.
	if _, err := repo.ResolvePackageManagerAuthority(ctx, "sha256:"+strings.Repeat("d", 64), now.Add(time.Second)); err == nil {
		t.Fatal("package-manager authority resolved for a foreign installation root")
	}
}

// TestPackageDeployDelegationRemainsV3Labelled proves that a delegation
// requesting the routing model as its policy is refused by the closed
// package-deploy containment rule: routing capability cannot leak into the
// deployment profile through the retention predicate.
func TestPackageDeployDelegationRemainsV3Labelled(t *testing.T) {
	repo, root, _, now := packageDeployDelegationFixture(t)
	ctx := context.Background()
	activateAuthorityModelRecordForTest(t, repo, "active-authority-model-v6", contracts.AuthorityModelRoutingVersion, contracts.AuthorityModelRoutingDigest(), now)
	expires := now.Add(time.Hour)
	scope, _ := contracts.PackageDeploymentScope(root.Digest)
	proposalDigest := "sha256:" + strings.Repeat("e", 64)
	delegation := contracts.DelegationRequest{Profile: contracts.DelegationProfilePackageDeploy, ParentRef: root.Ref, ParentVersion: root.Version, ParentDigest: root.Digest, DelegatedPrincipal: contracts.PackageManagerPrincipal(), TargetKind: contracts.PackageManagerPrincipalKind, TargetIdentity: contracts.PackageManagerPrincipalID, TargetVersion: "1", TargetDigest: root.Digest, TargetConstraints: []string{root.Digest}, RequestedCapabilities: []string{}, RequestedOperations: []string{}, RequestedAuthority: contracts.GovernedPackageDeploy, RequestedOperation: "deploy", RequestedScope: scope, ProposalVersion: "1", ProposalDigest: proposalDigest, ReviewVersion: "1", ReviewDigest: proposalDigest, ExpiresAt: expires, Reason: "routing-labelled deploy delegation", PolicyRef: contracts.AuthorityModelID, PolicyVersion: contracts.AuthorityModelRoutingVersion, PolicyDigest: contracts.AuthorityModelRoutingDigest(), SubjectKind: contracts.PackageManagerPrincipalKind, SubjectID: contracts.PackageManagerPrincipalID, SubjectVersion: "1", SubjectDigest: root.Digest}
	request := contracts.AuthorityRequest{ID: "package-manager-authority-request:v6-labelled", Version: "1", RequestedAuthority: contracts.GovernedPackageDeploy, RequestedScope: scope, Reason: delegation.Reason, Status: contracts.AuthorityRequestPending, Delegation: &delegation}
	requestDigest, err := repo.SaveAuthorityRequest(ctx, request, now, &expires)
	if err != nil {
		t.Fatal(err)
	}
	decision := contracts.AuthorityDecision{RequestID: request.ID, RequestVersion: "1", RequestDigest: requestDigest, DecisionRef: "authority-decision:" + request.ID, DecisionVersion: "1", DecidedBy: root.Principal, AuthorityRef: root.Ref, AuthorityVersion: root.Version, AuthorityGenerationDigest: root.Digest, GrantedScope: scope, Outcome: contracts.AuthorityApprove, AuthorityDigest: contracts.AuthorityModelRoutingDigest(), IssuedAt: now, ExpiresAt: &expires, Delegation: &delegation}
	if _, err := repo.SaveAuthorityDecisionAndDelegatedAuthorityGeneration(ctx, request.ID, "1", decision, builtinTestPolicy{}, now); err == nil || !strings.Contains(err.Error(), "authority-model v3") {
		t.Fatalf("package-deploy delegation under a v6 policy label must be refused by the closed profile, got %v", err)
	}
}

package contracts

import "testing"

// TestAuthorityModelRetainsDeploymentSemantics is the package.deploy admission
// matrix: the predicate must admit every model whose predecessor chain
// includes v3 and refuse everything else, without enumerating successors.
func TestAuthorityModelRetainsDeploymentSemantics(t *testing.T) {
	cases := []struct {
		name            string
		version, digest string
		want            bool
	}{
		{"v3 active admits", AuthorityModelDeploymentVersion, AuthorityModelDeploymentDigest(), true},
		{"v6 active admits (routing binds v3)", AuthorityModelRoutingVersion, AuthorityModelRoutingDigest(), true},
		{"v4 active admits (Goals branch off v3)", AuthorityModelGoalsPublicationVersion, AuthorityModelGoalsPublicationDigest(), true},
		{"v5 active admits (Goals branch off v3)", AuthorityModelGoalsRecoveryVersion, AuthorityModelGoalsRecoveryDigest(), true},
		{"v1 active refuses", AuthorityModelVersion, AuthorityModelDigest(), false},
		{"v2 active refuses", AuthorityModelSuccessorVersion, AuthorityModelSuccessorDigest(), false},
		{"forged v6 digest refuses", AuthorityModelRoutingVersion, AuthorityModelDeploymentDigest(), false},
		{"forged v3 digest refuses", AuthorityModelDeploymentVersion, "sha256:0000000000000000000000000000000000000000000000000000000000000000", false},
		{"unknown version refuses", "v7", AuthorityModelRoutingDigest(), false},
		{"empty identity refuses", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := AuthorityModelRetains(tc.version, tc.digest, AuthorityModelDeploymentVersion); got != tc.want {
				t.Fatalf("AuthorityModelRetains(%q, %q, v3) = %v, want %v", tc.version, tc.digest, got, tc.want)
			}
		})
	}
}

// TestAuthorityModelRetainsNeverFlowsBackwardOrAcrossBranches proves that
// routing capability does not leak into v3 and that the installation-scoped
// Goals branch does not retain the global routing branch.
func TestAuthorityModelRetainsNeverFlowsBackwardOrAcrossBranches(t *testing.T) {
	if AuthorityModelRetains(AuthorityModelDeploymentVersion, AuthorityModelDeploymentDigest(), AuthorityModelRoutingVersion) {
		t.Fatal("v3 must not retain v6 routing semantics")
	}
	if AuthorityModelRetains(AuthorityModelGoalsRecoveryVersion, AuthorityModelGoalsRecoveryDigest(), AuthorityModelRoutingVersion) {
		t.Fatal("v5 must not retain v6 routing semantics")
	}
	if AuthorityModelRetains(AuthorityModelRoutingVersion, AuthorityModelRoutingDigest(), AuthorityModelGoalsPublicationVersion) {
		t.Fatal("v6 must not retain v4 Goals publication semantics")
	}
	if AuthorityModelRetains(AuthorityModelRoutingVersion, AuthorityModelRoutingDigest(), AuthorityModelGoalsRecoveryVersion) {
		t.Fatal("v6 must not retain v5 Goals recovery semantics")
	}
	if AuthorityModelRetains(AuthorityModelDeploymentVersion, AuthorityModelDeploymentDigest(), AuthorityModelGoalsPublicationVersion) {
		t.Fatal("v3 must not retain v4")
	}
	if AuthorityModelRetains(AuthorityModelRoutingVersion, AuthorityModelRoutingDigest(), "") {
		t.Fatal("empty required version must refuse")
	}
}

// TestAuthorityModelRetainsPublishSemantics is the publisher (v2) admission
// matrix used by publisher enrollment and package publish paths.
func TestAuthorityModelRetainsPublishSemantics(t *testing.T) {
	admit := []struct{ version, digest string }{
		{AuthorityModelSuccessorVersion, AuthorityModelSuccessorDigest()},
		{AuthorityModelDeploymentVersion, AuthorityModelDeploymentDigest()},
		{AuthorityModelGoalsPublicationVersion, AuthorityModelGoalsPublicationDigest()},
		{AuthorityModelGoalsRecoveryVersion, AuthorityModelGoalsRecoveryDigest()},
		{AuthorityModelRoutingVersion, AuthorityModelRoutingDigest()},
	}
	for _, c := range admit {
		if !AuthorityModelRetains(c.version, c.digest, AuthorityModelSuccessorVersion) {
			t.Fatalf("%s must retain v2 publish semantics", c.version)
		}
	}
	if AuthorityModelRetains(AuthorityModelVersion, AuthorityModelDigest(), AuthorityModelSuccessorVersion) {
		t.Fatal("v1 must not retain v2")
	}
	if AuthorityModelRetains(AuthorityModelSuccessorVersion, AuthorityModelRoutingDigest(), AuthorityModelSuccessorVersion) {
		t.Fatal("forged v2 digest must refuse")
	}
}

func TestAuthorityModelStateRetainsRequiresCanonicalModelID(t *testing.T) {
	state := AuthorityModelState{ActiveModel: AuthorityModelID, ActiveVersion: AuthorityModelRoutingVersion, ActiveDigest: AuthorityModelRoutingDigest()}
	if !AuthorityModelStateRetains(state, AuthorityModelDeploymentVersion) {
		t.Fatal("active v6 state must retain v3")
	}
	state.ActiveModel = "authority-model:forged"
	if AuthorityModelStateRetains(state, AuthorityModelDeploymentVersion) {
		t.Fatal("foreign model id must refuse")
	}
	if AuthorityModelStateRetains(AuthorityModelState{}, AuthorityModelDeploymentVersion) {
		t.Fatal("empty state must refuse")
	}
}

// TestAuthorityModelPredecessorGraphMatchesDigestBindings pins the retention
// graph to the digest constructors so the two cannot drift apart silently.
func TestAuthorityModelPredecessorGraphMatchesDigestBindings(t *testing.T) {
	want := map[string]string{"v2": "v1", "v3": "v2", "v4": "v3", "v5": "v4", "v6": "v3"}
	if len(authorityModelPredecessor) != len(want) {
		t.Fatalf("unexpected predecessor graph: %#v", authorityModelPredecessor)
	}
	for version, predecessor := range want {
		if authorityModelPredecessor[version] != predecessor {
			t.Fatalf("%s predecessor = %q, want %q", version, authorityModelPredecessor[version], predecessor)
		}
	}
	if _, ok := authorityModelPredecessor[AuthorityModelVersion]; ok {
		t.Fatal("v1 is the origin and has no predecessor")
	}
}

package contracts

import (
	"testing"
	"time"
)

func TestInstallationRootIdentityDerivesCanonicalScopeAndPrincipal(t *testing.T) {
	bootstrapDigest := "sha256:" + "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	scope, err := InstallationGovernanceScope(bootstrapDigest)
	if err != nil {
		t.Fatal(err)
	}
	if want := "installation-governance:" + bootstrapDigest; scope != want {
		t.Fatalf("scope = %q, want %q", scope, want)
	}
	principal, err := InstallationOwnerPrincipal(bootstrapDigest)
	if err != nil {
		t.Fatal(err)
	}
	if principal.ID != "installation-owner:"+bootstrapDigest || principal.Kind != "human" {
		t.Fatalf("principal = %+v", principal)
	}
}

func TestInstallationRootIdentityRejectsNonCanonicalBootstrapDigest(t *testing.T) {
	for _, value := range []string{"", "goals-lifecycle:qualification", "sha256:not-a-digest", "sha256:" + "ABCDEF"} {
		if _, err := InstallationGovernanceScope(value); err == nil {
			t.Fatalf("scope derivation accepted invalid bootstrap digest %q", value)
		}
		if _, err := InstallationOwnerPrincipal(value); err == nil {
			t.Fatalf("principal derivation accepted invalid bootstrap digest %q", value)
		}
	}
}

func TestAuthorityGenerationDigestBindsCanonicalFields(t *testing.T) {
	generation := AuthorityGeneration{
		Ref:              "installation-governance:bootstrap",
		Version:          "1",
		Principal:        PrincipalRef{ID: "installation-owner:bootstrap", Kind: "human"},
		Scope:            "goal:example/proposal/example",
		ProvenanceRef:    "bootstrap-record:bootstrap:os-user:test",
		ProvenanceDigest: "sha256:bootstrap",
		State:            AuthorityGenerationActive,
		EffectiveAt:      time.Unix(1, 0).UTC(),
	}
	digest, err := generation.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	generation.Digest = digest
	if err := generation.VerifyDigest(); err != nil {
		t.Fatal(err)
	}
	changed := generation
	changed.Scope = "goal:other/proposal/other"
	if err := changed.VerifyDigest(); err == nil {
		t.Fatal("changed authority scope retained the original generation digest")
	}
}

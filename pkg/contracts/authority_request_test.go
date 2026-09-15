package contracts

import (
	"testing"
	"time"
)

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

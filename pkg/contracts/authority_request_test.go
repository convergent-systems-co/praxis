package contracts

import (
	"strings"
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

func TestPackageDeployDecisionSeparatesDecisionAndOperationalAuthority(t *testing.T) {
	now := time.Now().UTC()
	intent := ActionIntent{Version: "1", ID: "package-deployment:fixture", Actor: PackageManagerPrincipal(), Operation: GovernedPackageDeploy, Target: "fixture.package@1.0.0#sha256:" + strings.Repeat("a", 64), Parameters: map[string]string{"closure_digest": "sha256:" + strings.Repeat("b", 64), "verification_evidence_digest": "package-verification-closure:sha256:" + strings.Repeat("c", 64)}, Scope: "installation-governance:sha256:" + strings.Repeat("d", 64)}
	intentDigest, err := intent.Digest()
	if err != nil {
		t.Fatal(err)
	}
	request := AuthorityRequest{ID: "package-deploy-request:" + intentDigest, Version: "1", RequestedAuthority: GovernedPackageDeploy, RequestedScope: intent.Scope, Reason: "test exact deployment", Status: AuthorityRequestPending, IntentDigest: intentDigest, InstallationDigest: "sha256:" + strings.Repeat("e", 64), ClosureDigest: intent.Parameters["closure_digest"], VerificationEvidenceDigest: intent.Parameters["verification_evidence_digest"], Intent: &intent}
	root := PrincipalRef{ID: "installation-owner:root", Kind: "human"}
	requestDigest, err := request.Digest()
	if err != nil {
		t.Fatal(err)
	}
	decision := AuthorityDecision{RequestID: request.ID, RequestVersion: request.Version, RequestDigest: requestDigest, DecisionRef: "decision:fixture", DecisionVersion: "1", DecidedBy: root, AuthorityRef: "root", AuthorityVersion: "1", AuthorityGenerationDigest: "sha256:" + strings.Repeat("1", 64), GrantedScope: request.RequestedScope, Outcome: AuthorityApprove, AuthorityDigest: "sha256:" + strings.Repeat("2", 64), IssuedAt: now}
	if err := decision.Validate(request, now); err == nil {
		t.Fatal("package-deploy decision without operational authority was accepted")
	}
	decision.OperationalAuthorityRef = "authority-delegation:package-manager"
	decision.OperationalAuthorityVersion = "1"
	decision.OperationalAuthorityGenerationDigest = "sha256:" + strings.Repeat("3", 64)
	if err := decision.Validate(request, now); err != nil {
		t.Fatal(err)
	}
}

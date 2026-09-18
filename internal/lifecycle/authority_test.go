package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type fixtureAuthoritySource struct {
	request    contracts.AuthorityRequest
	decision   contracts.AuthorityDecision
	generation contracts.AuthorityGeneration
	validate   error
}

func (s fixtureAuthoritySource) LoadAuthorityRequest(context.Context, string, string, time.Time) (contracts.AuthorityRequest, error) {
	return s.request, nil
}
func (s fixtureAuthoritySource) LoadAuthorityDecision(context.Context, string, string, time.Time) (contracts.AuthorityDecision, error) {
	return s.decision, nil
}
func (s fixtureAuthoritySource) LoadAuthorityGeneration(context.Context, string, string, time.Time) (contracts.AuthorityGeneration, error) {
	return s.generation, nil
}
func (s fixtureAuthoritySource) ValidateAuthorityGeneration(context.Context, contracts.AuthorityDecision, time.Time) error {
	return s.validate
}

func lifecycleAuthorityFixture(t *testing.T) (fixtureAuthoritySource, contracts.LifecyclePlan, contracts.LifecycleTransitionStep, time.Time) {
	t.Helper()
	now := time.Now().UTC()
	installation := migrationDigest("a")
	scope, err := contracts.InstallationGovernanceScope(installation)
	if err != nil {
		t.Fatal(err)
	}
	principal, err := contracts.InstallationOwnerPrincipal(installation)
	if err != nil {
		t.Fatal(err)
	}
	generation := contracts.AuthorityGeneration{
		Ref: scope, Version: "2", Principal: principal, Scope: scope,
		ProvenanceRef: "installation-bootstrap", ProvenanceDigest: installation,
		State: contracts.AuthorityGenerationActive, EffectiveAt: now.Add(-time.Hour),
		Authorities:    []string{contracts.GovernedInstallationRepairStorageSchema},
		PredecessorRef: scope, PredecessorVersion: "1", PredecessorDigest: migrationDigest("8"),
	}
	generation.Digest, err = generation.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	expires := now.Add(time.Hour)
	successionDigest := migrationDigest("e")
	request := contracts.AuthorityRequest{
		ID: "repair-request", Version: "1", BaselineID: "baseline", BaselineVersion: "1", BaselineDigest: migrationDigest("b"),
		ProposalID: "proposal", ProposalVersion: "1", ProposalDigest: migrationDigest("c"), ReviewRef: "review", ReviewVersion: "1", ReviewDigest: migrationDigest("d"),
		RequestedAuthority: contracts.GovernedInstallationRepairStorageSchema, RequestedScope: scope, Reason: "repair", Status: contracts.AuthorityRequestPending,
		InstallationDigest: installation,
		Repair:             &contracts.InstallationRepairAuthorityRequest{BootstrapDigest: installation, RootRef: generation.Ref, RootVersion: generation.Version, RootDigest: generation.Digest, SuccessionDecisionDigest: successionDigest, Operation: contracts.GovernedInstallationRepairStorageSchema, ExpiresAt: expires},
	}
	requestDigest, err := request.DigestAt(now)
	if err != nil {
		t.Fatal(err)
	}
	decision := contracts.AuthorityDecision{
		RequestID: request.ID, RequestVersion: request.Version, RequestDigest: requestDigest,
		DecisionRef: "repair-decision", DecisionVersion: "1", DecidedBy: principal,
		AuthorityRef: generation.Ref, AuthorityVersion: generation.Version, AuthorityGenerationDigest: generation.Digest,
		GrantedScope: scope, Outcome: contracts.AuthorityApprove, AuthorityDigest: successionDigest, IssuedAt: now.Add(-time.Minute), ExpiresAt: &expires,
	}
	decisionDigest, err := decision.Digest()
	if err != nil {
		t.Fatal(err)
	}
	step := contracts.LifecycleTransitionStep{ID: "repair", Authority: contracts.LifecycleAuthorityRequirement{
		Required: true, Operation: request.RequestedAuthority, Scope: scope,
		RequestRef: request.ID, RequestVersion: request.Version, RequestDigest: requestDigest,
		DecisionRef: decision.DecisionRef, DecisionVersion: decision.DecisionVersion, DecisionDigest: decisionDigest,
		AuthorityRef: generation.Ref, AuthorityVersion: generation.Version, AuthorityGenerationDigest: generation.Digest,
	}}
	plan := contracts.LifecyclePlan{PlanID: "repair-plan", PlanVersion: "1", Digest: migrationDigest("f")}
	return fixtureAuthoritySource{request: request, decision: decision, generation: generation}, plan, step, now
}

func TestDurableAuthorityValidatorRequiresExactCanonicalRootPossession(t *testing.T) {
	source, plan, step, now := lifecycleAuthorityFixture(t)
	validator := DurableAuthorityValidator{Source: source, InstallationDigest: source.request.InstallationDigest, Now: func() time.Time { return now }}
	if _, err := validator.ValidateLifecycleAuthority(context.Background(), plan, step); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*fixtureAuthoritySource)
	}{
		{name: "owner", mutate: func(s *fixtureAuthoritySource) {
			s.generation.Principal.ID = "installation-owner:" + migrationDigest("0")
		}},
		{name: "scope", mutate: func(s *fixtureAuthoritySource) {
			s.generation.Scope = "installation-governance:" + migrationDigest("0")
		}},
		{name: "lineage", mutate: func(s *fixtureAuthoritySource) {
			s.generation.ParentRef, s.generation.ParentVersion, s.generation.ParentDigest = "parent", "1", migrationDigest("1")
		}},
		{name: "delegator", mutate: func(s *fixtureAuthoritySource) {
			s.generation.DelegatedBy = contracts.PrincipalRef{ID: "controller:x", Kind: "controller"}
		}},
		{name: "provenance", mutate: func(s *fixtureAuthoritySource) { s.generation.ProvenanceDigest = migrationDigest("0") }},
		{name: "operation possession", mutate: func(s *fixtureAuthoritySource) { s.generation.Authorities = nil }},
		{name: "durable generation rejection", mutate: func(s *fixtureAuthoritySource) { s.validate = errors.New("revoked") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := source
			tc.mutate(&changed)
			if _, err := (DurableAuthorityValidator{Source: changed, InstallationDigest: source.request.InstallationDigest, Now: func() time.Time { return now }}).ValidateLifecycleAuthority(context.Background(), plan, step); err == nil {
				t.Fatal("non-canonical repair authority was accepted")
			}
		})
	}
	if _, err := (DurableAuthorityValidator{Source: source, InstallationDigest: migrationDigest("0"), Now: func() time.Time { return now }}).ValidateLifecycleAuthority(context.Background(), plan, step); err == nil {
		t.Fatal("authority rooted in a different installation bootstrap was accepted")
	}
}

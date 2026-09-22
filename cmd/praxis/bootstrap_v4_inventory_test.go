package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/goaldrive"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// Regressions added by the consolidated guard/mutation inventory (repair 3).
// Each closes a guard that no earlier test observed in isolation.

// B9 (plan-level authority, each lineage fact on its own). The plan's governing
// authority is re-proved from durable records; every field the plan names must
// match the recorded request and decision, and each mismatch is refused for its
// own reason.
func TestKernelRepair3PlanAuthorityLineageIsRefusedFieldByField(t *testing.T) {
	h := newGateHarness(t)
	repo, db, err := openGovernedRepository(h.ctx, h.env)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	plan := *h.successor.WorkPlan
	if err := repo.VerifyPlanAuthorityLineage(h.ctx, plan, now); err != nil {
		t.Fatalf("control: the genuine plan lineage did not verify: %v", err)
	}
	other := "sha256:" + strings.Repeat("7", 64)
	cases := map[string]struct {
		mutate func(*contracts.WorkPlan)
		want   string
	}{
		"no authority request cited":          {func(p *contracts.WorkPlan) { p.AuthorityRequestID = "" }, "lacks authority-backed acceptance lineage"},
		"no decision cited":                   {func(p *contracts.WorkPlan) { p.AuthorityDecisionRef = "" }, "lacks authority-backed acceptance lineage"},
		"request is for another proposal":     {func(p *contracts.WorkPlan) { p.ProposalDigest = other }, "not the exact WorkPlan acceptance request"},
		"request is for another baseline":     {func(p *contracts.WorkPlan) { p.BaselineDigest = other }, "not the exact WorkPlan acceptance request"},
		"activation binding differs":          {func(p *contracts.WorkPlan) { s := *p.Safety; s.ActivationManifestDigest = other; p.Safety = &s }, "ceremony and activation binding"},
		"decision reference differs":          {func(p *contracts.WorkPlan) { p.AuthorityDecisionRef = "another-decision" }, "does not match its durable authority decision"},
		"decision version differs":            {func(p *contracts.WorkPlan) { p.AuthorityDecisionVersion = "9" }, "does not match its durable authority decision"},
		"authority reference differs":         {func(p *contracts.WorkPlan) { p.AuthorityRef = "another-authority" }, "does not match its durable authority decision"},
		"authority digest differs":            {func(p *contracts.WorkPlan) { p.AuthorityDigest = other }, "does not match its durable authority decision"},
		"authority generation digest differs": {func(p *contracts.WorkPlan) { p.AuthorityGenerationDigest = other }, "does not match its durable authority decision"},
		"accepted-by differs":                 {func(p *contracts.WorkPlan) { p.AcceptedBy = contracts.PrincipalRef{ID: "someone-else", Kind: "human"} }, "does not match its durable authority decision"},
	}
	for name, tc := range cases {
		mutated := plan
		mutated.Candidates = append([]contracts.WorkCandidate(nil), plan.Candidates...)
		tc.mutate(&mutated)
		if err := repo.VerifyPlanAuthorityLineage(h.ctx, mutated, now); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s: got %v, want an error containing %q", name, err, tc.want)
		}
	}

	// The baseline in hand must be the persisted generation, not merely a
	// self-consistent copy with its own valid digest.
	altered := h.successor
	altered.EvidenceRefs = append(append([]string(nil), altered.EvidenceRefs...), "inventory:edited-after-attachment")
	altered.Digest = ""
	digest, err := altered.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	altered.Digest = digest
	if err := repo.VerifyGoverningAuthority(h.ctx, altered, now); err == nil || !strings.Contains(err.Error(), "not the persisted Goal generation") {
		t.Fatalf("a self-consistent but unpersisted baseline was accepted as governed: %v", err)
	}

	// Scope: authority decided for another installation does not govern this one.
	foreign := repo
	foreign.InstallationDigest = "sha256:" + strings.Repeat("6", 64)
	// This case exercises the installation-scope predicate itself; a repository
	// for another installation would in any case be refused earlier by the
	// forward authority anchor (its facts belong to this installation), which
	// TestFAAForeignInstallationIsRefusedByTheAnchor covers.
	foreign.FAA = nil
	if err := foreign.VerifyPlanAuthorityLineage(h.ctx, plan, now); err == nil || !strings.Contains(err.Error(), "not scoped to this installation") {
		t.Fatalf("authority decided for another installation governed this one: %v", err)
	}

	// A revoked root generation no longer governs the plan.
	invalidation := contracts.AuthorityGenerationInvalidation{Ref: h.root.Ref, Version: h.root.Version, GenerationDigest: h.root.Digest, InvalidationRef: "r3-inv", InvalidationVersion: "1", Kind: "revoked", InvalidatedBy: h.root.Principal, EffectiveAt: now, Reason: "inventory"}
	if err := repo.SaveAuthorityGenerationInvalidation(h.ctx, invalidation, now, nil); err != nil {
		t.Fatal(err)
	}
	if err := repo.VerifyPlanAuthorityLineage(h.ctx, plan, time.Now().UTC().Add(time.Second)); err == nil || !strings.Contains(err.Error(), "validate decision authority generation") {
		t.Fatalf("an invalidated root generation still governed the plan: %v", err)
	}
}

// B8 (promoted from Astra's R1-E matrix). A protected WorkPlan-acceptance
// decision persists only when its ceremony record resolves and matches the
// decision field for field; each way of failing that is refused on its own, and
// a ceremony record that is not this installation owner's is refused when it is
// written.
func TestKernelRepair3CeremonyMismatchMatrixIsRefusedForAWorkPlanAcceptance(t *testing.T) {
	s := newSafetyLifecycle(t)
	digest := s.reviewAndRequest()
	repo, db, err := openGovernedRepository(s.ctx, s.env)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	req, err := repo.LoadAuthorityRequestByDigest(s.ctx, digest, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	current, _ := authenticatedOSUser()
	confirmation := sha256.Sum256([]byte("x"))
	evidence := func(mutate func(*contracts.OwnerCeremonyEvidence)) contracts.OwnerCeremonyEvidence {
		e := contracts.OwnerCeremonyEvidence{Profile: contracts.OwnerCeremonyProfile, RequestDigest: digest, Outcome: "approve", Owner: s.root.Principal, RootRef: s.root.Ref, RootVersion: s.root.Version, RootDigest: s.root.Digest, AuthenticatedOSUser: current.Username, ConfirmationDigest: "sha256:" + hex.EncodeToString(confirmation[:]), ConfirmedAt: time.Now().UTC()}
		mutate(&e)
		return e
	}
	decisionWith := func(ceremony string) contracts.AuthorityDecision {
		return contracts.AuthorityDecision{RequestID: req.ID, RequestVersion: req.Version, RequestDigest: digest, DecisionRef: "inv-" + ceremony[len(ceremony)-6:], DecisionVersion: "1", DecidedBy: s.root.Principal, AuthorityRef: s.root.Ref, AuthorityVersion: s.root.Version, AuthorityGenerationDigest: s.root.Digest, GrantedScope: s.root.Scope, Outcome: contracts.AuthorityApprove, AuthorityDigest: s.root.AuthorityModelDigest, IssuedAt: time.Now().UTC(), CeremonyEvidenceDigest: ceremony}
	}
	// Refused when the ceremony record is written.
	for name, mutate := range map[string]func(*contracts.OwnerCeremonyEvidence){
		"wrong root version": func(e *contracts.OwnerCeremonyEvidence) { e.RootVersion = "999" },
		"wrong root digest":  func(e *contracts.OwnerCeremonyEvidence) { e.RootDigest = "sha256:" + strings.Repeat("9", 64) },
		"wrong root ref":     func(e *contracts.OwnerCeremonyEvidence) { e.RootRef = "another-root" },
		"foreign owner":      func(e *contracts.OwnerCeremonyEvidence) { e.Owner.ID = "someone-else" },
		"foreign OS user":    func(e *contracts.OwnerCeremonyEvidence) { e.AuthenticatedOSUser += "x" },
	} {
		if _, err := repo.SaveOwnerCeremony(s.ctx, evidence(mutate), time.Now().UTC()); err == nil {
			t.Fatalf("%s: a ceremony record for another owner/root/user was written", name)
		}
	}
	// Refused when the decision is persisted against a resolvable but non-matching record.
	for name, mutate := range map[string]func(*contracts.OwnerCeremonyEvidence){
		"another request":   func(e *contracts.OwnerCeremonyEvidence) { e.RequestDigest = "sha256:" + strings.Repeat("9", 64) },
		"reject outcome":    func(e *contracts.OwnerCeremonyEvidence) { e.Outcome = "reject" },
		"stray alternative": func(e *contracts.OwnerCeremonyEvidence) { e.SelectedAlternative = "x" },
	} {
		recorded, err := repo.SaveOwnerCeremony(s.ctx, evidence(mutate), time.Now().UTC())
		if err != nil {
			t.Fatalf("%s: setup: %v", name, err)
		}
		if err := repo.SaveAuthorityDecision(s.ctx, req.ID, req.Version, decisionWith(recorded), time.Now().UTC(), nil); err == nil {
			t.Fatalf("%s: a decision persisted against a non-matching ceremony record", name)
		}
	}
	// Refused when no ceremony record exists at all (an arbitrary digest).
	if err := repo.SaveAuthorityDecision(s.ctx, req.ID, req.Version, decisionWith("sha256:"+strings.Repeat("f", 64)), time.Now().UTC(), nil); err == nil || !strings.Contains(err.Error(), "ceremony") {
		t.Fatalf("a decision with an arbitrary ceremony digest persisted: %v", err)
	}
	// Control: the genuine record resolves and the decision persists.
	good, err := repo.SaveOwnerCeremony(s.ctx, evidence(func(*contracts.OwnerCeremonyEvidence) {}), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveAuthorityDecision(s.ctx, req.ID, req.Version, decisionWith(good), time.Now().UTC(), nil); err != nil {
		t.Fatalf("control: a decision with its genuine ceremony record was refused: %v", err)
	}
}

// N7 on the status surface: inspect consumes completions through the same
// authenticating boundary as the controller. A forged completion is never shown
// as progress, the reason is reported, and the generation is not drivable.
func TestKernelRepair3InspectDoesNotDisplayAForgedCompletionAndIsNotDrivable(t *testing.T) {
	h := newGateHarness(t)
	repo, db, err := openGovernedRepository(h.ctx, h.env)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_ = repo
	durable := goaldrive.Ledger{Store: state.NewSQLiteEventStore(db), Actor: contracts.PrincipalRef{ID: "praxis-goal-drive", Kind: "controller"}}
	forged := goaldrive.UnitCompletion{GoalID: repairGoalID, GoalVersion: "2", UnitID: "one", InvocationID: "forge", TurnID: "forge", EndHead: "deadbeef", CompletedAt: time.Now().UTC(), MechanismTestsPassed: true, ConformanceQualified: true, SpecificationDigest: h.successor.WorkPlan.Candidates[0].SourceDigest, ValidationProfileDigest: h.successor.WorkPlan.Safety.ValidationProfileDigest, Evidence: []string{"forged"}}
	if err := durable.RecordCompletion(h.ctx, forged); err != nil {
		t.Fatal(err)
	}
	out, err := h.run("goals-lifecycle", "--operation=inspect", "--goal-id="+repairGoalID, "--goal-version=2")
	if err != nil {
		t.Fatalf("inspect must report, not fail: %v", err)
	}
	var doc struct {
		Drivable bool `json:"drivable"`
		WorkSet  struct {
			Error string `json:"completion_authentication_error"`
			Units []struct {
				Unit      string `json:"unit"`
				Completed bool   `json:"completed"`
			} `json:"units"`
		} `json:"work_set"`
	}
	start := strings.Index(string(out), "{")
	if start < 0 {
		t.Fatalf("no JSON from inspect: %q", out)
	}
	if err := json.Unmarshal(out[strings.LastIndex(string(out), "\n{")+1:], &doc); err != nil {
		if err2 := json.Unmarshal(out[start:], &doc); err2 != nil {
			t.Fatalf("decode inspect: %v\n%s", err, out)
		}
	}
	if doc.Drivable {
		t.Fatalf("a generation with a completion that cannot be authenticated is drivable:\n%s", out)
	}
	if !strings.Contains(doc.WorkSet.Error, "not authenticated") {
		t.Fatalf("inspect did not report why the completion is not trusted (%q):\n%s", doc.WorkSet.Error, out)
	}
	for _, unit := range doc.WorkSet.Units {
		if unit.Completed {
			t.Fatalf("inspect displayed a forged completion as progress: %+v", unit)
		}
	}
	_ = errors.New
}

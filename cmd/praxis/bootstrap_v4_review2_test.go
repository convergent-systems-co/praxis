package main

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/goaldrive"
	"github.com/convergent-systems-co/praxis/internal/goalstore"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// These regressions correspond to the second independent review's findings
// (B5-B14). They assert the repaired behavior at the boundary the review
// exercised, through the production constructors where the review showed a
// fixture had been stronger than production.

func TestKernelRepair2B5AcceptanceCannotInjectCompletedGate(t *testing.T) {
	s := newSafetyLifecycle(t)
	digest := s.reviewAndRequest()
	s.approveWorkPlan(digest)
	repo, db, err := openGovernedRepository(s.ctx, s.env)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	req, err := repo.LoadAuthorityRequestByDigest(s.ctx, digest, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	plan, err := contracts.MaterializeAcceptedPlanCandidate(s.proposal, "injected-completion", digest)
	if err != nil {
		t.Fatal(err)
	}
	plan.Candidates[1].Completed = true
	if _, err := repo.SaveAcceptedWorkPlanFromAuthorityDecision(s.ctx, req.ID, req.Version, plan, "injected-completion", "1", time.Now().UTC(), nil); err == nil {
		t.Fatal("a completed gate injected after owner approval was persisted")
	}
	if accepted, _ := repo.ListAcceptedWorkPlans(s.ctx, s.baseline.Digest, time.Now().UTC()); len(accepted) != 0 {
		t.Fatalf("the rejected acceptance left durable state: %d", len(accepted))
	}
}

// B6: the repository the native runtime hands the controller as its
// authority-gate coordinator carries the verified installation identity and the
// live activation provider, because the runtime is built by the production
// constructor (buildGoalDriveRuntime), not by a stronger test fixture.
func TestKernelRepair2B6NativeRuntimeGateRepositoryIsConfiguredByTheProductionConstructor(t *testing.T) {
	h := newGateHarness(t)
	getenv := func(key string) string {
		if key == "PRAXIS_GOAL_WORKER_ARGV" {
			return `["true"]`
		}
		return h.env(key)
	}
	repoDir := t.TempDir()
	invocation := goaldrive.InvocationRequest{ProviderID: "p", RepositoryPath: repoDir, Branch: "main", GoalVersion: "2", InvocationID: "native"}
	invocation.Input = contracts.GoalInput{Kind: contracts.GoalInputID, GoalID: repairGoalID}
	runtime, db, err := buildGoalDriveRuntime(h.ctx, normalizedOutput{GraphID: "g", GraphVersion: "1"}, invocation, getenv)
	if err != nil {
		t.Fatalf("build native runtime: %v", err)
	}
	defer db.Close()
	gates, ok := runtime.Controller.AuthorityGates.(goalstore.Repository)
	if !ok || gates.InstallationDigest == "" || gates.SafetyActivation == nil {
		t.Fatalf("native gate repository lacks the verified identity or activation provider: %+v", gates)
	}
	if runtime.Controller.SafetyActivation == nil || runtime.Controller.GoverningAuthority == nil {
		t.Fatal("native controller lacks its activation or governing-authority verifier")
	}
	result, err := runtime.Controller.AuthorityGates.ReconcileAuthorityGate(h.ctx, h.successor, h.successor.WorkPlan.Candidates[1], []contracts.GovernedArtifactEvidence{h.dossier}, time.Now().UTC())
	if err != nil || result.Approved || result.Request.GateCandidateID != "gate" {
		t.Fatalf("the native runtime could not create its gate request: %+v %v", result, err)
	}
}

// B7: generic baseline persistence is baseline-only; a safety-bearing
// WorkPlan can only arrive through the authenticated attachment transaction.
func TestKernelRepair2B7BaselinePersistenceSurfacesRefuseSafetyBearingPlans(t *testing.T) {
	h := newGateHarness(t)
	repo, db, err := openGovernedRepository(h.ctx, h.env)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	b := h.successor
	b.Version, b.Digest = "3", ""
	if _, err := repo.Save(h.ctx, b, time.Now().UTC(), nil); !errors.Is(err, goalstore.ErrSafetyPlanRequiresAttachment) {
		t.Fatalf("Save admitted a safety-bearing plan: %v", err)
	}
	unconfigured := repo
	unconfigured.SafetyActivation = nil
	if _, err := unconfigured.Save(h.ctx, b, time.Now().UTC(), nil); !errors.Is(err, goalstore.ErrSafetyPlanRequiresAttachment) {
		t.Fatalf("Save admitted a safety-bearing plan without an activation verifier: %v", err)
	}
	if _, err := repo.Finalize(h.ctx, goalstore.FinalizeRequest{Baseline: b, Persist: true}); err == nil {
		t.Fatal("Finalize persisted a safety-bearing plan")
	}
	for _, withActivation := range []bool{true, false} {
		h2 := newGateHarness(t)
		if !withActivation {
			if err := os.Remove(h2.manifestPath); err != nil {
				t.Fatal(err)
			}
		}
		imported := h2.successor
		imported.Version, imported.Digest = "3", ""
		path := writeBaselineImportDocument(t, h2.dir, "import-safety.json", imported, "")
		if _, err := h2.run("goals-lifecycle", "--operation=import", "--input="+path); err == nil {
			t.Fatalf("supported CLI import persisted a safety-bearing plan (activation present: %v)", withActivation)
		}
		check, cdb, err := openGovernedRepository(h2.ctx, h2.env)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := check.Load(h2.ctx, repairGoalID, "3", time.Now().UTC()); !errors.Is(err, state.ErrSecureBlobNotFound) {
			cdb.Close()
			t.Fatalf("a refused import left a durable generation: %v", err)
		}
		cdb.Close()
	}
}

// B8: neither legacy acceptance nor a caller-chosen ceremony digest can stand
// in for the owner ceremony.
func TestKernelRepair2B8PublicPersistenceCannotManufactureCeremonyBackedState(t *testing.T) {
	s := newSafetyLifecycle(t)
	requestDigest := s.reviewAndRequest()
	repo, db, err := openGovernedRepository(s.ctx, s.env)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	req, err := repo.LoadAuthorityRequestByDigest(s.ctx, requestDigest, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	plan, err := contracts.MaterializeAcceptedPlanCandidate(s.proposal, "legacy-acceptance", requestDigest)
	if err != nil {
		t.Fatal(err)
	}
	legacy := contracts.WorkPlanAcceptance{ProposalDigest: s.proposalDigest, BaselineDigest: s.baseline.Digest, AuthorityRef: "fabricated", AuthorityDigest: "fabricated", AuthorityScope: "fabricated", AcceptanceRef: "legacy-acceptance", AcceptanceDigest: "fabricated", AcceptedBy: contracts.PrincipalRef{ID: "fabricated-human", Kind: "human"}, Mode: "human", ReviewRef: req.ReviewRef, ReviewVersion: req.ReviewVersion, ReviewDigest: req.ReviewDigest}
	if _, err := repo.SaveAcceptedWorkPlan(s.ctx, s.proposal.ID, "4", plan, legacy, "1", time.Now().UTC(), nil); err == nil {
		t.Fatal("legacy acceptance with fabricated authority admitted a safety-bearing plan")
	}
	if _, err := repo.LoadAcceptedWorkPlan(s.ctx, "legacy-acceptance", "1", time.Now().UTC()); err == nil {
		t.Fatal("the refused legacy acceptance is durable")
	}

	h := newGateHarness(t)
	gate := h.pendingGate(h.ledger, "forge-ceremony")
	gateDigest, _ := gate.Digest()
	grepo, gdb, err := openGovernedRepository(h.ctx, h.env)
	if err != nil {
		t.Fatal(err)
	}
	defer gdb.Close()
	decision := func(ceremony string) contracts.AuthorityDecision {
		return contracts.AuthorityDecision{RequestID: gate.ID, RequestVersion: gate.Version, RequestDigest: gateDigest, DecisionRef: "review2-decision", DecisionVersion: "1", DecidedBy: h.root.Principal, AuthorityRef: h.root.Ref, AuthorityVersion: h.root.Version, AuthorityGenerationDigest: h.root.Digest, GrantedScope: h.root.Scope, Outcome: contracts.AuthorityApprove, AuthorityDigest: h.root.AuthorityModelDigest, IssuedAt: time.Now().UTC(), SelectedAlternative: gate.Alternatives[0], CeremonyEvidenceDigest: ceremony}
	}
	if err := grepo.SaveAuthorityDecision(h.ctx, gate.ID, gate.Version, decision("sha256:"+strings.Repeat("f", 64)), time.Now().UTC(), nil); err == nil {
		t.Fatal("an arbitrary ceremony digest was accepted as ceremony proof")
	}
	// A genuine ceremony record for a different outcome cannot be reused.
	rejectCeremony := h.ceremony(grepo, gateDigest, "reject", "")
	if err := grepo.SaveAuthorityDecision(h.ctx, gate.ID, gate.Version, decision(rejectCeremony), time.Now().UTC(), nil); err == nil {
		t.Fatal("a ceremony record for a different outcome was reused")
	}
	// A record naming a foreign OS user is not recorded at all.
	current, _ := authenticatedOSUser()
	forged := contracts.OwnerCeremonyEvidence{Profile: contracts.OwnerCeremonyProfile, RequestDigest: gateDigest, Outcome: "approve", SelectedAlternative: gate.Alternatives[0], Owner: h.root.Principal, RootRef: h.root.Ref, RootVersion: h.root.Version, RootDigest: h.root.Digest, AuthenticatedOSUser: current.Username + "-other", ConfirmationDigest: "sha256:" + strings.Repeat("1", 64), ConfirmedAt: time.Now().UTC()}
	if _, err := grepo.SaveOwnerCeremony(h.ctx, forged, time.Now().UTC()); err == nil {
		t.Fatal("ceremony evidence for an OS user that does not own the root was recorded")
	}
	if _, err := h.decisionFor(gate); err == nil {
		t.Fatal("a rejected forgery left a durable decision")
	}
	// The exact interactive ceremony still succeeds and is consumed.
	if _, err := h.decide(gateDigest, "approve", gate.Alternatives[0], "DECIDE-APPROVE "+gateDigest+" ALTERNATIVE "+gate.Alternatives[0]); err != nil {
		t.Fatalf("the genuine ceremony was refused: %v", err)
	}
	result, err := grepo.ReconcileAuthorityGate(h.ctx, h.successor, h.successor.WorkPlan.Candidates[1], []contracts.GovernedArtifactEvidence{h.dossier}, time.Now().UTC())
	if err != nil || !result.Approved {
		t.Fatalf("the genuine ceremony decision was not consumed: %+v %v", result, err)
	}
}

// B9 / Path C: revoking the governing acceptance leaves history readable and
// refuses every later safety-bearing consequence.
func TestKernelRepair2B9RevokedGoverningAuthorityStopsFutureExecution(t *testing.T) {
	h := newGateHarness(t)
	repo, db, err := openGovernedRepository(h.ctx, h.env)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	plan := h.successor.WorkPlan
	now := time.Now().UTC()
	if err := repo.VerifyGoverningAuthority(h.ctx, h.successor, now); err != nil {
		t.Fatalf("a valid accepted plan lost its authority: %v", err)
	}
	h.pendingGate(h.ledger, "before-revocation") // execution eligible

	evidence, err := repo.LoadAuthorityDecisionEvidence(h.ctx, plan.AuthorityRequestID, plan.AuthorityRequestVersion, now)
	if err != nil {
		t.Fatal(err)
	}
	digest, _ := evidence.Digest()
	revocation := contracts.AuthorityRevocation{RequestID: evidence.RequestID, RequestVersion: evidence.RequestVersion, DecisionRef: evidence.DecisionRef, DecisionVersion: evidence.DecisionVersion, DecisionDigest: digest, RevocationRef: "review2-revoke", RevocationVersion: "1", RevokedBy: h.root.Principal, AuthorityDigest: h.root.AuthorityModelDigest, EffectiveAt: now, Reason: "path C"}
	if err := repo.SaveAuthorityRevocation(h.ctx, evidence.RequestID, evidence.RequestVersion, revocation, now, nil); err != nil {
		t.Fatal(err)
	}
	later := time.Now().UTC().Add(time.Second)
	// History stays inspectable; effective authority does not.
	if again, err := repo.LoadAuthorityDecisionEvidence(h.ctx, evidence.RequestID, evidence.RequestVersion, later); err != nil || again.DecisionRef != evidence.DecisionRef {
		t.Fatalf("revocation erased historical evidence: %v", err)
	}
	if _, err := repo.LoadAuthorityDecision(h.ctx, evidence.RequestID, evidence.RequestVersion, later); !errors.Is(err, goalstore.ErrAuthorityDecisionRevoked) {
		t.Fatalf("the revoked decision is still effective: %v", err)
	}
	if err := repo.VerifyGoverningAuthority(h.ctx, h.successor, later); err == nil {
		t.Fatal("a revoked plan still verifies as governing authority")
	}
	// Every route refuses: gate reconciliation, the controller's selection
	// path on a fresh reload, and a continuous subsequent turn.
	reloaded, err := repo.Load(h.ctx, repairGoalID, "2", later)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ReconcileAuthorityGate(h.ctx, reloaded, reloaded.WorkPlan.Candidates[1], []contracts.GovernedArtifactEvidence{h.dossier}, later); err == nil {
		t.Fatal("gate reconciliation proceeded under revoked authority")
	}
	h.successor = reloaded
	if required, err := h.drive(h.ledger, "after-revocation"); required != nil || !errors.Is(err, goaldrive.ErrPlanAuthority) {
		t.Fatalf("the controller executed under revoked authority: required=%v err=%v", required, err)
	}
	if required, err := h.drive(h.ledger, "after-revocation-2"); required != nil || !errors.Is(err, goaldrive.ErrPlanAuthority) {
		t.Fatalf("a subsequent turn executed under revoked authority: required=%v err=%v", required, err)
	}
}

// B12 / Path D: package, manifest, or profile drift after a protected
// operation succeeded refuses the very next protected transition, with the
// verifier resolving current state each time rather than a startup snapshot.
func TestKernelRepair2B12DriveActivationIsResolvedAtEveryBoundary(t *testing.T) {
	s := newSafetyLifecycle(t)
	drive := newGoalDriveController(goaldrive.Ledger{}, nil, nil, goalstore.Repository{}, 0, s.env)
	binding := *s.proposal.Safety
	if err := drive.SafetyActivation.Verify(s.ctx, binding); err != nil {
		t.Fatalf("valid activation was refused: %v", err)
	}
	_, db, err := openGovernedRepository(s.ctx, s.env)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ExecContext(s.ctx, "UPDATE installed_packages SET state='disabled' WHERE state='active'"); err != nil {
		t.Fatal(err)
	}
	if err := drive.SafetyActivation.Verify(s.ctx, binding); err == nil {
		t.Fatal("drive verification used a stale package snapshot after the package was disabled")
	}
	if err := verifyLifecycleSafety(binding, s.env); err == nil {
		t.Fatal("lifecycle verification missed the package change")
	}
	if _, err := db.ExecContext(s.ctx, "UPDATE installed_packages SET state='active' WHERE state='disabled'"); err != nil {
		t.Fatal(err)
	}
	if err := drive.SafetyActivation.Verify(s.ctx, binding); err != nil {
		t.Fatalf("restored package should verify again: %v", err)
	}
	s.removeActivation()
	if err := drive.SafetyActivation.Verify(s.ctx, binding); err == nil {
		t.Fatal("drive verification survived a missing manifest")
	}
	s.restoreActivation()
	s.imageErr = errors.New("running image does not match the manifest")
	if err := drive.SafetyActivation.Verify(s.ctx, binding); err == nil {
		t.Fatal("drive verification survived a core-image mismatch")
	}
	wrongProfile := binding
	wrongProfile.ValidationProfileDigest = "sha256:" + strings.Repeat("9", 64)
	s.imageErr = nil
	if err := drive.SafetyActivation.Verify(s.ctx, wrongProfile); err == nil {
		t.Fatal("drive verification accepted a different validation profile")
	}
}

// Path D end to end: drift between two protected turns refuses the second and
// leaves no partial durable mutation.
func TestKernelRepair2PathDActivationDriftRefusesTheNextProtectedTransition(t *testing.T) {
	h := newGateHarness(t)
	h.pendingGate(h.ledger, "turn-1")
	before, _ := h.ledger.LoadCompletions(h.ctx, repairGoalID, "2")
	h.removeActivation()
	if _, err := h.drive(h.ledger, "turn-2"); err == nil || !errors.Is(err, goaldrive.ErrSafetyActivation) {
		t.Fatalf("the next protected transition ran after activation drift: %v", err)
	}
	after, _ := h.ledger.LoadCompletions(h.ctx, repairGoalID, "2")
	if len(after) != len(before) {
		t.Fatal("a refused transition left partial durable state")
	}
	h.restoreActivation()
	if _, err := h.drive(h.ledger, "turn-3"); err != nil && !strings.Contains(err.Error(), "authority required") {
		t.Fatalf("restored activation should resume: %v", err)
	}
}

// Path B tail: after the owner-approved gate completes, a fresh reload derives
// exactly one completion per unit from the ledger and the plan is structurally
// complete; nothing about the accepted plan itself carried completion.
func TestKernelRepair2PathBGateCompletionSurvivesReloadAndDerivesFromTheLedgerOnly(t *testing.T) {
	h := newGateHarness(t)
	request := h.pendingGate(h.ledger, "turn-1")
	digest, _ := request.Digest()
	if _, err := h.decide(digest, "approve", "finish and reconcile", "DECIDE-APPROVE "+digest+" ALTERNATIVE finish and reconcile"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.drive(h.ledger, "turn-2"); !errors.Is(err, goaldrive.ErrHumanAuthorityGate) {
		t.Fatalf("approved gate must record completion and stop: %v", err)
	}
	repo, db, err := openGovernedRepository(h.ctx, h.env)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	reloaded, err := repo.Load(h.ctx, repairGoalID, "2", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range reloaded.WorkPlan.Candidates {
		if candidate.Completed {
			t.Fatalf("the persisted plan carries execution state for %q", candidate.ID)
		}
	}
	completions, err := h.ledger.LoadCompletions(h.ctx, repairGoalID, "2")
	if err != nil {
		t.Fatal(err)
	}
	if err := goaldrive.VerifyPlanCompletions(reloaded.WorkPlan, completions); err != nil {
		t.Fatal(err)
	}
	assessment, err := goaldrive.AssessGoalCompletion(reloaded, completions)
	if err != nil || !assessment.AllUnitsComplete {
		t.Fatalf("gate completion did not make downstream state eligible: %+v %v", assessment, err)
	}
	if _, err := h.drive(h.ledger, "turn-3"); err == nil {
		t.Fatal("a fully complete plan selected further work")
	}
	_ = context.Background
}

// B15 (found by invariant-guided search, adjacent to the second review's
// "read-only replay" observation): replaying an existing gate request is a
// protected read of safety state, so activation is enforced by the shared gate
// boundary itself and not left to whichever controller happens to call it.
func TestKernelRepair2B15GateReplayRequiresActivationAtTheSharedBoundary(t *testing.T) {
	h := newGateHarness(t)
	h.pendingGate(h.ledger, "created")
	repo, db, err := openGovernedRepository(h.ctx, h.env)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	candidate := h.successor.WorkPlan.Candidates[1]
	if _, err := repo.ReconcileAuthorityGate(h.ctx, h.successor, candidate, []contracts.GovernedArtifactEvidence{h.dossier}, time.Now().UTC()); err != nil {
		t.Fatalf("replay under valid activation: %v", err)
	}
	h.removeActivation()
	if _, err := repo.ReconcileAuthorityGate(h.ctx, h.successor, candidate, []contracts.GovernedArtifactEvidence{h.dossier}, time.Now().UTC()); !errors.Is(err, goalstore.ErrSafetyActivationRequired) {
		t.Fatalf("gate replay proceeded without current activation: %v", err)
	}
	unconfigured := repo
	unconfigured.SafetyActivation = nil
	h.restoreActivation()
	if _, err := unconfigured.ReconcileAuthorityGate(h.ctx, h.successor, candidate, []contracts.GovernedArtifactEvidence{h.dossier}, time.Now().UTC()); !errors.Is(err, goalstore.ErrSafetyActivationRequired) {
		t.Fatalf("gate replay proceeded with no activation verifier: %v", err)
	}
}

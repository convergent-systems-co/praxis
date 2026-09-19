package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/goaldrive"
	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/packages/goals"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

const intakeSurfaceDocument = `# Goal: Scratch intake

Bring the scratch service to intake readiness by resolving the defects that
most impede governed operation.

## Non-goals

- No change outside the scratch repository.

## Success criteria

- go test ./... passes on a clean tree.
`

// TestGoalsLifecycleIntakeAdmitsAProseGoalWithoutConferringAuthority is the
// end-to-end predicate for the new-project path (#103): a prose Goal document
// enters through goals-lifecycle intake, crosses the same import boundary as a
// canonical baseline, and confers no authority. Only the existing
// propose/review/request/decide/accept/attach topology can make it drivable.
func TestGoalsLifecycleIntakeAdmitsAProseGoalWithoutConferringAuthority(t *testing.T) {
	ctx := context.Background()
	governed, root := governedInstallationFixture(t, ctx)
	db, err := state.OpenSQLite(ctx, governed("PRAXIS_DB"))
	if err != nil {
		t.Fatal(err)
	}
	input, err := goals.PackageBuildInput([]byte("fixture-goals-plugin-executable"))
	if err != nil {
		t.Fatal(err)
	}
	built, err := packagecatalog.BuildPackage(input)
	if err != nil {
		t.Fatal(err)
	}
	activateGoalsPackageFixture(t, ctx, db, built.Manifest, built.ArtifactBytes, governed, time.Date(2026, 9, 18, 20, 0, 0, 0, time.UTC))
	db.Close()
	dir := t.TempDir()
	document := filepath.Join(dir, "goal.md")
	if err := os.WriteFile(document, []byte(intakeSurfaceDocument), 0o600); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) error {
		return runDynamicInvocation(ctx, append([]string{"goals-lifecycle"}, args...), governed)
	}

	for name, args := range map[string][]string{
		"no goal id":   {"--operation=intake", "--input=" + document},
		"no document":  {"--operation=intake", "--goal-id=goal:scratch-intake"},
		"missing file": {"--operation=intake", "--goal-id=goal:scratch-intake", "--input=" + filepath.Join(dir, "absent.md")},
	} {
		if err := run(args...); err == nil {
			t.Fatalf("intake %s must be refused", name)
		}
	}
	empty := filepath.Join(dir, "empty.md")
	if err := os.WriteFile(empty, []byte("# Only a title\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run("--operation=intake", "--goal-id=goal:scratch-intake", "--input="+empty); err == nil {
		t.Fatal("a document with no intent text must be refused")
	}

	if err := run("--operation=intake", "--goal-id=goal:scratch-intake", "--input="+document); err != nil {
		t.Fatalf("intake of a prose Goal failed: %v", err)
	}
	if err := run("--operation=intake", "--goal-id=goal:scratch-intake", "--input="+document); err != nil {
		t.Fatalf("exact repeat of an intake must be idempotent: %v", err)
	}
	changed := filepath.Join(dir, "changed.md")
	if err := os.WriteFile(changed, []byte(intakeSurfaceDocument+"\nA different Goal.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run("--operation=intake", "--goal-id=goal:scratch-intake", "--input="+changed); err == nil {
		t.Fatal("different prose under the same generation must fail closed")
	}

	repo, gdb, err := openGovernedRepository(ctx, governed)
	if err != nil {
		t.Fatal(err)
	}
	defer gdb.Close()
	baseline, err := repo.Load(ctx, "goal:scratch-intake", "1", time.Now().UTC())
	if err != nil {
		t.Fatalf("the admitted baseline must load: %v", err)
	}
	if baseline.WorkPlan != nil || baseline.PredecessorDigest != "" {
		t.Fatalf("intake must admit a root generation without a WorkPlan: %+v", baseline)
	}
	if baseline.ImportSourceRef != document || baseline.ImportSourceDigest == "" {
		t.Fatalf("intake must record source provenance at the import boundary: %q %q", baseline.ImportSourceRef, baseline.ImportSourceDigest)
	}
	if baseline.OriginalIntent != intakeSurfaceDocument || len(baseline.NonGoals) != 1 || len(baseline.SuccessCriteria) != 1 {
		t.Fatalf("derived fields missing: %+v", baseline)
	}
	now := time.Now().UTC()
	if pending, err := repo.PendingAuthorityRequests(ctx, baseline.ID, baseline.Version, now); err != nil || len(pending) != 0 {
		t.Fatalf("intake must not create authority requests: %v %v", pending, err)
	}
	if proposals, _, err := repo.ListWorkPlanProposals(ctx, baseline.ID, baseline.Version, now); err != nil || len(proposals) != 0 {
		t.Fatalf("intake must not create proposals: %v %v", proposals, err)
	}
	if accepted, err := repo.ListAcceptedWorkPlans(ctx, baseline.Digest, now); err != nil || len(accepted) != 0 {
		t.Fatalf("intake must not accept a WorkPlan: %v %v", accepted, err)
	}

	// The unchanged topology carries the admitted Goal to a drivable generation.
	requirement := contracts.RequirementRef{ID: "req:intake", SourceRef: "goal:scratch-intake/requirement/1", SourceDigest: "sha256:" + strings.Repeat("1", 64)}
	candidate := contracts.WorkCandidate{ID: "unit:first", SourceRef: "issue:scratch/1", SourceDigest: "sha256:" + strings.Repeat("2", 64), Provenance: contracts.ProvenanceIssue, Requirements: []contracts.RequirementRef{requirement}}
	proposal := contracts.WorkPlanProposal{ID: "proposal:scratch-intake", ProposedBy: contracts.PrincipalRef{ID: "planner:scratch", Kind: "model"}, ProposerGeneration: "planner-generation-1", Candidates: []contracts.WorkCandidate{candidate}}
	if err := run("--operation=propose", "--input="+writeLifecycleInput(t, dir, "propose.json", map[string]any{"BaselineID": baseline.ID, "BaselineVersion": baseline.Version, "proposal": proposal})); err != nil {
		t.Fatalf("propose on the admitted Goal: %v", err)
	}
	full, err := goals.BuildWorkPlanProposal(baseline, proposal.ID, proposal.ProposedBy, proposal.ProposerGeneration, proposal.Candidates, nil)
	if err != nil {
		t.Fatal(err)
	}
	proposalDigest, err := full.Digest()
	if err != nil {
		t.Fatal(err)
	}
	review := contracts.WorkPlanProposalReview{ProposalDigest: proposalDigest, BaselineDigest: baseline.Digest, ReviewRef: "review:scratch-intake", ReviewDigest: "sha256:" + strings.Repeat("3", 64), ReviewedBy: contracts.PrincipalRef{ID: "reviewer:scratch", Kind: "agent"}, ReviewerGeneration: "reviewer-generation-1", Status: contracts.ReviewAcceptableForAuthority, CoveredRequirements: []string{requirement.ID}}
	if err := run("--operation=review", "--input="+writeLifecycleInput(t, dir, "review.json", map[string]any{"ProposalID": proposal.ID, "ProposalVersion": "1", "ReviewVersion": "1", "review": review})); err != nil {
		t.Fatalf("review: %v", err)
	}
	request := contracts.AuthorityRequest{ID: "workplan-accept:scratch-intake", Version: "1", BaselineID: baseline.ID, BaselineVersion: baseline.Version, BaselineDigest: baseline.Digest, ProposalID: proposal.ID, ProposalVersion: "1", ProposalDigest: proposalDigest, ReviewRef: review.ReviewRef, ReviewVersion: "1", ReviewDigest: review.ReviewDigest, RequestedAuthority: "workplan.accept", RequestedScope: root.Scope, Reason: "accept the WorkPlan for the intake-admitted Goal", Status: contracts.AuthorityRequestPending}
	if err := run("--operation=request", "--input="+writeLifecycleInput(t, dir, "request.json", request)); err != nil {
		t.Fatalf("request: %v", err)
	}
	// Without an owner decision the plan cannot be accepted, so the Goal stays undrivable.
	plan := contracts.WorkPlan{Candidates: []contracts.WorkCandidate{candidate}}
	acceptInput := writeLifecycleInput(t, dir, "accept.json", map[string]any{"RequestID": request.ID, "RequestVersion": "1", "AcceptanceRef": "acceptance:scratch-intake", "AcceptanceVersion": "1", "plan": plan})
	if err := run("--operation=accept", "--input="+acceptInput); err == nil {
		t.Fatal("accept without an owner decision must be refused: intake confers no authority")
	}
	requestDigest, err := request.Digest()
	if err != nil {
		t.Fatal(err)
	}
	decision := contracts.AuthorityDecision{RequestID: request.ID, RequestVersion: "1", RequestDigest: requestDigest, DecisionRef: "decision:scratch-intake", DecisionVersion: "1", DecidedBy: root.Principal, AuthorityRef: root.Ref, AuthorityVersion: root.Version, AuthorityGenerationDigest: root.Digest, GrantedScope: root.Scope, Outcome: contracts.AuthorityApprove, AuthorityDigest: root.AuthorityModelDigest, IssuedAt: time.Now().UTC()}
	if err := run("--operation=decide", "--input="+writeLifecycleInput(t, dir, "decide.json", map[string]any{"RequestID": request.ID, "RequestVersion": "1", "decision": decision})); err != nil {
		t.Fatalf("decide: %v", err)
	}
	if err := run("--operation=accept", "--input="+acceptInput); err != nil {
		t.Fatalf("accept after the owner decision: %v", err)
	}
	if err := run("--operation=attach", "--input="+writeLifecycleInput(t, dir, "attach.json", map[string]any{"GoalID": baseline.ID, "GoalVersion": "1", "BaselineDigest": baseline.Digest, "AcceptanceRef": "acceptance:scratch-intake", "AcceptanceVersion": "1", "SuccessorVersion": "2"})); err != nil {
		t.Fatalf("attach: %v", err)
	}
	err = runDynamicInvocation(ctx, []string{"goal-drive", "--goal-id=" + baseline.ID, "--goal-version=2", "--provider=local", "--invocation-id=inv-1", "--repo=" + dir, "--branch=main"}, governed)
	if err == nil || !strings.Contains(err.Error(), "construct native Goal-drive runtime") {
		t.Fatalf("goal-drive on the attached generation must reach runtime construction: %v", err)
	}
}

// TestGoalDriveRefusesProseGoalInputAndNamesTheIntakePath is the #103 contract
// predicate: goal-drive operates only on an existing durable Goal identity and
// never creates or accepts a Goal from prose. A caller who supplies prose gets
// the path, not a bare refusal.
func TestGoalDriveRefusesProseGoalInputAndNamesTheIntakePath(t *testing.T) {
	document := filepath.Join(t.TempDir(), "goal.md")
	if err := os.WriteFile(document, []byte(intakeSurfaceDocument), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, options := range []map[string]string{
		{"goal-file": document, "provider": "local", "invocation-id": "inv-1"},
		{"goal": "finish the MVP", "provider": "local", "invocation-id": "inv-1"},
	} {
		_, err := goaldrive.ParseInvocation(options)
		if err == nil || !strings.Contains(err.Error(), "goals-lifecycle") || !strings.Contains(err.Error(), "intake") {
			t.Fatalf("prose input must be refused with the intake path named: %v", err)
		}
	}
}

package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func continueFixture(t *testing.T, ctx context.Context, governed func(string) string, goalID, version string) map[string]any {
	t.Helper()
	out := captureStdout(t, func() {
		if err := runDynamicInvocation(ctx, []string{"goals-lifecycle", "--operation=continue", "--goal-id=" + goalID, "--goal-version=" + version}, governed); err != nil {
			t.Fatalf("continue %s/%s: %v", goalID, version, err)
		}
	})
	var result map[string]any
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("decode continuation: %v %s", err, out)
	}
	return result
}

// TestGovernedPlanningContinuationStopsOnlyAtJudgment proves the lifecycle
// distinction required by continuous Goal drive: planner/reviewer/owner
// judgment remain boundaries, while every state-derived transition after an
// acceptable review and an approval is performed without another decision.
func TestGovernedPlanningContinuationStopsOnlyAtJudgment(t *testing.T) {
	ctx := context.Background()
	governed, _, _, dir := lifecycleFixture(t, ctx)
	const goalID = "goal:pending-surface"

	inspected := inspectGoal(t, ctx, governed, goalID, "1")
	if got := inspected["continue_with"]; got != continueCommand(goalID, "1") {
		t.Fatalf("inspect must expose deterministic continuation as an executable next transition: %v", got)
	}
	if result := continueFixture(t, ctx, governed, goalID, "1"); result["status"] != "planning_required" {
		t.Fatalf("fresh Goal must stop at the planner judgment boundary: %v", result)
	}
	requirement := contracts.RequirementRef{ID: "req:governed", SourceRef: goalID + "/success_criteria/1", SourceDigest: "sha256:" + strings.Repeat("1", 64)}
	proposal := contracts.WorkPlanProposal{
		ID: "proposal:governed", ProposedBy: contracts.PrincipalRef{ID: "planner:model", Kind: "model"}, ProposerGeneration: "planner-generation-1",
		Candidates: []contracts.WorkCandidate{{ID: "unit:governed", Priority: 1, Sequence: 1, SourceRef: "model:proposal", SourceDigest: "sha256:" + strings.Repeat("2", 64), Provenance: contracts.ProvenanceModelProposal, Requirements: []contracts.RequirementRef{requirement}}},
	}
	proposeOut := captureStdout(t, func() {
		if err := runDynamicInvocation(ctx, []string{"goals-lifecycle", "--operation=propose", "--input=" + writeLifecycleInput(t, dir, "governed-proposal.json", map[string]any{"BaselineID": goalID, "BaselineVersion": "1", "proposal": proposal})}, governed); err != nil {
			t.Fatal(err)
		}
	})
	var proposed map[string]any
	if err := json.Unmarshal(proposeOut, &proposed); err != nil {
		t.Fatal(err)
	}
	proposalDigest := proposed["proposal_digest"].(string)
	if result := continueFixture(t, ctx, governed, goalID, "1"); result["status"] != "independent_review_required" {
		t.Fatalf("proposal must stop for independent judgment: %v", result)
	}
	reviewBySelector(t, ctx, governed, dir, proposalDigest, string(contracts.ReviewAcceptableForAuthority))

	continued := continueFixture(t, ctx, governed, goalID, "1")
	if continued["status"] != "owner_authority_required" {
		t.Fatalf("acceptable review must deterministically create the owner boundary: %v", continued)
	}
	requests := continued["authority_requests"].([]any)
	requestDigest := requests[0].(map[string]any)["request_digest"].(string)
	// Re-entry before the owner acts reconstructs the same boundary and does
	// not create another request.
	replayed := continueFixture(t, ctx, governed, goalID, "1")
	if got := replayed["authority_requests"].([]any)[0].(map[string]any)["request_digest"]; got != requestDigest {
		t.Fatalf("continuation changed the pending authority identity: %s != %s", got, requestDigest)
	}
	if _, err := decideInteractive(t, governed, requestDigest, "approve", "DECIDE-APPROVE "+requestDigest); err != nil {
		t.Fatal(err)
	}

	continued = continueFixture(t, ctx, governed, goalID, "1")
	if continued["status"] != "drivable" || continued["goal_version"] != "2" || continued["next_admissible_transition"] != "goal-drive" {
		t.Fatalf("approval must deterministically accept and attach: %v", continued)
	}
	repo, db, err := openGovernedRepository(ctx, governed)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	baseline, err := repo.Load(ctx, goalID, "2", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if baseline.WorkPlan == nil || baseline.WorkPlan.Candidates[0].Provenance != contracts.ProvenancePLAN || baseline.WorkPlan.Candidates[0].SourceDigest != requestDigest {
		t.Fatalf("owner approval did not promote advisory planning into exact authoritative work: %+v", baseline.WorkPlan)
	}
	if replay := continueFixture(t, ctx, governed, goalID, "1"); replay["replay"] != true || replay["goal_version"] != "2" {
		t.Fatalf("restart must reconstruct the existing deterministic consequence: %v", replay)
	}
}

func TestGovernedPlanningContinuationPreservesRejectAndAmbiguityBoundaries(t *testing.T) {
	ctx := context.Background()
	governed, _, _, dir := lifecycleFixture(t, ctx)
	const goalID = "goal:pending-surface"
	first := proposeFixture(t, ctx, governed, dir, "first-continuation")
	firstReview := reviewBySelector(t, ctx, governed, dir, first, string(contracts.ReviewAcceptableForAuthority))
	second := proposeFixture(t, ctx, governed, dir, "second-continuation")
	reviewBySelector(t, ctx, governed, dir, second, string(contracts.ReviewAcceptableForAuthority))

	// Two independently acceptable alternatives are not an ordering problem;
	// choosing between them is judgment, so continuation writes nothing.
	repo, db, err := openGovernedRepository(ctx, governed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := advanceGoalsLifecycle(ctx, repo, goalID, "1", governed, time.Now().UTC()); err == nil || !strings.Contains(err.Error(), "2 independently acceptable") {
		db.Close()
		t.Fatalf("ambiguous reviewed proposals must fail closed: %v", err)
	}
	pending, err := repo.PendingAuthorityRequests(ctx, goalID, "1", time.Now().UTC())
	db.Close()
	if err != nil || len(pending) != 0 {
		t.Fatalf("ambiguous continuation created authority: %+v %v", pending, err)
	}

	// Create an exact request for the first proposal through the public
	// selector, reject it, and prove continuation does not silently re-request
	// the rejected disposition. The second proposal remains a genuine choice.
	requestDigest, err := requestBySelector(t, ctx, governed, dir, first, firstReview)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decideInteractive(t, governed, requestDigest, "reject", "DECIDE-REJECT "+requestDigest); err != nil {
		t.Fatal(err)
	}
	result := continueFixture(t, ctx, governed, goalID, "1")
	if result["status"] != "owner_authority_required" {
		t.Fatalf("the remaining independently reviewed alternative should receive its own owner boundary: %v", result)
	}
	entries := result["authority_requests"].([]any)
	if entries[0].(map[string]any)["request_digest"] == requestDigest {
		t.Fatalf("rejected authority was retried: %v", result)
	}
}

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/packages/goals"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func lifecycleFixture(t *testing.T, ctx context.Context) (func(string) string, contracts.AuthorityGeneration, string, string) {
	t.Helper()
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
	baseline := goals.GoalBaseline{ID: "goal:pending-surface", Version: "1", OriginalIntent: "Prove pending authority is actionable", RefinedOutcome: "The owner resolves pending authority from the product surface", Rigor: goals.RigorDirect, RecommendationMode: goals.RecommendationReviewAll}
	if err := runDynamicInvocation(ctx, []string{"goals-lifecycle", "--operation=import", "--input=" + writeBaselineImportDocument(t, dir, "baseline.json", baseline, "")}, governed); err != nil {
		t.Fatal(err)
	}
	digest, _ := baseline.ComputeDigest()
	return governed, root, digest, dir
}

func proposeFixture(t *testing.T, ctx context.Context, governed func(string) string, dir, id string) string {
	t.Helper()
	requirement := contracts.RequirementRef{ID: "req:" + id, SourceRef: "goal:pending-surface/requirement/" + id, SourceDigest: "sha256:" + strings.Repeat("1", 64)}
	candidate := contracts.WorkCandidate{ID: "unit:" + id, SourceRef: "issue:" + id, SourceDigest: "sha256:" + strings.Repeat("2", 64), Provenance: contracts.ProvenanceIssue, Requirements: []contracts.RequirementRef{requirement}}
	proposal := contracts.WorkPlanProposal{ID: "proposal:" + id, ProposedBy: contracts.PrincipalRef{ID: "planner:model", Kind: "model"}, ProposerGeneration: "planner-generation-1", Candidates: []contracts.WorkCandidate{candidate}}
	out := captureStdout(t, func() {
		if err := runDynamicInvocation(ctx, []string{"goals-lifecycle", "--operation=propose", "--input=" + writeLifecycleInput(t, dir, "propose-"+id+".json", map[string]any{"BaselineID": "goal:pending-surface", "BaselineVersion": "1", "proposal": proposal})}, governed); err != nil {
			t.Fatalf("propose: %v", err)
		}
	})
	var result struct {
		RecordDigest string `json:"record_digest"`
	}
	if err := json.Unmarshal(out, &result); err != nil || result.RecordDigest == "" {
		t.Fatalf("proposal digest missing: %v %s", err, out)
	}
	return result.RecordDigest
}

func reviewBySelector(t *testing.T, ctx context.Context, governed func(string) string, dir, proposalDigest, status string) string {
	t.Helper()
	out := captureStdout(t, func() {
		if err := runDynamicInvocation(ctx, []string{"goals-lifecycle", "--operation=review", "--input=" + writeLifecycleInput(t, dir, "review-"+proposalDigest[7:15]+".json", map[string]any{"proposal_digest": proposalDigest, "status": status, "reviewed_by": map[string]string{"id": "reviewer:agent", "kind": "agent"}, "reviewer_generation": "reviewer-generation-1"})}, governed); err != nil {
			t.Fatalf("review: %v", err)
		}
	})
	var result struct {
		ReviewDigest string `json:"review_digest"`
	}
	if err := json.Unmarshal(out, &result); err != nil || result.ReviewDigest == "" {
		t.Fatalf("review digest missing: %v %s", err, out)
	}
	return result.ReviewDigest
}

func requestBySelector(t *testing.T, ctx context.Context, governed func(string) string, dir, proposalDigest, reviewDigest string) (string, error) {
	t.Helper()
	var result struct {
		RequestDigest string `json:"request_digest"`
	}
	var runErr error
	out := captureStdout(t, func() {
		runErr = runDynamicInvocation(ctx, []string{"goals-lifecycle", "--operation=request", "--goal-id=goal:pending-surface", "--goal-version=1", "--input=" + writeLifecycleInput(t, dir, "request-"+proposalDigest[7:15]+".json", map[string]any{"proposal_digest": proposalDigest, "review_digest": reviewDigest})}, governed)
	})
	if runErr != nil {
		return "", runErr
	}
	if err := json.Unmarshal(out, &result); err != nil || result.RequestDigest == "" {
		t.Fatalf("request digest missing: %v %s", err, out)
	}
	return result.RequestDigest, nil
}

func decideInteractive(t *testing.T, governed func(string) string, requestDigest, outcome, typed string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := runAuthorityDecideWithTerminal([]string{"--request", requestDigest, "--outcome", outcome}, governed, strings.NewReader(typed+"\n"), &out, true)
	return out.String(), err
}

// TestPendingAuthorityIsActionableFromTheProductSurface walks the lifecycle
// from a high-level Goal to a drivable generation using only the public
// operations and selector inputs: no internal governance document is
// authored by hand, and the owner resolves the pending decision with the
// generic authority surface.
func TestPendingAuthorityIsActionableFromTheProductSurface(t *testing.T) {
	ctx := context.Background()
	governed, root, baselineDigest, dir := lifecycleFixture(t, ctx)
	proposalDigest := proposeFixture(t, ctx, governed, dir, "one")
	reviewDigest := reviewBySelector(t, ctx, governed, dir, proposalDigest, string(contracts.ReviewAcceptableForAuthority))
	requestDigest, err := requestBySelector(t, ctx, governed, dir, proposalDigest, reviewDigest)
	if err != nil {
		t.Fatal(err)
	}
	// inspect and pending both surface the exact identity to act on.
	inspect := captureStdout(t, func() {
		if err := runDynamicInvocation(ctx, []string{"goals-lifecycle", "--operation=inspect", "--goal-id=goal:pending-surface", "--goal-version=1"}, governed); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(string(inspect), requestDigest) || !strings.Contains(string(inspect), "praxis authority decide --request "+requestDigest) || !strings.Contains(string(inspect), `"drivable": false`) {
		t.Fatalf("inspect must expose the pending request and how to resolve it: %s", inspect)
	}
	var pendingOut bytes.Buffer
	if err := runAuthorityPending(nil, governed, &pendingOut); err != nil || !strings.Contains(pendingOut.String(), requestDigest) || !strings.Contains(pendingOut.String(), `"status": "pending"`) {
		t.Fatalf("authority pending must list the request: %v %s", err, pendingOut.String())
	}
	// Non-interactive, unknown digest, wrong confirmation, and delegation
	// requests are all refused; nothing is written.
	if err := runAuthorityDecideWithTerminal([]string{"--request", requestDigest, "--outcome", "approve"}, governed, strings.NewReader("DECIDE-APPROVE "+requestDigest+"\n"), &bytes.Buffer{}, false); err == nil {
		t.Fatal("non-interactive decision must be refused")
	}
	if _, err := decideInteractive(t, governed, "sha256:"+strings.Repeat("0", 64), "approve", "DECIDE-APPROVE sha256:"+strings.Repeat("0", 64)); err == nil {
		t.Fatal("unknown request digest must fail closed")
	}
	if out, err := decideInteractive(t, governed, requestDigest, "approve", "DECIDE-REJECT "+requestDigest); err == nil || !strings.Contains(out, "DECIDE-APPROVE "+requestDigest) {
		t.Fatalf("mismatched confirmation must be refused: %v", err)
	}
	if _, err := decideInteractive(t, governed, requestDigest, "maybe", "x"); err == nil {
		t.Fatal("outcome must be approve or reject")
	}
	pendingOut.Reset()
	if err := runAuthorityPending(nil, governed, &pendingOut); err != nil || !strings.Contains(pendingOut.String(), `"status": "pending"`) {
		t.Fatalf("refused attempts must not change disposition: %v %s", err, pendingOut.String())
	}
	// The owner approves with the exact digest and confirmation.
	out, err := decideInteractive(t, governed, requestDigest, "approve", "DECIDE-APPROVE "+requestDigest)
	if err != nil {
		t.Fatalf("approve: %v %s", err, out)
	}
	if !strings.Contains(out, `"outcome": "approve"`) || !strings.Contains(out, root.Digest) || !strings.Contains(out, requestDigest) {
		t.Fatalf("decision must bind the exact request and root: %s", out)
	}
	// Replay returns the recorded decision truthfully instead of a second write.
	replay, err := decideInteractive(t, governed, requestDigest, "reject", "DECIDE-REJECT "+requestDigest)
	if err != nil || !strings.Contains(replay, `"replay": true`) || !strings.Contains(replay, `"outcome": "approve"`) {
		t.Fatalf("replay must report the durable disposition: %v %s", err, replay)
	}
	pendingOut.Reset()
	if err := runAuthorityPending([]string{"--all"}, governed, &pendingOut); err != nil || !strings.Contains(pendingOut.String(), `"status": "decided:approve"`) {
		t.Fatalf("pending --all must show the decided disposition: %v %s", err, pendingOut.String())
	}
	// accept and attach derive everything from the decided request.
	acceptOut := captureStdout(t, func() {
		if err := runDynamicInvocation(ctx, []string{"goals-lifecycle", "--operation=accept", "--input=" + writeLifecycleInput(t, dir, "accept.json", map[string]any{"request_digest": requestDigest})}, governed); err != nil {
			t.Fatalf("accept: %v", err)
		}
	})
	var accepted struct {
		AcceptanceRef string `json:"acceptance_ref"`
	}
	if err := json.Unmarshal(acceptOut, &accepted); err != nil || accepted.AcceptanceRef == "" {
		t.Fatalf("acceptance ref missing: %v %s", err, acceptOut)
	}
	attachOut := captureStdout(t, func() {
		if err := runDynamicInvocation(ctx, []string{"goals-lifecycle", "--operation=attach", "--goal-id=goal:pending-surface", "--goal-version=1", "--input=" + writeLifecycleInput(t, dir, "attach.json", map[string]any{"acceptance_ref": accepted.AcceptanceRef})}, governed); err != nil {
			t.Fatalf("attach: %v", err)
		}
	})
	if !strings.Contains(string(attachOut), `"goal_version": "2"`) || !strings.Contains(string(attachOut), baselineDigest) {
		t.Fatalf("attach must create generation 2 from generation 1: %s", attachOut)
	}
	inspect = captureStdout(t, func() {
		if err := runDynamicInvocation(ctx, []string{"goals-lifecycle", "--operation=inspect", "--goal-id=goal:pending-surface", "--goal-version=2"}, governed); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(string(inspect), `"drivable": true`) {
		t.Fatalf("generation 2 must be drivable: %s", inspect)
	}
}

// TestPendingAuthorityRejectAndBindingFailures proves the reject path, that a
// review bound to a different proposal cannot be requested, that a request
// cannot be accepted without an approving decision, and that with several
// pending requests nothing acts on an implicit "latest".
func TestPendingAuthorityRejectAndBindingFailures(t *testing.T) {
	ctx := context.Background()
	governed, _, _, dir := lifecycleFixture(t, ctx)
	first := proposeFixture(t, ctx, governed, dir, "first")
	second := proposeFixture(t, ctx, governed, dir, "second")
	firstReview := reviewBySelector(t, ctx, governed, dir, first, string(contracts.ReviewAcceptableForAuthority))
	secondReview := reviewBySelector(t, ctx, governed, dir, second, string(contracts.ReviewAcceptableForAuthority))
	if _, err := requestBySelector(t, ctx, governed, dir, first, secondReview); err == nil {
		t.Fatal("a review of another proposal must not bind a request")
	}
	if _, err := requestBySelector(t, ctx, governed, dir, first, "sha256:"+strings.Repeat("f", 64)); err == nil {
		t.Fatal("an unknown review digest must fail closed")
	}
	revisionReview := reviewBySelector(t, ctx, governed, dir, second, string(contracts.ReviewRevisionRequired))
	if _, err := requestBySelector(t, ctx, governed, dir, second, revisionReview); err == nil {
		t.Fatal("a revision-required review must not reach authority")
	}
	firstRequest, err := requestBySelector(t, ctx, governed, dir, first, firstReview)
	if err != nil {
		t.Fatal(err)
	}
	secondRequest, err := requestBySelector(t, ctx, governed, dir, second, secondReview)
	if err != nil {
		t.Fatal(err)
	}
	var pendingOut bytes.Buffer
	if err := runAuthorityPending(nil, governed, &pendingOut); err != nil || !strings.Contains(pendingOut.String(), firstRequest) || !strings.Contains(pendingOut.String(), secondRequest) {
		t.Fatalf("both pending requests must be listed with exact digests: %v", err)
	}
	if err := runAuthorityDecideWithTerminal([]string{"--outcome", "approve"}, governed, strings.NewReader("x\n"), &bytes.Buffer{}, true); err == nil {
		t.Fatal("decide without an exact request digest must be refused")
	}
	// accept before any decision fails closed.
	if err := runDynamicInvocation(ctx, []string{"goals-lifecycle", "--operation=accept", "--input=" + writeLifecycleInput(t, dir, "accept-early.json", map[string]any{"request_digest": firstRequest})}, governed); err == nil {
		t.Fatal("acceptance without a decision must fail closed")
	}
	// Reject the second; accept of a rejected request fails closed.
	if out, err := decideInteractive(t, governed, secondRequest, "reject", "DECIDE-REJECT "+secondRequest); err != nil || !strings.Contains(out, `"outcome": "reject"`) {
		t.Fatalf("reject: %v %s", err, out)
	}
	if err := runDynamicInvocation(ctx, []string{"goals-lifecycle", "--operation=accept", "--input=" + writeLifecycleInput(t, dir, "accept-rejected.json", map[string]any{"request_digest": secondRequest})}, governed); err == nil {
		t.Fatal("a rejected request must not be accepted")
	}
	pendingOut.Reset()
	if err := runAuthorityPending(nil, governed, &pendingOut); err != nil || strings.Contains(pendingOut.String(), secondRequest) || !strings.Contains(pendingOut.String(), firstRequest) {
		t.Fatalf("only the undecided request remains pending: %v %s", err, pendingOut.String())
	}
	// Attach with an acceptance that does not exist fails closed.
	if err := runDynamicInvocation(ctx, []string{"goals-lifecycle", "--operation=attach", "--goal-id=goal:pending-surface", "--goal-version=1", "--input=" + writeLifecycleInput(t, dir, "attach-missing.json", map[string]any{"acceptance_ref": "acceptance:missing"})}, governed); err == nil {
		t.Fatal("attach with an unknown acceptance must fail closed")
	}
	_ = filepath.Join
}

// captureStdout runs fn with os.Stdout redirected and returns what it wrote;
// the lifecycle adapter prints its results to stdout exactly as the CLI does.
func captureStdout(t *testing.T, fn func()) []byte {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = writer
	done := make(chan []byte)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, reader)
		done <- buf.Bytes()
	}()
	func() {
		defer func() {
			os.Stdout = original
			writer.Close()
		}()
		fn()
	}()
	out := <-done
	reader.Close()
	return out
}

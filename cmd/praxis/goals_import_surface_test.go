package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

func writeBaselineImportDocument(t *testing.T, dir, name string, baseline goals.GoalBaseline, sourceRef string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if sourceRef == "" {
		sourceRef = path
	}
	canonical, err := baseline.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(canonical)
	digest := "sha256:" + hex.EncodeToString(sum[:])
	baseline.Digest = digest
	body, err := json.MarshalIndent(map[string]any{"schema_version": "1", "source_ref": sourceRef, "source_digest": digest, "baseline": baseline}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestGoalsLifecycleImportReachesTheImportBoundary proves the ADR-068 import
// boundary is reachable through the installed Goals package: a canonical
// baseline document imports through goals-lifecycle, inspect then resolves
// it, a document that does not bind its own path is refused, and goal-drive
// on the imported Goal proceeds to the worker dependency gate.
func TestGoalsLifecycleImportReachesTheImportBoundary(t *testing.T) {
	ctx := context.Background()
	governed, _ := governedInstallationFixture(t, ctx)
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
	baseline := goals.GoalBaseline{ID: "goal:scratch-weather", Version: "1", OriginalIntent: "Prove the external bootstrap path", RefinedOutcome: "One small Goal drives one real turn", Rigor: goals.RigorDirect, RecommendationMode: goals.RecommendationReviewAll}
	if err := runDynamicInvocation(ctx, []string{"goals-lifecycle", "--operation=import"}, governed); err == nil || !strings.Contains(err.Error(), "--input") {
		t.Fatalf("import without a document must be refused: %v", err)
	}
	wrong := writeBaselineImportDocument(t, dir, "wrong-ref.json", baseline, "/elsewhere/baseline.json")
	if err := runDynamicInvocation(ctx, []string{"goals-lifecycle", "--operation=import", "--input=" + wrong}, governed); err == nil || !strings.Contains(err.Error(), "source_ref") {
		t.Fatalf("document not binding its own path must be refused: %v", err)
	}
	path := writeBaselineImportDocument(t, dir, "baseline.json", baseline, "")
	if err := runDynamicInvocation(ctx, []string{"goals-lifecycle", "--operation=import", "--input=" + path}, governed); err != nil {
		t.Fatalf("canonical import through the package surface failed: %v", err)
	}
	if err := runDynamicInvocation(ctx, []string{"goals-lifecycle", "--operation=import", "--input=" + path}, governed); err != nil {
		t.Fatalf("exact duplicate import must be idempotent: %v", err)
	}
	if err := runDynamicInvocation(ctx, []string{"goals-lifecycle", "--operation=inspect", "--goal-id=goal:scratch-weather", "--goal-version=1"}, governed); err != nil {
		t.Fatalf("imported Goal must be inspectable: %v", err)
	}
	conflicting := baseline
	conflicting.RefinedOutcome = "A different outcome under the same generation"
	conflict := writeBaselineImportDocument(t, dir, "conflict.json", conflicting, "")
	if err := runDynamicInvocation(ctx, []string{"goals-lifecycle", "--operation=import", "--input=" + conflict}, governed); err == nil {
		t.Fatal("conflicting generation must be refused")
	}
	// goal-drive resolves through the installed package and reaches the
	// controller's runtime construction; the fixture bootstrap provider is
	// not a real key provider, so construction fails closed there.
	err = runDynamicInvocation(ctx, []string{"goal-drive", "--goal-id=goal:scratch-weather", "--goal-version=1", "--provider=local", "--invocation-id=inv-1", "--repo=" + dir, "--branch=main"}, governed)
	if err == nil || !strings.Contains(err.Error(), "construct native Goal-drive runtime") {
		t.Fatalf("goal-drive on the imported Goal must reach runtime construction: %v", err)
	}
}

func writeLifecycleInput(t *testing.T, dir, name string, value any) string {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestGoalsLifecycleGovernsAWorkPlanToADrivableGeneration walks the small
// Goal lifecycle entirely through the installed package: import, propose,
// independent review, authority request, owner decision bound to the
// installation root, authority-backed acceptance, and attach, which yields
// the successor generation whose embedded WorkPlan goal-drive materializes.
func TestGoalsLifecycleGovernsAWorkPlanToADrivableGeneration(t *testing.T) {
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
	baseline := goals.GoalBaseline{ID: "goal:scratch-weather", Version: "1", OriginalIntent: "Prove the external bootstrap path", RefinedOutcome: "One small Goal drives one real turn", Rigor: goals.RigorDirect, RecommendationMode: goals.RecommendationReviewAll}
	digest, err := baseline.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	if err := runDynamicInvocation(ctx, []string{"goals-lifecycle", "--operation=import", "--input=" + writeBaselineImportDocument(t, dir, "baseline.json", baseline, "")}, governed); err != nil {
		t.Fatal(err)
	}
	baseline.Digest = digest
	requirement := contracts.RequirementRef{ID: "req:progress-log", SourceRef: "goal:scratch-weather/requirement/1", SourceDigest: "sha256:" + strings.Repeat("1", 64)}
	candidate := contracts.WorkCandidate{ID: "unit:first-turn", SourceRef: "issue:scratch/1", SourceDigest: "sha256:" + strings.Repeat("2", 64), Provenance: contracts.ProvenanceIssue, Requirements: []contracts.RequirementRef{requirement}}
	proposal := contracts.WorkPlanProposal{ID: "proposal:scratch-weather", ProposedBy: contracts.PrincipalRef{ID: "planner:scratch", Kind: "model"}, ProposerGeneration: "planner-generation-1", Candidates: []contracts.WorkCandidate{candidate}}
	if err := runDynamicInvocation(ctx, []string{"goals-lifecycle", "--operation=propose", "--input=" + writeLifecycleInput(t, dir, "propose.json", map[string]any{"BaselineID": baseline.ID, "BaselineVersion": baseline.Version, "proposal": proposal})}, governed); err != nil {
		t.Fatalf("propose: %v", err)
	}
	full, err := goals.BuildWorkPlanProposal(baseline, proposal.ID, proposal.ProposedBy, proposal.ProposerGeneration, proposal.Candidates, nil)
	if err != nil {
		t.Fatal(err)
	}
	proposalDigest, err := full.Digest()
	if err != nil {
		t.Fatal(err)
	}
	review := contracts.WorkPlanProposalReview{ProposalDigest: proposalDigest, BaselineDigest: digest, ReviewRef: "review:scratch-weather", ReviewDigest: "sha256:" + strings.Repeat("3", 64), ReviewedBy: contracts.PrincipalRef{ID: "reviewer:scratch", Kind: "agent"}, ReviewerGeneration: "reviewer-generation-1", Status: contracts.ReviewAcceptableForAuthority, CoveredRequirements: []string{requirement.ID}}
	if err := runDynamicInvocation(ctx, []string{"goals-lifecycle", "--operation=review", "--input=" + writeLifecycleInput(t, dir, "review.json", map[string]any{"ProposalID": proposal.ID, "ProposalVersion": "1", "ReviewVersion": "1", "review": review})}, governed); err != nil {
		t.Fatalf("review: %v", err)
	}
	request := contracts.AuthorityRequest{ID: "workplan-accept:scratch-weather", Version: "1", BaselineID: baseline.ID, BaselineVersion: baseline.Version, BaselineDigest: digest, ProposalID: proposal.ID, ProposalVersion: "1", ProposalDigest: proposalDigest, ReviewRef: review.ReviewRef, ReviewVersion: "1", ReviewDigest: review.ReviewDigest, RequestedAuthority: "workplan.accept", RequestedScope: root.Scope, Reason: "accept the small scratch WorkPlan", Status: contracts.AuthorityRequestPending}
	if err := runDynamicInvocation(ctx, []string{"goals-lifecycle", "--operation=request", "--input=" + writeLifecycleInput(t, dir, "request.json", request)}, governed); err != nil {
		t.Fatalf("request: %v", err)
	}
	requestDigest, err := request.Digest()
	if err != nil {
		t.Fatal(err)
	}
	decision := contracts.AuthorityDecision{RequestID: request.ID, RequestVersion: "1", RequestDigest: requestDigest, DecisionRef: "decision:scratch-weather", DecisionVersion: "1", DecidedBy: root.Principal, AuthorityRef: root.Ref, AuthorityVersion: root.Version, AuthorityGenerationDigest: root.Digest, GrantedScope: root.Scope, Outcome: contracts.AuthorityApprove, AuthorityDigest: root.AuthorityModelDigest, IssuedAt: time.Now().UTC()}
	if err := runDynamicInvocation(ctx, []string{"goals-lifecycle", "--operation=decide", "--input=" + writeLifecycleInput(t, dir, "decide.json", map[string]any{"RequestID": request.ID, "RequestVersion": "1", "decision": decision})}, governed); err == nil {
		t.Fatal("non-interactive goals-lifecycle decide must not persist authority")
	}
	repository, decisionDB, err := openGovernedRepository(ctx, governed)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.SaveAuthorityDecision(ctx, request.ID, request.Version, decision, time.Now().UTC(), nil); err != nil {
		decisionDB.Close()
		t.Fatal(err)
	}
	decisionDB.Close()
	plan := contracts.WorkPlan{Candidates: []contracts.WorkCandidate{candidate}}
	if err := runDynamicInvocation(ctx, []string{"goals-lifecycle", "--operation=accept", "--input=" + writeLifecycleInput(t, dir, "accept.json", map[string]any{"RequestID": request.ID, "RequestVersion": "1", "AcceptanceRef": "acceptance:scratch-weather", "AcceptanceVersion": "1", "plan": plan})}, governed); err != nil {
		t.Fatalf("accept: %v", err)
	}
	if err := runDynamicInvocation(ctx, []string{"goals-lifecycle", "--operation=attach", "--input=" + writeLifecycleInput(t, dir, "attach.json", map[string]any{"GoalID": baseline.ID, "GoalVersion": "1", "BaselineDigest": digest, "AcceptanceRef": "acceptance:scratch-weather", "AcceptanceVersion": "1", "SuccessorVersion": "2"})}, governed); err != nil {
		t.Fatalf("attach: %v", err)
	}
	if err := runDynamicInvocation(ctx, []string{"goals-lifecycle", "--operation=inspect", "--goal-id=" + baseline.ID, "--goal-version=2"}, governed); err != nil {
		t.Fatalf("successor generation must be inspectable: %v", err)
	}
	if err := runDynamicInvocation(ctx, []string{"goals-lifecycle", "--operation=attach", "--input=" + writeLifecycleInput(t, dir, "attach-again.json", map[string]any{"GoalID": baseline.ID, "GoalVersion": "2", "BaselineDigest": "sha256:" + strings.Repeat("9", 64), "AcceptanceRef": "acceptance:scratch-weather", "AcceptanceVersion": "1", "SuccessorVersion": "3"})}, governed); err == nil {
		t.Fatal("attach onto a generation that already carries a WorkPlan, with a wrong digest, must be refused")
	}
	err = runDynamicInvocation(ctx, []string{"goal-drive", "--goal-id=" + baseline.ID, "--goal-version=2", "--provider=local", "--invocation-id=inv-1", "--repo=" + dir, "--branch=main"}, governed)
	if err == nil || !strings.Contains(err.Error(), "construct native Goal-drive runtime") {
		t.Fatalf("goal-drive on the drivable generation must reach runtime construction: %v", err)
	}
}

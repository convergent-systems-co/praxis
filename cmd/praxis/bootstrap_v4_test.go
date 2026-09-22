package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/convergent-systems-co/praxis/internal/goaldrive"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/packages/goals"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func TestResolveProposalSourceBytesRejectsMissingAndDriftedCandidate(t *testing.T) {
	baseline := goals.GoalBaseline{ID: "goal", Version: "1", SuccessCriteria: []string{"criterion"}}
	reqBody := []byte("criterion")
	reqSum := sha256.Sum256(reqBody)
	reqDigest := "sha256:" + hex.EncodeToString(reqSum[:])
	req := contracts.RequirementRef{ID: "success_criteria:" + reqDigest, SourceRef: "goal:goal/1#success_criteria/1", SourceDigest: reqDigest, Specification: reqBody}
	candidate := contracts.WorkCandidate{ID: "unit", SourceRef: filepath.Join(t.TempDir(), "missing.json"), SourceDigest: "sha256:" + strings.Repeat("1", 64), Specification: []byte("x"), Requirements: []contracts.RequirementRef{req}}
	proposal := contracts.WorkPlanProposal{Candidates: []contracts.WorkCandidate{candidate}}
	if err := resolveProposalSourceBytes(proposal, baseline); err == nil || !strings.Contains(err.Error(), "repository-relative") {
		t.Fatalf("absolute source was not rejected: %v", err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "candidate.json")
	if err := os.WriteFile(path, []byte("tracked"), 0o600); err != nil {
		t.Fatal(err)
	}
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	proposal.Candidates[0].SourceRef = "candidate.json"
	if err := resolveProposalSourceBytes(proposal, baseline); err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("drifted source was not rejected: %v", err)
	}
	proposal.Candidates[0].SourceRef = "missing.json"
	if err := resolveProposalSourceBytes(proposal, baseline); err == nil || !strings.Contains(err.Error(), "no such file") {
		t.Fatalf("missing source was not rejected: %v", err)
	}
}

func TestUnauthenticatedStopIsFencedForSafetyTurn(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "state.db")
	db, err := state.OpenSQLite(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	log := goaldrive.ActivityLog{Store: state.NewSQLiteEventStore(db), Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}
	req := goaldrive.WorkerRequest{GoalID: "goal", GoalVersion: "2", InvocationID: "inv", TurnID: "turn", ProviderID: "provider"}
	if _, err := log.Emit(ctx, goaldrive.ActivityTurnAllocated, req, log.Actor, contracts.TrustObserved, "praxis.controller", map[string]string{"safety_kernel": contracts.WorkPlanSafetyKernelVersion}); err != nil {
		db.Close()
		t.Fatal(err)
	}
	db.Close()
	args := supervisionArgs{db: dbPath, goalID: "goal", goalVersion: "2", invocationID: "inv", turnID: "turn", actor: contracts.PrincipalRef{ID: "claimed", Kind: "human"}}
	err = appendHumanIntervention(args, goaldrive.ActivityCancelRequested)
	if err == nil || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("unauthenticated stop was accepted: %v", err)
	}
}

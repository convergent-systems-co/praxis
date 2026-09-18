package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// plantHistoricalRecord seals an arbitrary payload into a lifecycle namespace
// exactly as the repository does, standing in for a superseded historical
// lineage whose records no longer validate under current contracts (#149).
func plantHistoricalRecord(t *testing.T, ctx context.Context, dbPath, namespace, id string, payload []byte) {
	t.Helper()
	db, err := state.OpenSQLite(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	sum := sha256.Sum256(payload)
	digest := "sha256:" + hex.EncodeToString(sum[:])
	service := praxiscrypto.EnvelopeService{Wrapper: recoveryTestWrapper{}}
	envelope, err := service.Seal(ctx, "fixture-retention-key", contracts.CryptoClassicalCompatible, payload, state.SecureBlobAAD(namespace, id, "1", digest))
	if err != nil {
		t.Fatal(err)
	}
	if err := state.New(db).PutSecureBlob(ctx, state.SecureBlobRecord{Namespace: namespace, ObjectID: id, ObjectVersion: "1", ObjectDigest: digest, Sensitivity: state.SensitivityConfidential, CryptoProfile: contracts.CryptoClassicalCompatible, Envelope: envelope, CreatedAt: time.Date(2026, 9, 15, 0, 19, 18, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
}

// TestLifecycleInspectionTolleratesUnrelatedHistoricalRecords reproduces the
// live weather defect: with historical review, proposal, and acceptance
// records from another lineage that fail current validation present in the
// same namespaces, inspection and the derived lifecycle operations for a
// different Goal must still succeed before and after the owner decision.
func TestLifecycleInspectionToleratesUnrelatedHistoricalRecords(t *testing.T) {
	ctx := context.Background()
	governed, _, _, dir := lifecycleFixture(t, ctx)
	dbPath := governed("PRAXIS_DB")
	historicalProposal, _ := json.Marshal(map[string]any{"id": "wp-proposal-dogfood-migrated-v1", "goal_id": "dogfood-praxis-issues-96-plus-migrated", "goal_version": "1", "baseline_digest": "sha256:" + strings.Repeat("d", 64), "proposed_by": map[string]string{"id": "planner", "kind": "model"}, "candidates": []map[string]any{{"id": "unit-1", "completion_predicates": []string{"legacy"}}}})
	plantHistoricalRecord(t, ctx, dbPath, "work_plan_proposal", "wp-proposal-dogfood-migrated-v1", historicalProposal)
	historicalReview, _ := json.Marshal(map[string]any{"proposal": map[string]any{"id": "wp-proposal-dogfood-migrated-v1", "goal_id": "dogfood-praxis-issues-96-plus-migrated", "goal_version": "1"}, "review": map[string]any{"proposal_digest": "sha256:" + strings.Repeat("e", 64), "baseline_digest": "sha256:" + strings.Repeat("d", 64), "review_ref": "wp-review-dogfood-migrated-v1", "review_digest": "sha256:" + strings.Repeat("f", 64), "status": "acceptable_for_authority_decision"}})
	plantHistoricalRecord(t, ctx, dbPath, "work_plan_review", "wp-review-dogfood-migrated-v1", historicalReview)
	historicalAcceptance, _ := json.Marshal(map[string]any{"proposal": map[string]any{"id": "wp-proposal-dogfood-migrated-v1"}, "plan": map[string]any{"baseline_digest": "sha256:" + strings.Repeat("d", 64)}})
	plantHistoricalRecord(t, ctx, dbPath, "work_plan_acceptance", "acceptance-dogfood-migrated-workplan-v1", historicalAcceptance)

	proposalDigest := proposeFixture(t, ctx, governed, dir, "weather")
	reviewDigest := reviewBySelector(t, ctx, governed, dir, proposalDigest, string(contracts.ReviewAcceptableForAuthority))
	requestDigest, err := requestBySelector(t, ctx, governed, dir, proposalDigest, reviewDigest)
	if err != nil {
		t.Fatalf("request with historical records present: %v", err)
	}
	inspect := func(version string) string {
		return string(captureStdout(t, func() {
			if err := runDynamicInvocation(ctx, []string{"goals-lifecycle", "--operation=inspect", "--goal-id=goal:pending-surface", "--goal-version=" + version}, governed); err != nil {
				t.Fatalf("inspect %s with historical records present: %v", version, err)
			}
		}))
	}
	if before := inspect("1"); !strings.Contains(before, requestDigest) || !strings.Contains(before, `"drivable": false`) {
		t.Fatalf("inspect before decision: %s", before)
	}
	var pending bytes.Buffer
	if err := runAuthorityPending(nil, governed, &pending); err != nil || !strings.Contains(pending.String(), requestDigest) {
		t.Fatalf("authority pending: %v", err)
	}
	if _, err := decideInteractive(t, governed, requestDigest, "approve", "DECIDE-APPROVE "+requestDigest); err != nil {
		t.Fatalf("decide: %v", err)
	}
	if after := inspect("1"); !strings.Contains(after, `"status": "decided:approve"`) {
		t.Fatalf("inspect after decision must still succeed and show the decision: %s", after)
	}
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
	if err := runDynamicInvocation(ctx, []string{"goals-lifecycle", "--operation=attach", "--goal-id=goal:pending-surface", "--goal-version=1", "--input=" + writeLifecycleInput(t, dir, "attach.json", map[string]any{"acceptance_ref": accepted.AcceptanceRef})}, governed); err != nil {
		t.Fatalf("attach: %v", err)
	}
	if final := inspect("2"); !strings.Contains(final, `"drivable": true`) {
		t.Fatalf("generation 2 must be drivable: %s", final)
	}
	if one := inspect("1"); !strings.Contains(one, `"acceptances"`) || strings.Contains(one, "dogfood") {
		t.Fatalf("inspect must report the Goal's own lineage only: %s", one)
	}
}

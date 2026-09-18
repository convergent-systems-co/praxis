package lifecycle

import (
	"strings"
	"testing"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func previewDigest(seed string) string { return "sha256:" + strings.Repeat(seed, 64)[:64] }

func previewReadiness() contracts.LifecycleReadiness {
	return contracts.LifecycleReadiness{CryptoBootstrap: "ready", StateStore: "ready", SchemaCompatibility: "ready", GovernanceRoot: "ready", AuthorityTopology: "ready", PackageRuntimeClosure: "ready", LifecycleRecovery: "ready", Installation: "ready"}
}

func previewPlan(t *testing.T, effect contracts.LifecycleEffectClass, snapshot bool, authority bool) contracts.LifecyclePlan {
	t.Helper()
	class := contracts.LifecycleBinary
	step := contracts.LifecycleTransitionStep{ID: "binary-step", Sequence: 1, Class: class, Current: contracts.LifecycleComponentRef{Class: class, ID: "binary-current", Version: "1", Digest: previewDigest("a")}, Target: contracts.LifecycleComponentRef{Class: class, ID: "binary-target", Version: "2", Digest: previewDigest("b")}, Preconditions: []contracts.LifecycleEvidenceRef{{ID: "precondition", Kind: "test", Source: "fixture", Digest: previewDigest("c")}}, Authority: contracts.LifecycleAuthorityRequirement{Required: authority, Operation: func() string {
		if authority {
			return "lifecycle.accept"
		}
		return ""
	}(), Scope: func() string {
		if authority {
			return "installation:test"
		}
		return ""
	}()}, Effect: effect, SnapshotRequired: snapshot, Reversible: effect != contracts.LifecycleIrreversible, RecoveryStrategy: "reconcile-or-rollback", ReadinessImpact: "installation"}
	plan := contracts.LifecyclePlan{PlanID: "plan-preview", PlanVersion: "1", InstallationID: "installation-test", CurrentManifestDigest: previewDigest("d"), TargetManifestDigest: previewDigest("e"), Steps: []contracts.LifecycleTransitionStep{step}, SnapshotRequired: snapshot, ExpectedReadiness: previewReadiness()}
	digest, err := plan.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	plan.Digest = digest
	return plan
}

func previewState() InstallationState {
	return InstallationState{InstallationID: "installation-test", ManifestDigest: previewDigest("d"), Readiness: previewReadiness(), SnapshotAvailable: false}
}

func TestPreviewIsReadOnlyAndReadyForCompatibleAutomaticPlan(t *testing.T) {
	preview, err := PreviewLifecycle(PreviewRequest{Plan: previewPlan(t, contracts.LifecycleAutomatic, false, false), Current: previewState()})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Status != PreviewReady || len(preview.Blockers) != 0 || !preview.RollbackAvailable {
		t.Fatalf("unexpected preview: %+v", preview)
	}
}

func TestPreviewReportsAuthorityAndSnapshotSeparately(t *testing.T) {
	plan := previewPlan(t, contracts.LifecycleIrreversible, true, true)
	preview, err := PreviewLifecycle(PreviewRequest{Plan: plan, Current: previewState()})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Status != PreviewAuthorityRequired {
		t.Fatalf("expected authority-required status: %+v", preview)
	}
	if len(preview.Blockers) != 2 {
		t.Fatalf("expected authority and snapshot blockers: %+v", preview.Blockers)
	}
	preview, err = PreviewLifecycle(PreviewRequest{Plan: plan, Current: InstallationState{InstallationID: "installation-test", ManifestDigest: previewDigest("d"), Readiness: previewReadiness(), SnapshotAvailable: true}, AuthorityScopes: []string{"installation:*"}})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Status != PreviewReady || preview.RollbackAvailable {
		t.Fatalf("unexpected unblocked irreversible preview: %+v", preview)
	}
}

func TestPreviewRejectsStaleCurrentManifestAndInvalidPlanDigest(t *testing.T) {
	plan := previewPlan(t, contracts.LifecycleAutomatic, false, false)
	stale := previewState()
	stale.ManifestDigest = previewDigest("z")
	if _, err := PreviewLifecycle(PreviewRequest{Plan: plan, Current: stale}); err == nil {
		t.Fatal("stale current manifest was accepted")
	}
	plan.Digest = previewDigest("z")
	if _, err := PreviewLifecycle(PreviewRequest{Plan: plan, Current: previewState()}); err == nil {
		t.Fatal("invalid plan digest was accepted")
	}
}

func TestPreviewDoesNotTreatHistoricalEvidenceAsAuthority(t *testing.T) {
	plan := previewPlan(t, contracts.LifecycleAutomatic, false, true)
	plan.PreservedHistory = []contracts.LifecycleEvidenceRef{{ID: "old-decision", Kind: "historical-authority", Source: "restore", Digest: previewDigest("a")}}
	digest, err := plan.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	plan.Digest = digest
	preview, err := PreviewLifecycle(PreviewRequest{Plan: plan, Current: previewState()})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Status != PreviewAuthorityRequired {
		t.Fatalf("historical evidence bypassed authority: %+v", preview)
	}
	if len(preview.PreservedHistory) != 1 || preview.PreservedHistory[0].ID != "old-decision" {
		t.Fatal("historical evidence was not preserved in preview")
	}
}

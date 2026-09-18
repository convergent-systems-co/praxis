package lifecycle

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func stagedPointer(t *testing.T, contents []byte) StagedPointer {
	t.Helper()
	digest := contentDigest(contents)
	artifact := contracts.LifecycleArtifactRef{ID: "artifact", Version: "1", Digest: digest, MediaType: "application/octet-stream", Size: int64(len(contents)), ProvenanceDigest: migrationDigest("d"), Retention: "required", AvailabilityEvidence: "verified"}
	return StagedPointer{PlanID: "plan", PlanDigest: migrationDigest("a"), StepID: "step", InstallationID: "installation", Target: contracts.LifecycleComponentRef{Class: contracts.LifecycleBinary, ID: "binary", Version: "2", Digest: digest}, Artifact: artifact, StageDigest: digest}
}

func TestStagedArtifactActivationAndRestartRecovery(t *testing.T) {
	root := t.TempDir()
	store := StagedArtifactStore{Root: root}
	contents := []byte("verified-package")
	pointer := stagedPointer(t, contents)
	if err := store.Stage(context.Background(), pointer, contents); err != nil {
		t.Fatal(err)
	}
	if err := store.Activate(context.Background(), pointer); err != nil {
		t.Fatal(err)
	}
	recovered, err := (StagedArtifactStore{Root: root}).Recover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if recovered != pointer {
		t.Fatalf("recovered pointer differs: %#v != %#v", recovered, pointer)
	}
}

func TestStagedArtifactRejectsSubstitutionAndIncompleteActivation(t *testing.T) {
	root := t.TempDir()
	store := StagedArtifactStore{Root: root}
	contents := []byte("verified-package")
	pointer := stagedPointer(t, contents)
	if err := store.Stage(context.Background(), pointer, []byte("substituted")); err == nil {
		t.Fatal("substituted staged content was accepted")
	}
	if _, err := store.Recover(context.Background()); !os.IsNotExist(err) {
		t.Fatalf("missing active pointer was not reported as incomplete: %v", err)
	}
	if err := store.Stage(context.Background(), pointer, contents); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "staged", digestHex(pointer.StageDigest)), []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.Activate(context.Background(), pointer); err == nil || !strings.Contains(err.Error(), "digest") {
		t.Fatalf("tampered artifact was not rejected: %v", err)
	}
}

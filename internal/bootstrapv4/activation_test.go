package bootstrapv4

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func TestVerifyFileFailsClosedForMissingAndWrongActiveIdentity(t *testing.T) {
	binding := contracts.WorkPlanSafetyBinding{KernelVersion: contracts.WorkPlanSafetyKernelVersion, ActivationManifestDigest: "sha256:" + strings.Repeat("1", 64), ValidationProfileDigest: "sha256:" + strings.Repeat("2", 64), SpecificationBundleDigest: "sha256:" + strings.Repeat("3", 64)}
	if err := VerifyFile(filepath.Join(t.TempDir(), "missing.json"), binding, ActivePackageIdentity{}); err == nil {
		t.Fatal("missing activation manifest was accepted")
	}
	manifest := Manifest{Version: "1", KernelVersion: contracts.WorkPlanSafetyKernelVersion, SourceCommit: "commit", SourceTreeDigest: "sha256:" + strings.Repeat("9", 64), BuildModified: true, ActiveBinaryPath: "/not/the/running/binary", ActiveBinaryDigest: "sha256:" + strings.Repeat("4", 64), GoalsPackageID: "praxis.package.goals", GoalsPackageVersion: "1", GoalsPackageContentDigest: "sha256:" + strings.Repeat("5", 64), GoalsPackageExecutableDigest: "sha256:" + strings.Repeat("6", 64), GoalsPackageContractDigest: "sha256:" + strings.Repeat("7", 64), ValidationProfileDigest: binding.ValidationProfileDigest, SpecificationBundleDigest: binding.SpecificationBundleDigest, QualificationEvidenceDigest: "sha256:" + strings.Repeat("8", 64)}
	raw, _ := json.Marshal(manifest)
	sum := sha256.Sum256(raw)
	binding.ActivationManifestDigest = "sha256:" + hex.EncodeToString(sum[:])
	path := filepath.Join(t.TempDir(), "activation.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	identity := ActivePackageIdentity{ID: manifest.GoalsPackageID, Version: manifest.GoalsPackageVersion, ContentDigest: manifest.GoalsPackageContentDigest, ExecutableDigest: manifest.GoalsPackageExecutableDigest, ContractDigest: manifest.GoalsPackageContractDigest}
	if err := VerifyFile(path, binding, identity); err == nil {
		t.Fatal("wrong active binary identity was accepted")
	}
}

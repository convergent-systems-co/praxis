package goals

import (
	"testing"

	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
)

func TestPackageBuildInputBindsGoalsExecutableAndInvocation(t *testing.T) {
	input, err := PackageBuildInput([]byte("verified-goals-executable"))
	if err != nil {
		t.Fatal(err)
	}
	built, err := packagecatalog.BuildPackage(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(built.Manifest.Invocations) != 2 || built.Manifest.Invocations[0].EntryPointID != "goals-lifecycle" || built.Manifest.Invocations[1].EntryPointID != "goal-drive" {
		t.Fatalf("unexpected invocation set: %#v", built.Manifest.Invocations)
	}
	if built.Manifest.Version != PackageVersion || PackageVersion != "0.1.1" {
		t.Fatalf("successor package version must be 0.1.1, got %q", built.Manifest.Version)
	}
	for _, inv := range built.Manifest.Invocations {
		if inv.PackageVersion != PackageVersion {
			t.Fatalf("invocation %q carries package version %q, want %q", inv.EntryPointID, inv.PackageVersion, PackageVersion)
		}
	}
	if _, ok := built.Manifest.ExecutableBinding("goal-drive"); ok {
		t.Fatal("goal-drive is dispatched by the first-party controller and must not carry a plugin executable binding")
	}
	binding, ok := built.Manifest.ExecutableBinding("goals-lifecycle")
	if !ok {
		t.Fatal("Goals lifecycle executable binding is missing")
	}
	if binding.ExecutableDigest != built.Manifest.Contents[1].Digest || binding.PluginID != GoalsPluginID {
		t.Fatalf("binding does not identify declared executable: %#v", binding)
	}
	if len(built.ArtifactBytes) == 0 || built.ArtifactDigest == "" {
		t.Fatal("canonical package archive was not produced")
	}
}

func TestPackageBuildInputRejectsMissingExecutable(t *testing.T) {
	if _, err := PackageBuildInput(nil); err == nil {
		t.Fatal("missing executable must fail closed")
	}
}

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
	if len(built.Manifest.Invocations) != 1 || built.Manifest.Invocations[0].EntryPointID != "goals-lifecycle" {
		t.Fatalf("unexpected invocation set: %#v", built.Manifest.Invocations)
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

package publisher

import (
	"testing"

	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"github.com/convergent-systems-co/praxis/packages/goals"
)

// TestExecutableBoundInvocationAdmitsNativelyDispatchedInvocations proves the
// signing preview binds the plugin-executable invocation while admitting
// further invocations that carry no executable binding (they are covered by
// the manifest digest), and refuses manifests without exactly one binding.
func TestExecutableBoundInvocationAdmitsNativelyDispatchedInvocations(t *testing.T) {
	input, err := goals.PackageBuildInput([]byte("goals-plugin"))
	if err != nil {
		t.Fatal(err)
	}
	built, err := packagecatalog.BuildPackage(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(built.Manifest.Invocations) != 2 {
		t.Fatalf("canonical Goals manifest exposes %d invocations, want 2", len(built.Manifest.Invocations))
	}
	invocation, binding, err := executableBoundInvocation(built.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	if invocation.EntryPointID != "goals-lifecycle" || binding.EntryPointID != "goals-lifecycle" || binding.ExecutableDigest != built.Manifest.Contents[1].Digest {
		t.Fatalf("bound invocation %q / binding %q", invocation.EntryPointID, binding.EntryPointID)
	}
	single := built.Manifest
	single.Invocations = built.Manifest.Invocations[:1]
	onlyInvocation, onlyBinding, err := executableBoundInvocation(single)
	if err != nil || onlyInvocation.EntryPointID != invocation.EntryPointID || onlyBinding.ExecutableDigest != binding.ExecutableDigest {
		t.Fatalf("single-invocation manifest must bind identically: %v", err)
	}
	unbound := built.Manifest
	unbound.ExecutableBindings = nil
	if _, _, err := executableBoundInvocation(unbound); err == nil {
		t.Fatal("manifest without an executable binding must be refused")
	}
	doubled := built.Manifest
	second := binding
	second.EntryPointID = "goal-drive"
	doubled.ExecutableBindings = append(append(doubled.ExecutableBindings[:0:0], built.Manifest.ExecutableBindings...), second)
	if _, _, err := executableBoundInvocation(doubled); err == nil {
		t.Fatal("manifest with two executable bindings must be refused")
	}
	dangling := built.Manifest
	dangling.Invocations = built.Manifest.Invocations[1:]
	if _, _, err := executableBoundInvocation(dangling); err == nil {
		t.Fatal("binding without its invocation must be refused")
	}
}

package plugin

import (
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/capability"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func TestRestartedPluginCannotReuseInstanceBoundLease(t *testing.T) {
	now := time.Now().UTC()
	principal := contracts.PrincipalRef{ID: "plugin:workspace", Kind: "plugin"}
	binding := LeaseBinding{
		Lease:      contracts.CapabilityLease{ID: "l1", Principal: principal, Capability: "workspace.search.text", Operations: []string{"query"}, Scope: "workspace:1", IssuedAt: now},
		InstanceID: "instance-old", SessionID: "session-old",
	}
	instance := InstanceIdentity{InstanceID: "instance-new", PluginID: "workspace", PluginVersion: "1", ArtifactDigest: "sha256:a", RuntimeSession: "session-new"}
	err := binding.Evaluate(instance, capability.Request{Principal: principal, Capability: "workspace.search.text", Operation: "query", Scope: "workspace:1"}, now)
	if err == nil {
		t.Fatal("new plugin instance must not reuse old instance-bound lease")
	}
}

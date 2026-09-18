package client

import "testing"

func TestUnknownRequiredClientEnforcementFails(t *testing.T) {
	p := EnforcementProfile{ClientID: "client", Properties: map[string]EnforcementState{"tool_mediation": ClientUnknown}}
	if err := p.Require([]string{"tool_mediation"}, false); err == nil {
		t.Fatal("unknown required enforcement must fail closed")
	}
}

func TestExclusiveMediationRejectsBypassPath(t *testing.T) {
	p := EnforcementProfile{ClientID: "client", Properties: map[string]EnforcementState{"tool_mediation": ClientEnforced}, BypassPaths: []string{"native-shell"}}
	if err := p.Require([]string{"tool_mediation"}, true); err == nil {
		t.Fatal("exclusive mediation must reject alternate bypass path")
	}
}

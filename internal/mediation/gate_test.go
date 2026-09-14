package mediation

import (
	"testing"

	"github.com/convergent-systems-co/praxis/internal/client"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func mediatedIntent() contracts.ActionIntent {
	return contracts.ActionIntent{Version: "v1", ID: "intent-1", Actor: contracts.PrincipalRef{ID: "agent-1", Kind: "agent"}, Operation: "write", Target: "workspace:file", Scope: "workspace:1"}
}

func TestAuthorizeRequiresDeterministicClientMediation(t *testing.T) {
	intent := mediatedIntent()
	decision, err := Authorize(Request{
		Intent:                    intent,
		Client:                    client.EnforcementProfile{ClientID: "cli", Properties: map[string]client.EnforcementState{"tool_mediation": client.ClientEnforced}},
		RequiredClientProperties:  []string{"tool_mediation"},
		RequireExclusiveMediation: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	digest, _ := intent.Digest()
	if decision.IntentDigest != digest || decision.EnforcementComponent == "" {
		t.Fatalf("unexpected deterministic decision: %#v", decision)
	}
}

func TestAuthorizeFailsClosedForUnknownOrBypassableMediation(t *testing.T) {
	base := Request{Intent: mediatedIntent(), RequiredClientProperties: []string{"tool_mediation"}}
	for name, request := range map[string]Request{
		"unknown": {Intent: base.Intent, Client: client.EnforcementProfile{ClientID: "cli", Properties: map[string]client.EnforcementState{"tool_mediation": client.ClientUnknown}}, RequiredClientProperties: base.RequiredClientProperties},
		"bypass":  {Intent: base.Intent, Client: client.EnforcementProfile{ClientID: "cli", Properties: map[string]client.EnforcementState{"tool_mediation": client.ClientEnforced}, BypassPaths: []string{"native-shell"}}, RequiredClientProperties: base.RequiredClientProperties, RequireExclusiveMediation: true},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Authorize(request); err == nil {
				t.Fatal("required mediation must fail closed")
			}
		})
	}
}

func TestModelProposalCannotReplaceMediationDecision(t *testing.T) {
	if DeniedByModel == nil {
		t.Fatal("model denial sentinel must remain explicit")
	}
	if _, err := Authorize(Request{Intent: mediatedIntent(), Client: client.EnforcementProfile{ClientID: "cli"}, RequiredClientProperties: []string{"tool_mediation"}}); err == nil {
		t.Fatal("missing deterministic mediation must not be treated as model authorization")
	}
}

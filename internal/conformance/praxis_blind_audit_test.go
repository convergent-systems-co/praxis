package conformance

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// This fixture is derived only from the original architectural intent in ADR-001,
// ADR-003 and ADR-004 plus observable implementation evidence. It deliberately
// contains no qualification oracle or post-hoc known-gap text.
func TestPraxisBlindGoalAudit(t *testing.T) {
	claims:=[]Claim{
		{ID:"persistent-agent-entity",Statement:"agents are persistent versioned entities independent of model/provider identity",RequiredEvidence:[]string{"unit_test","runtime_test"},Behavioral:true,Critical:true,SourceRef:"ADR-001:4-5;ADR-003"},
		{ID:"agent-operational-graphs",Statement:"a persistent agent can execute versioned internal operational graphs for receiving goals, memory/context, action, evidence, reflection and learning",RequiredEvidence:[]string{"runtime_test"},Behavioral:true,Critical:true,SourceRef:"ADR-004"},
		{ID:"governed-self-modification",Statement:"learned graph changes are candidates evaluated before governed promotion with rollback",RequiredEvidence:[]string{"unit_test","runtime_test"},Behavioral:true,Critical:true,SourceRef:"ADR-001:9;ADR-012"},
		{ID:"deterministic-runtime-authority",Statement:"runtime state authority transitions recovery evidence and completion are owned below conversation history",RequiredEvidence:[]string{"runtime_test"},Behavioral:true,Critical:true,SourceRef:"ADR-001:3"},
	}
	evidence:=[]Evidence{
		{ID:"agent-definition-tests",ClaimID:"persistent-agent-entity",Kind:"unit_test",Subject:"agent identity/generation",Ref:"internal/agent/definition_test.go",Supports:true},
		{ID:"agent-identity-tests",ClaimID:"persistent-agent-entity",Kind:"unit_test",Subject:"agent lineage",Ref:"internal/agent/identity_test.go",Supports:true},
		// Operational graph evidence is intentionally absent unless an executable
		// agent runtime exists. Graph contracts/prose are not behavioral evidence.
		{ID:"learning-candidate-tests",ClaimID:"governed-self-modification",Kind:"unit_test",Subject:"candidate lifecycle",Ref:"internal/learning",Supports:true},
		{ID:"kernel-runtime-tests",ClaimID:"deterministic-runtime-authority",Kind:"runtime_test",Subject:"graph runtime",Ref:"internal/kernel",Supports:true},
	}
	r,err:=Evaluate("sha256:praxis-original-intent",claims,evidence,time.Date(2026,9,13,12,35,0,0,time.UTC)); if err!=nil{t.Fatal(err)}
	b,_:=json.MarshalIndent(r,"","  "); fmt.Printf("PRAXIS_BLIND_CONFORMANCE %s\n",b)
	if r.Conformant { t.Fatal("blind audit unexpectedly found all critical original-goal behaviors satisfied") }
}

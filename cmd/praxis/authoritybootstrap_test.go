package main

import (
	"strings"
	"testing"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func TestAuthorityBootstrapRequiresInteractiveConfirmation(t *testing.T) {
	err := runAuthorityBootstrapWithTerminal([]string{"--scope", "goal:test"}, func(string) string { return "" }, strings.NewReader("ENROLL anything\n"), &strings.Builder{}, false)
	if err != errAuthorityBootstrapConfirmation {
		t.Fatalf("non-interactive enrollment must fail closed: %v", err)
	}
}

func TestAuthorityBootstrapRejectsInvalidBootstrapBeforeConfirmation(t *testing.T) {
	err := runAuthorityBootstrapWithTerminal([]string{"--scope", "goal:test"}, func(key string) string {
		if key == "PRAXIS_BOOTSTRAP_RECORD" {
			return "/does/not/exist"
		}
		return ""
	}, strings.NewReader("approve\n"), &strings.Builder{}, true)
	if err == nil || !strings.Contains(err.Error(), "bootstrap metadata") {
		t.Fatalf("invalid bootstrap state must fail before enrollment: %v", err)
	}
}

func TestGoalsRecoveryDelegationCheckStepIsClosedByContractAndOperation(t *testing.T) {
	cases := []struct {
		name, contract, operation, want string
		ok                              bool
	}{
		{"failed verification uses existing-asset verification", "goals-established-state-publication/4", "publish-goals-from-established-state", "verify-draft", true},
		{"initial recovery uses upload-stage manifest", "goals-established-state-publication/1", "publish-goals-from-established-state", "manifest", true},
		{"chained recovery uses upload-stage manifest", "goals-established-state-publication/2", "publish-goals-from-established-state", "manifest", true},
		{"ordered recovery uses upload-stage manifest", "goals-established-state-publication/3", "publish-goals-from-established-state", "manifest", true},
		{"unknown contract rejected", "goals-established-state-publication/99", "publish-goals-from-established-state", "", false},
		{"contradictory operation rejected", "goals-established-state-publication/4", "other-operation", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			step, err := goalsRecoveryDelegationCheckStep(contracts.ActionIntent{Operation: tc.operation, Parameters: map[string]string{"contract": tc.contract}})
			if tc.ok && (err != nil || step != tc.want) {
				t.Fatalf("got step=%q err=%v want=%q", step, err, tc.want)
			}
			if !tc.ok && err == nil {
				t.Fatalf("expected closed rejection, got step=%q", step)
			}
		})
	}
}

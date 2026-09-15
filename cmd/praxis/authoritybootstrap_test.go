package main

import (
	"strings"
	"testing"
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

package client

import "testing"

// TestParseInvocationFieldsKeepsWhitespaceInOptionValues is the regression
// test for #159: an argv element is never re-split, so a free-text option
// value survives intact.
func TestParseInvocationFieldsKeepsWhitespaceInOptionValues(t *testing.T) {
	inv, err := ParseInvocationFields([]string{"praxis", "goals-lifecycle", "--operation=complete", "--reason=the contract needs a fourth capability", "--status=incomplete"})
	if err != nil {
		t.Fatal(err)
	}
	if inv.EntryPoint != "goals-lifecycle" || inv.Options["reason"] != "the contract needs a fourth capability" || inv.Options["status"] != "incomplete" || len(inv.Arguments) != 0 {
		t.Fatalf("option values must keep their whitespace: %+v", inv)
	}
	if _, err := ParseInvocationFields([]string{"praxis"}); err == nil {
		t.Fatal("an entry point is required")
	}
	slash, err := ParseSlashInvocation("praxis goals-lifecycle --operation=inspect")
	if err != nil || slash.Options["operation"] != "inspect" {
		t.Fatalf("string form still parses: %+v %v", slash, err)
	}
}

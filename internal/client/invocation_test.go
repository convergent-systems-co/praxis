package client

import "testing"

func TestParseDevelopDashboard(t *testing.T) {
	inv, err := ParseSlashInvocation("/praxis develop repo --dashboard --mode=fast")
	if err != nil { t.Fatal(err) }
	if inv.EntryPoint != "develop" || len(inv.Arguments) != 1 || inv.Arguments[0] != "repo" || inv.Options["dashboard"] != "true" || inv.Options["mode"] != "fast" {
		t.Fatalf("unexpected invocation: %+v", inv)
	}
}

func TestParseRejectsDuplicateOption(t *testing.T) {
	if _, err := ParseSlashInvocation("/praxis develop --dashboard --dashboard"); err == nil {
		t.Fatal("duplicate options must be rejected")
	}
}

func TestParserDoesNotInferAuthority(t *testing.T) {
	inv, err := ParseSlashInvocation("/praxis develop --dangerous=true")
	if err != nil { t.Fatal(err) }
	if inv.Options["dangerous"] != "true" { t.Fatal("parser should preserve option for contract validation") }
	// Authorization occurs downstream; parsing an option is never a permission grant.
}

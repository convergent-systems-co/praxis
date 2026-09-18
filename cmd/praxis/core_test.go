package main

import (
	"path/filepath"
	"testing"

	"github.com/convergent-systems-co/praxis/internal/kernel"
)

func TestControlArgsRequireExplicitActorAndDatabase(t *testing.T) {
	getenv := func(key string) string { return "" }
	if _, err := parseControlArgs("cancel", []string{"run-1"}, getenv); err == nil {
		t.Fatal("cancel must fail without explicit database and actor")
	}
}

func TestResumeArgsRequireExactWaitReference(t *testing.T) {
	db := filepath.Join(t.TempDir(), "praxis.db")
	getenv := func(key string) string {
		switch key {
		case "PRAXIS_DB":
			return db
		case "PRAXIS_ACTOR_ID":
			return "user-1"
		case "PRAXIS_ACTOR_KIND":
			return "user"
		default:
			return ""
		}
	}
	if _, err := parseControlArgs("resume", []string{"run-1", "--wait-kind", string(kernel.WaitApproval)}, getenv); err == nil {
		t.Fatal("resume must require the persisted wait reference")
	}
	parsed, err := parseControlArgs("resume", []string{"run-1", "--wait-kind", string(kernel.WaitApproval), "--wait-ref", "approval-1"}, getenv)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.waitKind != kernel.WaitApproval || parsed.waitRef != "approval-1" || parsed.actor.ID != "user-1" {
		t.Fatalf("unexpected parsed resume args: %#v", parsed)
	}
}

func TestVersionRejectsArguments(t *testing.T) {
	if err := runVersion([]string{"extra"}); err == nil {
		t.Fatal("version should reject extra arguments")
	}
}

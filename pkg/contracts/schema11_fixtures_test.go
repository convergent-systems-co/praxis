package contracts

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
	"time"
)

func sha(b []byte) string { s := sha256.Sum256(b); return "sha256:" + hex.EncodeToString(s[:]) }

// The testdata files are the exact decrypted payloads of governance records
// persisted by the real schema-11 dogfood installation on 2026-09-14. They pin
// that the current representation reproduces those bytes and recomputes the
// identities that later records reference, so an upgrade can never silently
// change what the accepted WorkPlan or its authority request are.
func TestSchema11LiveWorkPlanProposalRemainsVerifiable(t *testing.T) {
	payload, err := os.ReadFile("testdata/schema11-workplan-proposal.json")
	if err != nil {
		t.Fatal(err)
	}
	if got := sha(payload); got != "sha256:1a66cf03a46dc75009ccbc5d1500e4e31c2535d77c1814bcd2f71fbc4d45a7fb" {
		t.Fatalf("fixture bytes changed: %s", got)
	}
	var proposal WorkPlanProposal
	if err := json.Unmarshal(payload, &proposal); err != nil {
		t.Fatal(err)
	}
	again, err := json.Marshal(proposal)
	if err != nil || !bytes.Equal(again, payload) {
		t.Fatalf("current representation must reproduce the persisted proposal bytes: %v", err)
	}
	digest, err := proposal.Digest()
	if err != nil || digest != "sha256:f59d5fd97e5ac8bf6cddf0d65640cedae4785e390bec935659254d44e64c0ea3" {
		t.Fatalf("recomputed proposal digest drifted from the digest referenced by the live review, acceptance, and authority request: %s %v", digest, err)
	}
}

func TestSchema11LiveAuthorityRequestRemainsVerifiable(t *testing.T) {
	payload, err := os.ReadFile("testdata/schema11-authority-request.json")
	if err != nil {
		t.Fatal(err)
	}
	if got := sha(payload); got != "sha256:04784c8ba71f71423704f755db49786a06ccb573e86d8deb9c6725d09ac8d2ee" {
		t.Fatalf("fixture bytes changed: %s", got)
	}
	var request AuthorityRequest
	if err := json.Unmarshal(payload, &request); err != nil {
		t.Fatal(err)
	}
	again, err := json.Marshal(request)
	if err != nil || !bytes.Equal(again, payload) {
		t.Fatalf("current representation must reproduce the persisted request bytes: %v", err)
	}
	digest, err := request.DigestAt(time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC))
	if err != nil || digest != "sha256:04784c8ba71f71423704f755db49786a06ccb573e86d8deb9c6725d09ac8d2ee" {
		t.Fatalf("recomputed request digest drifted from the digest bound by the live decision: %s %v", digest, err)
	}
}

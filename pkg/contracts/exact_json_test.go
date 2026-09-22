package contracts

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

func TestUnmarshalExactJSONRefusesAmbiguousSafetyEvidence(t *testing.T) {
	type doc struct {
		Status string `json:"status"`
		Inner  struct {
			A int `json:"a"`
		} `json:"inner"`
	}
	valid := `{"status":"undecided","inner":{"a":1}}`
	var ok doc
	if err := UnmarshalExactJSON([]byte(valid), &ok, true); err != nil || ok.Status != "undecided" {
		t.Fatalf("a single complete value was refused: %v", err)
	}
	for name, body := range map[string]string{
		"trailing garbage":         valid + " trailing-not-json",
		"second object":            valid + ` {"status":"approved"}`,
		"duplicate key":            `{"status":"undecided","status":"approved"}`,
		"nested duplicate key":     `{"inner":{"a":1,"a":2}}`,
		"unknown field":            `{"status":"undecided","extra":true}`,
		"empty":                    ``,
		"truncated":                `{"status":"undecided"`,
		"oversized":                `{"status":"` + strings.Repeat("x", MaxGovernedArtifactBytes) + `"}`,
		"duplicate key in a array": `[{"a":1,"a":2}]`,
	} {
		var out doc
		if err := UnmarshalExactJSON([]byte(body), &out, true); err == nil {
			t.Fatalf("%s: ambiguous evidence was accepted", name)
		}
	}
	// Duplicate detection does not depend on the unknown-field policy.
	var lenient map[string]any
	if err := UnmarshalExactJSON([]byte(`{"a":1,"a":2}`), &lenient, false); !errors.Is(err, ErrAmbiguousJSON) {
		t.Fatalf("duplicate key was accepted by the lenient policy: %v", err)
	}
}

// B13: a matching digest authenticates bytes, it does not make ambiguous bytes
// valid dossier evidence.
func TestDossierResolutionRefusesMalformedOrAmbiguousBytes(t *testing.T) {
	for name, mutate := range map[string]func([]byte) []byte{
		"trailing garbage":     func(b []byte) []byte { return append(b, []byte(" trailing-not-json")...) },
		"second approved JSON": func(b []byte) []byte { return append(b, []byte(` {"status":"approved"}`)...) },
		"duplicate status key": func(b []byte) []byte {
			return bytes.Replace(b, []byte(`"status":"undecided"`), []byte(`"status":"undecided","status":"approved"`), 1)
		},
	} {
		id, contract, artifact := gateEvidenceFixture(t)
		artifact.Bytes = mutate(append([]byte(nil), artifact.Bytes...))
		sum := sha256.Sum256(artifact.Bytes)
		artifact.Digest = "sha256:" + hex.EncodeToString(sum[:])
		if _, _, err := ResolveGateDossier(id, contract, []GovernedArtifactEvidence{artifact}); err == nil {
			t.Fatalf("%s: malformed dossier evidence with a matching digest was accepted", name)
		}
	}
	id, contract, artifact := gateEvidenceFixture(t)
	if _, _, err := ResolveGateDossier(id, contract, []GovernedArtifactEvidence{artifact}); err != nil {
		t.Fatalf("the exact dossier was refused: %v", err)
	}
}

// B5: executable completion is never acceptable proposal, acceptance, or
// attachment input for a safety-bearing plan.
func TestSafetyPlanRejectsSuppliedCompletion(t *testing.T) {
	proposal := safetyProposalFixture()
	proposal.Candidates[1].Completed = true
	if err := proposal.Validate(); err == nil {
		t.Fatal("a proposal that supplies completed state validated")
	}

	proposal = safetyProposalFixture()
	acceptance := WorkPlanAcceptance{}
	_ = acceptance
	plan := WorkPlan{BaselineDigest: proposal.BaselineDigest, AuthorityRef: "a", AuthorityDigest: "d", AcceptanceRef: "r", AcceptanceDigest: "d", ProposalDigest: "p", AcceptedBy: PrincipalRef{ID: "owner", Kind: "human"}, Safety: proposal.Safety}
	accepted, err := MaterializeAcceptedPlanCandidate(proposal, "acceptance", "sha256:"+strings.Repeat("e", 64))
	if err != nil {
		t.Fatal(err)
	}
	plan.Candidates, plan.Relationships = accepted.Candidates, accepted.Relationships
	if err := plan.Validate(); err != nil {
		t.Fatalf("the honest accepted plan was refused: %v", err)
	}
	plan.Candidates = append([]WorkCandidate(nil), plan.Candidates...)
	plan.Candidates[1].Completed = true
	if err := plan.Validate(); err == nil {
		t.Fatal("an accepted plan carrying completed state validated")
	}
}

package contracts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

func gateEvidenceFixture(t *testing.T) (string, AuthorityGateContract, GovernedArtifactEvidence) {
	t.Helper()
	gateID := "gate:hi-supersession-and-active-turn"
	contract := AuthorityGateContract{
		QuestionSchemaID: "praxis/gate-c-question/1", Question: "Choose the active-turn disposition.",
		RequiredDossier:  GateDossierRequirement{ProducerCandidateID: "unit:hi-governance-decision-dossier", Role: "gate-c", EvidenceClass: "authority-decision-dossier", SchemaID: "praxis/gate-c-dossier/1"},
		AlternativesRule: GateAlternativesFromDossier, RequiredAlternatives: []string{"finish and reconcile", "authenticated interrupt", "pause or quarantine"},
		AuthorityPrincipal: GateAuthorityHuman, DownstreamSemantics: "implement only the exact owner-selected alternative", ContentAddressing: GateContentAddressingSHA256,
	}
	dossier := GateDossier{
		SchemaID: contract.RequiredDossier.SchemaID, GateID: gateID, ProducerCandidateID: contract.RequiredDossier.ProducerCandidateID,
		Role: contract.RequiredDossier.Role, EvidenceClass: contract.RequiredDossier.EvidenceClass, Status: GateDossierStatusUndecided,
		Question: contract.Question, OfferedAlternatives: append([]string(nil), contract.RequiredAlternatives...),
		Consequences: map[string]string{"finish and reconcile": "preserve and reconcile", "authenticated interrupt": "interrupt and preserve", "pause or quarantine": "pause and preserve"},
		Evidence:     []GateDossierEvidenceRef{{SourceRef: "docs/evidence.md", SourceDigest: "sha256:" + strings.Repeat("a", 64)}},
	}
	body, err := json.Marshal(dossier)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	artifact := GovernedArtifactEvidence{
		ProducerCandidateID: contract.RequiredDossier.ProducerCandidateID, ProducerSpecificationHash: "sha256:" + strings.Repeat("b", 64),
		ValidationProfileDigest: "sha256:" + strings.Repeat("c", 64), ConformanceQualified: true, Checkpoint: "commit",
		Role: contract.RequiredDossier.Role, EvidenceClass: contract.RequiredDossier.EvidenceClass, SourceRef: "docs/gate-c.json", SchemaID: contract.RequiredDossier.SchemaID,
		Digest: "sha256:" + hex.EncodeToString(sum[:]), Bytes: body,
	}
	return gateID, contract, artifact
}

func TestGateSpecificationSeparatesFutureDossierEvidence(t *testing.T) {
	_, contract, _ := gateEvidenceFixture(t)
	body, _ := json.Marshal(map[string]any{
		"question_schema_id": contract.QuestionSchemaID, "question": contract.Question, "required_dossier": contract.RequiredDossier,
		"alternatives_rule": contract.AlternativesRule, "required_alternatives": contract.RequiredAlternatives,
		"authority_principal_kind": contract.AuthorityPrincipal, "downstream_consequence_semantics": contract.DownstreamSemantics, "content_addressing": contract.ContentAddressing,
	})
	if _, err := ParseAuthorityGateContract(body); err != nil {
		t.Fatalf("schema-only gate contract rejected: %v", err)
	}
	legacy := append(body[:len(body)-1], []byte(`,"dossier":"future bytes","dossier_ref":"future","dossier_digest":"sha256:`+strings.Repeat("d", 64)+`"}`)...)
	if _, err := ParseAuthorityGateContract(legacy); err == nil {
		t.Fatal("proposal-time concrete dossier was accepted")
	}
}

func TestResolveGateDossierFailsClosedForEvidenceDefects(t *testing.T) {
	gateID, contract, artifact := gateEvidenceFixture(t)
	if _, dossier, err := ResolveGateDossier(gateID, contract, []GovernedArtifactEvidence{artifact}); err != nil || len(dossier.OfferedAlternatives) != 3 {
		t.Fatalf("valid dossier rejected: %+v %v", dossier, err)
	}
	tests := map[string]func(*GovernedArtifactEvidence){
		"unqualified producer": func(a *GovernedArtifactEvidence) { a.ConformanceQualified = false },
		"wrong producer":       func(a *GovernedArtifactEvidence) { a.ProducerCandidateID = "other" },
		"wrong role":           func(a *GovernedArtifactEvidence) { a.Role = "other" },
		"missing bytes":        func(a *GovernedArtifactEvidence) { a.Bytes = nil },
		"wrong digest":         func(a *GovernedArtifactEvidence) { a.Digest = "sha256:" + strings.Repeat("f", 64) },
		"wrong schema": func(a *GovernedArtifactEvidence) {
			var dossier GateDossier
			_ = json.Unmarshal(a.Bytes, &dossier)
			dossier.SchemaID = "wrong"
			a.Bytes, _ = json.Marshal(dossier)
			sum := sha256.Sum256(a.Bytes)
			a.Digest = "sha256:" + hex.EncodeToString(sum[:])
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			copy := artifact
			copy.Bytes = append([]byte(nil), artifact.Bytes...)
			mutate(&copy)
			if _, _, err := ResolveGateDossier(gateID, contract, []GovernedArtifactEvidence{copy}); err == nil {
				t.Fatal("defective dossier evidence was accepted")
			}
		})
	}
	if _, _, err := ResolveGateDossier(gateID, contract, nil); err == nil {
		t.Fatal("missing DOS dossier was accepted")
	}
	if _, _, err := ResolveGateDossier(gateID, contract, []GovernedArtifactEvidence{artifact, artifact}); err == nil {
		t.Fatal("ambiguous DOS dossier was accepted")
	}
}

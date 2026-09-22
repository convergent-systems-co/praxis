package goalstore

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/packages/goals"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

const testInstallationDigest = "sha256:" + "9999999999999999999999999999999999999999999999999999999999999999"

var testGovernanceScope = contracts.InstallationGovernanceScopePrefix + testInstallationDigest

func TestAuthorityGateRequestBindsExactRuntimeDossierAndAlternatives(t *testing.T) {
	contract := contracts.AuthorityGateContract{QuestionSchemaID: "q/1", Question: "choose", RequiredDossier: contracts.GateDossierRequirement{ProducerCandidateID: "dos", Role: "gate-a", EvidenceClass: "decision-dossier", SchemaID: "d/1"}, AlternativesRule: contracts.GateAlternativesFromDossier, AuthorityPrincipal: contracts.GateAuthorityHuman, DownstreamSemantics: "exact selection only", ContentAddressing: contracts.GateContentAddressingSHA256}
	dossier := contracts.GateDossier{SchemaID: "d/1", GateID: "gate-a", ProducerCandidateID: "dos", Role: "gate-a", EvidenceClass: "decision-dossier", Status: contracts.GateDossierStatusUndecided, Question: "choose", OfferedAlternatives: []string{"a", "b"}, Consequences: map[string]string{"a": "one", "b": "two"}, Evidence: []contracts.GateDossierEvidenceRef{{SourceRef: "evidence", SourceDigest: "sha256:" + strings.Repeat("a", 64)}}}
	body, _ := json.Marshal(dossier)
	sum := sha256.Sum256(body)
	artifact := contracts.GovernedArtifactEvidence{ProducerCandidateID: "dos", ProducerSpecificationHash: "sha256:" + strings.Repeat("b", 64), ValidationProfileDigest: "sha256:" + strings.Repeat("c", 64), ConformanceQualified: true, Checkpoint: "head", Role: "gate-a", EvidenceClass: "decision-dossier", SourceRef: "docs/a.json", SchemaID: "d/1", Digest: "sha256:" + hex.EncodeToString(sum[:]), Bytes: body}
	baseline := goals.GoalBaseline{ID: "goal", Version: "1", Digest: "sha256:" + strings.Repeat("d", 64), WorkPlan: &contracts.WorkPlan{Safety: &contracts.WorkPlanSafetyBinding{ActivationManifestDigest: "sha256:" + strings.Repeat("e", 64)}}}
	candidate := contracts.WorkCandidate{ID: "gate-a", SourceDigest: "sha256:" + strings.Repeat("f", 64)}
	request, err := BuildAuthorityGateRequest(baseline, candidate, contract, artifact, dossier, testGovernanceScope)
	if err != nil {
		t.Fatal(err)
	}
	if request.DossierDigest != artifact.Digest || request.DossierCheckpoint != "head" || request.DossierProducerCandidate != "dos" || len(request.Alternatives) != 2 {
		t.Fatalf("runtime lineage not bound: %+v", request)
	}
	originalDigest, _ := request.Digest()
	dossier.Recommendation = "non-binding"
	artifact.Bytes, _ = json.Marshal(dossier)
	changedSum := sha256.Sum256(artifact.Bytes)
	artifact.Digest = "sha256:" + hex.EncodeToString(changedSum[:])
	changed, err := BuildAuthorityGateRequest(baseline, candidate, contract, artifact, dossier, testGovernanceScope)
	if err != nil {
		t.Fatal(err)
	}
	changedDigest, _ := changed.Digest()
	if changed.ID == request.ID || changedDigest == originalDigest {
		t.Fatal("dossier substitution preserved request identity")
	}
	now := time.Now().UTC()
	decision := contracts.AuthorityDecision{RequestID: request.ID, RequestVersion: request.Version, RequestDigest: originalDigest, DecisionRef: "decision", DecisionVersion: "1", DecidedBy: contracts.PrincipalRef{ID: "installation-owner:" + testInstallationDigest, Kind: "human"}, AuthorityRef: testGovernanceScope, AuthorityVersion: "1", AuthorityGenerationDigest: "sha256:" + strings.Repeat("3", 64), GrantedScope: request.RequestedScope, Outcome: contracts.AuthorityApprove, AuthorityDigest: "authority", SelectedAlternative: "a", IssuedAt: now, CeremonyEvidenceDigest: "sha256:" + strings.Repeat("2", 64)}
	if err := decision.Validate(request, now); err != nil {
		t.Fatalf("original lineage rejected: %v", err)
	}
	if err := decision.Validate(changed, now); err == nil {
		t.Fatal("decision for original request accepted after dossier substitution")
	}
}

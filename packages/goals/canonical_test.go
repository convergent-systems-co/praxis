package goals

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func baselineFixture() GoalBaseline {
	return GoalBaseline{ID: "g1", Version: "1", OriginalIntent: "goal", RefinedOutcome: "outcome", Rigor: RigorStructured, RecommendationMode: RecommendationReviewAll, Constraints: []string{"b", "a"}, EvidenceRefs: []string{"e2", "e1"}, Decisions: []Decision{{ID: "d2", Statement: "two", Status: DecisionResolved}, {ID: "d1", Statement: "one", Status: DecisionResolved}}}
}

func TestAcceptedWorkPlanSurvivesCanonicalBaselineRoundTrip(t *testing.T) {
	b := baselineFixture()
	b.WorkPlan = &contracts.WorkPlan{
		BaselineDigest: "sha256:baseline",
		AuthorityRef:   "docs/PLAN/example.md#unit", AuthorityDigest: "sha256:accepted", AcceptanceRef: "acceptance:unit", AcceptanceDigest: "sha256:acceptance", AcceptedBy: contracts.PrincipalRef{ID: "reviewer", Kind: "human"}, ProposalDigest: "sha256:proposal",
		Candidates: []contracts.WorkCandidate{{ID: "unit", SourceRef: "docs/PLAN/example.md#unit", SourceDigest: "sha256:accepted", Provenance: contracts.ProvenancePLAN, Requirements: []contracts.RequirementRef{{ID: "req-1", SourceRef: "goal:requirement/1", SourceDigest: "sha256:req"}}}},
	}
	digest, err := b.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	b.Digest = digest
	payload, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	var restored GoalBaseline
	if err := json.Unmarshal(payload, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.WorkPlan == nil || restored.WorkPlan.Candidates[0].ID != "unit" {
		t.Fatalf("accepted work plan was not persisted: %+v", restored.WorkPlan)
	}
	if err := restored.VerifyDigest(); err != nil {
		t.Fatalf("round-tripped baseline digest failed: %v", err)
	}
}

func TestBaselineDigestIndependentOfSetOrdering(t *testing.T) {
	a := baselineFixture()
	b := baselineFixture()
	b.Constraints = []string{"a", "b"}
	b.EvidenceRefs = []string{"e1", "e2"}
	b.Decisions = []Decision{b.Decisions[1], b.Decisions[0]}
	da, err := a.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	db, err := b.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	if da != db {
		t.Fatalf("canonical digest changed with ordering: %s != %s", da, db)
	}
}

func TestBaselineDigestChangesWithOutcome(t *testing.T) {
	a := baselineFixture()
	b := baselineFixture()
	b.RefinedOutcome = "different"
	da, _ := a.ComputeDigest()
	db, _ := b.ComputeDigest()
	if da == db {
		t.Fatal("material baseline change must change digest")
	}
}

func TestVerifyBaselineDigestDetectsMutation(t *testing.T) {
	b := baselineFixture()
	d, _ := b.ComputeDigest()
	b.Digest = d
	b.RefinedOutcome = "mutated"
	if err := b.VerifyDigest(); !errors.Is(err, ErrBaselineDigestMismatch) {
		t.Fatalf("expected digest mismatch, got %v", err)
	}
}

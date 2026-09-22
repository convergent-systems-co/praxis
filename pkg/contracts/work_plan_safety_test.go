package contracts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func safetyDigest(body []byte) string {
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func safetyProposalFixture() WorkPlanProposal {
	reqText := []byte("requirement text")
	reqDigest := safetyDigest(reqText)
	req := RequirementRef{ID: "success_criteria:" + reqDigest, SourceRef: "goal:goal/1#success_criteria/1", SourceDigest: reqDigest, Specification: reqText}
	reqJSON := fmt.Sprintf(`{"id":%q,"source_ref":%q,"source_digest":%q}`, req.ID, req.SourceRef, req.SourceDigest)
	one := []byte(fmt.Sprintf(`{"id":"one","kind":"work","provenance":"model_proposal","priority":1,"sequence":1,"responsibility":"implement one bounded unit","exclusions":["no authority decisions"],"requirements":[%s],"qualification_predicates":["candidate/one/conformance"]}`, reqJSON))
	two := []byte(fmt.Sprintf(`{"id":"gate","kind":"authority_gate","provenance":"model_gate_proposal","priority":2,"sequence":2,"responsibility":"present one authority question","exclusions":["no provider execution"],"requirements":[%s],"question_schema_id":"gate-question/1","question":"choose","required_dossier":{"producer_candidate_id":"one","role":"gate","evidence_class":"decision_dossier","schema_id":"gate-dossier/1"},"alternatives_rule":"dossier.offered_alternatives","authority_principal_kind":"human","downstream_consequence_semantics":"only the selected alternative is authoritative","content_addressing":"sha256-exact-bytes"}`, reqJSON))
	edge := []byte(`{"dependent":"gate","prerequisite":"one","kind":"hard_dependency","provenance":"model_proposal","rationale":"gate consumes the completed unit"}`)
	p := WorkPlanProposal{
		ID: "proposal-v4", Version: "4", GoalID: "goal", GoalVersion: "1", BaselineDigest: "sha256:" + strings.Repeat("b", 64),
		ProposedBy: PrincipalRef{ID: "planner", Kind: "model"}, ProposerGeneration: "planner-generation",
		Safety: &WorkPlanSafetyBinding{KernelVersion: WorkPlanSafetyKernelVersion, ActivationManifestDigest: "sha256:" + strings.Repeat("1", 64), ValidationProfileDigest: "sha256:" + strings.Repeat("2", 64)},
		Candidates: []WorkCandidate{
			{ID: "one", Kind: WorkCandidateOrdinary, Priority: 1, Sequence: 1, SourceRef: "specs/one.json", SourceDigest: safetyDigest(one), Provenance: ProvenanceModelProposal, Requirements: []RequirementRef{req}, QualificationPredicates: []string{"candidate/one/conformance"}, Responsibility: "implement one bounded unit", Exclusions: []string{"no authority decisions"}, Specification: one},
			{ID: "gate", Kind: WorkCandidateAuthorityGate, Priority: 2, Sequence: 2, SourceRef: "specs/gate.json", SourceDigest: safetyDigest(two), Provenance: ProvenanceModelGateProposal, Requirements: []RequirementRef{req}, Responsibility: "present one authority question", Exclusions: []string{"no provider execution"}, Specification: two},
		},
		Relationships: []WorkRelationship{{Dependent: "gate", Prerequisite: "one", Kind: RelationshipHardDependency, SourceRef: "specs/edge.json", SourceDigest: safetyDigest(edge), Provenance: ProvenanceModelProposal, Specification: edge, Rationale: "gate consumes the completed unit"}},
	}
	p.Safety.SpecificationBundleDigest, _ = ComputeSpecificationBundleDigest(p.Candidates, p.Relationships)
	return p
}

func TestSafetyProposalFailsClosedForSpecificationAndGraphDefects(t *testing.T) {
	if err := safetyProposalFixture().Validate(); err != nil {
		t.Fatalf("valid fixture: %v", err)
	}
	tests := map[string]func(*WorkPlanProposal){
		"missing specification":             func(p *WorkPlanProposal) { p.Candidates[0].Specification = nil },
		"missing requirement specification": func(p *WorkPlanProposal) { p.Candidates[0].Requirements[0].Specification = nil },
		"wrong digest":                      func(p *WorkPlanProposal) { p.Candidates[0].SourceDigest = "sha256:" + strings.Repeat("f", 64) },
		"duplicate sequence":                func(p *WorkPlanProposal) { p.Candidates[1].Sequence = p.Candidates[0].Sequence },
		"duplicate endpoint":                func(p *WorkPlanProposal) { p.Relationships = append(p.Relationships, p.Relationships[0]) },
		"hard cycle": func(p *WorkPlanProposal) {
			edge := []byte(`{"dependent":"one","prerequisite":"gate","kind":"hard_dependency","provenance":"model_proposal","rationale":"cycle fixture"}`)
			p.Relationships = append(p.Relationships, WorkRelationship{Dependent: "one", Prerequisite: "gate", Kind: RelationshipHardDependency, SourceRef: "specs/reverse.json", SourceDigest: safetyDigest(edge), Provenance: ProvenanceModelProposal, Specification: edge, Rationale: "cycle fixture"})
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			p := safetyProposalFixture()
			mutate(&p)
			if err := p.Validate(); err == nil {
				t.Fatal("defect was accepted")
			}
		})
	}
}

func TestSafetyMaterializationPreservesSpecificationAndKeepsGateNonWorker(t *testing.T) {
	proposal := safetyProposalFixture()
	plan, err := MaterializeAcceptedPlanCandidate(proposal, "request", "sha256:"+strings.Repeat("4", 64))
	if err != nil {
		t.Fatal(err)
	}
	if plan.Candidates[0].SourceRef != proposal.Candidates[0].SourceRef || string(plan.Candidates[0].Specification) != string(proposal.Candidates[0].Specification) {
		t.Fatal("ordinary specification provenance changed")
	}
	if plan.Candidates[1].Provenance != ProvenanceAuthorityGate || plan.Candidates[1].Kind != WorkCandidateAuthorityGate {
		t.Fatalf("gate transition: %+v", plan.Candidates[1])
	}
}

func TestSafetyProposalRoundTripPreservesDigestAndPredicate(t *testing.T) {
	p := safetyProposalFixture()
	before, err := p.Digest()
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var restored WorkPlanProposal
	if err := json.Unmarshal(body, &restored); err != nil {
		t.Fatal(err)
	}
	after, err := restored.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if before != after || string(restored.Candidates[0].Specification) != string(p.Candidates[0].Specification) || string(restored.Candidates[0].Requirements[0].Specification) != "requirement text" {
		t.Fatal("restart round-trip changed safety evidence")
	}
}

func goalGateRequestFixture() (AuthorityRequest, PrincipalRef) {
	bootstrap := "sha256:" + strings.Repeat("8", 64)
	scope, _ := InstallationGovernanceScope(bootstrap)
	owner, _ := InstallationOwnerPrincipal(bootstrap)
	return AuthorityRequest{ID: "gate", Version: "1", BaselineID: "goal", BaselineVersion: "1", BaselineDigest: "sha256:" + strings.Repeat("a", 64), RequestedAuthority: "goal.gate.decide", RequestedScope: scope, SubjectScope: GoalGateSubjectScope("goal", "1"), Reason: "choose", Alternatives: []string{"finish and reconcile", "authenticated interrupt"}, Status: AuthorityRequestPending, CeremonyProfile: "interactive-os-owner-v1", ActivationManifestDigest: "sha256:" + strings.Repeat("b", 64), GateCandidateID: "gate-c", GateSpecificationDigest: "sha256:" + strings.Repeat("c", 64), DossierRef: "dossier", DossierDigest: "sha256:" + strings.Repeat("d", 64), DossierProducerCandidate: "dos", DossierRole: "gate-c", DossierEvidenceClass: "decision_dossier", DossierSchemaID: "gate-c/1", DossierCheckpoint: "head"}, owner
}

func TestGoalGateDecisionMustSelectExactOfferedAlternative(t *testing.T) {
	now := time.Now().UTC()
	request, owner := goalGateRequestFixture()
	digest, err := request.DigestAt(now)
	if err != nil {
		t.Fatal(err)
	}
	decision := AuthorityDecision{RequestID: "gate", RequestVersion: "1", RequestDigest: digest, DecisionRef: "decision", DecisionVersion: "1", DecidedBy: owner, AuthorityRef: request.RequestedScope, AuthorityVersion: "1", AuthorityGenerationDigest: "sha256:" + strings.Repeat("7", 64), GrantedScope: request.RequestedScope, Outcome: AuthorityApprove, AuthorityDigest: "authority", IssuedAt: now, CeremonyEvidenceDigest: "sha256:" + strings.Repeat("e", 64)}
	if err := decision.Validate(request, now); err == nil {
		t.Fatal("gate approval without exact alternative was accepted")
	}
	decision.SelectedAlternative = request.Alternatives[1]
	if err := decision.Validate(request, now); err != nil {
		t.Fatalf("exact gate alternative rejected: %v", err)
	}
	decision.DecidedBy = PrincipalRef{ID: "model", Kind: "model"}
	if err := decision.Validate(request, now); err == nil {
		t.Fatal("non-human gate decision was accepted")
	}
}

// The authority scope (who may decide) and the subject scope (what is being
// decided) are separate, and neither may be weakened or substituted.
func TestGoalGateSeparatesAuthorityScopeFromSubjectScope(t *testing.T) {
	now := time.Now().UTC()
	request, owner := goalGateRequestFixture()
	if err := request.ValidateAt(now); err != nil {
		t.Fatalf("canonical gate request rejected: %v", err)
	}
	digest, _ := request.DigestAt(now)
	approved := AuthorityDecision{RequestID: "gate", RequestVersion: "1", RequestDigest: digest, DecisionRef: "decision", DecisionVersion: "1", DecidedBy: owner, AuthorityRef: request.RequestedScope, AuthorityVersion: "1", AuthorityGenerationDigest: "sha256:" + strings.Repeat("7", 64), GrantedScope: request.RequestedScope, Outcome: AuthorityApprove, AuthorityDigest: "authority", IssuedAt: now, CeremonyEvidenceDigest: "sha256:" + strings.Repeat("e", 64), SelectedAlternative: request.Alternatives[0]}

	requestMutations := map[string]func(*AuthorityRequest){
		"goal scope used as authority scope": func(r *AuthorityRequest) { r.RequestedScope = GoalGateSubjectScope("goal", "1") },
		"arbitrary authority scope":          func(r *AuthorityRequest) { r.RequestedScope = "scope" },
		"missing subject scope":              func(r *AuthorityRequest) { r.SubjectScope = "" },
		"other goal generation subject":      func(r *AuthorityRequest) { r.SubjectScope = GoalGateSubjectScope("goal", "2") },
		"other goal subject":                 func(r *AuthorityRequest) { r.SubjectScope = GoalGateSubjectScope("other", "1") },
		"malformed governance digest":        func(r *AuthorityRequest) { r.RequestedScope = InstallationGovernanceScopePrefix + "sha256:xyz" },
	}
	for name, mutate := range requestMutations {
		t.Run("request/"+name, func(t *testing.T) {
			r := request
			mutate(&r)
			if err := r.ValidateAt(now); err == nil {
				t.Fatal("non-canonical gate request scope was accepted")
			}
		})
	}
	t.Run("subject scope is gate-only", func(t *testing.T) {
		r := AuthorityRequest{ID: "x", Version: "1", RequestedAuthority: "workplan.accept", RequestedScope: request.RequestedScope, Reason: "r", Status: AuthorityRequestPending, SubjectScope: "goal:goal/1"}
		if err := r.ValidateAt(now); err == nil {
			t.Fatal("subject scope accepted outside a goal gate")
		}
	})

	decisionMutations := map[string]func(*AuthorityDecision){
		"another installation's owner": func(d *AuthorityDecision) {
			d.DecidedBy = PrincipalRef{ID: "installation-owner:sha256:" + strings.Repeat("1", 64), Kind: "human"}
		},
		"human with arbitrary identity":  func(d *AuthorityDecision) { d.DecidedBy = PrincipalRef{ID: "owner", Kind: "human"} },
		"subject scope as granted scope": func(d *AuthorityDecision) { d.GrantedScope = GoalGateSubjectScope("goal", "1") },
		"missing root lineage":           func(d *AuthorityDecision) { d.AuthorityGenerationDigest = "" },
		"wrong alternative":              func(d *AuthorityDecision) { d.SelectedAlternative = "not offered" },
		"wrong request digest":           func(d *AuthorityDecision) { d.RequestDigest = "sha256:" + strings.Repeat("0", 64) },
	}
	for name, mutate := range decisionMutations {
		t.Run("decision/"+name, func(t *testing.T) {
			d := approved
			mutate(&d)
			if err := d.Validate(request, now); err == nil {
				t.Fatal("decision that is not the installation owner's exact decision was accepted")
			}
		})
	}
	if err := approved.Validate(request, now); err != nil {
		t.Fatalf("canonical decision rejected: %v", err)
	}
}

// Acceptance rewrites executable provenance but never the immutable
// specification bytes. Only the two sanctioned rewrites validate, and only for
// an accepted plan; a proposal remains exact.
func TestAcceptedPlanSpecificationAdmitsOnlyTheSanctionedProvenanceRewrite(t *testing.T) {
	proposal := safetyProposalFixture()
	plan, err := MaterializeAcceptedPlanCandidate(proposal, "request", "sha256:"+strings.Repeat("4", 64))
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range plan.Candidates {
		if err := candidate.ValidateSpecification(); err == nil {
			t.Fatalf("%s: proposal-time validation admitted a rewritten provenance", candidate.ID)
		}
		if err := candidate.ValidateAcceptedSpecification(); err != nil {
			t.Fatalf("%s: accepted candidate rejected: %v", candidate.ID, err)
		}
	}
	relationship := plan.Relationships[0]
	if err := relationship.ValidateSpecification(); err == nil {
		t.Fatal("proposal-time validation admitted a rewritten relationship provenance")
	}
	if err := relationship.ValidateAcceptedSpecification(); err != nil {
		t.Fatalf("accepted relationship rejected: %v", err)
	}
	if err := validateSafetyGraph(plan.Candidates, plan.Relationships, false); err != nil {
		t.Fatalf("materialized plan does not validate as an accepted plan: %v", err)
	}
	if err := validateSafetyGraph(plan.Candidates, plan.Relationships, true); err == nil {
		t.Fatal("materialized plan validated as a proposal")
	}

	forbidden := map[string]func(*WorkCandidate){
		"ordinary work promoted to gate provenance": func(c *WorkCandidate) { c.Provenance = ProvenanceAuthorityGate },
		"gate demoted to plan provenance":           func(c *WorkCandidate) { c.Provenance = ProvenancePLAN },
		"provenance from an unrelated source":       func(c *WorkCandidate) { c.Provenance = ProvenanceADR },
	}
	for name, mutate := range forbidden {
		t.Run(name, func(t *testing.T) {
			for _, candidate := range plan.Candidates {
				if (name == "gate demoted to plan provenance") != (candidate.Kind == WorkCandidateAuthorityGate) && name != "provenance from an unrelated source" {
					continue
				}
				mutated := candidate
				mutate(&mutated)
				if err := mutated.ValidateAcceptedSpecification(); err == nil {
					t.Fatalf("%s: unsanctioned provenance rewrite accepted for %s", name, candidate.ID)
				}
			}
		})
	}
}

// I9 (content half): a plan that speaks only the safety kernel's vocabulary
// (explicit kinds, preserved specification bytes, gate provenance) but carries
// no safety binding is either a downgrade attempt or malformed. It is refused at
// proposal and accepted-plan validation, whichever field carries the tell.
func TestKernelShapedContentWithoutSafetyBindingIsRefused(t *testing.T) {
	base := safetyProposalFixture()
	cases := map[string]func(*WorkPlanProposal){
		"binding removed":                func(p *WorkPlanProposal) { p.Safety = nil },
		"binding removed, kinds cleared": func(p *WorkPlanProposal) { p.Safety = nil; p.Candidates[0].Kind, p.Candidates[1].Kind = "", "" },
		"binding removed, kinds and provenance cleared": func(p *WorkPlanProposal) {
			p.Safety = nil
			for i := range p.Candidates {
				p.Candidates[i].Kind = ""
				p.Candidates[i].Provenance = ProvenanceModelProposal
			}
		},
		"binding removed, only a relationship specification remains": func(p *WorkPlanProposal) {
			p.Safety = nil
			for i := range p.Candidates {
				p.Candidates[i].Kind, p.Candidates[i].Specification = "", nil
				p.Candidates[i].Provenance = ProvenanceModelProposal
			}
		},
	}
	for name, mutate := range cases {
		t.Run("proposal/"+name, func(t *testing.T) {
			p := safetyProposalFixture()
			mutate(&p)
			if err := p.Validate(); err == nil || !strings.Contains(err.Error(), ErrKernelShapedWithoutSafety.Error()) {
				t.Fatalf("a kernel-shaped proposal without a safety binding validated: %v", err)
			}
		})
	}
	// Accepted-plan form.
	plan := WorkPlan{
		BaselineDigest: base.BaselineDigest, AuthorityRef: "a", AuthorityDigest: "d", AcceptanceRef: "r", AcceptanceDigest: "d", ProposalDigest: "d",
		AcceptedBy: PrincipalRef{ID: "owner", Kind: "human"},
		Candidates: append([]WorkCandidate(nil), base.Candidates...), Relationships: append([]WorkRelationship(nil), base.Relationships...),
	}
	if err := plan.Validate(); err == nil || !strings.Contains(err.Error(), ErrKernelShapedWithoutSafety.Error()) {
		t.Fatalf("a kernel-shaped accepted plan without a safety binding validated: %v", err)
	}
}

// Each tell of safety-kernel content refuses on its own: a plan cannot pass as a
// legacy plan by clearing all but one of them.
func TestRejectKernelShapedWithoutSafetyEachTellRefusesIndependently(t *testing.T) {
	legacy := WorkCandidate{ID: "u", SourceRef: "r", SourceDigest: "sha256:d", Provenance: ProvenancePLAN}
	if err := RejectKernelShapedWithoutSafety([]WorkCandidate{legacy}, nil); err != nil {
		t.Fatalf("control: a genuine legacy candidate was refused: %v", err)
	}
	tells := map[string]WorkCandidate{
		"only an explicit kind":              {ID: "u", Kind: WorkCandidateOrdinary, SourceRef: "r", SourceDigest: "sha256:d", Provenance: ProvenancePLAN},
		"only preserved specification bytes": {ID: "u", SourceRef: "r", SourceDigest: "sha256:d", Provenance: ProvenancePLAN, Specification: []byte("{}")},
		"only gate provenance":               {ID: "u", SourceRef: "r", SourceDigest: "sha256:d", Provenance: ProvenanceAuthorityGate},
		"only proposal-time gate provenance": {ID: "u", SourceRef: "r", SourceDigest: "sha256:d", Provenance: ProvenanceModelGateProposal},
	}
	for name, candidate := range tells {
		if err := RejectKernelShapedWithoutSafety([]WorkCandidate{candidate}, nil); err == nil {
			t.Fatalf("%s: a kernel-shaped candidate passed as legacy", name)
		}
	}
}

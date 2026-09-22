package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type manifest struct {
	Schema                  string                                                `json:"schema"`
	GoalID                  string                                                `json:"goal_id"`
	GoalVersion             string                                                `json:"goal_version"`
	GoalDigest              string                                                `json:"goal_digest"`
	FrozenPlan              string                                                `json:"frozen_plan"`
	FrozenPlanSHA256        string                                                `json:"frozen_plan_sha256"`
	CanonicalContractDigest string                                                `json:"canonical_contract_digest"`
	Counts                  struct{ Candidates, Relationships, Requirements int } `json:"counts"`
	Candidates              []candidateEntry                                      `json:"candidates"`
	Relationships           []relationshipEntry                                   `json:"relationships"`
	Requirements            []requirementEntry                                    `json:"requirements"`
}

type candidateEntry struct {
	ID           string   `json:"id"`
	Kind         string   `json:"kind"`
	Provenance   string   `json:"provenance"`
	Path         string   `json:"path"`
	SHA256       string   `json:"sha256"`
	Priority     int      `json:"priority"`
	Sequence     int      `json:"sequence"`
	Requirements []string `json:"requirements"`
}

type relationshipEntry struct {
	Dependent    string `json:"dependent"`
	Prerequisite string `json:"prerequisite"`
	Kind         string `json:"kind"`
	Provenance   string `json:"provenance"`
	Rationale    string `json:"rationale"`
	Path         string `json:"path"`
	SHA256       string `json:"sha256"`
}

type requirementEntry struct {
	ID        string `json:"id"`
	SourceRef string `json:"source_ref"`
	Path      string `json:"path"`
	SHA256    string `json:"sha256"`
}

func sum(body []byte) string {
	d := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(d[:])
}

func main() {
	root, err := os.Getwd()
	must(err)
	bundle := filepath.Join(root, "docs/research/dogfood/praxis-human-interface/bootstrap-v4/specification-bundle")
	raw, err := os.ReadFile(filepath.Join(bundle, "manifest.json"))
	must(err)
	var m manifest
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	must(decoder.Decode(&m))
	if m.Schema != "praxis-human-interface/v4-specification-bundle/1" || m.GoalID != "praxis-human-interface" || m.GoalVersion != "1" || m.Counts.Candidates != 22 || m.Counts.Relationships != 68 || m.Counts.Requirements != 31 {
		panic("manifest identity or cardinality mismatch")
	}
	plan, err := os.ReadFile(filepath.Join(root, m.FrozenPlan))
	must(err)
	if sum(plan) != m.FrozenPlanSHA256 {
		panic("frozen plan digest mismatch")
	}
	requirements := map[string]contracts.RequirementRef{}
	for _, item := range m.Requirements {
		body := readExact(bundle, item.Path, item.SHA256)
		ref := contracts.RequirementRef{ID: item.ID, SourceRef: item.SourceRef, SourceDigest: item.SHA256, Specification: body}
		must(ref.ValidateSpecification())
		if _, duplicate := requirements[item.ID]; duplicate {
			panic("duplicate requirement " + item.ID)
		}
		requirements[item.ID] = ref
	}
	var candidates []contracts.WorkCandidate
	ids, sequences := map[string]bool{}, map[int]bool{}
	coverage := map[string]bool{}
	for _, item := range m.Candidates {
		body := readExact(bundle, item.Path, item.SHA256)
		var spec struct {
			Responsibility          string                     `json:"responsibility"`
			Exclusions              []string                   `json:"exclusions"`
			QualificationPredicates []string                   `json:"qualification_predicates"`
			Requirements            []contracts.RequirementRef `json:"requirements"`
		}
		must(json.Unmarshal(body, &spec))
		if ids[item.ID] || sequences[item.Sequence] {
			panic("duplicate candidate identity or sequence")
		}
		ids[item.ID], sequences[item.Sequence] = true, true
		var refs []contracts.RequirementRef
		for i, id := range item.Requirements {
			ref, ok := requirements[id]
			if !ok || i >= len(spec.Requirements) || spec.Requirements[i].ID != ref.ID || spec.Requirements[i].SourceRef != ref.SourceRef || spec.Requirements[i].SourceDigest != ref.SourceDigest {
				panic("candidate requirement mismatch: " + item.ID)
			}
			refs = append(refs, ref)
			coverage[id] = true
		}
		candidate := contracts.WorkCandidate{ID: item.ID, Kind: contracts.WorkCandidateKind(item.Kind), Priority: item.Priority, Sequence: item.Sequence, SourceRef: filepath.ToSlash(filepath.Join("docs/research/dogfood/praxis-human-interface/bootstrap-v4/specification-bundle", item.Path)), SourceDigest: item.SHA256, Provenance: contracts.RelationshipProvenance(item.Provenance), Requirements: refs, QualificationPredicates: spec.QualificationPredicates, Responsibility: spec.Responsibility, Exclusions: spec.Exclusions, Specification: body}
		must(candidate.ValidateSpecification())
		candidates = append(candidates, candidate)
	}
	if len(coverage) != 31 {
		panic(fmt.Sprintf("authoritative coverage is %d/31", len(coverage)))
	}
	var relationships []contracts.WorkRelationship
	endpoints := map[string]bool{}
	kindCounts := map[string]int{}
	for _, item := range m.Relationships {
		body := readExact(bundle, item.Path, item.SHA256)
		key := item.Dependent + "\x00" + item.Prerequisite
		if endpoints[key] || !ids[item.Dependent] || !ids[item.Prerequisite] {
			panic("duplicate or unknown relationship endpoint")
		}
		endpoints[key] = true
		kindCounts[item.Kind]++
		relationship := contracts.WorkRelationship{Dependent: item.Dependent, Prerequisite: item.Prerequisite, Kind: contracts.WorkRelationshipKind(item.Kind), SourceRef: filepath.ToSlash(filepath.Join("docs/research/dogfood/praxis-human-interface/bootstrap-v4/specification-bundle", item.Path)), SourceDigest: item.SHA256, Provenance: contracts.RelationshipProvenance(item.Provenance), Specification: body, Rationale: item.Rationale}
		must(relationship.ValidateSpecification())
		relationships = append(relationships, relationship)
	}
	if kindCounts["hard_dependency"] != 28 || kindCounts["consumer"] != 24 || kindCounts["interaction"] != 13 || kindCounts["advisory"] != 3 {
		panic(fmt.Sprintf("relationship kind counts mismatch: %+v", kindCounts))
	}
	digest, err := contracts.ComputeSpecificationBundleDigest(candidates, relationships)
	must(err)
	if m.CanonicalContractDigest != "PENDING_GO_VERIFIER" && m.CanonicalContractDigest != digest {
		panic(fmt.Sprintf("canonical contract digest mismatch: got %s want %s", digest, m.CanonicalContractDigest))
	}
	proposal := contracts.WorkPlanProposal{ID: "verification-only-not-materialized", Version: "4", GoalID: m.GoalID, GoalVersion: m.GoalVersion, BaselineDigest: m.GoalDigest, ProposedBy: contracts.PrincipalRef{ID: "bundle-verifier", Kind: "model"}, ProposerGeneration: "verification-only", Candidates: candidates, Relationships: relationships, Safety: &contracts.WorkPlanSafetyBinding{KernelVersion: contracts.WorkPlanSafetyKernelVersion, ActivationManifestDigest: "sha256:" + strings.Repeat("1", 64), ValidationProfileDigest: "sha256:" + strings.Repeat("2", 64), SpecificationBundleDigest: digest}}
	must(proposal.Validate())
	reversedCandidates := append([]contracts.WorkCandidate(nil), candidates...)
	reversedRelationships := append([]contracts.WorkRelationship(nil), relationships...)
	for left, right := 0, len(reversedCandidates)-1; left < right; left, right = left+1, right-1 {
		reversedCandidates[left], reversedCandidates[right] = reversedCandidates[right], reversedCandidates[left]
	}
	for left, right := 0, len(reversedRelationships)-1; left < right; left, right = left+1, right-1 {
		reversedRelationships[left], reversedRelationships[right] = reversedRelationships[right], reversedRelationships[left]
	}
	permuted, err := contracts.ComputeSpecificationBundleDigest(reversedCandidates, reversedRelationships)
	must(err)
	if permuted != digest {
		panic("bundle digest depends on record order")
	}
	verifyDOGClosure(candidates, relationships)
	result := map[string]any{"status": "PASS", "canonical_contract_digest": digest, "manifest_file_sha256": sum(raw), "candidates": len(candidates), "relationships": len(relationships), "requirements": len(requirements), "relationship_kinds": kindCounts, "coverage": "31/31", "deterministic_permutation": true}
	encoded, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(encoded))
}

func readExact(bundle, rel, expected string) []byte {
	if filepath.IsAbs(rel) || strings.Contains(rel, ".claude") || strings.HasPrefix(rel, "/tmp") || strings.HasPrefix(rel, "../") || strings.Contains(rel, "provider-private") {
		panic("non-repository or private specification path: " + rel)
	}
	body, err := os.ReadFile(filepath.Join(bundle, filepath.Clean(rel)))
	must(err)
	if sum(body) != expected {
		panic("record digest mismatch: " + rel)
	}
	return body
}

func verifyDOGClosure(candidates []contracts.WorkCandidate, relationships []contracts.WorkRelationship) {
	const dog = "unit:hi-dogfood-acceptance"
	adj := map[string][]string{}
	for _, edge := range relationships {
		if edge.Kind == contracts.RelationshipHardDependency {
			adj[edge.Dependent] = append(adj[edge.Dependent], edge.Prerequisite)
		}
	}
	seen := map[string]bool{}
	var visit func(string)
	visit = func(id string) {
		for _, next := range adj[id] {
			if !seen[next] {
				seen[next] = true
				visit(next)
			}
		}
	}
	visit(dog)
	for _, candidate := range candidates {
		if candidate.ID != dog && !seen[candidate.ID] {
			panic("candidate outside DOG hard closure: " + candidate.ID)
		}
	}
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

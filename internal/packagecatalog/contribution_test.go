package packagecatalog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/internal/transfer"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type contributionAuthorizer struct{}

func (contributionAuthorizer) AuthorizeTransfer(_ context.Context, operation string, authority contracts.PrincipalRef, target, evidenceRef string) error {
	if operation != "publish" || authority.ID != "catalog-governor" || target != "public:catalog" || evidenceRef != "approval:catalog" {
		return errors.New("publication denied")
	}
	return nil
}

type contributionProcessor struct {
	content []byte
	now     time.Time
}

func (p contributionProcessor) GeneralizeAndSanitize(_ context.Context, request transfer.Request, policy transfer.TransferPolicy) (transfer.Artifact, error) {
	return transfer.Artifact{RequestID: request.ID, Kind: request.ArtifactKind, Scope: request.TargetScope, ContentRef: "artifact://catalog/reusable", ContentDigest: bytesDigest(p.content), SourceRefs: request.Sources, GeneralizerID: policy.GeneralizerID, GeneralizerVersion: policy.GeneralizerVersion, SanitizerID: policy.SanitizerID, SanitizerVersion: policy.SanitizerVersion, SanitizationEvidenceRefs: []string{"strip-private-context"}, CreatedAt: p.now}, nil
}

func (p contributionProcessor) Evaluate(_ context.Context, artifact transfer.Artifact, policy transfer.TransferPolicy) (transfer.Evaluation, error) {
	return transfer.Evaluation{ArtifactID: artifact.ID, EvaluatorID: policy.EvaluatorID, EvaluatorVersion: policy.EvaluatorVersion, IndependentRoots: []string{"run:one", "run:two"}, Invariants: []transfer.InvariantResult{{Class: "privacy", ControlID: "strip-private-context", Passed: true, EvidenceRef: "privacy-report:clean"}, {Class: "security", ControlID: "no-authority-content", Passed: true, EvidenceRef: "security-report:clean"}}, Accepted: true, EvaluatedAt: p.now.Add(time.Minute)}, nil
}

func TestCatalogContributionRequiresGovernedGeneralizedPublication(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 14, 4, 0, 0, 0, time.UTC)
	privateBytes := []byte("private participant Alice credential SECRET-RESEARCH")
	generalizedBytes := []byte("triangulate claims with two independent sources")
	policy, err := transfer.FreezePolicy(transfer.TransferPolicy{PackageID: "research/local-descendant", SourceScope: "human:private", TargetScope: "public:catalog", GeneralizerID: "research.generalizer", GeneralizerVersion: "2", SanitizerID: "research.sanitizer", SanitizerVersion: "3", EvaluatorID: "research.evaluator", EvaluatorVersion: "4", RequiredPrivacyControls: []string{"strip-private-context"}, MinIndependentRoots: 2})
	if err != nil {
		t.Fatal(err)
	}
	request, err := transfer.FreezeRequest(transfer.Request{Proposer: contracts.PrincipalRef{ID: "research-agent", Kind: "agent"}, SourceScope: policy.SourceScope, TargetScope: policy.TargetScope, ArtifactKind: "research-procedure", Sources: []transfer.SourceReference{{AgentID: "research-agent", GenerationID: "generation-7", MemoryID: "private-memory-1", ContentDigest: bytesDigest(privateBytes), CausationRoot: "run:one"}, {AgentID: "research-agent", GenerationID: "generation-7", MemoryID: "private-memory-2", ContentDigest: "sha256:independent-private-source", CausationRoot: "run:two"}}, Policy: policy, RequestedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	lifecycle, err := transfer.NewLifecycle(eventstore.NewMemoryStore(), contributionAuthorizer{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lifecycle.Propose(ctx, request, policy, contributionProcessor{content: generalizedBytes, now: now}); err != nil {
		t.Fatal(err)
	}
	if _, err := lifecycle.ResolvePublished(ctx, request.ID); err == nil {
		t.Fatal("evaluated but unpublished artifact became catalog input")
	}
	if _, err := lifecycle.Publish(ctx, request.ID, contracts.PrincipalRef{ID: "catalog-governor", Kind: "governance"}, "approval:catalog", now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	published, err := lifecycle.ResolvePublished(ctx, request.ID)
	if err != nil {
		t.Fatal(err)
	}
	contribution, err := BuildCatalogContribution(CatalogContributionSpec{PackageID: "catalog/research-procedure", PackageVersion: "1", Publisher: "research-team", ContentKind: ContentTemplate, ContentID: "research.triangulation", ContentVersion: "1", ArtifactPath: "templates/triangulation.txt", PackagingPolicyID: "research.package-map", PackagingPolicyVersion: "1"}, published, generalizedBytes)
	if err != nil {
		t.Fatal(err)
	}
	serialized := append([]byte(nil), contribution.Artifact...)
	manifestBytes, _ := json.Marshal(contribution.Manifest)
	serialized = append(serialized, manifestBytes...)
	evidenceBytes, _ := json.Marshal(contribution.Evidence)
	serialized = append(serialized, evidenceBytes...)
	for _, forbidden := range [][]byte{privateBytes, []byte("private-memory-1"), []byte("research-agent"), []byte("generation-7")} {
		if bytes.Contains(serialized, forbidden) {
			t.Fatalf("private personalized state entered catalog package bytes: %q", forbidden)
		}
	}
	verified := verifiedFixture(t, contribution.Manifest, contribution.Artifact, nil)
	generalizedRef, ok := contribution.Manifest.Content(ContentTemplate, "research.triangulation", "1")
	if !ok {
		t.Fatal("catalog contribution omitted the package-policy-mapped generalized content")
	}
	provenanceRef, ok := contribution.Manifest.Content(ContentDocumentation, "research.triangulation.catalog-provenance", "1")
	if !ok {
		t.Fatal("catalog contribution omitted its safe provenance sidecar")
	}
	reordered := contribution.Manifest
	reordered.Contents = append([]ContentRef(nil), contribution.Manifest.Contents...)
	slices.Reverse(reordered.Contents)
	if err := validateCatalogContributionOutput(reordered, contribution.Artifact, generalizedRef, provenanceRef); err != nil {
		t.Fatalf("manifest position became catalog content identity: %v", err)
	}
	empty := contribution.Manifest
	empty.Contents = nil
	if err := validateCatalogContributionOutput(empty, contribution.Artifact, generalizedRef, provenanceRef); err == nil {
		t.Fatal("catalog contribution succeeded without its required semantic contents")
	}
	content, err := verified.ContentBytes(generalizedRef)
	if err != nil || !bytes.Equal(content, generalizedBytes) {
		t.Fatalf("new installation could not bootstrap generalized behavior: %q %v", content, err)
	}
	if _, err := BuildCatalogContribution(CatalogContributionSpec{PackageID: "catalog/research-procedure", PackageVersion: "1", Publisher: "research-team", ContentKind: ContentTemplate, ContentID: "research.triangulation", ContentVersion: "1", ArtifactPath: "templates/triangulation.txt", PackagingPolicyID: "research.package-map", PackagingPolicyVersion: "1"}, published, privateBytes); err == nil {
		t.Fatal("raw private source bytes substituted for the published generalized artifact")
	}
	if _, err := BuildCatalogContribution(CatalogContributionSpec{}, transfer.PublishedArtifact{}, generalizedBytes); err == nil {
		t.Fatal("caller-authored publication state minted a catalog contribution")
	}
}

func TestCatalogContributionPreservesTwoDomainPackagePolicyMappings(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 14, 5, 0, 0, 0, time.UTC)
	domains := []struct {
		name, sourcePackage, artifactKind, packageID, contentID, path string
		kind                                                          ContentKind
		generalized                                                   []byte
	}{
		{name: "software-delivery", sourcePackage: "delivery/local", artifactKind: "verification-graph", packageID: "catalog/delivery-verification", contentID: "delivery.verify", path: "graphs/verify.json", kind: ContentGraph, generalized: []byte(`{"ID":"delivery.verify","Version":"1","EntryNode":"done","Nodes":[{"ID":"done","Class":"terminal","TerminalState":"succeeded"}]}`)},
		{name: "research", sourcePackage: "research/local", artifactKind: "source-procedure", packageID: "catalog/research-sources", contentID: "research.sources", path: "templates/sources.txt", kind: ContentTemplate, generalized: []byte("triangulate claims across independent source classes")},
	}
	for _, domain := range domains {
		t.Run(domain.name, func(t *testing.T) {
			policy, err := transfer.FreezePolicy(transfer.TransferPolicy{PackageID: domain.sourcePackage, SourceScope: domain.name + ":private", TargetScope: "public:catalog", GeneralizerID: domain.name + ".generalize", GeneralizerVersion: "1", SanitizerID: domain.name + ".sanitize", SanitizerVersion: "1", EvaluatorID: domain.name + ".evaluate", EvaluatorVersion: "1", RequiredPrivacyControls: []string{"strip-private-context"}, MinIndependentRoots: 2})
			if err != nil {
				t.Fatal(err)
			}
			request, err := transfer.FreezeRequest(transfer.Request{Proposer: contracts.PrincipalRef{ID: domain.name + "-agent", Kind: "agent"}, SourceScope: policy.SourceScope, TargetScope: policy.TargetScope, ArtifactKind: domain.artifactKind, Sources: []transfer.SourceReference{{AgentID: domain.name + "-agent", GenerationID: "generation-1", MemoryID: "private-one", ContentDigest: "sha256:private-one", CausationRoot: "run:one"}, {AgentID: domain.name + "-agent", GenerationID: "generation-1", MemoryID: "private-two", ContentDigest: "sha256:private-two", CausationRoot: "run:two"}}, Policy: policy, RequestedAt: now})
			if err != nil {
				t.Fatal(err)
			}
			lifecycle, _ := transfer.NewLifecycle(eventstore.NewMemoryStore(), contributionAuthorizer{})
			if _, err := lifecycle.Propose(ctx, request, policy, contributionProcessor{content: domain.generalized, now: now}); err != nil {
				t.Fatal(err)
			}
			if _, err := lifecycle.Publish(ctx, request.ID, contracts.PrincipalRef{ID: "catalog-governor", Kind: "governance"}, "approval:catalog", now.Add(time.Minute)); err != nil {
				t.Fatal(err)
			}
			published, err := lifecycle.ResolvePublished(ctx, request.ID)
			if err != nil {
				t.Fatal(err)
			}
			contribution, err := BuildCatalogContribution(CatalogContributionSpec{PackageID: domain.packageID, PackageVersion: "1", Publisher: domain.name + "-team", ContentKind: domain.kind, ContentID: domain.contentID, ContentVersion: "1", ArtifactPath: domain.path, PackagingPolicyID: domain.name + ".package-map", PackagingPolicyVersion: "1"}, published, domain.generalized)
			if err != nil {
				t.Fatal(err)
			}
			mapped, ok := contribution.Manifest.Content(domain.kind, domain.contentID, "1")
			if !ok || mapped.Artifact != domain.path || contribution.Evidence.PackagingPolicyID != domain.name+".package-map" {
				t.Fatalf("core replaced package-owned domain mapping semantics: %+v", contribution)
			}
		})
	}
}

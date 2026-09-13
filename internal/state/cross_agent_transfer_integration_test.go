package state_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/internal/transfer"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type transferAuthority struct{}

func (transferAuthority) AuthorizeTransfer(_ context.Context, operation string, authority contracts.PrincipalRef, target, evidenceRef string) error {
	if authority.Kind != "governance" || authority.ID != "governor" || evidenceRef != "approval:"+operation+":"+target {
		return errors.New("transfer authority denied")
	}
	return nil
}

type domainTransferProcessor struct {
	contentRef    string
	contentDigest string
	privacy       string
	now           time.Time
	reject        bool
}

func (p domainTransferProcessor) GeneralizeAndSanitize(_ context.Context, request transfer.Request, policy transfer.TransferPolicy) (transfer.Artifact, error) {
	return transfer.Artifact{RequestID: request.ID, Kind: request.ArtifactKind, Scope: request.TargetScope, ContentRef: p.contentRef, ContentDigest: p.contentDigest, SourceRefs: request.Sources, GeneralizerID: policy.GeneralizerID, GeneralizerVersion: policy.GeneralizerVersion, SanitizerID: policy.SanitizerID, SanitizerVersion: policy.SanitizerVersion, SanitizationEvidenceRefs: []string{p.privacy, "scan:no-credentials"}, CreatedAt: p.now}, nil
}

func (p domainTransferProcessor) Evaluate(_ context.Context, artifact transfer.Artifact, policy transfer.TransferPolicy) (transfer.Evaluation, error) {
	results := []transfer.InvariantResult{{Class: "privacy", ControlID: p.privacy, Passed: !p.reject, EvidenceRef: "report:" + p.privacy}, {Class: "security", ControlID: "no-authority-content", Passed: true, EvidenceRef: "report:no-authority-content"}}
	return transfer.Evaluation{ArtifactID: artifact.ID, EvaluatorID: policy.EvaluatorID, EvaluatorVersion: policy.EvaluatorVersion, IndependentRoots: []string{"run:one", "run:two"}, Invariants: results, Accepted: true, EvaluatedAt: p.now.Add(time.Minute)}, nil
}

func TestCrossAgentTransferGeneralizesSanitizesPublishesAndAdoptsAcrossDomains(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 13, 22, 0, 0, 0, time.UTC)
	domains := []struct {
		name, packageID, kind, sourceScope, targetScope, sourceAgent, sourceGeneration, sourceMemory, sourceDigest, privateText, contentRef, contentDigest, privacyControl, targetAgent, targetGeneration, receivingRecord string
	}{
		{"software-delivery", "package.delivery", "verification-procedure", "project:private-repo", "organization:delivery", "agent-delivery-source", "generation-delivery-7", "memory-private-build-42", "sha256:private-build-transcript", "customer repository /Users/private/acme and token SECRET-DELIVERY", "artifact://delivery/verify-change-v3", "sha256:generalized-verify-change-v3", "strip-project-and-credential-context", "agent-delivery-receiver", "generation-delivery-2", "memory-derived-verification"},
		{"research", "package.research", "source-triangulation-rule", "human:private-reading", "team:research", "agent-research-source", "generation-research-4", "memory-private-notes-18", "sha256:private-reading-notes", "private participant Alice and credential SECRET-RESEARCH", "artifact://research/triangulate-v2", "sha256:generalized-triangulate-v2", "strip-human-and-participant-context", "agent-research-receiver", "generation-research-9", "memory-derived-triangulation"},
	}
	for _, domain := range domains {
		t.Run(domain.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "praxis.db")
			db, err := state.OpenSQLite(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			lifecycle, err := transfer.NewLifecycle(state.NewSQLiteEventStore(db), transferAuthority{})
			if err != nil {
				t.Fatal(err)
			}
			policy, err := transfer.FreezePolicy(transfer.TransferPolicy{PackageID: domain.packageID, SourceScope: domain.sourceScope, TargetScope: domain.targetScope, GeneralizerID: domain.packageID + ".generalizer", GeneralizerVersion: "3", SanitizerID: domain.packageID + ".privacy", SanitizerVersion: "4", EvaluatorID: domain.packageID + ".transfer-evaluator", EvaluatorVersion: "2", RequiredPrivacyControls: []string{domain.privacyControl}, MinIndependentRoots: 2})
			if err != nil {
				t.Fatal(err)
			}
			request, err := transfer.FreezeRequest(transfer.Request{Proposer: contracts.PrincipalRef{ID: domain.sourceAgent, Kind: "agent"}, SourceScope: domain.sourceScope, TargetScope: domain.targetScope, ArtifactKind: domain.kind, Sources: []transfer.SourceReference{{AgentID: domain.sourceAgent, GenerationID: domain.sourceGeneration, MemoryID: domain.sourceMemory, ContentDigest: domain.sourceDigest, CausationRoot: "run:one"}, {AgentID: domain.sourceAgent, GenerationID: domain.sourceGeneration, MemoryID: domain.sourceMemory + "-independent", ContentDigest: domain.sourceDigest + "-independent", CausationRoot: "run:two"}}, Policy: policy, RequestedAt: now})
			if err != nil {
				t.Fatal(err)
			}
			aggregate, err := lifecycle.Propose(ctx, request, policy, domainTransferProcessor{contentRef: domain.contentRef, contentDigest: domain.contentDigest, privacy: domain.privacyControl, now: now})
			if err != nil {
				t.Fatal(err)
			}
			if aggregate.Artifact.ContentRef != domain.contentRef || aggregate.Artifact.Scope != domain.targetScope || aggregate.Artifact.GeneralizerID != domain.packageID+".generalizer" || aggregate.Evaluation.EvaluatorID != domain.packageID+".transfer-evaluator" || !aggregate.Evaluation.Accepted {
				t.Fatalf("generalized candidate lost package semantics: %#v", aggregate)
			}
			if aggregate.Artifact.ContentDigest == domain.sourceDigest || aggregate.Artifact.SourceRefs[0].MemoryID != domain.sourceMemory {
				t.Fatalf("artifact either copied raw content or lost source provenance: %#v", aggregate.Artifact)
			}
			if _, err := lifecycle.Publish(ctx, request.ID, request.Proposer, "approval:publish:"+domain.targetScope, now.Add(2*time.Minute)); err == nil {
				t.Fatal("candidate published itself")
			}
			if _, err := lifecycle.Publish(ctx, request.ID, contracts.PrincipalRef{ID: "intruder", Kind: "governance"}, "approval:publish:"+domain.targetScope, now.Add(2*time.Minute)); err == nil {
				t.Fatal("unauthorized publication succeeded")
			}
			publication, err := lifecycle.Publish(ctx, request.ID, contracts.PrincipalRef{ID: "governor", Kind: "governance"}, "approval:publish:"+domain.targetScope, now.Add(2*time.Minute))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := lifecycle.Adopt(ctx, request.ID, domain.targetAgent, domain.targetGeneration, domain.receivingRecord, contracts.PrincipalRef{ID: domain.targetAgent, Kind: "agent"}, "approval:adopt:"+domain.targetAgent, now.Add(3*time.Minute)); err == nil {
				t.Fatal("receiving agent authorized its own adoption")
			}
			if _, err := lifecycle.Adopt(ctx, request.ID, domain.targetAgent, domain.targetGeneration, domain.receivingRecord, contracts.PrincipalRef{ID: "intruder", Kind: "governance"}, "approval:adopt:"+domain.targetAgent, now.Add(3*time.Minute)); err == nil {
				t.Fatal("unauthorized adoption succeeded")
			}
			adoption, err := lifecycle.Adopt(ctx, request.ID, domain.targetAgent, domain.targetGeneration, domain.receivingRecord, contracts.PrincipalRef{ID: "governor", Kind: "governance"}, "approval:adopt:"+domain.targetAgent, now.Add(3*time.Minute))
			if err != nil {
				t.Fatal(err)
			}
			if adoption.PublicationID != publication.ID || adoption.ArtifactID != aggregate.Artifact.ID || adoption.TargetAgentID != domain.targetAgent || adoption.TargetGenerationID != domain.targetGeneration || adoption.ReceivingRecordID != domain.receivingRecord || adoption.TransferMechanismRef != policy.ID || adoption.Trust != contracts.TrustDerived {
				t.Fatalf("receiving-agent derivation record lost transfer lineage: %#v", adoption)
			}
			if !contains(adoption.SourceMemoryIDs, domain.sourceMemory) || !contains(adoption.SourceGenerationIDs, domain.sourceGeneration) {
				t.Fatalf("adoption does not retain source memory/generation lineage: %#v", adoption)
			}

			// Transfer events contain references only. Even a package-specific private
			// episode supplied to the fixture cannot enter authoritative transfer bytes.
			events, err := state.NewSQLiteEventStore(db).LoadAggregate(ctx, "knowledge-transfer:"+request.ID, 0)
			if err != nil {
				t.Fatal(err)
			}
			for _, event := range events {
				if strings.Contains(string(event.Payload), domain.privateText) || strings.Contains(string(event.Payload), "SECRET-") {
					t.Fatalf("raw private episode entered transfer event %s", event.Type)
				}
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			db, err = state.OpenSQLite(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			restarted, err := transfer.NewLifecycle(state.NewSQLiteEventStore(db), transferAuthority{})
			if err != nil {
				t.Fatal(err)
			}
			replayed, err := restarted.Inspect(ctx, request.ID)
			if err != nil {
				t.Fatal(err)
			}
			if replayed.Request.ID != request.ID || replayed.Artifact.ID != aggregate.Artifact.ID || replayed.Evaluation.ID != aggregate.Evaluation.ID || replayed.Publication.ID != publication.ID {
				t.Fatalf("restart changed transfer identities: %#v", replayed)
			}
			found := false
			for _, record := range replayed.Adoptions {
				if record.ID == adoption.ID && record.ReceivingRecordID == domain.receivingRecord {
					found = true
				}
			}
			if !found {
				t.Fatalf("restart lost semantic adoption record %s", adoption.ID)
			}
		})
	}
}

func TestCrossAgentTransferRetainsRejectedPrivacyCandidate(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 13, 23, 0, 0, 0, time.UTC)
	db, err := state.OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	lifecycle, _ := transfer.NewLifecycle(state.NewSQLiteEventStore(db), transferAuthority{})
	policy, _ := transfer.FreezePolicy(transfer.TransferPolicy{PackageID: "package.household", SourceScope: "human:private", TargetScope: "household:shared", GeneralizerID: "household.generalize", GeneralizerVersion: "1", SanitizerID: "household.sanitize", SanitizerVersion: "1", EvaluatorID: "household.evaluate", EvaluatorVersion: "1", RequiredPrivacyControls: []string{"strip-human-identifiers"}, MinIndependentRoots: 2})
	request, _ := transfer.FreezeRequest(transfer.Request{Proposer: contracts.PrincipalRef{ID: "household-source", Kind: "agent"}, SourceScope: policy.SourceScope, TargetScope: policy.TargetScope, ArtifactKind: "maintenance-procedure", Sources: []transfer.SourceReference{{AgentID: "household-source", GenerationID: "generation-1", MemoryID: "private-episode-1", ContentDigest: "sha256:raw-one", CausationRoot: "run:one"}, {AgentID: "household-source", GenerationID: "generation-1", MemoryID: "private-episode-2", ContentDigest: "sha256:raw-two", CausationRoot: "run:two"}}, Policy: policy, RequestedAt: now})
	if _, err := lifecycle.Propose(ctx, request, policy, domainTransferProcessor{contentRef: "sha256:raw-one", contentDigest: "sha256:raw-one", privacy: "strip-human-identifiers", now: now}); err == nil {
		t.Fatal("raw episodic source was accepted as transfer artifact")
	}
	aggregate, err := lifecycle.Propose(ctx, request, policy, domainTransferProcessor{contentRef: "artifact://household/maintenance", contentDigest: "sha256:generalized-household", privacy: "strip-human-identifiers", now: now, reject: true})
	if err != nil {
		t.Fatal(err)
	}
	if aggregate.Evaluation.Accepted {
		t.Fatal("failed privacy invariant was averaged away")
	}
	if _, err := lifecycle.Publish(ctx, request.ID, contracts.PrincipalRef{ID: "governor", Kind: "governance"}, "approval:publish:"+policy.TargetScope, now.Add(time.Minute)); err == nil {
		t.Fatal("privacy-rejected candidate published")
	}
	retained, err := lifecycle.Inspect(ctx, request.ID)
	if err != nil {
		t.Fatal(err)
	}
	if retained.Artifact.ID != aggregate.Artifact.ID || retained.Evaluation.ID != aggregate.Evaluation.ID || retained.Evaluation.Invariants[0].Passed {
		t.Fatalf("rejected candidate or privacy evidence was discarded: %#v", retained)
	}
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

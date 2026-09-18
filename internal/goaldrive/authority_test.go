package goaldrive

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type authorityReaderFixture struct {
	requests []contracts.AuthorityRequest
}

func (f authorityReaderFixture) PendingAuthorityRequests(context.Context, string, string, time.Time) ([]contracts.AuthorityRequest, error) {
	return f.requests, nil
}

func authorityCandidate(id string, completed bool) contracts.WorkCandidate {
	return contracts.WorkCandidate{ID: id, Completed: completed, SourceRef: "docs/PLAN/example.md#" + id, SourceDigest: "sha256:source", Provenance: contracts.ProvenancePLAN}
}

func TestControllerSurfacesPendingAuthorityOnlyWhenNoRunnableSiblingExists(t *testing.T) {
	req := turnRequest()
	req.ChildObjective = ""
	req.WorkCandidates = []contracts.WorkCandidate{authorityCandidate("blocked", false)}
	req.WorkRelationships = []contracts.WorkRelationship{{Dependent: "blocked", Prerequisite: "missing", Kind: contracts.RelationshipHardDependency, SourceRef: "docs/PLAN/example.md", SourceDigest: "sha256:plan", Provenance: contracts.ProvenancePLAN}}
	controller := Controller{Ledger: controllerFixture(&fakeWorker{}).Ledger, Worker: &fakeWorker{}, AuthorityRequests: authorityReaderFixture{requests: []contracts.AuthorityRequest{{ID: "request-1"}}}}
	var required *AuthorityRequiredError
	if _, err := controller.ExecuteTurn(context.Background(), req); !errors.As(err, &required) || len(required.Requests) != 1 {
		t.Fatalf("pending authority was not surfaced at the no-runnable boundary: err=%v", err)
	}

	worker := &fakeWorker{result: WorkerResult{Outcome: OutcomeComplete, EndHead: "b", CheckpointValid: true}}
	controller = Controller{Ledger: controllerFixture(worker).Ledger, Worker: worker, AuthorityRequests: authorityReaderFixture{requests: []contracts.AuthorityRequest{{ID: "request-1"}}}}
	req.WorkCandidates = []contracts.WorkCandidate{authorityCandidate("blocked", false), authorityCandidate("ready", false)}
	req.WorkRelationships = []contracts.WorkRelationship{{Dependent: "blocked", Prerequisite: "missing", Kind: contracts.RelationshipHardDependency, SourceRef: "docs/PLAN/example.md", SourceDigest: "sha256:plan", Provenance: contracts.ProvenancePLAN}}
	if _, err := controller.ExecuteTurn(context.Background(), req); err != nil {
		t.Fatalf("unrelated runnable sibling was incorrectly stopped for authority: %v", err)
	}
	if worker.called != 1 {
		t.Fatalf("expected exactly one runnable sibling execution, got %d", worker.called)
	}
}

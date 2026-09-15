package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/user"
	"strings"
	"time"

	"github.com/convergent-systems-co/praxis/internal/goalstore"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func runPublisherAuthorityProposalPreview(args []string, out io.Writer) error {
	f := flag.NewFlagSet("publisher authority-proposal-preview", flag.ContinueOnError)
	generationDigest := f.String("publisher-generation-digest", "", "exact enrolled PublisherGeneration digest")
	namespace := f.String("namespace", "praxis.package", "exact package namespace")
	expiresAt := f.String("expires-at", "", "owner-selected RFC3339 expiry")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 || *generationDigest == "" || *expiresAt == "" {
		return errors.New("usage: praxis publisher authority-proposal-preview --publisher-generation-digest <digest> --namespace <namespace> --expires-at <RFC3339>")
	}
	expires, err := time.Parse(time.RFC3339Nano, *expiresAt)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	repo, db, record, err := openGovernedRepositoryReadOnly(context.Background(), os.Getenv)
	if err != nil {
		return err
	}
	defer db.Close()
	bootstrapDigest, err := record.Digest()
	if err != nil {
		return err
	}
	owner, err := contracts.InstallationOwnerPrincipal(bootstrapDigest)
	if err != nil {
		return err
	}
	model, err := repo.LoadAuthorityModelState(context.Background(), now)
	if err != nil || model.ActiveVersion != contracts.AuthorityModelSuccessorVersion {
		return errors.New("package.publish proposal requires authority-model v2")
	}
	publisherRecord, err := state.New(db).PublisherGeneration(context.Background(), *generationDigest)
	if err != nil {
		return err
	}
	if publisherRecord.Generation.PackageNamespace != *namespace {
		return errors.New("proposal namespace does not match enrolled publisher")
	}
	scope, err := contracts.PackagePublishScope(*namespace)
	if err != nil {
		return err
	}
	root, err := currentInstallationRoot(context.Background(), repo, owner, now)
	if err != nil {
		return err
	}
	proposal := contracts.PublisherAuthorityProposal{ID: "publisher-authority-proposal:" + *generationDigest + ":" + *namespace, Version: "1", Kind: contracts.PublisherAuthorityProposalKind, BootstrapDigest: bootstrapDigest, OwnerID: owner.ID, OwnerKind: owner.Kind, AuthorityModel: contracts.AuthorityModelID, AuthorityModelVersion: model.ActiveVersion, AuthorityModelDigest: model.ActiveDigest, PublisherGenerationDigest: publisherRecord.Digest, PublisherPrincipal: publisherRecord.Generation.Principal.ID, PublicKeyDigest: publisherRecord.Generation.PublicKeyDigest, Namespace: *namespace, ParentRef: root.Ref, ParentVersion: root.Version, ParentDigest: root.Digest, Capability: contracts.GovernedPackagePublish, Scope: scope, Reason: "first-party package publishing", ExpiresAt: expires.UTC(), CreatedAt: now}
	digest, err := proposal.Digest()
	if err != nil {
		return err
	}
	return printJSONTo(out, map[string]any{"operation": "publisher.authority-proposal", "preview": true, "proposal": proposal, "proposal_digest": digest})
}

func runPublisherAuthorityProposal(args []string, getenv func(string) string, out io.Writer) error {
	f := flag.NewFlagSet("publisher authority-proposal", flag.ContinueOnError)
	previewFile := f.String("preview-file", "", "system-produced proposal preview JSON")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 || *previewFile == "" {
		return errors.New("usage: praxis publisher authority-proposal --preview-file <file>")
	}
	payload, err := os.ReadFile(*previewFile)
	if err != nil {
		return err
	}
	var envelope struct {
		Proposal contracts.PublisherAuthorityProposal `json:"proposal"`
		Digest   string                               `json:"proposal_digest"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return err
	}
	digest, err := envelope.Proposal.Digest()
	if err != nil || digest != envelope.Digest {
		return errors.New("proposal preview digest mismatch")
	}
	repo, db, err := openGovernedRepository(context.Background(), getenv)
	if err != nil {
		return err
	}
	defer db.Close()
	now := time.Now().UTC()
	if err := validatePublisherAuthorityProposalCurrent(context.Background(), repo, state.New(db), envelope.Proposal, now); err != nil {
		return err
	}
	stored, err := repo.SavePublisherAuthorityProposal(context.Background(), envelope.Proposal, now)
	if err != nil {
		return err
	}
	return printJSONTo(out, map[string]any{"operation": "publisher.authority-proposal", "proposal_digest": stored})
}

func runPublisherAuthorityReview(args []string, out io.Writer) error {
	f := flag.NewFlagSet("publisher authority-review", flag.ContinueOnError)
	proposalDigest := f.String("proposal-digest", "", "exact system-produced proposal digest")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 || *proposalDigest == "" || !isInteractiveTerminal() {
		return errAuthorityBootstrapConfirmation
	}
	repo, db, record, err := openGovernedRepositoryReadOnly(context.Background(), os.Getenv)
	if err != nil {
		return err
	}
	defer db.Close()
	proposal, err := repo.LoadPublisherAuthorityProposalByDigest(context.Background(), *proposalDigest, time.Now().UTC())
	if err != nil {
		return err
	}
	actual, _ := proposal.Digest()
	if actual != *proposalDigest {
		return errors.New("proposal digest mismatch")
	}
	bootstrapDigest, err := record.Digest()
	if err != nil || proposal.BootstrapDigest != bootstrapDigest {
		return errors.New("proposal belongs to another installation")
	}
	owner, err := contracts.InstallationOwnerPrincipal(bootstrapDigest)
	if err != nil {
		return err
	}
	current, err := user.Current()
	if err != nil || current.Username == "" {
		return err
	}
	root, err := currentInstallationRoot(context.Background(), repo, owner, time.Now().UTC())
	if err != nil || root.Ref != proposal.ParentRef || root.Digest != proposal.ParentDigest {
		return errors.New("proposal root is no longer current")
	}
	model, err := repo.LoadAuthorityModelState(context.Background(), time.Now().UTC())
	if err != nil || model.ActiveVersion != contracts.AuthorityModelSuccessorVersion || model.ActiveDigest != contracts.AuthorityModelSuccessorDigest() {
		return errors.New("package.publish review requires authority-model v2")
	}
	if !strings.HasSuffix(root.ProvenanceRef, ":os-user:"+current.Username) {
		return errors.New("authenticated reviewer is not the installation owner")
	}
	fmt.Fprintf(out, "Review exact package.publish proposal %s. Type %q to approve: ", *proposalDigest, "REVIEW-PUBLISHER "+*proposalDigest)
	answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return err
	}
	if strings.TrimSpace(answer) != "REVIEW-PUBLISHER "+*proposalDigest {
		return errAuthorityBootstrapConfirmation
	}
	repo2, db2, err := openGovernedRepository(context.Background(), os.Getenv)
	if err != nil {
		return err
	}
	defer db2.Close()
	review := contracts.PublisherAuthorityReview{ID: "publisher-authority-review:" + *proposalDigest, Version: "1", Kind: contracts.PublisherAuthorityReviewKind, ProposalID: proposal.ID, ProposalVersion: proposal.Version, ProposalDigest: *proposalDigest, ReviewedBy: owner.ID, ReviewedKind: owner.Kind, Decision: "approve", Namespace: proposal.Namespace, PublisherGenerationDigest: proposal.PublisherGenerationDigest, ReviewedAt: time.Now().UTC()}
	digest, err := repo2.SavePublisherAuthorityReview(context.Background(), review, time.Now().UTC())
	if err != nil {
		return err
	}
	return printJSONTo(out, map[string]any{"operation": "publisher.authority-review", "review_digest": digest, "proposal_digest": *proposalDigest})
}

func runPublisherAuthorityRequestCanonical(args []string, getenv func(string) string, out io.Writer) error {
	f := flag.NewFlagSet("publisher authority-request", flag.ContinueOnError)
	proposalDigest := f.String("proposal-digest", "", "exact proposal digest")
	reviewDigest := f.String("review-digest", "", "exact review digest")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 || *proposalDigest == "" || *reviewDigest == "" {
		return errors.New("usage: praxis publisher authority-request --proposal-digest <digest> --review-digest <digest>")
	}
	repo, db, record, err := openGovernedRepositoryReadOnly(context.Background(), getenv)
	if err != nil {
		return err
	}
	defer db.Close()
	now := time.Now().UTC()
	proposal, err := repo.LoadPublisherAuthorityProposalByDigest(context.Background(), *proposalDigest, now)
	if err != nil {
		return err
	}
	review, err := repo.LoadPublisherAuthorityReviewByDigest(context.Background(), *reviewDigest, now)
	if err != nil {
		return err
	}
	if review.ProposalDigest != *proposalDigest {
		return errors.New("review does not bind requested proposal")
	}
	bootstrapDigest, err := record.Digest()
	if err != nil || proposal.BootstrapDigest != bootstrapDigest {
		return errors.New("proposal belongs to another installation")
	}
	owner, err := contracts.InstallationOwnerPrincipal(bootstrapDigest)
	if err != nil {
		return err
	}
	root, err := currentInstallationRoot(context.Background(), repo, owner, now)
	if err != nil || root.Ref != proposal.ParentRef || root.Digest != proposal.ParentDigest {
		return errors.New("proposal root is no longer current")
	}
	model, err := repo.LoadAuthorityModelState(context.Background(), now)
	if err != nil || model.ActiveVersion != contracts.AuthorityModelSuccessorVersion || model.ActiveDigest != contracts.AuthorityModelSuccessorDigest() {
		return errors.New("package.publish request requires authority-model v2")
	}
	if err := validatePublisherAuthorityProposalCurrent(context.Background(), repo, state.New(db), proposal, now); err != nil {
		return err
	}
	publisherRecord, err := state.New(db).PublisherGeneration(context.Background(), proposal.PublisherGenerationDigest)
	if err != nil {
		return err
	}
	if publisherRecord.Generation.PublicKeyDigest != proposal.PublicKeyDigest {
		return errors.New("publisher generation changed since proposal")
	}
	delegation := contracts.DelegationRequest{Profile: contracts.DelegationProfilePackagePublish, ParentRef: proposal.ParentRef, ParentVersion: proposal.ParentVersion, ParentDigest: proposal.ParentDigest, DelegatedPrincipal: publisherRecord.Generation.Principal, TargetKind: "publisher-generation", TargetIdentity: publisherRecord.Generation.Principal.ID, TargetVersion: proposal.Namespace, TargetDigest: proposal.PublisherGenerationDigest, TargetConstraints: []string{proposal.Namespace}, SubjectKind: publisherRecord.Generation.Principal.Kind, SubjectID: publisherRecord.Generation.Principal.ID, SubjectVersion: publisherRecord.Generation.Generation, SubjectDigest: proposal.PublisherGenerationDigest, SubjectKeyDigest: publisherRecord.Generation.PublicKeyDigest, RequestedAuthority: contracts.GovernedPackagePublish, RequestedOperation: "sign", RequestedScope: proposal.Scope, ProposalVersion: proposal.Version, ProposalDigest: *proposalDigest, ReviewVersion: review.Version, ReviewDigest: *reviewDigest, ExpiresAt: proposal.ExpiresAt, Reason: proposal.Reason, PolicyRef: contracts.AuthorityModelID, PolicyVersion: contracts.AuthorityModelSuccessorVersion, PolicyDigest: contracts.AuthorityModelSuccessorDigest()}
	request := contracts.AuthorityRequest{ID: "publisher-authority-request:" + *proposalDigest + ":" + *reviewDigest, Version: "1", RequestedAuthority: contracts.GovernedPackagePublish, RequestedScope: proposal.Scope, Reason: proposal.Reason, Status: contracts.AuthorityRequestPending, Delegation: &delegation}
	_, err = request.Digest()
	if err != nil {
		return err
	}
	writable, writeDB, err := openGovernedRepository(context.Background(), getenv)
	if err != nil {
		return err
	}
	defer writeDB.Close()
	stored, err := writable.SaveAuthorityRequest(context.Background(), request, now, &proposal.ExpiresAt)
	if err != nil {
		return err
	}
	return printJSONTo(out, map[string]any{"operation": "authority.delegate", "request": request, "request_digest": stored, "proposal_digest": *proposalDigest, "review_digest": *reviewDigest})
}

func currentInstallationRoot(ctx context.Context, repo goalstore.Repository, owner contracts.PrincipalRef, now time.Time) (contracts.AuthorityGeneration, error) {
	gens, err := repo.ListAuthorityGenerations(ctx, now)
	if err != nil {
		return contracts.AuthorityGeneration{}, err
	}
	for _, generation := range gens {
		if generation.ParentRef == "" && generation.Principal == owner {
			return generation, nil
		}
	}
	return contracts.AuthorityGeneration{}, errors.New("installation governance root unavailable")
}

func validatePublisherAuthorityProposalCurrent(ctx context.Context, repo goalstore.Repository, store *state.Store, proposal contracts.PublisherAuthorityProposal, now time.Time) error {
	owner, err := contracts.InstallationOwnerPrincipal(proposal.BootstrapDigest)
	if err != nil || proposal.OwnerID != owner.ID || proposal.OwnerKind != owner.Kind {
		return errors.New("proposal owner does not bind installation bootstrap")
	}
	model, err := repo.LoadAuthorityModelState(ctx, now)
	if err != nil || model.ActiveVersion != contracts.AuthorityModelSuccessorVersion || model.ActiveDigest != contracts.AuthorityModelSuccessorDigest() || proposal.AuthorityModel != model.ActiveModel || proposal.AuthorityModelVersion != model.ActiveVersion || proposal.AuthorityModelDigest != model.ActiveDigest {
		return errors.New("proposal authority model is not current")
	}
	root, err := currentInstallationRoot(ctx, repo, owner, now)
	if err != nil || root.Ref != proposal.ParentRef || root.Version != proposal.ParentVersion || root.Digest != proposal.ParentDigest {
		return errors.New("proposal parent root is not current")
	}
	publisherRecord, err := store.PublisherGeneration(ctx, proposal.PublisherGenerationDigest)
	if err != nil {
		return err
	}
	if publisherRecord.Generation.Principal.ID != proposal.PublisherPrincipal || publisherRecord.Generation.PublicKeyDigest != proposal.PublicKeyDigest || publisherRecord.Generation.PackageNamespace != proposal.Namespace {
		return errors.New("proposal publisher generation does not bind exact subject")
	}
	if _, err := contracts.PackagePublishScope(proposal.Namespace); err != nil {
		return err
	}
	return nil
}

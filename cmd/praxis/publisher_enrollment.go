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

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func publisherEnrollmentPreview(args []string, out io.Writer) error {
	keyID, namespace, generation, predecessor, output, err := parseEnrollmentPreviewArgs(args)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	bootstrapPath, dbPath := os.Getenv("PRAXIS_BOOTSTRAP_RECORD"), os.Getenv("PRAXIS_DB")
	if bootstrapPath == "" || dbPath == "" {
		return errors.New("PRAXIS_BOOTSTRAP_RECORD and PRAXIS_DB are required")
	}
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
	if err != nil {
		return err
	}
	if !contracts.AuthorityModelStateRetains(model, contracts.AuthorityModelSuccessorVersion) {
		return errors.New("publisher enrollment requires adopted authority-model v2")
	}
	if _, err := contracts.PackagePublishScope(namespace); err != nil {
		return err
	}
	flags := publisherFlags{keyID: keyID, namespace: namespace, generation: generation, predecessor: predecessor, effectiveAt: now.Format(time.RFC3339Nano), enrollmentRef: "publisher-enrollment-preview:" + generation}
	generationRecord, err := publisherGeneration(context.Background(), flags)
	if err != nil {
		return err
	}
	generationDigest, err := generationRecord.Digest()
	if err != nil {
		return err
	}
	preview := contracts.PublisherEnrollmentPreview{ID: "publisher-enrollment-preview:" + generationDigest, Version: "1", BootstrapDigest: bootstrapDigest, OwnerID: owner.ID, OwnerKind: owner.Kind, AuthorityModel: contracts.AuthorityModelID, AuthorityModelVersion: contracts.AuthorityModelSuccessorVersion, AuthorityModelDigest: contracts.AuthorityModelSuccessorDigest(), PublisherPrincipal: generationRecord.Principal.ID, KeyID: generationRecord.KeyID, Algorithm: generationRecord.Algorithm, KeyPurpose: "publisher-signing", PublicKeyDigest: generationRecord.PublicKeyDigest, Generation: generationRecord.Generation, Predecessor: generationRecord.Predecessor, Namespace: generationRecord.PackageNamespace, GenerationDigest: generationDigest, GenerationRecord: generationRecord, CreatedAt: now}
	digest, err := preview.Digest()
	if err != nil {
		return err
	}
	result := map[string]any{"operation": "publisher.enroll", "preview": true, "preview_record": preview, "preview_digest": digest, "confirmation": "APPROVE-PUBLISHER " + digest}
	if output != "" {
		payload, _ := json.MarshalIndent(result, "", "  ")
		if err := writeCanonicalPreviewFile(output, append(payload, '\n')); err != nil {
			return err
		}
	}
	return printJSONTo(out, result)
}

func parseEnrollmentPreviewArgs(args []string) (string, string, string, string, string, error) {
	keyID, namespace, generation, predecessor, output := "", "praxis.package", "", "", ""
	f := flag.NewFlagSet("publisher enroll-preview", flag.ContinueOnError)
	f.StringVar(&keyID, "key-id", "", "protected publisher signing key reference")
	f.StringVar(&namespace, "namespace", namespace, "exact package namespace")
	f.StringVar(&generation, "generation", "", "publisher generation identifier")
	f.StringVar(&predecessor, "predecessor", "", "exact predecessor generation digest")
	f.StringVar(&output, "output", "", "system-produced preview output path")
	if err := f.Parse(args); err != nil {
		return "", "", "", "", "", err
	}
	if f.NArg() != 0 || strings.TrimSpace(keyID) == "" || strings.TrimSpace(generation) == "" {
		return "", "", "", "", "", errors.New("usage: praxis publisher enroll-preview --key-id <protected-key> --generation <generation> [--predecessor <digest>] [--namespace <namespace>] [--output <file>]")
	}
	return keyID, namespace, generation, predecessor, output, nil
}

func runPublisherEnrollmentApprove(args []string, out io.Writer) error {
	f := flag.NewFlagSet("publisher enroll-approve", flag.ContinueOnError)
	previewPath := f.String("preview-file", "", "system-produced enrollment preview JSON")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 || *previewPath == "" || !isInteractiveTerminal() {
		return errAuthorityBootstrapConfirmation
	}
	payload, err := os.ReadFile(*previewPath)
	if err != nil {
		return err
	}
	var envelope struct {
		Preview contracts.PublisherEnrollmentPreview `json:"preview_record"`
		Digest  string                               `json:"preview_digest"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return err
	}
	digest, err := envelope.Preview.Digest()
	if err != nil || digest != envelope.Digest {
		return errors.New("publisher enrollment preview digest mismatch")
	}
	record, err := loadBootstrapForOwner()
	if err != nil {
		return err
	}
	bootstrapDigest, err := record.Digest()
	if err != nil || bootstrapDigest != envelope.Preview.BootstrapDigest {
		return errors.New("publisher enrollment preview is for another installation")
	}
	current, err := user.Current()
	if err != nil {
		return err
	}
	owner, err := contracts.InstallationOwnerPrincipal(bootstrapDigest)
	if err != nil || owner.ID != envelope.Preview.OwnerID || current.Username == "" {
		return errors.New("authenticated installation owner does not match enrollment preview")
	}
	fmt.Fprintf(out, "Authorize exact publisher enrollment preview %s. Type %q to continue: ", digest, "APPROVE-PUBLISHER "+digest)
	answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return err
	}
	if strings.TrimSpace(answer) != "APPROVE-PUBLISHER "+digest {
		return errAuthorityBootstrapConfirmation
	}
	repo, db, err := openGovernedRepository(context.Background(), os.Getenv)
	if err != nil {
		return err
	}
	defer db.Close()
	now := time.Now().UTC()
	approval, approvalDigest, err := repo.ApprovePublisherEnrollment(context.Background(), envelope.Preview, bootstrapDigest, owner.ID, current.Username, "APPROVE-PUBLISHER "+digest, now)
	if err != nil {
		return err
	}
	return printJSONTo(out, map[string]any{"operation": "publisher.enroll-approve", "approval": approval, "approval_digest": approvalDigest})
}

func loadBootstrapForOwner() (record praxiscrypto.BootstrapRecord, err error) {
	path := os.Getenv("PRAXIS_BOOTSTRAP_RECORD")
	if path == "" {
		return record, errors.New("PRAXIS_BOOTSTRAP_RECORD is required")
	}
	return praxiscrypto.LoadBootstrapRecord(path)
}

func runPublisherEnrollmentApprovalInspect(args []string, out io.Writer) error {
	f := flag.NewFlagSet("publisher enroll-approval-inspect", flag.ContinueOnError)
	previewDigest := f.String("preview-digest", "", "exact system-produced enrollment preview digest")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 || *previewDigest == "" {
		return errors.New("usage: praxis publisher enroll-approval-inspect --preview-digest <preview-digest>")
	}
	repo, db, _, err := openGovernedRepositoryReadOnly(context.Background(), os.Getenv)
	if err != nil {
		return err
	}
	defer db.Close()
	approval, err := repo.LoadPublisherEnrollmentApproval(context.Background(), *previewDigest, time.Now().UTC())
	if err != nil {
		return err
	}
	digest, err := approval.Digest()
	if err != nil {
		return err
	}
	return printJSONTo(out, map[string]any{"operation": "publisher.enroll-approve", "approval": approval, "approval_digest": digest})
}

func runCanonicalPublisherEnroll(args []string, getenv func(string) string, out io.Writer) error {
	f := flag.NewFlagSet("publisher enroll", flag.ContinueOnError)
	approvalDigest := f.String("approval", "", "exact system-produced publisher enrollment approval digest")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 || *approvalDigest == "" {
		return errors.New("usage: praxis publisher enroll --approval <approval-digest>")
	}
	bootstrap, err := loadBootstrapForOwnerWithEnv(getenv)
	if err != nil {
		return err
	}
	current, err := user.Current()
	if err != nil || current.Username == "" {
		return errors.New("authenticated installation owner is unavailable")
	}
	repo, db, err := openGovernedRepository(context.Background(), getenv)
	if err != nil {
		return err
	}
	defer db.Close()
	// Resolve the approval before opening the protected key so a missing or
	// substituted approval cannot trigger backend access.
	approval, err := repo.LoadPublisherEnrollmentApprovalByDigest(context.Background(), *approvalDigest, time.Now().UTC())
	if err != nil {
		return err
	}
	if approval.Version != contracts.PublisherEnrollmentApprovalVersion {
		return errors.New("legacy publisher enrollment approval cannot authorize enrollment")
	}
	signer, err := publisherBackend().Open(context.Background(), approval.GenerationRecord.KeyID)
	if err != nil {
		return err
	}
	generation, err := repo.EnrollPublisherFromApproval(context.Background(), *approvalDigest, bootstrap, signer, current.Username, time.Now().UTC())
	if err != nil {
		return err
	}
	generationDigest, err := generation.Digest()
	if err != nil {
		return err
	}
	return printJSONTo(out, map[string]any{"operation": "publisher.enroll", "publisher_principal": generation.Principal, "generation": generation.Generation, "generation_digest": generationDigest, "public_key_digest": generation.PublicKeyDigest, "namespace": generation.PackageNamespace, "approval_digest": *approvalDigest, "preview_digest": approval.PreviewDigest, "enrollment_event_time": time.Now().UTC(), "authority_model": contracts.AuthorityModelID, "authority_model_version": contracts.AuthorityModelSuccessorVersion, "authority_model_digest": contracts.AuthorityModelSuccessorDigest()})
}

func loadBootstrapForOwnerWithEnv(getenv func(string) string) (praxiscrypto.BootstrapRecord, error) {
	if getenv == nil {
		getenv = os.Getenv
	}
	path := getenv("PRAXIS_BOOTSTRAP_RECORD")
	if path == "" {
		return praxiscrypto.BootstrapRecord{}, errors.New("PRAXIS_BOOTSTRAP_RECORD is required")
	}
	return praxiscrypto.LoadBootstrapRecord(path)
}

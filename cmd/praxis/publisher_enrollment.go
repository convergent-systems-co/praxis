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
	if model.ActiveVersion != contracts.AuthorityModelSuccessorVersion || model.ActiveDigest != contracts.AuthorityModelSuccessorDigest() {
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
	preview := contracts.PublisherEnrollmentPreview{ID: "publisher-enrollment-preview:" + generationDigest, Version: "1", BootstrapDigest: bootstrapDigest, OwnerID: owner.ID, OwnerKind: owner.Kind, AuthorityModel: contracts.AuthorityModelID, AuthorityModelVersion: model.ActiveVersion, AuthorityModelDigest: model.ActiveDigest, PublisherPrincipal: generationRecord.Principal.ID, KeyID: generationRecord.KeyID, Algorithm: generationRecord.Algorithm, KeyPurpose: "publisher-signing", PublicKeyDigest: generationRecord.PublicKeyDigest, Generation: generationRecord.Generation, Predecessor: generationRecord.Predecessor, Namespace: generationRecord.PackageNamespace, GenerationDigest: generationDigest, GenerationRecord: generationRecord, CreatedAt: now}
	digest, err := preview.Digest()
	if err != nil {
		return err
	}
	result := map[string]any{"operation": "publisher.enroll", "preview": true, "preview_record": preview, "preview_digest": digest, "confirmation": "APPROVE-PUBLISHER " + digest}
	if output != "" {
		payload, _ := json.MarshalIndent(result, "", "  ")
		if err := os.WriteFile(output, append(payload, '\n'), 0600); err != nil {
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
	approval, approvalDigest, err := repo.ApprovePublisherEnrollment(context.Background(), envelope.Preview, bootstrapDigest, owner.ID, "APPROVE-PUBLISHER "+digest, now)
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

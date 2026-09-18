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

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func runRootAuthoritySuccessionPreview(args []string, getenv func(string) string, out io.Writer) error {
	f := flag.NewFlagSet("authority root-successor-preview", flag.ContinueOnError)
	output := f.String("output", "", "optional system-produced preview JSON path")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return errors.New("usage: praxis authority root-successor-preview [--output <file>]")
	}
	repo, db, record, err := openGovernedRepositoryReadOnly(context.Background(), getenv)
	if err != nil {
		return err
	}
	defer db.Close()
	root, err := repo.LoadCurrentInstallationRoot(context.Background(), repo.InstallationDigest, time.Now().UTC())
	if err != nil {
		return err
	}
	proposal, err := contracts.BuildRootAuthoritySuccession(root, repo.InstallationDigest, time.Now().UTC())
	if err != nil {
		return err
	}
	digest, err := proposal.Digest()
	if err != nil {
		return err
	}
	payload, err := json.MarshalIndent(map[string]any{"preview": true, "proposal": proposal, "proposal_digest": digest, "bootstrap_provider": record.ProviderID}, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	if *output != "" {
		if err := writeCanonicalPreviewFile(*output, payload); err != nil {
			return err
		}
	}
	_, err = out.Write(payload)
	return err
}

func loadRootSuccessionPreview(path string) (contracts.RootAuthoritySuccessionProposal, string, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return contracts.RootAuthoritySuccessionProposal{}, "", err
	}
	var envelope struct {
		Proposal       contracts.RootAuthoritySuccessionProposal `json:"proposal"`
		ProposalDigest string                                    `json:"proposal_digest"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return contracts.RootAuthoritySuccessionProposal{}, "", err
	}
	digest, err := envelope.Proposal.Digest()
	if err != nil || digest != envelope.ProposalDigest {
		return contracts.RootAuthoritySuccessionProposal{}, "", errors.New("root-successor preview digest mismatch")
	}
	return envelope.Proposal, digest, nil
}

func runRootAuthoritySuccessionProposal(args []string, getenv func(string) string, out io.Writer) error {
	f := flag.NewFlagSet("authority root-successor-proposal", flag.ContinueOnError)
	preview := f.String("preview-file", "", "system-produced root-successor preview")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 || *preview == "" {
		return errors.New("usage: praxis authority root-successor-proposal --preview-file <file>")
	}
	proposal, digest, err := loadRootSuccessionPreview(*preview)
	if err != nil {
		return err
	}
	repo, db, err := openGovernedRepository(context.Background(), getenv)
	if err != nil {
		return err
	}
	defer db.Close()
	if proposal.BootstrapDigest != repo.InstallationDigest {
		return errors.New("root-successor preview belongs to a different installation")
	}
	current, err := repo.LoadCurrentInstallationRoot(context.Background(), repo.InstallationDigest, time.Now().UTC())
	if err != nil {
		return err
	}
	if current.Digest != proposal.Predecessor.Digest {
		return errors.New("root-successor preview predecessor is stale")
	}
	stored, err := repo.SaveRootAuthoritySuccessionProposal(context.Background(), proposal, time.Now().UTC())
	if err != nil {
		return err
	}
	if stored != digest {
		return errors.New("root-successor proposal digest changed during persistence")
	}
	return printJSONTo(out, map[string]any{"operation": "authority.root-successor-proposal", "proposal_digest": stored})
}

func runRootAuthoritySuccessionReview(args []string, getenv func(string) string, input io.Reader, out io.Writer) error {
	f := flag.NewFlagSet("authority root-successor-review", flag.ContinueOnError)
	digest := f.String("proposal", "", "exact durable succession proposal digest")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 || *digest == "" || !isInteractiveTerminal() {
		return errors.New("usage: praxis authority root-successor-review --proposal <digest> (interactive confirmation required)")
	}
	repo, db, err := openGovernedRepository(context.Background(), getenv)
	if err != nil {
		return err
	}
	defer db.Close()
	bootstrapDigest := repo.InstallationDigest
	owner, err := contracts.InstallationOwnerPrincipal(bootstrapDigest)
	if err != nil {
		return err
	}
	current, err := user.Current()
	if err != nil || current.Username == "" {
		return errors.New("authenticated local OS user is unavailable")
	}
	confirmation := "REVIEW-ROOT-SUCCESSOR " + *digest
	fmt.Fprintf(out, "Review exact root-authority successor %s. Type %q to continue: ", *digest, confirmation)
	answer, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if strings.TrimSpace(answer) != confirmation {
		return errAuthorityBootstrapConfirmation
	}
	review, reviewDigest, err := repo.SaveRootAuthoritySuccessionReview(context.Background(), *digest, owner, current.Username, confirmation, time.Now().UTC())
	if err != nil {
		return err
	}
	return printJSONTo(out, map[string]any{"operation": "authority.root-successor-review", "review": review, "review_digest": reviewDigest})
}

func runRootAuthoritySuccessionAccept(args []string, getenv func(string) string, input io.Reader, out io.Writer) error {
	f := flag.NewFlagSet("authority root-successor-accept", flag.ContinueOnError)
	proposalDigest := f.String("proposal", "", "exact durable succession proposal digest")
	reviewDigest := f.String("review", "", "exact durable succession review digest")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 || *proposalDigest == "" || *reviewDigest == "" || !isInteractiveTerminal() {
		return errors.New("usage: praxis authority root-successor-accept --proposal <digest> --review <digest> (interactive confirmation required)")
	}
	repo, db, err := openGovernedRepository(context.Background(), getenv)
	if err != nil {
		return err
	}
	defer db.Close()
	bootstrapDigest := repo.InstallationDigest
	current, err := user.Current()
	if err != nil || current.Username == "" {
		return errors.New("authenticated local OS user is unavailable")
	}
	confirmation := "ACCEPT-ROOT-SUCCESSOR " + *proposalDigest + " " + *reviewDigest
	fmt.Fprintf(out, "Accept exact root-authority successor %s reviewed as %s. Type %q to continue: ", *proposalDigest, *reviewDigest, confirmation)
	answer, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if strings.TrimSpace(answer) != confirmation {
		return errAuthorityBootstrapConfirmation
	}
	successor, decision, err := repo.AcceptRootAuthoritySuccession(context.Background(), *proposalDigest, *reviewDigest, bootstrapDigest, current.Username, confirmation, time.Now().UTC())
	if err != nil {
		return err
	}
	decisionDigest, _ := decision.Digest()
	return printJSONTo(out, map[string]any{"operation": "authority.root-successor-accept", "successor": successor, "decision": decision, "decision_digest": decisionDigest})
}

func runInstallationRepairAuthorityRequest(args []string, getenv func(string) string, out io.Writer) error {
	f := flag.NewFlagSet("authority installation-repair-request", flag.ContinueOnError)
	operation := f.String("operation", "", "storage_schema or runtime_state")
	expires := f.String("expires-at", "", "RFC3339 expiry")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 || *operation == "" || *expires == "" {
		return errors.New("usage: praxis authority installation-repair-request --operation <storage_schema|runtime_state> --expires-at <RFC3339>")
	}
	op := map[string]string{"storage_schema": contracts.GovernedInstallationRepairStorageSchema, "runtime_state": contracts.GovernedInstallationRepairRuntimeState}[*operation]
	if op == "" {
		return errors.New("installation-repair operation must be storage_schema or runtime_state")
	}
	expiresAt, err := time.Parse(time.RFC3339, *expires)
	if err != nil {
		return err
	}
	repo, db, err := openGovernedRepository(context.Background(), getenv)
	if err != nil {
		return err
	}
	defer db.Close()
	request, digest, err := repo.SaveInstallationRepairAuthorityRequest(context.Background(), op, expiresAt, time.Now().UTC())
	if err != nil {
		return err
	}
	return printJSONTo(out, map[string]any{"operation": "authority.installation-repair-request", "request": request, "request_digest": digest})
}

func runInstallationRepairAuthorityApprove(args []string, getenv func(string) string, input io.Reader, out io.Writer) error {
	f := flag.NewFlagSet("authority installation-repair-approve", flag.ContinueOnError)
	digest := f.String("request", "", "exact durable repair request digest")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 || *digest == "" || !isInteractiveTerminal() {
		return errors.New("usage: praxis authority installation-repair-approve --request <digest> (interactive confirmation required)")
	}
	repo, db, err := openGovernedRepository(context.Background(), getenv)
	if err != nil {
		return err
	}
	defer db.Close()
	bootstrapDigest := repo.InstallationDigest
	owner, err := contracts.InstallationOwnerPrincipal(bootstrapDigest)
	if err != nil {
		return err
	}
	current, err := user.Current()
	if err != nil || current.Username == "" {
		return errors.New("authenticated local OS user is unavailable")
	}
	root, err := repo.LoadCurrentInstallationRoot(context.Background(), bootstrapDigest, time.Now().UTC())
	if err != nil {
		return err
	}
	if !strings.HasSuffix(root.ProvenanceRef, ":os-user:"+current.Username) {
		return errors.New("authenticated OS user does not own current installation root")
	}
	confirmation := "APPROVE-INSTALLATION-REPAIR " + *digest
	fmt.Fprintf(out, "Approve exact installation-repair request %s. Type %q to continue: ", *digest, confirmation)
	answer, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if strings.TrimSpace(answer) != confirmation {
		return errAuthorityBootstrapConfirmation
	}
	decision, err := repo.ApproveInstallationRepairAuthorityRequest(context.Background(), *digest, owner, confirmation, time.Now().UTC())
	if err != nil {
		return err
	}
	decisionDigest, _ := decision.Digest()
	return printJSONTo(out, map[string]any{"operation": "authority.installation-repair-approve", "decision": decision, "decision_digest": decisionDigest})
}

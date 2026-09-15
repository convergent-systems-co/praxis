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
	"github.com/convergent-systems-co/praxis/internal/goalstore"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

var errAuthorityBootstrapConfirmation = errors.New("explicit interactive authority enrollment confirmation is required")

const authorityBootstrapConfirmationPrefix = "ENROLL"

func runAuthorityCommand(args []string) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		return writeAuthorityHelp(os.Stdout)
	}
	switch args[0] {
	case "bootstrap":
		return runAuthorityBootstrap(args[1:], os.Getenv, os.Stdin, os.Stdout)
	case "delegate":
		return runAuthorityDelegate(args[1:], os.Getenv, os.Stdin, os.Stdout)
	case "model-preview":
		return runAuthorityModelPreview(args[1:], os.Getenv, os.Stdout)
	case "model-adopt":
		return runAuthorityModelAdopt(args[1:], os.Getenv, os.Stdin, os.Stdout)
	case "model-status":
		return runAuthorityModelStatus(args[1:], os.Getenv, os.Stdout)
	default:
		return errors.New("usage: praxis authority {bootstrap|delegate|model-preview|model-adopt|model-status}")
	}
}

func writeAuthorityHelp(output io.Writer) error {
	if _, err := io.WriteString(output, "usage: praxis authority <bootstrap|delegate> [options]\n\n"); err != nil {
		return err
	}
	if err := writeAuthorityBootstrapHelp(output); err != nil {
		return err
	}
	return writeAuthorityDelegateHelp(output)
}

func writeAuthorityDelegateHelp(output io.Writer) error {
	_, err := io.WriteString(output, `
usage: praxis authority delegate --request-file <path>

Purpose:
  Authenticate the enrolled installation root and decide one exact v1
  delegation request, then persist its bounded controller generation.

Required option:
  --request-file <path>  Canonical JSON AuthorityRequest containing the exact
                         v1 delegation payload.

Interaction:
  The command displays the exact request and requires confirmation:
  DELEGATE <request-digest>

Semantics:
  Only the enrolled installation principal may authorize the request. The
  built-in v1 model permits a distinct controller's exact workplan.accept
  authority over the exact protected WorkPlan target. Runtime capabilities,
  package activation, invocation, provider, repository, and self-targeted
  authority are not granted. Policy/model, parent, decision, expiry, and
  provenance mismatches fail closed.
`)
	return err
}

func runAuthorityDelegate(args []string, getenv func(string) string, input io.Reader, output io.Writer) error {
	flags := flag.NewFlagSet("authority delegate", flag.ContinueOnError)
	flags.SetOutput(output)
	requestFile := flags.String("request-file", "", "canonical JSON AuthorityRequest containing an exact v1 delegation payload")
	flags.Usage = func() { _ = writeAuthorityDelegateHelp(output) }
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 || strings.TrimSpace(*requestFile) == "" {
		return errors.New("usage: praxis authority delegate --request-file <path> (interactive confirmation required)")
	}
	if !isInteractiveTerminal() {
		return errAuthorityBootstrapConfirmation
	}
	if getenv == nil {
		getenv = os.Getenv
	}
	payload, err := os.ReadFile(*requestFile)
	if err != nil {
		return fmt.Errorf("read delegation request: %w", err)
	}
	var request contracts.AuthorityRequest
	if err := json.Unmarshal(payload, &request); err != nil {
		return fmt.Errorf("decode delegation request: %w", err)
	}
	if (request.RequestedAuthority != contracts.AuthorityDelegateCapability && request.RequestedAuthority != contracts.GovernedPackagePublish) || request.Delegation == nil {
		return errors.New("request-file must contain a supported closed delegation request")
	}
	repo, db, err := openGovernedRepository(context.Background(), getenv)
	if err != nil {
		return err
	}
	defer db.Close()
	now := time.Now().UTC()
	requestDigest, err := repo.SaveAuthorityRequest(context.Background(), request, now, &request.Delegation.ExpiresAt)
	if err != nil {
		return err
	}
	parent, err := repo.LoadAuthorityGeneration(context.Background(), request.Delegation.ParentRef, request.Delegation.ParentVersion, now)
	if err != nil {
		return fmt.Errorf("load delegation parent: %w", err)
	}
	current, err := user.Current()
	if err != nil || current.Username == "" || !strings.HasSuffix(parent.ProvenanceRef, ":os-user:"+current.Username) {
		return errors.New("authenticated root OS user does not match the enrolled generation")
	}
	if err := contracts.ValidateBuiltinDelegation(parent, *request.Delegation, now); err != nil {
		return err
	}
	if _, err := io.WriteString(output, fmt.Sprintf("Authorize exact delegation request %s for parent %s/%s. Type %q to continue: ", requestDigest, parent.Ref, parent.Version, "DELEGATE "+requestDigest)); err != nil {
		return err
	}
	answer, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if strings.TrimSpace(answer) != "DELEGATE "+requestDigest {
		return errAuthorityBootstrapConfirmation
	}
	decision := contracts.AuthorityDecision{RequestID: request.ID, RequestVersion: request.Version, RequestDigest: requestDigest, DecisionRef: "authority-decision:" + request.ID, DecisionVersion: "1", DecidedBy: parent.Principal, AuthorityRef: parent.Ref, AuthorityVersion: parent.Version, AuthorityGenerationDigest: parent.Digest, GrantedScope: parent.Scope, Outcome: contracts.AuthorityApprove, AuthorityDigest: contracts.AuthorityModelDigest(), IssuedAt: now, ExpiresAt: &request.Delegation.ExpiresAt, Delegation: request.Delegation}
	if err := repo.SaveAuthorityDecision(context.Background(), request.ID, request.Version, decision, now, &request.Delegation.ExpiresAt); err != nil {
		return err
	}
	child, err := repo.SaveDelegatedAuthorityGeneration(context.Background(), request.ID, request.Version, decision, builtinDelegationPolicy{}, now)
	if err != nil {
		return err
	}
	fresh, err := repo.LoadAuthorityGeneration(context.Background(), child.Ref, child.Version, now)
	if err != nil || fresh.Digest != child.Digest {
		return fmt.Errorf("verify delegated generation recovery: %w", err)
	}
	return printJSON(map[string]any{"operation": "authority.delegate", "request_digest": requestDigest, "decision": decision, "child_generation": fresh})
}

type builtinDelegationPolicy struct{}

func (builtinDelegationPolicy) ContainDelegation(parent contracts.AuthorityGeneration, request contracts.DelegationRequest, now time.Time) error {
	if request.Profile == contracts.DelegationProfilePackagePublish {
		return contracts.ValidateBuiltinPackagePublishDelegation(parent, request, now)
	}
	return contracts.ValidateBuiltinDelegation(parent, request, now)
}

func writeAuthorityBootstrapHelp(output io.Writer) error {
	_, err := io.WriteString(output, `usage: praxis authority bootstrap [--scope <canonical-installation-scope>]

Purpose:
  Enroll exactly one corrected, least-scope human governance root for this
  Praxis installation. This establishes governance identity; it does not
  approve a WorkPlan, grant provider or repository authority, or authorize
  unrestricted future work.

Optional assertion:
  --scope <canonical-installation-scope>
                         Must exactly equal the derived
                         installation-governance:<BootstrapRecord.Digest()>.
                         Omit it to use the derived value directly.

Inputs:
  PRAXIS_BOOTSTRAP_RECORD  Existing metadata-only record created by
                           'praxis key-bootstrap'.
  PRAXIS_DB                Fresh or existing Praxis authoritative state path,
                           initialized by 'praxis state-init'.

Interaction:
  The command displays the derived installation principal and requires the
  exact confirmation phrase: ENROLL <installation-principal-id>

Semantics:
  The protected bootstrap record and authenticated local OS user bind the
  principal. The governance-root scope is derived from the protected
  BootstrapRecord digest and grants no work, package, provider, repository,
  invocation, or execution authority. A conflicting or second root enrollment
  fails closed. The command is interactive and must be run by the human
  authority owner.
`)
	return err
}

func runAuthorityBootstrap(args []string, getenv func(string) string, input io.Reader, output io.Writer) error {
	return runAuthorityBootstrapWithTerminal(args, getenv, input, output, isInteractiveTerminal())
}

func runAuthorityBootstrapWithTerminal(args []string, getenv func(string) string, input io.Reader, output io.Writer, interactive bool) error {
	flags := flag.NewFlagSet("authority bootstrap", flag.ContinueOnError)
	flags.SetOutput(output)
	scope := flags.String("scope", "", "optional exact assertion of the derived installation governance scope")
	flags.Usage = func() { _ = writeAuthorityBootstrapHelp(output) }
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("usage: praxis authority bootstrap [--scope <canonical-installation-scope>] (interactive confirmation required)")
	}
	if !interactive {
		return errAuthorityBootstrapConfirmation
	}
	if getenv == nil {
		getenv = os.Getenv
	}
	bootstrapPath := getenv("PRAXIS_BOOTSTRAP_RECORD")
	if bootstrapPath == "" {
		return errors.New("PRAXIS_BOOTSTRAP_RECORD is required before authority enrollment")
	}
	record, err := praxiscrypto.LoadBootstrapRecord(bootstrapPath)
	if err != nil {
		return fmt.Errorf("load bootstrap metadata: %w", err)
	}
	recordDigest, err := record.Digest()
	if err != nil {
		return fmt.Errorf("digest bootstrap metadata: %w", err)
	}
	canonicalScope, err := contracts.InstallationGovernanceScope(recordDigest)
	if err != nil {
		return fmt.Errorf("derive installation governance scope: %w", err)
	}
	if supplied := strings.TrimSpace(*scope); supplied != "" && supplied != canonicalScope {
		return fmt.Errorf("invalid installation governance scope: expected %s", canonicalScope)
	}
	registry, err := praxiscrypto.NewFirstPartyBootstrapRegistry()
	if err != nil {
		return err
	}
	if _, err := registry.Open(context.Background(), record); err != nil {
		return fmt.Errorf("open bootstrap provider for authority enrollment: %w", err)
	}
	current, err := user.Current()
	if err != nil || current.Username == "" {
		return errors.New("authenticated local OS user is unavailable")
	}
	principal, err := contracts.InstallationOwnerPrincipal(recordDigest)
	if err != nil {
		return fmt.Errorf("derive installation owner principal: %w", err)
	}
	ref := canonicalScope
	prompt := fmt.Sprintf("This establishes principal %s for scope %q using the protected Praxis installation owned by the current OS user. It does not grant provider, repository, organization, or unrestricted authority. Type %q to continue: ", principal.ID, canonicalScope, authorityBootstrapConfirmationPrefix+" "+principal.ID)
	if _, err := io.WriteString(output, prompt); err != nil {
		return err
	}
	answer, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if strings.TrimSpace(answer) != authorityBootstrapConfirmationPrefix+" "+principal.ID {
		return errAuthorityBootstrapConfirmation
	}

	dbPath := getenv("PRAXIS_DB")
	if dbPath == "" {
		return errors.New("PRAXIS_DB is required before authority enrollment")
	}
	db, err := state.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		return fmt.Errorf("open authoritative Praxis state: %w", err)
	}
	defer db.Close()
	keys := praxiscrypto.NewProviderRegistry()
	wrapper, err := registry.Open(context.Background(), record)
	if err != nil {
		return fmt.Errorf("reopen bootstrap provider: %w", err)
	}
	if err := keys.Register(record.ProviderID, wrapper); err != nil {
		return err
	}
	service, err := keys.Service(record.ProviderID, praxiscrypto.EnvelopePolicy{})
	if err != nil {
		return err
	}
	repo := goalstore.Repository{Store: state.New(db), Crypto: service, KeyRef: record.KeyID, Profile: record.Profile, Sensitivity: state.SensitivityConfidential}
	ctx := context.Background()
	generations, err := repo.ListAuthorityGenerations(ctx, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("load existing authority generations: %w", err)
	}
	if existing, err := repo.LoadAuthorityGeneration(ctx, ref, "1", time.Now().UTC()); err == nil {
		if existing.Principal == principal && existing.Scope == canonicalScope && existing.ProvenanceDigest == recordDigest && authorityContainsCapability(existing.Capabilities, contracts.AuthorityDelegateCapability) {
			fmt.Fprintf(output, "authority principal already enrolled: principal=%s generation=%s/%s scope=%s\n", principal.ID, existing.Ref, existing.Version, existing.Scope)
			return nil
		}
		return errors.New("conflicting authority enrollment already exists")
	} else if len(generations) > 0 {
		return errors.New("a different authority generation already exists; second root enrollment is forbidden")
	}
	generation := contracts.AuthorityGeneration{Ref: ref, Version: "1", Principal: principal, Scope: canonicalScope, Capabilities: []string{contracts.AuthorityDelegateCapability}, AuthorityModel: contracts.AuthorityModelID, AuthorityModelVersion: contracts.AuthorityModelVersion, AuthorityModelDigest: contracts.AuthorityModelDigest(), ProvenanceRef: "bootstrap-record:" + recordDigest + ":os-user:" + current.Username, ProvenanceDigest: recordDigest, State: contracts.AuthorityGenerationActive, EffectiveAt: time.Now().UTC()}
	generation.Digest, err = generation.ComputeDigest()
	if err != nil {
		return fmt.Errorf("derive authority generation digest: %w", err)
	}
	if err := repo.SaveAuthorityGeneration(ctx, generation, generation.EffectiveAt, nil); err != nil {
		return fmt.Errorf("persist authority generation: %w", err)
	}
	fresh, err := repo.LoadAuthorityGeneration(ctx, generation.Ref, generation.Version, time.Now().UTC())
	if err != nil || fresh.Digest != generation.Digest {
		return fmt.Errorf("verify authority generation recovery: %w", err)
	}
	fmt.Fprintf(output, "authority principal enrolled: principal=%s generation=%s/%s digest=%s scope=%s\n", principal.ID, fresh.Ref, fresh.Version, fresh.Digest, fresh.Scope)
	return nil
}

func authorityContainsCapability(capabilities []string, want string) bool {
	for _, capability := range capabilities {
		if capability == want {
			return true
		}
	}
	return false
}

func isInteractiveTerminal() bool {
	info, err := os.Stdin.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

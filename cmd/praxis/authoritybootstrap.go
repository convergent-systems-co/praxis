package main

import (
	"bufio"
	"context"
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
	if len(args) == 0 || args[0] != "bootstrap" {
		return errors.New("usage: praxis authority bootstrap --scope <least-scope> (interactive confirmation required)")
	}
	return runAuthorityBootstrap(args[1:], os.Getenv, os.Stdin, os.Stdout)
}

func runAuthorityBootstrap(args []string, getenv func(string) string, input io.Reader, output io.Writer) error {
	return runAuthorityBootstrapWithTerminal(args, getenv, input, output, isInteractiveTerminal())
}

func runAuthorityBootstrapWithTerminal(args []string, getenv func(string) string, input io.Reader, output io.Writer, interactive bool) error {
	flags := flag.NewFlagSet("authority bootstrap", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	scope := flags.String("scope", "", "one explicit least-scope authority to grant")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || strings.TrimSpace(*scope) == "" {
		return errors.New("usage: praxis authority bootstrap --scope <least-scope> (interactive confirmation required)")
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
	principal := contracts.PrincipalRef{ID: "installation-owner:" + recordDigest, Kind: "human"}
	ref := "installation-governance:" + recordDigest
	prompt := fmt.Sprintf("This establishes principal %s for scope %q using the protected Praxis installation owned by the current OS user. It does not grant provider, repository, organization, or unrestricted authority. Type %q to continue: ", principal.ID, *scope, authorityBootstrapConfirmationPrefix+" "+principal.ID)
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
		if existing.Principal == principal && existing.Scope == strings.TrimSpace(*scope) && existing.ProvenanceDigest == recordDigest {
			fmt.Fprintf(output, "authority principal already enrolled: principal=%s generation=%s/%s scope=%s\n", principal.ID, existing.Ref, existing.Version, existing.Scope)
			return nil
		}
		return errors.New("conflicting authority enrollment already exists")
	} else if len(generations) > 0 {
		return errors.New("a different authority generation already exists; second root enrollment is forbidden")
	}
	generation := contracts.AuthorityGeneration{Ref: ref, Version: "1", Principal: principal, Scope: strings.TrimSpace(*scope), ProvenanceRef: "bootstrap-record:" + recordDigest + ":os-user:" + current.Username, ProvenanceDigest: recordDigest, State: contracts.AuthorityGenerationActive, EffectiveAt: time.Now().UTC()}
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

func isInteractiveTerminal() bool {
	info, err := os.Stdin.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

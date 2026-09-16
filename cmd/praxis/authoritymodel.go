package main

import (
	"bufio"
	"context"
	"database/sql"
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

func openGovernedRepositoryReadOnly(ctx context.Context, getenv func(string) string) (goalstore.Repository, *sql.DB, praxiscrypto.BootstrapRecord, error) {
	bootstrapPath, dbPath := getenv("PRAXIS_BOOTSTRAP_RECORD"), getenv("PRAXIS_DB")
	if bootstrapPath == "" || dbPath == "" {
		return goalstore.Repository{}, nil, praxiscrypto.BootstrapRecord{}, errors.New("PRAXIS_BOOTSTRAP_RECORD and PRAXIS_DB are required")
	}
	record, err := praxiscrypto.LoadBootstrapRecord(bootstrapPath)
	if err != nil {
		return goalstore.Repository{}, nil, record, err
	}
	registry, err := praxiscrypto.NewFirstPartyBootstrapRegistry()
	if err != nil {
		return goalstore.Repository{}, nil, record, err
	}
	wrapper, err := registry.Open(ctx, record)
	if err != nil {
		return goalstore.Repository{}, nil, record, err
	}
	providers := praxiscrypto.NewProviderRegistry()
	if err = providers.Register(record.ProviderID, wrapper); err != nil {
		return goalstore.Repository{}, nil, record, err
	}
	service, err := providers.Service(record.ProviderID, praxiscrypto.EnvelopePolicy{})
	if err != nil {
		return goalstore.Repository{}, nil, record, err
	}
	db, err := state.OpenSQLiteReadOnly(ctx, dbPath)
	if err != nil {
		return goalstore.Repository{}, nil, record, err
	}
	return goalstore.Repository{Store: state.New(db), Crypto: service, KeyRef: record.KeyID, Profile: record.Profile, Sensitivity: state.SensitivityConfidential}, db, record, nil
}

func adoptionFromRepository(ctx context.Context, repo goalstore.Repository, record praxiscrypto.BootstrapRecord, now time.Time) (contracts.AuthorityModelAdoption, error) {
	bootstrapDigest, err := record.Digest()
	if err != nil {
		return contracts.AuthorityModelAdoption{}, err
	}
	owner, err := contracts.InstallationOwnerPrincipal(bootstrapDigest)
	if err != nil {
		return contracts.AuthorityModelAdoption{}, err
	}
	model, err := repo.LoadAuthorityModelState(ctx, now)
	if err != nil {
		return contracts.AuthorityModelAdoption{}, err
	}
	gens, err := repo.ListAuthorityGenerations(ctx, now)
	if err != nil {
		return contracts.AuthorityModelAdoption{}, err
	}
	for _, g := range gens {
		if g.ParentRef == "" && g.Principal == owner {
			if model.ActiveVersion == contracts.AuthorityModelVersion {
				return contracts.AuthorityModelAdoption{ID: "authority-model-adoption:v1-to-v2", Version: "1", FromModel: contracts.AuthorityModelID, FromVersion: contracts.AuthorityModelVersion, FromDigest: contracts.AuthorityModelDigest(), ToModel: contracts.AuthorityModelID, ToVersion: contracts.AuthorityModelSuccessorVersion, ToDigest: contracts.AuthorityModelSuccessorDigest(), RootRef: g.Ref, RootVersion: g.Version, RootDigest: g.Digest, Reason: "adopt accepted built-in authority model successor", CreatedAt: now.UTC()}, nil
			}
			if model.ActiveVersion == contracts.AuthorityModelSuccessorVersion {
				legacyID := "authority-model-adoption:v2-to-v3"
				if stale, err := repo.LoadAuthorityModelAdoptionByID(ctx, legacyID, "1", now); err == nil {
					digest, _ := stale.Digest()
					if _, err := repo.LoadAuthorityModelAdoptionDecision(ctx, digest, now); err != nil {
						superseded, err := repo.IsAuthorityModelAdoptionSuperseded(ctx, stale.ID, stale.Version, now)
						if err != nil {
							return contracts.AuthorityModelAdoption{}, err
						}
						if !superseded {
							return contracts.AuthorityModelAdoption{}, errors.New("historical incomplete v3 adoption must be abandoned before a new preview")
						}
					}
				}
				return contracts.AuthorityModelAdoption{ID: "authority-model-adoption:v2-to-v3:" + now.UTC().Format(time.RFC3339Nano), Version: "1", FromModel: contracts.AuthorityModelID, FromVersion: contracts.AuthorityModelSuccessorVersion, FromDigest: contracts.AuthorityModelSuccessorDigest(), ToModel: contracts.AuthorityModelID, ToVersion: contracts.AuthorityModelDeploymentVersion, ToDigest: contracts.AuthorityModelDeploymentDigest(), RootRef: g.Ref, RootVersion: g.Version, RootDigest: g.Digest, Reason: "adopt accepted built-in package-deployment authority model", CreatedAt: now.UTC()}, nil
			}
			if model.ActiveVersion == contracts.AuthorityModelDeploymentVersion && model.ActiveDigest == contracts.AuthorityModelDeploymentDigest() && bootstrapDigest == contracts.GoalsPublicationBootstrap && g.Digest == contracts.GoalsPublicationRoot {
				return contracts.AuthorityModelAdoption{ID: "authority-model-adoption:v3-to-v4:" + now.UTC().Format(time.RFC3339Nano), Version: "1", FromModel: contracts.AuthorityModelID, FromVersion: contracts.AuthorityModelDeploymentVersion, FromDigest: contracts.AuthorityModelDeploymentDigest(), ToModel: contracts.AuthorityModelID, ToVersion: contracts.AuthorityModelGoalsPublicationVersion, ToDigest: contracts.AuthorityModelGoalsPublicationDigest(), RootRef: g.Ref, RootVersion: g.Version, RootDigest: g.Digest, Reason: "adopt accepted exact Goals initial-publication edge", CreatedAt: now.UTC()}, nil
			}
			if model.ActiveVersion == contracts.AuthorityModelGoalsPublicationVersion && model.ActiveDigest == contracts.AuthorityModelGoalsPublicationDigest() && bootstrapDigest == contracts.GoalsPublicationBootstrap && g.Digest == contracts.GoalsPublicationRoot {
				return contracts.AuthorityModelAdoption{ID: "authority-model-adoption:v4-to-v5:" + now.UTC().Format(time.RFC3339Nano), Version: "1", FromModel: contracts.AuthorityModelID, FromVersion: contracts.AuthorityModelGoalsPublicationVersion, FromDigest: contracts.AuthorityModelGoalsPublicationDigest(), ToModel: contracts.AuthorityModelID, ToVersion: contracts.AuthorityModelGoalsRecoveryVersion, ToDigest: contracts.AuthorityModelGoalsRecoveryDigest(), RootRef: g.Ref, RootVersion: g.Version, RootDigest: g.Digest, Reason: "adopt accepted exact Goals established-state successor edge", CreatedAt: now.UTC()}, nil
			}
			return contracts.AuthorityModelAdoption{}, errors.New("authority model has no adoptable successor")
		}
	}
	return contracts.AuthorityModelAdoption{}, errors.New("installation root generation unavailable")
}

func currentOSUser() string {
	current, err := user.Current()
	if err != nil {
		return ""
	}
	return current.Username
}

func runAuthorityModelPreview(args []string, getenv func(string) string, out io.Writer) error {
	f := flag.NewFlagSet("authority model-preview", flag.ContinueOnError)
	f.SetOutput(out)
	output := f.String("output", "", "optional preview JSON output path")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return errors.New("usage: praxis authority model-preview [--output <path>]")
	}
	repo, db, record, err := openGovernedRepositoryReadOnly(context.Background(), getenv)
	if err != nil {
		return err
	}
	defer db.Close()
	adoption, err := adoptionFromRepository(context.Background(), repo, record, time.Now().UTC())
	if err != nil {
		return err
	}
	payload, err := authorityModelPreviewPayload(adoption)
	if err != nil {
		return err
	}
	if *output != "" {
		if err := writeCanonicalPreviewFile(*output, payload); err != nil {
			return err
		}
	}
	_, err = fmt.Fprint(out, string(payload))
	return err
}

func authorityModelPreviewPayload(adoption contracts.AuthorityModelAdoption) ([]byte, error) {
	digest, err := adoption.Digest()
	if err != nil {
		return nil, err
	}
	payload, err := json.MarshalIndent(map[string]any{
		"preview":        true,
		"adoption":       adoption,
		"preview_digest": digest,
		"confirmation":   "ADOPT " + digest,
	}, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(payload, '\n'), nil
}

func runAuthorityModelAdopt(args []string, getenv func(string) string, input io.Reader, out io.Writer) error {
	f := flag.NewFlagSet("authority model-adopt", flag.ContinueOnError)
	f.SetOutput(out)
	previewPath := f.String("preview-file", "", "system-produced adoption preview JSON")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 || *previewPath == "" {
		return errors.New("usage: praxis authority model-adopt --preview-file <system-produced-preview.json> (interactive confirmation required)")
	}
	if !isInteractiveTerminal() {
		return errAuthorityBootstrapConfirmation
	}
	payload, err := os.ReadFile(*previewPath)
	if err != nil {
		return err
	}
	var envelope struct {
		Adoption      contracts.AuthorityModelAdoption `json:"adoption"`
		PreviewDigest string                           `json:"preview_digest"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return err
	}
	d, err := envelope.Adoption.Digest()
	if err != nil || d != envelope.PreviewDigest {
		return errors.New("adoption preview digest mismatch")
	}
	record, err := praxiscrypto.LoadBootstrapRecord(getenv("PRAXIS_BOOTSTRAP_RECORD"))
	if err != nil {
		return err
	}
	current, err := user.Current()
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Authorize exact authority-model adoption %s. Type %q to continue: ", d, "ADOPT "+d)
	answer, err := bufio.NewReader(input).ReadString('\n')
	if err != nil {
		return err
	}
	if strings.TrimSpace(answer) != "ADOPT "+d {
		return errAuthorityBootstrapConfirmation
	}
	repo, db, err := openGovernedRepository(context.Background(), getenv)
	if err != nil {
		return err
	}
	defer db.Close()
	bootstrapDigest, err := record.Digest()
	if err != nil {
		return err
	}
	result, err := repo.AdoptAuthorityModel(context.Background(), envelope.Adoption, bootstrapDigest, current.Username, "ADOPT "+d, time.Now().UTC())
	if err != nil {
		return err
	}
	return printJSONTo(out, map[string]any{"operation": "authority.model-adopt", "adoption_digest": result, "model": envelope.Adoption.ToVersion})
}

func runAuthorityModelAbandon(args []string, getenv func(string) string, input io.Reader, out io.Writer) error {
	f := flag.NewFlagSet("authority model-abandon", flag.ContinueOnError)
	f.SetOutput(out)
	digest := f.String("adoption", "", "exact stale adoption digest")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 || *digest == "" || !isInteractiveTerminal() {
		return errAuthorityBootstrapConfirmation
	}
	repo, db, err := openGovernedRepository(context.Background(), getenv)
	if err != nil {
		return err
	}
	defer db.Close()
	record, err := praxiscrypto.LoadBootstrapRecord(getenv("PRAXIS_BOOTSTRAP_RECORD"))
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Abandon exact incomplete authority-model adoption %s. Type %q to continue: ", *digest, "ABANDON "+*digest)
	answer, err := bufio.NewReader(input).ReadString('\n')
	if err != nil {
		return err
	}
	if strings.TrimSpace(answer) != "ABANDON "+*digest {
		return errAuthorityBootstrapConfirmation
	}
	bootstrapDigest, err := record.Digest()
	if err != nil {
		return err
	}
	supersession, err := repo.AbandonAuthorityModelAdoption(context.Background(), *digest, bootstrapDigest, currentOSUser(), "ABANDON "+*digest, time.Now().UTC())
	if err != nil {
		return err
	}
	return printJSONTo(out, map[string]any{"operation": "authority.model-abandon", "supersession_digest": supersession, "adoption_digest": *digest})
}

func runAuthorityModelStatus(args []string, getenv func(string) string, out io.Writer) error {
	if len(args) != 0 {
		return errors.New("usage: praxis authority model-status")
	}
	repo, db, _, err := openGovernedRepositoryReadOnly(context.Background(), getenv)
	if err != nil {
		return err
	}
	defer db.Close()
	s, err := repo.LoadAuthorityModelState(context.Background(), time.Now().UTC())
	if err != nil {
		return err
	}
	return printJSONTo(out, s)
}

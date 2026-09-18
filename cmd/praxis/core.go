package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"runtime"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/internal/goalstore"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/internal/stateprovider"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

const praxisVersion = "2.0.0-dev"

func runVersion(args []string) error {
	if len(args) != 0 {
		return errors.New("usage: praxis version")
	}
	fmt.Println(praxisVersion)
	return nil
}

func runDoctor(args []string) error {
	if len(args) != 0 {
		return errors.New("usage: praxis doctor")
	}
	result := map[string]any{
		"praxis_version":         praxisVersion,
		"go_version":             runtime.Version(),
		"state":                  "not configured",
		"governance_root":        map[string]any{"status": "unavailable"},
		"authority_topology":     "unqualified",
		"installation_readiness": "not ready",
	}
	bootstrapPath := os.Getenv("PRAXIS_BOOTSTRAP_RECORD")
	var doctorRecord *praxiscrypto.BootstrapRecord
	var bootstrapErr error
	if bootstrapPath == "" {
		result["bootstrap"] = "not configured"
		bootstrapErr = errors.New("bootstrap metadata is not configured")
	} else if record, err := praxiscrypto.LoadBootstrapRecord(bootstrapPath); err != nil {
		result["bootstrap"] = "failed"
		result["bootstrap_error"] = err.Error()
		bootstrapErr = err
	} else {
		doctorRecord = &record
		registry, err := praxiscrypto.NewFirstPartyBootstrapRegistry()
		if err != nil {
			bootstrapErr = err
		} else if _, err := registry.Open(context.Background(), record); err != nil {
			bootstrapErr = err
		}
		if bootstrapErr != nil {
			result["bootstrap"] = "unavailable"
			result["bootstrap_error"] = bootstrapErr.Error()
		} else {
			result["bootstrap"] = "ready"
		}
		result["bootstrap_record"] = bootstrapPath
	}
	path := os.Getenv("PRAXIS_DB")
	if path != "" {
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			result["state"] = "uninitialized"
			result["database"] = path
			return printJSON(result)
		}
		db, err := state.OpenSQLiteReadOnly(context.Background(), path)
		if err != nil {
			result["state"] = "failed"
			result["state_error"] = err.Error()
			_ = printJSON(result)
			return fmt.Errorf("state provider: %w", err)
		}
		defer db.Close()
		if bootstrapErr != nil {
			result["state"] = "bootstrap mismatch"
			result["database"] = path
			_ = printJSON(result)
			return fmt.Errorf("bootstrap/state mismatch: %w", bootstrapErr)
		}
		provider := stateprovider.NewSQLite(db)
		if err := provider.Profile().Require(
			stateprovider.EventsAppendOptimistic,
			stateprovider.EventsReplay,
			stateprovider.LeaseAtomicConsume,
			stateprovider.PackagesAtomicActivation,
			stateprovider.RunsDurableReplay,
		); err != nil {
			result["state"] = "failed"
			result["state_error"] = err.Error()
			_ = printJSON(result)
			return err
		}
		result["state"] = "ok"
		result["state_provider"] = "sqlite"
		result["database"] = path
		if bootstrapErr == nil {
			topology, err := inspectAuthorityTopology(context.Background(), db, doctorRecord)
			if err != nil {
				result["governance_root"] = map[string]any{"status": "invalid", "error": err.Error()}
			} else {
				result["governance_root"] = topology.Root
				result["authority_topology"] = topology.Status
				if topology.Status == "qualified" {
					result["installation_readiness"] = "ready"
				}
			}
		}
	}
	return printJSON(result)
}

type authorityTopologyInspection struct {
	Status string
	Root   map[string]any
}

func inspectAuthorityTopology(ctx context.Context, db *sql.DB, record *praxiscrypto.BootstrapRecord) (authorityTopologyInspection, error) {
	if record == nil {
		return authorityTopologyInspection{Status: "unqualified"}, errors.New("bootstrap record is unavailable")
	}
	digest, err := record.Digest()
	if err != nil {
		return authorityTopologyInspection{Status: "unqualified"}, err
	}
	registry, err := praxiscrypto.NewFirstPartyBootstrapRegistry()
	if err != nil {
		return authorityTopologyInspection{Status: "unqualified"}, err
	}
	wrapper, err := registry.Open(ctx, *record)
	if err != nil {
		return authorityTopologyInspection{Status: "unqualified"}, err
	}
	providers := praxiscrypto.NewProviderRegistry()
	if err := providers.Register(record.ProviderID, wrapper); err != nil {
		return authorityTopologyInspection{Status: "unqualified"}, err
	}
	service, err := providers.Service(record.ProviderID, praxiscrypto.EnvelopePolicy{})
	if err != nil {
		return authorityTopologyInspection{Status: "unqualified"}, err
	}
	repo := goalstore.Repository{Store: state.New(db), Crypto: service, KeyRef: record.KeyID, Profile: record.Profile, Sensitivity: state.SensitivityConfidential, InstallationDigest: digest}
	generations, err := repo.ListAuthorityGenerations(ctx, time.Now().UTC())
	if err != nil {
		return authorityTopologyInspection{Status: "unqualified"}, err
	}
	rootScope, err := contracts.InstallationGovernanceScope(digest)
	if err != nil {
		return authorityTopologyInspection{Status: "unqualified"}, err
	}
	principal, err := contracts.InstallationOwnerPrincipal(digest)
	if err != nil {
		return authorityTopologyInspection{Status: "unqualified"}, err
	}
	root, err := repo.LoadCurrentInstallationRoot(ctx, digest, time.Now().UTC())
	if err != nil {
		return authorityTopologyInspection{Status: "unqualified"}, err
	}
	if root.Ref != rootScope || root.Digest == "" || root.Principal != principal || root.Scope != rootScope || root.AuthorityModel != contracts.AuthorityModelID || root.AuthorityModelVersion != contracts.AuthorityModelVersion || root.AuthorityModelDigest != contracts.AuthorityModelDigest() || !authorityContainsCapability(root.Capabilities, contracts.AuthorityDelegateCapability) || root.ParentRef != "" {
		return authorityTopologyInspection{Status: "unqualified"}, errors.New("installation governance root does not match authority model v1")
	}
	if err := root.VerifyDigest(); err != nil {
		return authorityTopologyInspection{Status: "unqualified"}, err
	}
	return authorityTopologyInspection{Status: "qualified", Root: map[string]any{"status": "ready", "principal": root.Principal, "generation_ref": root.Ref, "generation_version": root.Version, "generation_digest": root.Digest, "scope": root.Scope, "capabilities": root.Capabilities, "authorities": root.Authorities, "predecessor_ref": root.PredecessorRef, "predecessor_version": root.PredecessorVersion, "predecessor_digest": root.PredecessorDigest, "retained_root_generations": len(generations), "authority_model": root.AuthorityModel, "authority_model_version": root.AuthorityModelVersion, "authority_model_digest": root.AuthorityModelDigest}}, nil
}

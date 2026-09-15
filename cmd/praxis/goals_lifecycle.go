package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/convergent-systems-co/praxis/internal/client"
	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/internal/goalstore"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/packages/goals"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func init() {
	if err := client.RegisterInvocationHandler("praxis.package.goals", "goals-lifecycle", dispatchGoalsLifecycle); err != nil {
		panic(err)
	}
}

// dispatchGoalsLifecycle is deliberately an adapter: the validation and
// authority-bearing transitions remain in packages/goals and goalstore.
func dispatchGoalsLifecycle(ctx context.Context, in client.ResolvedInvocation, getenv func(string) string) error {
	operation := in.Options["operation"]
	inputPath := in.Options["input"]
	if operation == "" {
		return errors.New("goals-lifecycle requires --operation")
	}
	var input []byte
	var err error
	if inputPath != "" {
		path, pathErr := filepath.Abs(inputPath)
		if pathErr != nil {
			return pathErr
		}
		input, err = os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read Goals lifecycle input: %w", err)
		}
	}
	if operation == "inspect" {
		return inspectGoalsLifecycle(ctx, in.Options, getenv)
	}
	if len(input) == 0 {
		return errors.New("Goals lifecycle mutation requires --input <canonical-json>")
	}
	repo, db, err := openGovernedRepository(ctx, getenv)
	if err != nil {
		return err
	}
	defer db.Close()
	now := time.Now().UTC()

	switch operation {
	case "propose":
		var req struct {
			BaselineID, BaselineVersion, ProposalVersion string
			Proposal                                     contracts.WorkPlanProposal `json:"proposal"`
		}
		if err := json.Unmarshal(input, &req); err != nil {
			return err
		}
		baseline, err := repo.Load(ctx, req.BaselineID, req.BaselineVersion, now)
		if err != nil {
			return err
		}
		proposal, err := goals.BuildWorkPlanProposal(baseline, req.Proposal.ID, req.Proposal.ProposedBy, req.Proposal.ProposerGeneration, req.Proposal.Candidates, req.Proposal.Relationships)
		if err != nil {
			return err
		}
		version := req.ProposalVersion
		if version == "" {
			version = "1"
		}
		digest, err := repo.SaveWorkPlanProposal(ctx, proposal, version, now, nil)
		if err != nil {
			return err
		}
		return printJSON(map[string]any{"operation": operation, "proposal": proposal, "record_digest": digest})
	case "review":
		var req struct {
			ProposalID, ProposalVersion, ReviewVersion string
			Review                                     contracts.WorkPlanProposalReview `json:"review"`
		}
		if err := json.Unmarshal(input, &req); err != nil {
			return err
		}
		if err := repo.SaveWorkPlanReview(ctx, req.ProposalID, req.ProposalVersion, req.Review, req.ReviewVersion, now, nil); err != nil {
			return err
		}
		return printJSON(map[string]any{"operation": operation, "review": req.Review})
	case "request":
		var req contracts.AuthorityRequest
		if err := json.Unmarshal(input, &req); err != nil {
			return err
		}
		digest, err := repo.SaveAuthorityRequest(ctx, req, now, nil)
		if err != nil {
			return err
		}
		return printJSON(map[string]any{"operation": operation, "request": req, "record_digest": digest})
	case "decide":
		var req struct {
			RequestID, RequestVersion string
			Decision                  contracts.AuthorityDecision `json:"decision"`
		}
		if err := json.Unmarshal(input, &req); err != nil {
			return err
		}
		if err := repo.SaveAuthorityDecision(ctx, req.RequestID, req.RequestVersion, req.Decision, now, nil); err != nil {
			return err
		}
		return printJSON(map[string]any{"operation": operation, "decision": req.Decision})
	case "accept", "bind":
		var req struct {
			RequestID, RequestVersion, AcceptanceRef, AcceptanceVersion string
			Plan                                                        contracts.WorkPlan `json:"plan"`
		}
		if err := json.Unmarshal(input, &req); err != nil {
			return err
		}
		plan, err := repo.SaveAcceptedWorkPlanFromAuthorityDecision(ctx, req.RequestID, req.RequestVersion, req.Plan, req.AcceptanceRef, req.AcceptanceVersion, now, nil)
		if err != nil {
			return err
		}
		return printJSON(map[string]any{"operation": operation, "plan": plan})
	default:
		return fmt.Errorf("unsupported Goals lifecycle operation %q", operation)
	}
}

func openGovernedRepository(ctx context.Context, getenv func(string) string) (goalstore.Repository, *sql.DB, error) {
	if getenv == nil {
		getenv = os.Getenv
	}
	bootstrapPath, dbPath := getenv("PRAXIS_BOOTSTRAP_RECORD"), getenv("PRAXIS_DB")
	if bootstrapPath == "" || dbPath == "" {
		return goalstore.Repository{}, nil, errors.New("PRAXIS_BOOTSTRAP_RECORD and PRAXIS_DB are required for governed Goals lifecycle operations")
	}
	record, err := praxiscrypto.LoadBootstrapRecord(bootstrapPath)
	if err != nil {
		return goalstore.Repository{}, nil, err
	}
	registry, err := praxiscrypto.NewFirstPartyBootstrapRegistry()
	if err != nil {
		return goalstore.Repository{}, nil, err
	}
	wrapper, err := registry.Open(ctx, record)
	if err != nil {
		return goalstore.Repository{}, nil, err
	}
	providers := praxiscrypto.NewProviderRegistry()
	if err := providers.Register(record.ProviderID, wrapper); err != nil {
		return goalstore.Repository{}, nil, err
	}
	service, err := providers.Service(record.ProviderID, praxiscrypto.EnvelopePolicy{})
	if err != nil {
		return goalstore.Repository{}, nil, err
	}
	db, err := state.OpenSQLite(ctx, dbPath)
	if err != nil {
		return goalstore.Repository{}, nil, err
	}
	repo := goalstore.Repository{Store: state.New(db), Crypto: service, KeyRef: record.KeyID, Profile: record.Profile, Sensitivity: state.SensitivityConfidential}
	return repo, db, nil
}

func inspectGoalsLifecycle(ctx context.Context, options map[string]string, getenv func(string) string) error {
	repo, db, err := openGovernedRepository(ctx, getenv)
	if err != nil {
		return err
	}
	defer db.Close()
	goalID, version := options["goal-id"], options["goal-version"]
	if goalID == "" || version == "" {
		return errors.New("Goals inspection requires exact --goal-id and --goal-version")
	}
	baseline, err := repo.Load(ctx, goalID, version, time.Now().UTC())
	if err != nil {
		return err
	}
	pending, err := repo.PendingAuthorityRequests(ctx, goalID, version, time.Now().UTC())
	if err != nil {
		return err
	}
	return printJSON(map[string]any{"goal": baseline, "pending_authority": pending})
}

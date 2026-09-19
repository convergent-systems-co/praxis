package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/convergent-systems-co/praxis/internal/client"
	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/internal/goaldrive"
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
	if _, selected := selectorFromOptions(in.Options); len(input) == 0 && !selected {
		return errors.New("Goals lifecycle mutation requires exact selector options or --input <document>")
	}
	repo, db, err := openGovernedRepository(ctx, getenv)
	if err != nil {
		return err
	}
	defer db.Close()
	now := time.Now().UTC()

	switch operation {
	case "import":
		path, err := filepath.Abs(inputPath)
		if err != nil {
			return err
		}
		imported, err := importGoalBaseline(ctx, repo, path, input, now)
		if err != nil {
			return err
		}
		return printJSON(map[string]any{"operation": operation, "goal_id": imported.ID, "goal_version": imported.Version, "baseline_digest": imported.Digest, "status": "authoritative"})
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
		return printJSON(map[string]any{"operation": operation, "proposal": proposal, "record_digest": digest, "proposal_digest": digest, "review_accept_with": reviewCommand(digest, string(contracts.ReviewAcceptableForAuthority)), "review_revise_with": reviewCommand(digest, string(contracts.ReviewRevisionRequired))})
	case "review":
		if selector, ok := lifecycleSelectorFor(in.Options, input); ok {
			if selector.ReviewedBy.ID == "" || selector.ReviewerGeneration == "" {
				owner, root, err := installationOwnerAndRoot(ctx, repo, getenv, now)
				if err != nil {
					return err
				}
				if selector.ReviewedBy.ID == "" {
					selector.ReviewedBy = owner
				}
				if selector.ReviewerGeneration == "" {
					selector.ReviewerGeneration = root.Ref + "/" + root.Version + "@" + root.Digest
				}
			}
			review, proposalID, proposalVersion, err := reviewFromSelector(ctx, repo, selector, now)
			if err != nil {
				return err
			}
			if err := repo.SaveWorkPlanReview(ctx, proposalID, proposalVersion, review, "1", now, nil); err != nil {
				return err
			}
			proposal, _, err := repo.LoadWorkPlanProposalByDigest(ctx, selector.ProposalDigest, now)
			if err != nil {
				return err
			}
			result := map[string]any{"operation": operation, "review": review, "review_ref": review.ReviewRef, "review_digest": review.ReviewDigest, "proposal_digest": review.ProposalDigest}
			if review.Status == contracts.ReviewAcceptableForAuthority {
				result["request_with"] = requestCommand(proposal.GoalID, proposal.GoalVersion, review.ProposalDigest, review.ReviewDigest)
			}
			return printJSON(result)
		}
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
		if selector, ok := lifecycleSelectorFor(in.Options, input); ok {
			goalID, goalVersion := in.Options["goal-id"], in.Options["goal-version"]
			if goalID == "" || goalVersion == "" || selector.ProposalDigest == "" || selector.ReviewDigest == "" {
				return errors.New("request requires --goal-id, --goal-version, --proposal-digest, and --review-digest")
			}
			baseline, err := repo.Load(ctx, goalID, goalVersion, now)
			if err != nil {
				return err
			}
			proposal, proposalVersion, err := repo.LoadWorkPlanProposalByDigest(ctx, selector.ProposalDigest, now)
			if err != nil {
				return err
			}
			if proposal.GoalID != baseline.ID || proposal.GoalVersion != baseline.Version || proposal.BaselineDigest != baseline.Digest {
				return errors.New("proposal is not bound to this exact Goal generation")
			}
			review, reviewVersion, err := repo.LoadWorkPlanReviewByDigest(ctx, selector.ProposalDigest, selector.ReviewDigest, now)
			if err != nil {
				return err
			}
			if review.Status != contracts.ReviewAcceptableForAuthority {
				return fmt.Errorf("review %s is %q, not acceptable for an authority decision", review.ReviewDigest, review.Status)
			}
			_, root, err := installationOwnerAndRoot(ctx, repo, getenv, now)
			if err != nil {
				return err
			}
			reason := selector.Reason
			if reason == "" {
				reason = "accept the reviewed WorkPlan decomposition of " + baseline.ID + "/" + baseline.Version
			}
			req := contracts.AuthorityRequest{ID: "workplan-accept:" + baseline.ID + "/" + baseline.Version + ":" + strconv.FormatInt(now.UnixNano(), 10), Version: "1", BaselineID: baseline.ID, BaselineVersion: baseline.Version, BaselineDigest: baseline.Digest, ProposalID: proposal.ID, ProposalVersion: proposalVersion, ProposalDigest: selector.ProposalDigest, ReviewRef: review.ReviewRef, ReviewVersion: reviewVersion, ReviewDigest: review.ReviewDigest, RequestedAuthority: "workplan.accept", RequestedScope: root.Scope, Reason: reason, Status: contracts.AuthorityRequestPending}
			digest, err := repo.SaveAuthorityRequest(ctx, req, now, nil)
			if err != nil {
				return err
			}
			return printJSON(map[string]any{"operation": operation, "request": req, "request_digest": digest, "resolve_with": decideCommand(digest, "approve"), "reject_with": decideCommand(digest, "reject")})
		}
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
		if selector, ok := lifecycleSelectorFor(in.Options, input); ok {
			if selector.RequestDigest == "" {
				return errors.New("accept requires --request-digest")
			}
			request, err := repo.LoadAuthorityRequestByDigest(ctx, selector.RequestDigest, now)
			if err != nil {
				return err
			}
			if request.RequestedAuthority != "workplan.accept" || request.ProposalID == "" {
				return errors.New("request is not a WorkPlan acceptance request")
			}
			proposal, err := repo.LoadWorkPlanProposal(ctx, request.ProposalID, request.ProposalVersion, now)
			if err != nil {
				return err
			}
			plan := contracts.WorkPlan{Candidates: append([]contracts.WorkCandidate(nil), proposal.Candidates...), Relationships: append([]contracts.WorkRelationship(nil), proposal.Relationships...)}
			acceptanceRef := selector.AcceptanceRef
			if acceptanceRef == "" {
				acceptanceRef = "acceptance:" + request.ID
			}
			accepted, err := repo.SaveAcceptedWorkPlanFromAuthorityDecision(ctx, request.ID, request.Version, plan, acceptanceRef, "1", now, nil)
			if err != nil {
				return err
			}
			return printJSON(map[string]any{"operation": operation, "plan": accepted, "acceptance_ref": acceptanceRef, "acceptance_version": "1", "attach_with": attachCommand(request.BaselineID, request.BaselineVersion, acceptanceRef)})
		}
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
	case "attach":
		// attach creates the successor immutable Goal generation that carries
		// an authority-backed accepted WorkPlan. It is the only way a Goal
		// becomes drivable: goal-drive materializes work from the baseline's
		// embedded WorkPlan and never from prose, PlanRef, or model output.
		if selector, ok := lifecycleSelectorFor(in.Options, input); ok {
			goalID, goalVersion := in.Options["goal-id"], in.Options["goal-version"]
			if goalID == "" || goalVersion == "" || selector.AcceptanceRef == "" {
				return errors.New("attach requires --goal-id, --goal-version, and --acceptance-ref")
			}
			source, err := repo.Load(ctx, goalID, goalVersion, now)
			if err != nil {
				return err
			}
			next, err := strconv.Atoi(goalVersion)
			if err != nil {
				return fmt.Errorf("Goal generation %q is not numeric; supply the successor version explicitly", goalVersion)
			}
			successorVersion := strconv.Itoa(next + 1)
			// Replay: the successor already carries this exact acceptance.
			if existing, err := repo.Load(ctx, source.ID, successorVersion, now); err == nil {
				if existing.PredecessorDigest == source.Digest && existing.WorkPlan != nil && existing.WorkPlan.AcceptanceRef == selector.AcceptanceRef {
					return printJSON(map[string]any{"operation": operation, "replay": true, "goal_id": existing.ID, "goal_version": existing.Version, "baseline_digest": existing.Digest, "predecessor_digest": existing.PredecessorDigest, "work_plan_candidates": len(existing.WorkPlan.Candidates), "drive_template": driveTemplate(existing)})
				}
				return fmt.Errorf("Goal generation %s/%s already exists with a different lineage", existing.ID, existing.Version)
			}
			successor, err := repo.AttachAcceptedWorkPlan(ctx, source.ID, source.Version, source.Digest, selector.AcceptanceRef, "1", successorVersion, now, nil)
			if err != nil {
				return err
			}
			return printJSON(map[string]any{"operation": operation, "goal_id": successor.ID, "goal_version": successor.Version, "baseline_digest": successor.Digest, "predecessor_digest": successor.PredecessorDigest, "work_plan_candidates": len(successor.WorkPlan.Candidates), "drive_template": driveTemplate(successor)})
		}
		var req struct {
			GoalID, GoalVersion, BaselineDigest, AcceptanceRef, AcceptanceVersion, SuccessorVersion string
		}
		if err := json.Unmarshal(input, &req); err != nil {
			return err
		}
		if req.AcceptanceVersion == "" {
			req.AcceptanceVersion = "1"
		}
		successor, err := repo.AttachAcceptedWorkPlan(ctx, req.GoalID, req.GoalVersion, req.BaselineDigest, req.AcceptanceRef, req.AcceptanceVersion, req.SuccessorVersion, now, nil)
		if err != nil {
			return err
		}
		return printJSON(map[string]any{"operation": operation, "goal_id": successor.ID, "goal_version": successor.Version, "baseline_digest": successor.Digest, "predecessor_digest": successor.PredecessorDigest, "work_plan_candidates": len(successor.WorkPlan.Candidates)})
	default:
		return fmt.Errorf("unsupported Goals lifecycle operation %q", operation)
	}
}

// openGovernedRepository is a variable so governed CLI qualification can open
// fixture installations without the platform bootstrap backend.
var openGovernedRepository = openGovernedRepositoryFromBootstrap

func openGovernedRepositoryFromBootstrap(ctx context.Context, getenv func(string) string) (goalstore.Repository, *sql.DB, error) {
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
	installationDigest, err := record.Digest()
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
	repo := goalstore.Repository{Store: state.New(db), Crypto: service, KeyRef: record.KeyID, Profile: record.Profile, Sensitivity: state.SensitivityConfidential, InstallationDigest: installationDigest}
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
	now := time.Now().UTC()
	pending, err := repo.PendingAuthorityRequests(ctx, goalID, version, now)
	if err != nil {
		return err
	}
	proposals, proposalDigests, err := repo.ListWorkPlanProposals(ctx, goalID, version, now)
	if err != nil {
		return err
	}
	proposalEntries := make([]map[string]any, 0, len(proposals))
	reviewEntries := make([]map[string]any, 0)
	for i, proposal := range proposals {
		proposalEntries = append(proposalEntries, map[string]any{"id": proposal.ID, "proposal_digest": proposalDigests[i], "proposed_by": proposal.ProposedBy, "proposer_generation": proposal.ProposerGeneration, "candidates": len(proposal.Candidates)})
		reviews, _, err := repo.ListWorkPlanReviews(ctx, proposalDigests[i], now)
		if err != nil {
			return err
		}
		for _, review := range reviews {
			reviewEntries = append(reviewEntries, map[string]any{"review_ref": review.ReviewRef, "review_digest": review.ReviewDigest, "proposal_digest": review.ProposalDigest, "reviewed_by": review.ReviewedBy, "status": review.Status})
		}
	}
	requests, err := repo.ListAuthorityRequests(ctx, goalID, version, now)
	if err != nil {
		return err
	}
	requestEntries := make([]map[string]any, 0, len(requests))
	for _, item := range requests {
		entry := map[string]any{"id": item.Request.ID, "request_digest": item.RequestDigest, "status": item.Status, "requested_authority": item.Request.RequestedAuthority, "proposal_digest": item.Request.ProposalDigest, "review_digest": item.Request.ReviewDigest}
		if item.Status == string(contracts.AuthorityRequestPending) {
			entry["resolve_with"] = decideCommand(item.RequestDigest, "approve")
			entry["reject_with"] = decideCommand(item.RequestDigest, "reject")
		}
		if item.Decision != nil {
			entry["decision_ref"], entry["outcome"], entry["decided_by"] = item.Decision.DecisionRef, item.Decision.Outcome, item.Decision.DecidedBy
			if item.Decision.Outcome == contracts.AuthorityApprove {
				entry["accept_with"] = acceptCommand(item.RequestDigest)
			}
		}
		requestEntries = append(requestEntries, entry)
	}
	acceptances, err := repo.ListAcceptedWorkPlans(ctx, baseline.Digest, now)
	if err != nil {
		return err
	}
	acceptanceEntries := make([]map[string]any, 0, len(acceptances))
	for key, plan := range acceptances {
		acceptanceEntries = append(acceptanceEntries, map[string]any{"acceptance": key, "acceptance_ref": plan.AcceptanceRef, "proposal_digest": plan.ProposalDigest, "accepted_by": plan.AcceptedBy, "candidates": len(plan.Candidates), "attach_with": attachCommand(baseline.ID, baseline.Version, plan.AcceptanceRef)})
	}
	for _, entry := range proposalEntries {
		digest := entry["proposal_digest"].(string)
		entry["review_accept_with"] = reviewCommand(digest, string(contracts.ReviewAcceptableForAuthority))
		entry["review_revise_with"] = reviewCommand(digest, string(contracts.ReviewRevisionRequired))
	}
	for _, entry := range reviewEntries {
		if entry["status"] == contracts.ReviewAcceptableForAuthority {
			entry["request_with"] = requestCommand(baseline.ID, baseline.Version, entry["proposal_digest"].(string), entry["review_digest"].(string))
		}
	}
	result := map[string]any{"goal": baseline, "drivable": baseline.WorkPlan != nil, "pending_authority": pending, "proposals": proposalEntries, "reviews": reviewEntries, "authority_requests": requestEntries, "acceptances": acceptanceEntries}
	if baseline.WorkPlan != nil {
		result["drive_template"] = driveTemplate(baseline)
		turns, err := goaldrive.Ledger{Store: state.NewSQLiteEventStore(db), Actor: contracts.PrincipalRef{ID: "praxis-goal-drive", Kind: "controller"}}.Load(ctx, goalID, version)
		if err != nil {
			return fmt.Errorf("load Goal-drive ledger: %w", err)
		}
		result["turns"] = len(turns)
		result["blocked_turns"] = recoverableTurns(baseline, turns)
		ledger := goaldrive.Ledger{Store: state.NewSQLiteEventStore(db), Actor: contracts.PrincipalRef{ID: "praxis-goal-drive", Kind: "controller"}}
		completions, err := ledger.LoadCompletions(ctx, goalID, version)
		if err != nil {
			return fmt.Errorf("load unit completions: %w", err)
		}
		workSet, err := workSetState(baseline, completions)
		if err != nil {
			return err
		}
		result["work_set"] = workSet
	} else if len(proposalEntries) == 0 {
		result["next_step"] = "propose: a planner supplies the WorkPlan decomposition with praxis goals-lifecycle --operation=propose --input=<planner-proposal.json>"
	}
	return printJSON(result)
}

// installationOwnerAndRoot resolves the installation owner principal and the
// current root generation from durable state.
func installationOwnerAndRoot(ctx context.Context, repo goalstore.Repository, getenv func(string) string, now time.Time) (contracts.PrincipalRef, contracts.AuthorityGeneration, error) {
	bootstrapDigest, err := installationBootstrapDigest(getenv)
	if err != nil {
		return contracts.PrincipalRef{}, contracts.AuthorityGeneration{}, err
	}
	owner, err := contracts.InstallationOwnerPrincipal(bootstrapDigest)
	if err != nil {
		return contracts.PrincipalRef{}, contracts.AuthorityGeneration{}, err
	}
	root, err := currentInstallationRoot(ctx, repo, owner, now)
	if err != nil {
		return contracts.PrincipalRef{}, contracts.AuthorityGeneration{}, err
	}
	return owner, root, nil
}

// The emitted next-action commands are product contracts: each is a complete
// public CLI invocation carrying full durable identities, never a placeholder.
func reviewCommand(proposalDigest, status string) string {
	return "praxis goals-lifecycle --operation=review --proposal-digest=" + proposalDigest + " --status=" + status
}

func requestCommand(goalID, goalVersion, proposalDigest, reviewDigest string) string {
	return "praxis goals-lifecycle --operation=request --goal-id=" + goalID + " --goal-version=" + goalVersion + " --proposal-digest=" + proposalDigest + " --review-digest=" + reviewDigest
}

func decideCommand(requestDigest, outcome string) string {
	return "praxis authority decide --request " + requestDigest + " --outcome " + outcome
}

func acceptCommand(requestDigest string) string {
	return "praxis goals-lifecycle --operation=accept --request-digest=" + requestDigest
}

func attachCommand(goalID, goalVersion, acceptanceRef string) string {
	return "praxis goals-lifecycle --operation=attach --goal-id=" + goalID + " --goal-version=" + goalVersion + " --acceptance-ref=" + acceptanceRef
}

// workSetState renders the durable completion state of the generation's
// WorkPlan: which units the controller has recorded complete (with the
// checkpoint evidence), which unit goal-drive would select next, and the
// Goal-level completion assessment (#158). The immutable plan's
// proposal-time `completed` flag is never mutated.
func workSetState(baseline goals.GoalBaseline, completions []goaldrive.UnitCompletion) (map[string]any, error) {
	candidates, relationships, err := goaldrive.MaterializeGoalWork(baseline)
	if err != nil {
		return nil, err
	}
	candidates = goaldrive.ApplyCompletions(candidates, completions)
	byUnit := map[string]goaldrive.UnitCompletion{}
	for _, completion := range completions {
		byUnit[completion.UnitID] = completion
	}
	assessment, err := contracts.AssessWorkCandidates(candidates, relationships)
	if err != nil {
		return nil, err
	}
	units := make([]map[string]any, 0, len(assessment.Candidates))
	for _, item := range assessment.Candidates {
		entry := map[string]any{"unit": item.Candidate.ID, "completed": item.Candidate.Completed, "readiness": item.Readiness}
		if len(item.BlockedBy) > 0 {
			entry["blocked_by"] = item.BlockedBy
		}
		if completion, ok := byUnit[item.Candidate.ID]; ok {
			entry["completed_by_turn"] = completion.TurnID
			entry["completion_checkpoint"] = completion.EndHead
			entry["completed_at"] = completion.CompletedAt
		}
		units = append(units, entry)
	}
	goalAssessment, err := goaldrive.AssessGoalCompletion(baseline, completions)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"state": assessment.State, "units": units, "goal_completion": goalAssessment, "completion_proposal": "a worker proposes completion of the selected unit with the commit trailer `" + goaldrive.CompletionTrailer + ": <unit id>`; Praxis records completion only after the checkpoint is published and validated"}
	if assessment.Selected != nil {
		out["next_unit"] = assessment.Selected.ID
	}
	return out, nil
}

// recoverableTurns lists the generation's BLOCKED turns without checkpoint
// whose objective has not progressed since. A turn whose end HEAD is known
// carries the exact public recovery template; one whose end HEAD was never
// observed is listed as not recoverable, with the reason. The checkout is
// not durable state, so whether the consequence still exists is established
// by goal-drive itself when --recover-turn binds it.
func recoverableTurns(baseline goals.GoalBaseline, turns []goaldrive.TurnRecord) []map[string]any {
	out := make([]map[string]any, 0)
	for i, turn := range turns {
		if turn.Outcome != goaldrive.OutcomeBlocked || turn.Progress {
			continue
		}
		superseded := false
		for _, later := range turns[i+1:] {
			if later.ChildObjective == turn.ChildObjective && later.Progress {
				superseded = true
			}
		}
		if superseded {
			continue
		}
		entry := map[string]any{"turn_id": turn.TurnID, "invocation_id": turn.InvocationID, "child_objective": turn.ChildObjective, "end_head": turn.EndHead, "blocker": turn.Blocker, "recoverable": turn.EndHead != "", "consequence_fingerprint": turn.ConsequenceFingerprint, "consequence_files": turn.ConsequenceFiles, "consequence_commits": turn.ConsequenceCommits}
		if turn.EndHead != "" {
			entry["recover_template"] = recoverTemplate(baseline.ID, baseline.Version, turn.TurnID)
		} else {
			entry["reason"] = "the turn's end HEAD was never observed, so no consequence can be bound to it"
		}
		out = append(out, entry)
	}
	return out
}

// recoverTemplate is the drive template pinned to one BLOCKED turn: the same
// operator intent (provider, new invocation identity, repository, branch)
// plus the exact turn whose uncommitted consequence the invocation binds.
func recoverTemplate(goalID, goalVersion, turnID string) map[string]any {
	return map[string]any{"command": "praxis goal-drive --goal-id=" + goalID + " --goal-version=" + goalVersion + " --mode=supervised --recover-turn=" + turnID, "operator_supplies": []string{"--provider=<registered provider>", "--invocation-id=<new durable invocation identity>", "--repo=<repository path holding the blocked turn's checkout>", "--branch=<exact branch>"}, "providers_with": "praxis providers"}
}

// driveTemplate names the drivable generation exactly; the provider,
// invocation identity, repository, and branch are operator intent and are
// deliberately not invented here.
func driveTemplate(baseline goals.GoalBaseline) map[string]any {
	return map[string]any{"goal_id": baseline.ID, "goal_version": baseline.Version, "command": "praxis goal-drive --goal-id=" + baseline.ID + " --goal-version=" + baseline.Version + " --mode=supervised", "operator_supplies": []string{"--provider=<registered provider>", "--invocation-id=<durable invocation identity>", "--repo=<repository path>", "--branch=<exact branch>"}, "providers_with": "praxis providers", "available_providers": availableProviderIDs(os.Getenv)}
}

// lifecycleSelector is the documented public input for the derived lifecycle
// operations. It names exact durable identities (digests and refs printed by
// propose, review, inspect, and `praxis authority pending`) and, for review,
// the reviewer's judgment. It never carries Praxis's internal governance
// representation; the adapter derives every binding from durable state.
type lifecycleSelector struct {
	ProposalDigest     string                 `json:"proposal_digest"`
	ReviewDigest       string                 `json:"review_digest"`
	RequestDigest      string                 `json:"request_digest"`
	AcceptanceRef      string                 `json:"acceptance_ref"`
	Status             string                 `json:"status"`
	ReviewedBy         contracts.PrincipalRef `json:"reviewed_by"`
	ReviewerGeneration string                 `json:"reviewer_generation"`
	Findings           []string               `json:"findings"`
	Reason             string                 `json:"reason"`
}

// selectorFromOptions builds the lifecycle selector from the invocation's
// declared options. This is the ordinary product path: the operator passes
// exact durable identities as options and never authors a document.
func selectorFromOptions(options map[string]string) (lifecycleSelector, bool) {
	selector := lifecycleSelector{ProposalDigest: options["proposal-digest"], ReviewDigest: options["review-digest"], RequestDigest: options["request-digest"], AcceptanceRef: options["acceptance-ref"], Status: options["status"], ReviewerGeneration: options["reviewer-generation"], Reason: options["reason"]}
	if options["reviewer-id"] != "" {
		selector.ReviewedBy = contracts.PrincipalRef{ID: options["reviewer-id"], Kind: options["reviewer-kind"]}
		if selector.ReviewedBy.Kind == "" {
			selector.ReviewedBy.Kind = "human"
		}
	}
	if options["finding"] != "" {
		selector.Findings = []string{options["finding"]}
	}
	present := selector.ProposalDigest != "" || selector.ReviewDigest != "" || selector.RequestDigest != "" || selector.AcceptanceRef != "" || selector.Status != ""
	return selector, present
}

// lifecycleSelectorFor resolves the operation's selector: declared options
// first, then a selector document passed through --input.
func lifecycleSelectorFor(options map[string]string, input []byte) (lifecycleSelector, bool) {
	if selector, ok := selectorFromOptions(options); ok {
		return selector, true
	}
	if len(input) == 0 {
		return lifecycleSelector{}, false
	}
	return decodeSelector(input)
}

func decodeSelector(input []byte) (lifecycleSelector, bool) {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(input, &probe); err != nil {
		return lifecycleSelector{}, false
	}
	for _, full := range []string{"proposal", "review", "decision", "plan", "Plan", "Proposal", "Review", "Decision", "id", "ID", "GoalID"} {
		if _, ok := probe[full]; ok {
			return lifecycleSelector{}, false
		}
	}
	var selector lifecycleSelector
	if err := json.Unmarshal(input, &selector); err != nil {
		return lifecycleSelector{}, false
	}
	return selector, true
}

func reviewFromSelector(ctx context.Context, repo goalstore.Repository, selector lifecycleSelector, now time.Time) (contracts.WorkPlanProposalReview, string, string, error) {
	proposal, proposalVersion, err := repo.LoadWorkPlanProposalByDigest(ctx, selector.ProposalDigest, now)
	if err != nil {
		return contracts.WorkPlanProposalReview{}, "", "", err
	}
	if selector.Status == "" || selector.ReviewedBy.ID == "" || selector.ReviewerGeneration == "" {
		return contracts.WorkPlanProposalReview{}, "", "", errors.New("review selector requires proposal_digest, status, reviewed_by, and reviewer_generation")
	}
	covered := make([]string, 0)
	seen := map[string]struct{}{}
	for _, candidate := range proposal.Candidates {
		for _, requirement := range candidate.Requirements {
			if _, ok := seen[requirement.ID]; !ok {
				seen[requirement.ID] = struct{}{}
				covered = append(covered, requirement.ID)
			}
		}
	}
	reviewRef := "review:" + proposal.ID + ":" + selector.ReviewedBy.ID + ":" + strconv.FormatInt(now.UnixNano(), 10)
	review := contracts.WorkPlanProposalReview{ProposalDigest: selector.ProposalDigest, BaselineDigest: proposal.BaselineDigest, ReviewRef: reviewRef, ReviewDigest: bytesDigestString([]byte(reviewRef + "\n" + selector.ProposalDigest + "\n" + selector.Status)), ReviewedBy: selector.ReviewedBy, ReviewerGeneration: selector.ReviewerGeneration, Status: contracts.WorkPlanReviewStatus(selector.Status), Findings: selector.Findings}
	if review.Status == contracts.ReviewAcceptableForAuthority {
		review.CoveredRequirements = covered
	} else {
		review.MissingRequirements = covered
	}
	return review, proposal.ID, proposalVersion, nil
}

func bytesDigestString(body []byte) string {
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// installationBootstrapDigest resolves the exact bootstrap identity that names
// the installation owner, from the same record every governed command uses.
func installationBootstrapDigest(getenv func(string) string) (string, error) {
	_, db, record, err := openGovernedRepositoryReadOnly(context.Background(), getenv)
	if err != nil {
		return "", err
	}
	db.Close()
	return record.Digest()
}

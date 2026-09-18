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
		return printJSON(map[string]any{"operation": operation, "proposal": proposal, "record_digest": digest})
	case "review":
		if selector, ok := decodeSelector(input); ok {
			review, proposalID, proposalVersion, err := reviewFromSelector(ctx, repo, selector, now)
			if err != nil {
				return err
			}
			if err := repo.SaveWorkPlanReview(ctx, proposalID, proposalVersion, review, "1", now, nil); err != nil {
				return err
			}
			return printJSON(map[string]any{"operation": operation, "review": review, "review_ref": review.ReviewRef, "review_digest": review.ReviewDigest, "proposal_digest": review.ProposalDigest})
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
		if selector, ok := decodeSelector(input); ok {
			goalID, goalVersion := in.Options["goal-id"], in.Options["goal-version"]
			if goalID == "" || goalVersion == "" || selector.ProposalDigest == "" || selector.ReviewDigest == "" {
				return errors.New("request selector requires --goal-id, --goal-version, proposal_digest, and review_digest")
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
			bootstrapDigest, err := installationBootstrapDigest(getenv)
			if err != nil {
				return err
			}
			owner, err := contracts.InstallationOwnerPrincipal(bootstrapDigest)
			if err != nil {
				return err
			}
			root, err := currentInstallationRoot(ctx, repo, owner, now)
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
			return printJSON(map[string]any{"operation": operation, "request": req, "request_digest": digest, "resolve_with": "praxis authority decide --request " + digest + " --outcome approve|reject"})
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
		if selector, ok := decodeSelector(input); ok {
			if selector.RequestDigest == "" {
				return errors.New("accept selector requires request_digest")
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
			return printJSON(map[string]any{"operation": operation, "plan": accepted, "acceptance_ref": acceptanceRef, "acceptance_version": "1", "attach_with": "praxis goals-lifecycle --operation=attach --goal-id " + request.BaselineID + " --goal-version " + request.BaselineVersion + " --input <{\"acceptance_ref\": \"" + acceptanceRef + "\"}>"})
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
		if selector, ok := decodeSelector(input); ok {
			goalID, goalVersion := in.Options["goal-id"], in.Options["goal-version"]
			if goalID == "" || goalVersion == "" || selector.AcceptanceRef == "" {
				return errors.New("attach selector requires --goal-id, --goal-version, and acceptance_ref")
			}
			source, err := repo.Load(ctx, goalID, goalVersion, now)
			if err != nil {
				return err
			}
			next, err := strconv.Atoi(goalVersion)
			if err != nil {
				return fmt.Errorf("Goal generation %q is not numeric; supply the successor version explicitly", goalVersion)
			}
			successor, err := repo.AttachAcceptedWorkPlan(ctx, source.ID, source.Version, source.Digest, selector.AcceptanceRef, "1", strconv.Itoa(next+1), now, nil)
			if err != nil {
				return err
			}
			return printJSON(map[string]any{"operation": operation, "goal_id": successor.ID, "goal_version": successor.Version, "baseline_digest": successor.Digest, "predecessor_digest": successor.PredecessorDigest, "work_plan_candidates": len(successor.WorkPlan.Candidates), "drive_with": "praxis goal-drive --goal-id=" + successor.ID + " --goal-version=" + successor.Version + " --provider=<provider> --invocation-id=<id> --repo=<path> --branch=<branch>"})
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
			entry["resolve_with"] = "praxis authority decide --request " + item.RequestDigest + " --outcome approve|reject"
		}
		if item.Decision != nil {
			entry["decision_ref"], entry["outcome"], entry["decided_by"] = item.Decision.DecisionRef, item.Decision.Outcome, item.Decision.DecidedBy
			if item.Decision.Outcome == contracts.AuthorityApprove {
				entry["accept_with"] = "praxis goals-lifecycle --operation=accept --input <{\"request_digest\": ...}>"
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
		acceptanceEntries = append(acceptanceEntries, map[string]any{"acceptance": key, "acceptance_ref": plan.AcceptanceRef, "proposal_digest": plan.ProposalDigest, "accepted_by": plan.AcceptedBy, "candidates": len(plan.Candidates)})
	}
	return printJSON(map[string]any{"goal": baseline, "drivable": baseline.WorkPlan != nil, "pending_authority": pending, "proposals": proposalEntries, "reviews": reviewEntries, "authority_requests": requestEntries, "acceptances": acceptanceEntries})
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

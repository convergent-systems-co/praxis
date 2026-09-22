package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/convergent-systems-co/praxis/internal/goalstore"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/packages/goals"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// continueGoalsLifecycle performs only transitions that are completely
// determined by durable state. Planning, independent review, and owner
// disposition remain explicit boundaries; request construction, approved
// proposal promotion, acceptance, and successor attachment do not require an
// operator to relay identities between commands.
func continueGoalsLifecycle(ctx context.Context, options map[string]string, getenv func(string) string) error {
	goalID, version := options["goal-id"], options["goal-version"]
	if goalID == "" || version == "" {
		return errors.New("Goals continuation requires exact --goal-id and --goal-version")
	}
	repo, db, err := openGovernedRepository(ctx, getenv)
	if err != nil {
		return err
	}
	defer db.Close()
	result, err := advanceGoalsLifecycle(ctx, repo, goalID, version, getenv, time.Now().UTC())
	if err != nil {
		return err
	}
	return printJSON(result)
}

type reviewedProposal struct {
	proposal        contracts.WorkPlanProposal
	proposalVersion string
	proposalDigest  string
	review          contracts.WorkPlanProposalReview
	reviewVersion   string
}

func advanceGoalsLifecycle(ctx context.Context, repo goalstore.Repository, goalID, version string, getenv func(string) string, now time.Time) (map[string]any, error) {
	baseline, err := repo.Load(ctx, goalID, version, now)
	if err != nil {
		return nil, err
	}
	result := map[string]any{
		"operation": "continue", "goal_id": baseline.ID, "goal_version": baseline.Version,
		"goal_digest": baseline.Digest, "transitions": []string{},
	}
	if baseline.WorkPlan != nil {
		result["status"] = "drivable"
		result["next_admissible_transition"] = "goal-drive"
		result["drive_template"] = driveTemplate(baseline)
		return result, nil
	}

	acceptances, err := repo.ListAcceptedWorkPlans(ctx, baseline.Digest, now)
	if err != nil {
		return nil, err
	}
	if len(acceptances) > 1 {
		return nil, fmt.Errorf("deterministic continuation refused: Goal %s/%s has %d accepted WorkPlans", goalID, version, len(acceptances))
	}
	if len(acceptances) == 1 {
		for _, plan := range acceptances {
			return attachContinuedPlan(ctx, repo, baseline, plan, result, getenv, now, []string{"attach_accepted_workplan"})
		}
	}

	requests, err := repo.ListAuthorityRequests(ctx, goalID, version, now)
	if err != nil {
		return nil, err
	}
	var approved []goalstore.AuthorityRequestDisposition
	var pending []goalstore.AuthorityRequestDisposition
	requestedReview := map[string]bool{}
	var rejected []string
	for _, item := range requests {
		if item.Request.RequestedAuthority != contracts.GovernedWorkPlanAccept {
			continue
		}
		requestedReview[item.Request.ProposalDigest+"\x00"+item.Request.ReviewDigest] = true
		switch item.Status {
		case "decided:" + string(contracts.AuthorityApprove):
			approved = append(approved, item)
		case string(contracts.AuthorityRequestPending):
			pending = append(pending, item)
		case "decided:" + string(contracts.AuthorityReject):
			rejected = append(rejected, item.RequestDigest)
		}
	}
	if len(approved) > 1 {
		return nil, fmt.Errorf("deterministic continuation refused: Goal %s/%s has %d approved WorkPlan requests", goalID, version, len(approved))
	}
	if len(approved) == 1 {
		item := approved[0]
		proposal, err := repo.LoadWorkPlanProposal(ctx, item.Request.ProposalID, item.Request.ProposalVersion, now)
		if err != nil {
			return nil, err
		}
		// Continuation is a second route to the same durable mutation as
		// `accept`, so it enforces the identical activation predicate before
		// anything is persisted. It must not depend on a later goal-drive refusal.
		if proposal.Safety != nil {
			if item.Request.CeremonyProfile != "interactive-os-owner-v1" || item.Request.ActivationManifestDigest != proposal.Safety.ActivationManifestDigest {
				return nil, errors.New("protected WorkPlan acceptance lacks exact ceremony/activation binding")
			}
			if err := verifyLifecycleSafety(*proposal.Safety, getenv); err != nil {
				return nil, err
			}
		}
		acceptanceRef := "acceptance:" + item.Request.ID
		plan, err := contracts.MaterializeAcceptedPlanCandidate(proposal, acceptanceRef, item.RequestDigest)
		if err != nil {
			return nil, err
		}
		accepted, err := repo.SaveAcceptedWorkPlanFromAuthorityDecision(ctx, item.Request.ID, item.Request.Version, plan, acceptanceRef, "1", now, nil)
		if err != nil {
			return nil, err
		}
		return attachContinuedPlan(ctx, repo, baseline, accepted, result, getenv, now, []string{"accept_approved_workplan", "attach_accepted_workplan"})
	}
	if len(pending) > 0 {
		entries := make([]map[string]any, 0, len(pending))
		for _, item := range pending {
			entries = append(entries, map[string]any{"request_digest": item.RequestDigest, "resolve_with": decideCommand(item.RequestDigest, "approve"), "reject_with": decideCommand(item.RequestDigest, "reject")})
		}
		result["status"] = "owner_authority_required"
		result["next_admissible_transition"] = "owner decides an exact pending WorkPlan request"
		result["authority_requests"] = entries
		return result, nil
	}

	proposals, proposalDigests, err := repo.ListWorkPlanProposals(ctx, goalID, version, now)
	if err != nil {
		return nil, err
	}
	if len(proposals) == 0 {
		result["status"] = "planning_required"
		result["next_admissible_transition"] = "an authorized planner proposes a requirement-bound WorkPlan"
		return result, nil
	}
	var reviewed []reviewedProposal
	var unreviewed bool
	var revisionRequired bool
	for i, proposal := range proposals {
		_, proposalVersion, err := repo.LoadWorkPlanProposalByDigest(ctx, proposalDigests[i], now)
		if err != nil {
			return nil, err
		}
		reviews, reviewVersions, err := repo.ListWorkPlanReviews(ctx, proposalDigests[i], now)
		if err != nil {
			return nil, err
		}
		if len(reviews) == 0 {
			unreviewed = true
		}
		for j, review := range reviews {
			key := proposalDigests[i] + "\x00" + review.ReviewDigest
			if review.Status == contracts.ReviewAcceptableForAuthority && !requestedReview[key] {
				reviewed = append(reviewed, reviewedProposal{proposal: proposal, proposalVersion: proposalVersion, proposalDigest: proposalDigests[i], review: review, reviewVersion: reviewVersions[j]})
			} else if review.Status != contracts.ReviewAcceptableForAuthority {
				revisionRequired = true
			}
		}
	}
	if len(reviewed) > 1 {
		return nil, fmt.Errorf("deterministic continuation refused: Goal %s/%s has %d independently acceptable unrequested proposal/review pairs", goalID, version, len(reviewed))
	}
	if len(reviewed) == 0 {
		if unreviewed {
			result["status"] = "independent_review_required"
			result["next_admissible_transition"] = "an independent reviewer evaluates an exact proposal"
		} else if len(rejected) > 0 || revisionRequired {
			result["status"] = "planning_revision_required"
			if len(rejected) > 0 {
				result["rejected_request_digests"] = rejected
				result["next_admissible_transition"] = "an authorized planner revises the decomposition; rejected authority is never retried implicitly"
			} else {
				result["next_admissible_transition"] = "an authorized planner revises the decomposition after a non-acceptable independent review"
			}
		} else {
			result["status"] = "independent_review_required"
			result["next_admissible_transition"] = "an independent reviewer evaluates an exact proposal"
		}
		return result, nil
	}

	item := reviewed[0]
	_, root, err := installationOwnerAndRoot(ctx, repo, getenv, now)
	if err != nil {
		return nil, err
	}
	requestID := "workplan-accept:continued:" + strings.TrimPrefix(bytesDigestString([]byte(baseline.Digest+"\n"+item.proposalDigest+"\n"+item.review.ReviewDigest)), "sha256:")
	request := contracts.AuthorityRequest{
		ID: requestID, Version: "1", BaselineID: baseline.ID, BaselineVersion: baseline.Version, BaselineDigest: baseline.Digest,
		ProposalID: item.proposal.ID, ProposalVersion: item.proposalVersion, ProposalDigest: item.proposalDigest,
		ReviewRef: item.review.ReviewRef, ReviewVersion: item.reviewVersion, ReviewDigest: item.review.ReviewDigest,
		RequestedAuthority: contracts.GovernedWorkPlanAccept, RequestedScope: root.Scope,
		Reason: "accept the independently reviewed WorkPlan decomposition of " + baseline.ID + "/" + baseline.Version,
		Status: contracts.AuthorityRequestPending,
	}
	if item.proposal.Safety != nil {
		// Same protected request the direct `request` operation builds: the
		// interactive ceremony and exact activation binding are part of the
		// request identity, never added later.
		if err := verifyLifecycleSafety(*item.proposal.Safety, getenv); err != nil {
			return nil, err
		}
		request.CeremonyProfile = "interactive-os-owner-v1"
		request.ActivationManifestDigest = item.proposal.Safety.ActivationManifestDigest
	}
	requestDigest, err := repo.SaveAuthorityRequest(ctx, request, now, nil)
	if err != nil {
		// A concurrent or restarted continuation may have completed this exact
		// deterministic write. Verify exact durable state before treating it as
		// failure; never repeat an ambiguous mutation.
		existing, loadErr := repo.LoadAuthorityRequest(ctx, request.ID, request.Version, now)
		if loadErr != nil {
			return nil, err
		}
		existingDigest, digestErr := existing.Digest()
		wantedDigest, wantedErr := request.Digest()
		if digestErr != nil || wantedErr != nil || existingDigest != wantedDigest {
			return nil, err
		}
		requestDigest = existingDigest
	}
	result["transitions"] = []string{"create_workplan_authority_request"}
	result["status"] = "owner_authority_required"
	result["next_admissible_transition"] = "owner decides the exact pending WorkPlan request"
	result["authority_requests"] = []map[string]any{{"request_digest": requestDigest, "resolve_with": decideCommand(requestDigest, "approve"), "reject_with": decideCommand(requestDigest, "reject")}}
	return result, nil
}

func attachContinuedPlan(ctx context.Context, repo goalstore.Repository, source goals.GoalBaseline, plan contracts.WorkPlan, result map[string]any, getenv func(string) string, now time.Time, transitions []string) (map[string]any, error) {
	// Identical to direct `attach`: the activation predicate precedes both the
	// replay observation and the successor write.
	if plan.Safety != nil {
		if err := verifyLifecycleSafety(*plan.Safety, getenv); err != nil {
			return nil, err
		}
	}
	n, err := strconv.Atoi(source.Version)
	if err != nil {
		return nil, fmt.Errorf("Goal generation %q is not numeric; deterministic continuation cannot derive its successor", source.Version)
	}
	successorVersion := strconv.Itoa(n + 1)
	if existing, loadErr := repo.Load(ctx, source.ID, successorVersion, now); loadErr == nil {
		if existing.PredecessorDigest != source.Digest || existing.WorkPlan == nil || existing.WorkPlan.AcceptanceRef != plan.AcceptanceRef {
			return nil, fmt.Errorf("Goal generation %s/%s already exists with a different lineage", source.ID, successorVersion)
		}
		result["replay"] = true
		result["status"] = "drivable"
		result["goal_version"] = existing.Version
		result["goal_digest"] = existing.Digest
		result["transitions"] = transitions
		result["next_admissible_transition"] = "goal-drive"
		result["drive_template"] = driveTemplate(existing)
		return result, nil
	} else if !errors.Is(loadErr, state.ErrSecureBlobNotFound) {
		return nil, loadErr
	}
	successor, err := repo.AttachAcceptedWorkPlan(ctx, source.ID, source.Version, source.Digest, plan.AcceptanceRef, "1", successorVersion, now, nil)
	if err != nil {
		// The immutable successor may have been committed by a concurrent
		// continuation even when this writer observed a conflict or an
		// ambiguous persistence error. Accept only the exact derived lineage;
		// otherwise preserve the original failure.
		existing, loadErr := repo.Load(ctx, source.ID, successorVersion, now)
		if loadErr != nil || existing.PredecessorDigest != source.Digest || existing.WorkPlan == nil || existing.WorkPlan.AcceptanceRef != plan.AcceptanceRef {
			return nil, err
		}
		successor = existing
		result["replay"] = true
	}
	result["status"] = "drivable"
	result["goal_version"] = successor.Version
	result["goal_digest"] = successor.Digest
	result["predecessor_digest"] = successor.PredecessorDigest
	result["transitions"] = transitions
	result["next_admissible_transition"] = "goal-drive"
	result["drive_template"] = driveTemplate(successor)
	return result, nil
}

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

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// runAuthorityPending lists durable authority requests with their truthful
// disposition. It is read-only and never orders or selects by recency: every
// item carries the exact request digest a decision must name.
func runAuthorityPending(args []string, getenv func(string) string, out io.Writer) error {
	f := flag.NewFlagSet("authority pending", flag.ContinueOnError)
	f.SetOutput(out)
	goalID := f.String("goal-id", "", "restrict to requests bound to this exact Goal identity")
	goalVersion := f.String("goal-version", "", "restrict to requests bound to this exact Goal generation")
	all := f.Bool("all", false, "include decided requests")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 || (*goalID == "") != (*goalVersion == "") {
		return errors.New("usage: praxis authority pending [--goal-id <id> --goal-version <version>] [--all]")
	}
	repo, db, _, err := openGovernedRepositoryReadOnly(context.Background(), getenv)
	if err != nil {
		return err
	}
	defer db.Close()
	items, err := repo.ListAuthorityRequests(context.Background(), *goalID, *goalVersion, time.Now().UTC())
	if err != nil {
		return err
	}
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if !*all && item.Status != string(contracts.AuthorityRequestPending) {
			continue
		}
		entry := map[string]any{"request_digest": item.RequestDigest, "id": item.Request.ID, "version": item.Request.Version, "status": item.Status, "requested_authority": item.Request.RequestedAuthority, "requested_scope": item.Request.RequestedScope, "reason": item.Request.Reason}
		if item.Request.BaselineID != "" {
			entry["goal_id"], entry["goal_version"], entry["baseline_digest"] = item.Request.BaselineID, item.Request.BaselineVersion, item.Request.BaselineDigest
		}
		if item.Request.ProposalID != "" {
			entry["proposal_id"], entry["proposal_version"], entry["proposal_digest"] = item.Request.ProposalID, item.Request.ProposalVersion, item.Request.ProposalDigest
		}
		if item.Request.ReviewRef != "" {
			entry["review_ref"], entry["review_version"], entry["review_digest"] = item.Request.ReviewRef, item.Request.ReviewVersion, item.Request.ReviewDigest
		}
		if item.Request.Delegation != nil {
			entry["delegation_profile"] = item.Request.Delegation.Profile
			entry["resolve_with"] = "praxis authority delegate --request " + item.RequestDigest
		} else if item.Status == string(contracts.AuthorityRequestPending) {
			entry["resolve_with"] = "praxis authority decide --request " + item.RequestDigest + " --outcome approve|reject"
		}
		if item.Decision != nil {
			entry["decision_ref"], entry["decision_digest"], entry["outcome"], entry["decided_by"] = item.Decision.DecisionRef, item.DecisionDigest, item.Decision.Outcome, item.Decision.DecidedBy
		}
		result = append(result, entry)
	}
	return printJSONTo(out, map[string]any{"operation": "authority.pending", "requests": result})
}

func runAuthorityDecide(args []string, getenv func(string) string, input io.Reader, output io.Writer) error {
	return runAuthorityDecideWithTerminal(args, getenv, input, output, isInteractiveTerminal())
}

// runAuthorityDecideWithTerminal records the installation owner's decision on
// one exact pending authority request. The human supplies only the exact
// request digest and the outcome; every binding (request digest, current
// root, granted scope, model identity, decider) is derived from durable
// state. The authenticated OS user must own the root, the confirmation must
// name the exact digest and outcome, and a replay returns the recorded
// decision instead of writing a second one.
func runAuthorityDecideWithTerminal(args []string, getenv func(string) string, input io.Reader, output io.Writer, interactive bool) error {
	f := flag.NewFlagSet("authority decide", flag.ContinueOnError)
	f.SetOutput(output)
	requestDigest := f.String("request", "", "exact durable AuthorityRequest digest")
	outcome := f.String("outcome", "", "approve or reject")
	reason := f.String("reason", "", "optional human reason recorded with the decision")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 || *requestDigest == "" || (*outcome != string(contracts.AuthorityApprove) && *outcome != string(contracts.AuthorityReject)) {
		return errors.New("usage: praxis authority decide --request <digest> --outcome approve|reject [--reason <text>] (interactive confirmation required)")
	}
	if !interactive {
		return errAuthorityBootstrapConfirmation
	}
	if getenv == nil {
		getenv = os.Getenv
	}
	ctx := context.Background()
	now := time.Now().UTC()
	repo, db, record, err := openGovernedRepositoryReadOnly(ctx, getenv)
	if err != nil {
		return err
	}
	defer db.Close()
	request, err := repo.LoadAuthorityRequestByDigest(ctx, *requestDigest, now)
	if err != nil {
		return fmt.Errorf("pending authority request %s: %w", *requestDigest, err)
	}
	if request.Delegation != nil {
		return errors.New("delegation requests are resolved with `praxis authority delegate`, not decide")
	}
	if existing, err := repo.LoadAuthorityDecision(ctx, request.ID, request.Version, now); err == nil {
		digest, _ := existing.Digest()
		return printJSONTo(output, map[string]any{"operation": "authority.decide", "replay": true, "request_digest": *requestDigest, "decision": existing, "decision_digest": digest})
	}
	if request.Status != contracts.AuthorityRequestPending {
		return fmt.Errorf("authority request %s is %s, not pending", *requestDigest, request.Status)
	}
	bootstrapDigest, err := record.Digest()
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
	current, err := user.Current()
	if err != nil || current.Username == "" || !strings.HasSuffix(root.ProvenanceRef, ":os-user:"+current.Username) {
		return errors.New("authenticated root OS user does not match the enrolled installation root")
	}
	if request.RequestedScope != root.Scope {
		return fmt.Errorf("authority request scope %q is not the installation root scope %q; the root cannot decide it", request.RequestedScope, root.Scope)
	}
	confirmation := "DECIDE-" + strings.ToUpper(*outcome) + " " + *requestDigest
	prompt := fmt.Sprintf("Record the installation owner's decision %q on exact authority request %s (%s, %s) using root %s/%s. Type %q to continue: ", *outcome, *requestDigest, request.RequestedAuthority, request.Reason, root.Ref, root.Version, confirmation)
	if _, err := io.WriteString(output, prompt); err != nil {
		return err
	}
	answer, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if strings.TrimSpace(answer) != confirmation {
		return errAuthorityBootstrapConfirmation
	}
	decision := contracts.AuthorityDecision{RequestID: request.ID, RequestVersion: request.Version, RequestDigest: *requestDigest, DecisionRef: "authority-decision:" + request.ID, DecisionVersion: "1", DecidedBy: owner, AuthorityRef: root.Ref, AuthorityVersion: root.Version, AuthorityGenerationDigest: root.Digest, GrantedScope: root.Scope, Outcome: contracts.AuthorityDecisionOutcome(*outcome), AuthorityDigest: root.AuthorityModelDigest, IssuedAt: now}
	writable, db2, err := openGovernedRepository(ctx, getenv)
	if err != nil {
		return err
	}
	defer db2.Close()
	if err := writable.SaveAuthorityDecision(ctx, request.ID, request.Version, decision, now, nil); err != nil {
		return err
	}
	digest, err := decision.Digest()
	if err != nil {
		return err
	}
	return printJSONTo(output, map[string]any{"operation": "authority.decide", "request_digest": *requestDigest, "outcome": *outcome, "reason": *reason, "decision": decision, "decision_digest": digest})
}

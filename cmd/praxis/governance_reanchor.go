package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/convergent-systems-co/praxis/internal/goalstore"
)

type stringList []string

func (l *stringList) String() string     { return strings.Join(*l, ",") }
func (l *stringList) Set(v string) error { *l = append(*l, v); return nil }

// runGovernanceStatus reports the relation of the governance store to the
// forward authority anchor. It is read-only and works precisely when the store
// is NOT consumable, because that is when an owner needs it.
func runGovernanceStatus(args []string, getenv func(string) string, out io.Writer) error {
	f := flag.NewFlagSet("authority governance-status", flag.ContinueOnError)
	f.SetOutput(out)
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return errors.New("usage: praxis authority governance-status")
	}
	if getenv == nil {
		getenv = os.Getenv
	}
	repo, db, _, err := openGovernedRepositoryReadOnly(context.Background(), getenv)
	if err != nil {
		return err
	}
	defer db.Close()
	status, err := repo.GovernanceStatus(context.Background())
	if err != nil {
		return err
	}
	result := map[string]any{
		"operation": "authority.governance-status", "anchored": status.Anchored, "relation": status.Relation,
		"anchor_seq": status.AnchorSeq, "anchor_head": status.AnchorHead, "store_seq": status.StoreSeq, "store_head": status.StoreHead,
		"store_chain_error": status.StoreChainError, "anchor_error": status.AnchorError, "orphaned_facts": status.OrphanFacts,
		"consumable": status.Relation == "consistent",
	}
	recoveries := make([]map[string]any, 0, len(status.Reanchors))
	for _, fact := range status.Reanchors {
		recoveries = append(recoveries, map[string]any{"seq": fact.Seq, "at": fact.At, "cause": fact.Reanchor.Cause, "store_seq": fact.Reanchor.DBSeq, "anchor_seq": fact.Reanchor.AnchorSeq, "root": fact.Reanchor.RootRef + "/" + fact.Reanchor.RootVersion, "root_digest": fact.Reanchor.RootDigest, "os_user": fact.Reanchor.OSUser, "ceremony_digest": fact.Reanchor.CeremonyDigest, "classified_goals": fact.Reanchor.ClassifiedGoals})
	}
	result["reanchors"] = recoveries
	if status.Relation != "consistent" && status.Relation != "unanchored" {
		result["next"] = "praxis authority governance-reanchor (interactive owner ceremony)"
	}
	return printJSONTo(out, result)
}

func runGovernanceReanchor(args []string, getenv func(string) string, input io.Reader, output io.Writer) error {
	return runGovernanceReanchorWithTerminal(args, getenv, input, output, isInteractiveTerminal())
}

// runGovernanceReanchorWithTerminal is the governed recovery of a governance
// store that is not current against the forward authority anchor (a restored
// backup, a lost or reset anchor, an interrupted write). The authority is the
// installation owner's existing one: the authenticated OS user who enrolled the
// root, an interactive terminal, and a typed confirmation bound to the digest of
// exactly the plan shown. It appends one durable reanchor fact, voids every
// decision and generation admitted before it, re-admits only the installation
// root shown, and may add classifications the owner names. It never grants a
// decision or recreates anything retired.
func runGovernanceReanchorWithTerminal(args []string, getenv func(string) string, input io.Reader, output io.Writer, interactive bool) error {
	f := flag.NewFlagSet("authority governance-reanchor", flag.ContinueOnError)
	f.SetOutput(output)
	var classify stringList
	f.Var(&classify, "classify-goal", "a Goal identity to treat as safety-bearing after recovery (repeatable; restrictions only)")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return errors.New("usage: praxis authority governance-reanchor [--classify-goal <goal-id>]... (interactive confirmation required)")
	}
	if !interactive {
		return errAuthorityBootstrapConfirmation
	}
	if getenv == nil {
		getenv = os.Getenv
	}
	ctx := context.Background()
	repo, db, err := openGovernedRepository(ctx, getenv)
	if err != nil {
		return err
	}
	defer db.Close()
	plan, err := repo.PlanReanchor(ctx)
	if err != nil {
		return err
	}
	current, err := authenticatedOSUser()
	if err != nil || current.Username == "" || plan.RootUser == "" || current.Username != plan.RootUser {
		return errors.New("the authenticated OS user is not the user who enrolled the installation root; only the installation owner may re-anchor governance")
	}
	goals := append([]string(nil), classify...)
	sort.Strings(goals)
	planDigest := plan.Digest()
	confirmation := "REANCHOR " + planDigest
	if len(goals) > 0 {
		confirmation += " CLASSIFY " + strings.Join(goals, ",")
	}
	prompt := fmt.Sprintf("Governance is %s relative to the forward authority anchor (store sequence %d, anchor sequence %d, %d fact(s) the store lacks, %d orphaned). Re-anchoring appends one durable recovery fact, VOIDS every decision and generation admitted before it (they must be established again through the ordinary ceremonies), re-admits ONLY installation root %s/%s (%s), and grants nothing else.", plan.Cause, plan.StoreSeq, plan.AnchorSeq, plan.LostFacts, plan.OrphanFacts, plan.RootRef, plan.RootVersion, plan.RootDigest)
	if len(goals) > 0 {
		prompt += " It also marks these Goals safety-bearing: " + strings.Join(goals, ", ") + "."
	}
	prompt += fmt.Sprintf(" Type %q to continue: ", confirmation)
	if plan.Consistent {
		prompt = "Governance is already consistent with the anchor; only a pending root re-admission can remain. Type " + fmt.Sprintf("%q", confirmation) + " to complete it: "
	}
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
	now := time.Now().UTC()
	confirmationSum := sha256.Sum256([]byte(confirmation))
	ceremony, _ := json.Marshal(map[string]any{"installation": plan.Installation, "plan": planDigest, "os_user": current.Username, "confirmation_digest": "sha256:" + hex.EncodeToString(confirmationSum[:]), "confirmed_at": now, "classified_goals": goals})
	ceremonySum := sha256.Sum256(append([]byte("praxis-reanchor-ceremony/v1\n"), ceremony...))
	fact, err := repo.Reanchor(ctx, goalstore.ReanchorRequest{PlanDigest: planDigest, OSUser: current.Username, CeremonyDigest: "sha256:" + hex.EncodeToString(ceremonySum[:]), ClassifiedGoals: goals, Now: now})
	if err != nil {
		return err
	}
	return printJSONTo(output, map[string]any{"operation": "authority.governance-reanchor", "plan_digest": planDigest, "recovery_fact_seq": fact.Seq, "recovery_fact_head": fact.Head, "cause": plan.Cause, "root": plan.RootRef + "/" + plan.RootVersion, "classified_goals": goals, "voided": "every decision and generation admitted before this recovery", "granted": "nothing"})
}

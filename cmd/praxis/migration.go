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

	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/migrations/sqlite"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func runMigrationCommand(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		return errors.New("usage: praxis migration {status|preview|execute|recover}")
	}
	switch args[0] {
	case "status":
		return runMigrationStatus(args[1:], os.Getenv, os.Stdout)
	case "preview":
		return runMigrationPreview(args[1:], os.Getenv, os.Stdout)
	case "execute":
		return runMigrationExecute(args[1:], os.Getenv, os.Stdin, os.Stdout)
	case "recover":
		return runMigrationRecover(args[1:], os.Getenv, os.Stdout)
	default:
		return errors.New("usage: praxis migration {status|preview|execute|recover}")
	}
}

func runMigrationStatus(args []string, getenv func(string) string, out io.Writer) error {
	if len(args) != 0 {
		return errors.New("usage: praxis migration status")
	}
	dbPath := getenv("PRAXIS_DB")
	db, err := state.OpenSQLiteReadOnly(context.Background(), dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	status, err := sqlite.StatusOf(context.Background(), db)
	if err != nil {
		return err
	}
	return printJSONTo(out, status)
}

func runMigrationPreview(args []string, getenv func(string) string, out io.Writer) error {
	f := flag.NewFlagSet("migration preview", flag.ContinueOnError)
	f.SetOutput(out)
	output := f.String("output", "", "optional system-produced migration preview JSON path")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return errors.New("usage: praxis migration preview [--output <path>]")
	}
	repo, db, record, err := openGovernedRepositoryReadOnly(context.Background(), getenv)
	if err != nil {
		return err
	}
	defer db.Close()
	bootstrapDigest, err := record.Digest()
	if err != nil {
		return err
	}
	owner, err := contracts.InstallationOwnerPrincipal(bootstrapDigest)
	if err != nil {
		return err
	}
	root, err := currentInstallationRoot(context.Background(), repo, owner, time.Now().UTC())
	if err != nil {
		return err
	}
	plan, err := sqlite.NewPlan(context.Background(), db, bootstrapDigest, owner.ID, root.Ref, root.Digest, time.Now().UTC())
	if err != nil {
		return err
	}
	payload, err := json.MarshalIndent(map[string]any{"preview": true, "plan": plan, "plan_digest": plan.PlanDigest, "confirmation": "MIGRATE " + plan.PlanDigest}, "", "  ")
	if err != nil {
		return err
	}
	if *output != "" {
		if err := os.WriteFile(*output, append(payload, '\n'), 0600); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintln(out, string(payload))
	return err
}

func runMigrationExecute(args []string, getenv func(string) string, input io.Reader, out io.Writer) error {
	f := flag.NewFlagSet("migration execute", flag.ContinueOnError)
	f.SetOutput(out)
	previewPath := f.String("preview-file", "", "system-produced migration preview JSON")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 || *previewPath == "" || !isInteractiveTerminal() {
		return errAuthorityBootstrapConfirmation
	}
	payload, err := os.ReadFile(*previewPath)
	if err != nil {
		return err
	}
	var envelope struct {
		Plan         sqlite.Plan `json:"plan"`
		PlanDigest   string      `json:"plan_digest"`
		Confirmation string      `json:"confirmation"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return err
	}
	if err := envelope.Plan.Verify(); err != nil || envelope.PlanDigest != envelope.Plan.PlanDigest || envelope.Confirmation != "MIGRATE "+envelope.Plan.PlanDigest {
		return errors.New("migration preview digest or confirmation mismatch")
	}
	repo, readDB, record, err := openGovernedRepositoryReadOnly(context.Background(), getenv)
	if err != nil {
		return err
	}
	bootstrapDigest, err := record.Digest()
	if err != nil {
		readDB.Close()
		return err
	}
	owner, err := contracts.InstallationOwnerPrincipal(bootstrapDigest)
	if err != nil {
		readDB.Close()
		return err
	}
	if envelope.Plan.BootstrapDigest != bootstrapDigest || envelope.Plan.OwnerID != owner.ID {
		readDB.Close()
		return errors.New("migration preview belongs to another installation")
	}
	root, err := currentInstallationRoot(context.Background(), repo, owner, time.Now().UTC())
	readDB.Close()
	if err != nil || root.Ref != envelope.Plan.InstallationRoot || root.Digest != envelope.Plan.RootDigest {
		return errors.New("migration preview root is no longer current")
	}
	current, err := user.Current()
	if err != nil || current.Username == "" || !strings.HasSuffix(root.ProvenanceRef, ":os-user:"+current.Username) {
		return errors.New("authenticated migration owner does not match installation root")
	}
	fmt.Fprintf(out, "Authorize exact schema migration %s. Type %q to continue: ", envelope.Plan.PlanDigest, "MIGRATE "+envelope.Plan.PlanDigest)
	answer, err := bufio.NewReader(input).ReadString('\n')
	if err != nil {
		return err
	}
	if strings.TrimSpace(answer) != "MIGRATE "+envelope.Plan.PlanDigest {
		return errAuthorityBootstrapConfirmation
	}
	db, err := state.OpenSQLiteForGovernedMigration(context.Background(), getenv("PRAXIS_DB"))
	if err != nil {
		return err
	}
	defer db.Close()
	snapshot := getenv("PRAXIS_DB") + ".migration-" + strings.TrimPrefix(envelope.Plan.PlanDigest, "sha256:") + ".snapshot"
	journal, err := sqlite.ApplyPlan(context.Background(), db, envelope.Plan, snapshot, owner.ID, time.Now().UTC())
	if err != nil {
		return err
	}
	return printJSONTo(out, map[string]any{"operation": "migration.execute", "plan_digest": envelope.Plan.PlanDigest, "journal": journal})
}

func runMigrationRecover(args []string, getenv func(string) string, out io.Writer) error {
	f := flag.NewFlagSet("migration recover", flag.ContinueOnError)
	planDigest := f.String("plan-digest", "", "exact system-produced migration plan digest")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 || *planDigest == "" {
		return errors.New("usage: praxis migration recover --plan-digest <digest>")
	}
	db, err := state.OpenSQLiteReadOnly(context.Background(), getenv("PRAXIS_DB"))
	if err != nil {
		return err
	}
	defer db.Close()
	journal, err := sqlite.ReadJournal(context.Background(), db, *planDigest)
	if err != nil {
		return err
	}
	return printJSONTo(out, journal)
}

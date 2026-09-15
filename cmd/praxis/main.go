package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/convergent-systems-co/praxis/internal/client"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type normalizedOutput struct {
	EntryPointID   string            `json:"entry_point_id"`
	PackageID      string            `json:"package_id"`
	PackageVersion string            `json:"package_version"`
	PackageDigest  string            `json:"package_digest"`
	GraphID        string            `json:"graph_id"`
	GraphVersion   string            `json:"graph_version"`
	Arguments      []string          `json:"arguments,omitempty"`
	Options        map[string]string `json:"options"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "praxis:", err)
		os.Exit(2)
	}
}

func run(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		return runHelp(args)
	}
	switch args[0] {
	case "status":
		return runStatus(args[1:])
	case "discover", "info", "install", "update", "rollback", "disable", "uninstall", "list":
		return runPackageCommand(args[0], args[1:])
	case "doctor":
		return runDoctor(args[1:])
	case "key-bootstrap":
		return runKeyBootstrap(args[1:])
	case "state-init":
		return runStateInit(args[1:], os.Getenv)
	case "version":
		return runVersion(args[1:])
	case "resume", "cancel":
		return runControlCommand(args[0], args[1:])
	case "goal-drive":
		return runNativeGoalDriveInvocation(context.Background(), args[1:], os.Getenv)
	case "goal":
		return runGoalCommand(context.Background(), args[1:], os.Getenv)
	}
	return runDynamicInvocation(context.Background(), args, os.Getenv)
}

// goal-drive is a first-party control-plane surface. It must be able to reach
// runtime-owned SQLite creation on first use; resolving it through the
// package registry first would require opening a database that does not yet
// exist. Other dynamic/package commands retain registry resolution.
func runNativeGoalDriveInvocation(ctx context.Context, args []string, getenv func(string) string) error {
	parsed, err := client.ParseSlashInvocation("praxis goal-drive " + strings.Join(args, " "))
	if err != nil {
		return err
	}
	return dispatchGoalDrive(ctx, normalizedOutput{EntryPointID: "goal-drive", PackageID: "praxis.package.goals", PackageVersion: "0.1.0", GraphID: "praxis.package.goals.default", GraphVersion: "0.2.0", Options: parsed.Options}, getenv)
}

func runDynamicInvocation(ctx context.Context, args []string, getenv func(string) string) error {
	out, err := resolveDynamicInvocation(ctx, args, getenv)
	if err != nil {
		return err
	}
	if out.EntryPointID == "goal-drive" {
		return dispatchGoalDrive(ctx, out, getenv)
	}
	encoded, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(encoded))
	return nil
}

func resolveDynamicInvocation(ctx context.Context, args []string, getenv func(string) string) (normalizedOutput, error) {
	if len(args) == 0 {
		return normalizedOutput{}, errors.New("entry point is required")
	}
	dbPath := ""
	if getenv != nil {
		dbPath = getenv("PRAXIS_DB")
	}
	if dbPath == "" {
		return normalizedOutput{}, errors.New("PRAXIS_DB is required to resolve installed package commands")
	}
	db, err := state.OpenSQLiteReadOnly(ctx, dbPath)
	if err != nil {
		return normalizedOutput{}, err
	}
	defer db.Close()
	registered, err := state.New(db).ActiveInvocations(ctx)
	if err != nil {
		return normalizedOutput{}, err
	}
	contractsList := make([]contracts.InvocationContract, 0, len(registered))
	for _, item := range registered {
		contractsList = append(contractsList, item.Contract)
	}
	registry, err := client.NewRegistry(contractsList)
	if err != nil {
		return normalizedOutput{}, err
	}
	input := "praxis " + strings.Join(args, " ")
	inv, err := client.ParseSlashInvocation(input)
	if err != nil {
		return normalizedOutput{}, err
	}
	contract, options, err := registry.Resolve(inv)
	if err != nil {
		return normalizedOutput{}, err
	}
	var packageDigest string
	for _, item := range registered {
		if item.Contract.EntryPointID == contract.EntryPointID && item.Contract.PackageID == contract.PackageID && item.Contract.PackageVersion == contract.PackageVersion {
			packageDigest = item.ContentDigest
			break
		}
	}
	if packageDigest == "" {
		return normalizedOutput{}, errors.New("resolved invocation is not bound to an active package generation")
	}
	return normalizedOutput{EntryPointID: contract.EntryPointID, PackageID: contract.PackageID, PackageVersion: contract.PackageVersion, PackageDigest: packageDigest, GraphID: contract.GraphID, GraphVersion: contract.GraphVersion, Arguments: inv.Arguments, Options: options}, nil
}

func runHelp(args []string) error {
	fmt.Println("usage: praxis <command|installed-entry-point> [arguments] [options]")
	fmt.Println("core: discover, info, install, update, disable, uninstall, list, help, status, resume, cancel, doctor, key-bootstrap, state-init, goal import, version")
	path := os.Getenv("PRAXIS_DB")
	if path == "" {
		fmt.Println("installed entry points: unavailable (set PRAXIS_DB to inspect the active registry)")
		return nil
	}
	db, err := state.OpenSQLiteReadOnly(context.Background(), path)
	if err != nil {
		fmt.Printf("installed entry points: unavailable (%v)\n", err)
		return nil
	}
	defer db.Close()
	items, err := state.New(db).ActiveInvocations(context.Background())
	if err != nil {
		fmt.Printf("installed entry points: unavailable (%v)\n", err)
		return nil
	}
	aliases := make([]string, 0)
	for _, item := range items {
		aliases = append(aliases, item.Contract.Aliases...)
	}
	sort.Strings(aliases)
	if len(aliases) == 0 {
		fmt.Println("installed entry points: none")
		return nil
	}
	fmt.Println("installed entry points:", strings.Join(aliases, ", "))
	return nil
}

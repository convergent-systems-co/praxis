package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/convergent-systems-co/praxis/internal/client"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type normalizedOutput struct {
	EntryPointID string            `json:"entry_point_id"`
	PackageID    string            `json:"package_id"`
	GraphID      string            `json:"graph_id"`
	Arguments    []string          `json:"arguments,omitempty"`
	Options      map[string]string `json:"options"`
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
	case "discover", "info", "install", "update", "uninstall", "list":
		return runPackageCommand(args[0], args[1:])
	case "doctor", "version", "resume", "cancel":
		return fmt.Errorf("core command %q is reserved but not yet wired", args[0])
	}
	return runDynamicInvocation(context.Background(), args, os.Getenv)
}

func runDynamicInvocation(ctx context.Context, args []string, getenv func(string) string) error {
	if len(args) == 0 { return errors.New("entry point is required") }
	dbPath := ""
	if getenv != nil { dbPath = getenv("PRAXIS_DB") }
	if dbPath == "" { return errors.New("PRAXIS_DB is required to resolve installed package commands") }
	db, err := state.OpenSQLiteReadOnly(ctx, dbPath)
	if err != nil { return err }
	defer db.Close()
	registered, err := state.New(db).ActiveInvocations(ctx)
	if err != nil { return err }
	contractsList := make([]contracts.InvocationContract,0,len(registered))
	for _, item := range registered { contractsList=append(contractsList,item.Contract) }
	registry, err := client.NewRegistry(contractsList)
	if err != nil { return err }
	input := "praxis " + strings.Join(args, " ")
	inv, err := client.ParseSlashInvocation(input)
	if err != nil { return err }
	contract, options, err := registry.Resolve(inv)
	if err != nil { return err }
	out := normalizedOutput{EntryPointID: contract.EntryPointID, PackageID: contract.PackageID, GraphID: contract.GraphID, Arguments: inv.Arguments, Options: options}
	encoded, err := json.MarshalIndent(out, "", "  ")
	if err != nil { return err }
	fmt.Println(string(encoded))
	return nil
}

func runHelp(args []string) error {
	fmt.Println("usage: praxis <command|installed-entry-point> [arguments] [options]")
	fmt.Println("core: discover, info, install, update, uninstall, list, help, status, resume, cancel, doctor, version")
	fmt.Println("package entry points are discovered dynamically from installed InvocationContracts")
	return nil
}

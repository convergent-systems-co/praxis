package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/convergent-systems-co/praxis/internal/client"
	"github.com/convergent-systems-co/praxis/packages/develop"
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
		fmt.Println("usage: praxis <entry-point> [arguments] [options]")
		fmt.Println("available: develop")
		return nil
	}

	input := "praxis " + strings.Join(args, " ")
	inv, err := client.ParseSlashInvocation(input)
	if err != nil {
		return err
	}
	registry, err := client.NewRegistry([]contracts.InvocationContract{develop.InvocationContract()})
	if err != nil {
		return err
	}
	contract, options, err := registry.Resolve(inv)
	if err != nil {
		return err
	}

	out := normalizedOutput{EntryPointID: contract.EntryPointID, PackageID: contract.PackageID, GraphID: contract.GraphID, Arguments: inv.Arguments, Options: options}
	encoded, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(encoded))
	return nil
}

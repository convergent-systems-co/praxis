package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"

	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/internal/stateprovider"
)

const praxisVersion = "2.0.0-dev"

func runVersion(args []string) error {
	if len(args) != 0 {
		return errors.New("usage: praxis version")
	}
	fmt.Println(praxisVersion)
	return nil
}

func runDoctor(args []string) error {
	if len(args) != 0 {
		return errors.New("usage: praxis doctor")
	}
	result := map[string]any{
		"praxis_version": praxisVersion,
		"go_version":     runtime.Version(),
		"state":          "not configured",
	}
	path := os.Getenv("PRAXIS_DB")
	if path != "" {
		db, err := state.OpenSQLiteReadOnly(context.Background(), path)
		if err != nil {
			result["state"] = "failed"
			result["state_error"] = err.Error()
			_ = printJSON(result)
			return fmt.Errorf("state provider: %w", err)
		}
		defer db.Close()
		provider := stateprovider.NewSQLite(db)
		if err := provider.Profile().Require(
			stateprovider.EventsAppendOptimistic,
			stateprovider.EventsReplay,
			stateprovider.LeaseAtomicConsume,
			stateprovider.PackagesAtomicActivation,
			stateprovider.RunsDurableReplay,
		); err != nil {
			result["state"] = "failed"
			result["state_error"] = err.Error()
			_ = printJSON(result)
			return err
		}
		result["state"] = "ok"
		result["state_provider"] = "sqlite"
		result["database"] = path
	}
	return printJSON(result)
}

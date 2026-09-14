package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
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
	bootstrapPath := os.Getenv("PRAXIS_BOOTSTRAP_RECORD")
	var bootstrapErr error
	if bootstrapPath == "" {
		result["bootstrap"] = "not configured"
		bootstrapErr = errors.New("bootstrap metadata is not configured")
	} else if record, err := praxiscrypto.LoadBootstrapRecord(bootstrapPath); err != nil {
		result["bootstrap"] = "failed"
		result["bootstrap_error"] = err.Error()
		bootstrapErr = err
	} else {
		registry, err := praxiscrypto.NewFirstPartyBootstrapRegistry()
		if err != nil {
			bootstrapErr = err
		} else if _, err := registry.Open(context.Background(), record); err != nil {
			bootstrapErr = err
		}
		if bootstrapErr != nil {
			result["bootstrap"] = "unavailable"
			result["bootstrap_error"] = bootstrapErr.Error()
		} else {
			result["bootstrap"] = "ready"
		}
		result["bootstrap_record"] = bootstrapPath
	}
	path := os.Getenv("PRAXIS_DB")
	if path != "" {
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			result["state"] = "uninitialized"
			result["database"] = path
			return printJSON(result)
		}
		db, err := state.OpenSQLiteReadOnly(context.Background(), path)
		if err != nil {
			result["state"] = "failed"
			result["state_error"] = err.Error()
			_ = printJSON(result)
			return fmt.Errorf("state provider: %w", err)
		}
		defer db.Close()
		if bootstrapErr != nil {
			result["state"] = "bootstrap mismatch"
			result["database"] = path
			_ = printJSON(result)
			return fmt.Errorf("bootstrap/state mismatch: %w", bootstrapErr)
		}
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

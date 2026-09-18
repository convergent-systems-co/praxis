package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/internal/state"
)

// runStateInit is the explicit first-party state initializer. It proves the
// configured bootstrap can be opened before creating or migrating SQLite.
func runStateInit(args []string, getenv func(string) string) error {
	if len(args) != 0 {
		return errors.New("usage: praxis state-init")
	}
	if getenv == nil {
		getenv = os.Getenv
	}
	bootstrapPath := getenv("PRAXIS_BOOTSTRAP_RECORD")
	if bootstrapPath == "" {
		return errors.New("PRAXIS_BOOTSTRAP_RECORD is required before state initialization")
	}
	record, err := praxiscrypto.LoadBootstrapRecord(bootstrapPath)
	if err != nil {
		return fmt.Errorf("load bootstrap metadata: %w", err)
	}
	registry, err := praxiscrypto.NewFirstPartyBootstrapRegistry()
	if err != nil {
		return err
	}
	if _, err := registry.Open(context.Background(), record); err != nil {
		return fmt.Errorf("open configured bootstrap provider: %w", err)
	}
	dbPath := getenv("PRAXIS_DB")
	if dbPath == "" {
		return errors.New("PRAXIS_DB is required for state initialization")
	}
	db, err := state.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		return fmt.Errorf("initialize Praxis state: %w", err)
	}
	defer db.Close()
	fmt.Printf("Praxis state initialized: %s\n", dbPath)
	return nil
}

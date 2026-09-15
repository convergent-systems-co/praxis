package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/internal/goalstore"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/packages/goals"
)

type baselineImportDocument struct {
	SchemaVersion string             `json:"schema_version"`
	SourceRef     string             `json:"source_ref"`
	SourceDigest  string             `json:"source_digest"`
	Baseline      goals.GoalBaseline `json:"baseline"`
}

func runGoalCommand(ctx context.Context, args []string, getenv func(string) string) error {
	if len(args) != 2 || args[0] != "import" {
		return errors.New("usage: praxis goal import <canonical-baseline.json>")
	}
	path, err := filepath.Abs(args[1])
	if err != nil {
		return err
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read Goal Baseline import: %w", err)
	}
	var doc baselineImportDocument
	if err := json.Unmarshal(body, &doc); err != nil {
		return fmt.Errorf("decode Goal Baseline import: %w", err)
	}
	if doc.SchemaVersion != "1" || doc.SourceRef == "" || doc.SourceDigest == "" {
		return errors.New("Goal Baseline import requires schema_version 1 and source provenance")
	}
	if doc.SourceRef != path {
		return errors.New("Goal Baseline import source_ref must be the canonical absolute path")
	}
	doc.Baseline.ImportSourceRef, doc.Baseline.ImportSourceDigest = doc.SourceRef, doc.SourceDigest
	if err := doc.Baseline.Validate(); err != nil {
		return fmt.Errorf("validate Goal Baseline import: %w", err)
	}
	canonical, err := doc.Baseline.CanonicalBytes()
	if err != nil {
		return fmt.Errorf("canonicalize Goal Baseline import: %w", err)
	}
	if getenv == nil {
		getenv = os.Getenv
	}
	bootstrapPath := getenv("PRAXIS_BOOTSTRAP_RECORD")
	if bootstrapPath == "" {
		return errors.New("PRAXIS_BOOTSTRAP_RECORD is required before Goal Baseline import")
	}
	record, err := praxiscrypto.LoadBootstrapRecord(bootstrapPath)
	if err != nil {
		return fmt.Errorf("load bootstrap metadata: %w", err)
	}
	registry, err := praxiscrypto.NewFirstPartyBootstrapRegistry()
	if err != nil {
		return err
	}
	wrapper, err := registry.Open(ctx, record)
	if err != nil {
		return fmt.Errorf("open configured bootstrap provider: %w", err)
	}
	providers := praxiscrypto.NewProviderRegistry()
	if err := providers.Register(record.ProviderID, wrapper); err != nil {
		return err
	}
	service, err := providers.Service(record.ProviderID, praxiscrypto.EnvelopePolicy{})
	if err != nil {
		return err
	}
	dbPath := getenv("PRAXIS_DB")
	if dbPath == "" {
		return errors.New("PRAXIS_DB is required before Goal Baseline import")
	}
	db, err := state.OpenSQLite(ctx, dbPath)
	if err != nil {
		return fmt.Errorf("open authoritative Praxis state: %w", err)
	}
	defer db.Close()
	repo := goalstore.Repository{Store: state.New(db), Crypto: service, KeyRef: record.KeyID, Profile: record.Profile, Sensitivity: state.SensitivityConfidential}
	imported, err := repo.ImportBaseline(ctx, goalstore.ImportBaselineRequest{Baseline: doc.Baseline, SourceRef: doc.SourceRef, Source: canonical, CreatedAt: time.Now().UTC()})
	if err != nil {
		return fmt.Errorf("import Goal Baseline: %w", err)
	}
	encoded, _ := json.Marshal(map[string]string{"goal_id": imported.ID, "goal_version": imported.Version, "baseline_digest": imported.Digest, "status": "authoritative"})
	fmt.Println(string(encoded))
	return nil
}

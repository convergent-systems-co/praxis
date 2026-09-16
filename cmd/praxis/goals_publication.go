package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"time"

	"github.com/convergent-systems-co/praxis/internal/distribution"
	"github.com/convergent-systems-co/praxis/internal/goalspublication"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// This producer exposes only the accepted fixed operation. It never accepts a
// caller-authored intent file, destination, package, grant or signing command.
func runGoalsPublication(mode string, args []string, getenv func(string) string, out io.Writer) error {
	f := flag.NewFlagSet("publisher goals-publication-"+mode, flag.ContinueOnError)
	f.SetOutput(out)
	dir := f.String("package-dir", "", "directory containing the exact existing signed Goals assets")
	request := f.String("request-id", "", "exact system-produced publication request ID")
	expiry := f.String("expires-at", "", "finite authorization expiry (RFC3339)")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return errors.New("unexpected publication arguments")
	}
	ctx := context.Background()
	now := time.Now().UTC()
	remote := goalspublication.GitHub{}
	switch mode {
	case "prepare":
		if *dir == "" || *expiry == "" || *request != "" {
			return errors.New("usage: praxis publisher goals-publication-prepare --package-dir <dir> --expires-at <RFC3339>")
		}
		until, err := time.Parse(time.RFC3339, *expiry)
		if err != nil || !until.After(now) {
			return errors.New("future publication expiry required")
		}
		assets, err := goalspublication.ReadAssets(*dir)
		if err != nil {
			return err
		}
		// Verify protected state read-only before persisting a canonical request.
		repo, db, record, err := openGovernedRepositoryReadOnly(ctx, getenv)
		if err != nil {
			return err
		}
		bd, err := record.Digest()
		if err != nil || bd != contracts.GoalsPublicationBootstrap {
			db.Close()
			return errors.New("wrong publication installation")
		}
		err = goalspublication.VerifySigning(ctx, repo, assets, now)
		db.Close()
		if err != nil {
			return err
		}
		identity, err := remote.Identity(ctx)
		if err != nil {
			return err
		}
		if err = remote.Empty(ctx); err != nil {
			return err
		}
		nonce := make([]byte, 32)
		if _, err = rand.Read(nonce); err != nil {
			return err
		}
		intent, err := contracts.NewGoalsPublicationIntent(contracts.GoalsPublicationInput{RepositoryID: identity.RepositoryID, OwnerID: identity.OwnerID, AccountID: identity.AccountID, CreatedAt: now, ExpiresAt: until, Nonce: hex.EncodeToString(nonce), ManifestSize: int64(len(assets[0])), ArchiveSize: int64(len(assets[1])), SignatureSize: int64(len(assets[2]))})
		if err != nil {
			return err
		}
		writable, db, err := openGovernedRepository(ctx, getenv)
		if err != nil {
			return err
		}
		defer db.Close()
		req, digest, err := writable.SaveGoalsPublicationRequest(ctx, intent, now)
		if err != nil {
			return err
		}
		return printJSONTo(out, map[string]any{"request": req, "request_digest": digest, "authorized": false, "published": false})
	case "execute":
		if *dir == "" || *request == "" || *expiry != "" {
			return errors.New("usage: praxis publisher goals-publication-execute --package-dir <dir> --request-id <id>")
		}
		assets, err := goalspublication.ReadAssets(*dir)
		if err != nil {
			return err
		}
		repo, db, err := openGovernedRepository(ctx, getenv)
		if err != nil {
			return err
		}
		defer db.Close()
		id, err := (goalspublication.Execution{Repository: repo, Adapter: remote, Assets: assets}).Execute(ctx, *request)
		if err != nil {
			return err
		}
		return printJSONTo(out, map[string]any{"completion_event": id, "published": true})
	case "reconcile":
		if *request == "" || *dir != "" || *expiry != "" {
			return errors.New("usage: praxis publisher goals-publication-reconcile --request-id <id>")
		}
		repo, db, err := openGovernedRepository(ctx, getenv)
		if err != nil {
			return err
		}
		defer db.Close()
		return (goalspublication.Execution{Repository: repo, Adapter: remote}).Reconcile(ctx, *request)
	case "inspect":
		if *request == "" || *dir != "" || *expiry != "" {
			return errors.New("usage: praxis publisher goals-publication-inspect --request-id <id>")
		}
		repo, db, _, err := openGovernedRepositoryReadOnly(ctx, getenv)
		if err != nil {
			return err
		}
		defer db.Close()
		rows, err := repo.Store.DB().QueryContext(ctx, `SELECT e.effect_id,e.state,e.attempts,e.request_payload,e.observed_result,e.reconciliation_evidence FROM effects e JOIN commands c ON c.command_id=e.command_id WHERE c.command_type='goals-publication.step' ORDER BY e.created_at`)
		if err != nil {
			return err
		}
		defer rows.Close()
		return printPublicationInspection(rows, *request, out)
	default:
		return errors.New("unknown Goals publication operation")
	}
}

func checkGoalsPublicationAcquisition(ctx context.Context, release distribution.Release, artifact []byte, getenv func(string) string) error {
	if !goalspublication.RequiresLocalLineage(release) {
		return nil
	}
	repo, db, err := openGovernedRepository(ctx, getenv)
	if err != nil {
		return err
	}
	defer db.Close()
	execution := goalspublication.Execution{Repository: repo, Adapter: goalspublication.GitHub{}}
	completion, err := execution.CheckAcquisition(ctx, release, artifact)
	if err != nil {
		return err
	}
	return execution.RecordAcquisitionCheck(ctx, completion)
}
func printPublicationInspection(rows *sql.Rows, requestID string, out io.Writer) error {
	result := []map[string]any{}
	for rows.Next() {
		var id, status string
		var attempts int
		var payload, observed, reconciliation []byte
		if err := rows.Scan(&id, &status, &attempts, &payload, &observed, &reconciliation); err != nil {
			return err
		}
		var p struct{ RequestID string }
		if err := json.Unmarshal(payload, &p); err != nil {
			return err
		}
		if p.RequestID != requestID {
			continue
		}
		result = append(result, map[string]any{"effect_id": id, "state": status, "attempts": attempts, "request": json.RawMessage(payload), "observed": json.RawMessage(observed), "reconciliation": json.RawMessage(reconciliation)})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return printJSONTo(out, result)
}

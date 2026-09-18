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
	"os"
	"os/user"
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
	effectID := f.String("effect-id", "", "exact observational effect ID")
	priorRequest := f.String("prior-recovery-request-id", "", "exact prior abandoned recovery request ID")
	expiry := f.String("expires-at", "", "finite authorization expiry (RFC3339)")
	ownerConfirmation := f.String("confirmation", "", "exact owner confirmation: ABANDON <payload-digest>")
	reason := f.String("reason", "", "reason for terminally abandoning the exact execution")
	previewFile := f.String("preview-file", "", "exact frozen abandonment payload to confirm")
	output := f.String("output", "", "write the exact frozen abandonment payload to this new file")
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
	case "abandon-preview":
		if *request == "" || *dir != "" || *expiry != "" || *reason == "" || *ownerConfirmation != "" || *previewFile != "" || *output == "" {
			return errors.New("usage: praxis publisher goals-publication-abandon-preview --request-id <id> --reason <text> --output <new-file>")
		}
		current, err := user.Current()
		if err != nil || current.Username == "" {
			return errors.New("cannot authenticate current OS user")
		}
		repo, db, _, err := openGovernedRepositoryReadOnly(ctx, getenv)
		if err != nil {
			return err
		}
		defer db.Close()
		payload, digest, err := (goalspublication.Execution{Repository: repo}).PrepareAbandonment(ctx, *request, current.Username, *reason)
		if err != nil {
			return err
		}
		if err := writeCanonicalPreviewFile(*output, payload); err != nil {
			return err
		}
		return printJSONTo(out, map[string]any{"preview_file": *output, "payload_digest": digest, "confirmation": "ABANDON " + digest})
	case "abandon":
		if *request != "" || *dir != "" || *expiry != "" || *reason != "" || *ownerConfirmation == "" || *previewFile == "" || *output != "" {
			return errors.New("usage: praxis publisher goals-publication-abandon --preview-file <frozen-payload> --confirmation 'ABANDON <digest>'")
		}
		current, err := user.Current()
		if err != nil || current.Username == "" {
			return errors.New("cannot authenticate current OS user")
		}
		payload, err := os.ReadFile(*previewFile)
		if err != nil {
			return err
		}
		repo, db, err := openGovernedRepository(ctx, getenv)
		if err != nil {
			return err
		}
		defer db.Close()
		id, err := (goalspublication.Execution{Repository: repo}).ConfirmAbandonment(ctx, payload, current.Username, *ownerConfirmation)
		if err != nil {
			return err
		}
		return printJSONTo(out, map[string]any{"abandonment_event": id, "completion_established": false, "authority_revoked": false})
	case "recovery-prepare":
		if *dir == "" || *request == "" || *expiry == "" || *ownerConfirmation != "" || *reason != "" {
			return errors.New("usage: praxis publisher goals-publication-recovery-prepare --package-dir <dir> --predecessor-request-id <id> --expires-at <RFC3339>")
		}
		until, err := time.Parse(time.RFC3339, *expiry)
		if err != nil || !until.After(now) {
			return errors.New("future finite successor expiry required")
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
		nonce := make([]byte, 32)
		if _, err = rand.Read(nonce); err != nil {
			return err
		}
		intent, err := (goalspublication.RecoveryExecution{Repository: repo, Adapter: goalspublication.RecoveryGitHub{}, Assets: assets}).PrepareIntent(ctx, *request, hex.EncodeToString(nonce), until)
		if err != nil {
			return err
		}
		req, digest, err := repo.SaveGoalsPublicationRecoveryRequest(ctx, intent, now)
		if err != nil {
			return err
		}
		return printJSONTo(out, map[string]any{"request": req, "request_digest": digest, "authorized": false, "published": false})
	case "recovery-execute":
		if *dir == "" || *request == "" || *expiry != "" || *ownerConfirmation != "" || *reason != "" {
			return errors.New("usage: praxis publisher goals-publication-recovery-execute --package-dir <dir> --request-id <id>")
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
		id, err := (goalspublication.RecoveryExecution{Repository: repo, Adapter: goalspublication.RecoveryGitHub{}, Assets: assets}).Execute(ctx, *request)
		if err != nil {
			return err
		}
		return printJSONTo(out, map[string]any{"completion_event": id, "published": true})
	case "recovery-chain-prepare":
		if *dir == "" || *priorRequest == "" || *expiry == "" || *request != "" || *ownerConfirmation != "" || *reason != "" {
			return errors.New("usage: praxis publisher goals-publication-recovery-chain-prepare --package-dir <dir> --prior-recovery-request-id <id> --expires-at <RFC3339>")
		}
		until, err := time.Parse(time.RFC3339, *expiry)
		if err != nil || !until.After(now) {
			return errors.New("future finite successor expiry required")
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
		nonce := make([]byte, 32)
		if _, err = rand.Read(nonce); err != nil {
			return err
		}
		intent, err := (goalspublication.RecoveryExecution{Repository: repo, Adapter: goalspublication.RecoveryGitHub{}, Assets: assets}).PrepareChainedIntent(ctx, *priorRequest, hex.EncodeToString(nonce), until)
		if err != nil {
			return err
		}
		req, digest, err := repo.SaveGoalsPublicationRecoveryRequest(ctx, intent, now)
		if err != nil {
			return err
		}
		return printJSONTo(out, map[string]any{"request": req, "request_digest": digest, "authorized": false, "published": false})
	case "recovery-ordered-prepare":
		if *dir == "" || *priorRequest == "" || *expiry == "" || *request != "" || *ownerConfirmation != "" || *reason != "" {
			return errors.New("usage: praxis publisher goals-publication-recovery-ordered-prepare --package-dir <dir> --latest-recovery-request-id <id> --expires-at <RFC3339>")
		}
		until, err := time.Parse(time.RFC3339, *expiry)
		if err != nil || !until.After(now) {
			return errors.New("future finite successor expiry required")
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
		nonce := make([]byte, 32)
		if _, err = rand.Read(nonce); err != nil {
			return err
		}
		intent, err := (goalspublication.RecoveryExecution{Repository: repo, Adapter: goalspublication.RecoveryGitHub{}, Assets: assets}).PrepareOrderedIntent(ctx, *priorRequest, hex.EncodeToString(nonce), until)
		if err != nil {
			return err
		}
		req, digest, err := repo.SaveGoalsPublicationRecoveryRequest(ctx, intent, now)
		if err != nil {
			return err
		}
		return printJSONTo(out, map[string]any{"request": req, "request_digest": digest, "authorized": false, "published": false})
	case "recovery-failed-verification-prepare":
		if *dir == "" || *request == "" || *expiry == "" || *ownerConfirmation != "" || *reason != "" {
			return errors.New("usage: praxis publisher goals-publication-recovery-failed-verification-prepare --package-dir <dir> --request-id <failed-request-id> --expires-at <RFC3339>")
		}
		until, err := time.Parse(time.RFC3339, *expiry)
		if err != nil || !until.After(now) {
			return errors.New("future finite successor expiry required")
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
		nonce := make([]byte, 32)
		if _, err = rand.Read(nonce); err != nil {
			return err
		}
		intent, err := (goalspublication.RecoveryExecution{Repository: repo, Adapter: goalspublication.RecoveryGitHub{}, Assets: assets}).PrepareFailedVerificationIntent(ctx, *request, hex.EncodeToString(nonce), until)
		if err != nil {
			return err
		}
		req, digest, err := repo.SaveGoalsPublicationRecoveryRequest(ctx, intent, now)
		if err != nil {
			return err
		}
		return printJSONTo(out, map[string]any{"request": req, "request_digest": digest, "authorized": false, "published": false})
	case "recovery-failed-publication-prepare":
		if *dir == "" || *request == "" || *expiry == "" || *ownerConfirmation != "" || *reason != "" {
			return errors.New("usage: praxis publisher goals-publication-recovery-failed-publication-prepare --package-dir <dir> --request-id <failed-request-id> --expires-at <RFC3339>")
		}
		until, err := time.Parse(time.RFC3339, *expiry)
		if err != nil || !until.After(now) {
			return errors.New("future finite successor expiry required")
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
		nonce := make([]byte, 32)
		if _, err = rand.Read(nonce); err != nil {
			return err
		}
		intent, err := (goalspublication.RecoveryExecution{Repository: repo, Adapter: goalspublication.RecoveryGitHub{}, Assets: assets}).PrepareFailedPublicationIntent(ctx, *request, hex.EncodeToString(nonce), until)
		if err != nil {
			return err
		}
		req, digest, err := repo.SaveGoalsPublicationRecoveryRequest(ctx, intent, now)
		if err != nil {
			return err
		}
		return printJSONTo(out, map[string]any{"request": req, "request_digest": digest, "authorized": false, "published": false})
	case "recovery-reconcile":
		if *request == "" || *dir != "" || *expiry != "" || *ownerConfirmation != "" || *reason != "" {
			return errors.New("usage: praxis publisher goals-publication-recovery-reconcile --request-id <id>")
		}
		repo, db, err := openGovernedRepository(ctx, getenv)
		if err != nil {
			return err
		}
		defer db.Close()
		return (goalspublication.RecoveryExecution{Repository: repo, Adapter: goalspublication.RecoveryGitHub{}}).Reconcile(ctx, *request)
	case "recovery-resolve-observation":
		if *request == "" || *effectID == "" || *dir != "" || *expiry != "" || *reason != "" || *previewFile != "" || *output != "" {
			return errors.New("usage: praxis publisher goals-publication-recovery-resolve-observation --request-id <id> --effect-id <id> [--confirmation 'RESOLVE <digest>']")
		}
		if *ownerConfirmation == "" {
			repo, db, _, err := openGovernedRepositoryReadOnly(ctx, getenv)
			if err != nil {
				return err
			}
			defer db.Close()
			digest, err := (goalspublication.RecoveryExecution{Repository: repo}).ObservationResolutionChallenge(ctx, *request, *effectID)
			if err != nil {
				return err
			}
			return printJSONTo(out, map[string]any{"request_id": *request, "effect_id": *effectID, "resolution_digest": digest, "confirmation": "RESOLVE " + digest, "resolved": false})
		}
		repo, db, err := openGovernedRepository(ctx, getenv)
		if err != nil {
			return err
		}
		defer db.Close()
		id, err := (goalspublication.RecoveryExecution{Repository: repo}).ResolveObservation(ctx, *request, *effectID, *ownerConfirmation)
		if err != nil {
			return err
		}
		return printJSONTo(out, map[string]any{"resolution_event": id, "resolved": true})
	case "recovery-abandon-preview":
		if *request == "" || *reason == "" || *output == "" || *dir != "" || *previewFile != "" || *ownerConfirmation != "" {
			return errors.New("usage: praxis publisher goals-publication-recovery-abandon-preview --request-id <id> --reason <text> --output <new-file>")
		}
		current, err := user.Current()
		if err != nil || current.Username == "" {
			return errors.New("cannot authenticate current OS user")
		}
		repo, db, err := openGovernedRepository(ctx, getenv)
		if err != nil {
			return err
		}
		defer db.Close()
		payload, digest, err := (goalspublication.RecoveryExecution{Repository: repo}).PrepareAbandonment(ctx, *request, current.Username, *reason)
		if err != nil {
			return err
		}
		if err = writeCanonicalPreviewFile(*output, payload); err != nil {
			return err
		}
		return printJSONTo(out, map[string]any{"preview_file": *output, "payload_digest": digest, "confirmation": "ABANDON " + digest})
	case "recovery-abandon":
		if *previewFile == "" || *ownerConfirmation == "" || *request != "" || *dir != "" || *reason != "" || *output != "" {
			return errors.New("usage: praxis publisher goals-publication-recovery-abandon --preview-file <frozen-payload> --confirmation 'ABANDON <digest>'")
		}
		current, err := user.Current()
		if err != nil || current.Username == "" {
			return errors.New("cannot authenticate current OS user")
		}
		payload, err := os.ReadFile(*previewFile)
		if err != nil {
			return err
		}
		repo, db, err := openGovernedRepository(ctx, getenv)
		if err != nil {
			return err
		}
		defer db.Close()
		id, err := (goalspublication.RecoveryExecution{Repository: repo}).ConfirmAbandonment(ctx, payload, current.Username, *ownerConfirmation)
		if err != nil {
			return err
		}
		return printJSONTo(out, map[string]any{"abandonment_event": id, "completion_established": false, "authority_revoked": false})
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
	// The acquisition-lineage check binds the publishing installation to its
	// own publication. An installation that holds no publication lineage is
	// an ordinary consumer: the release is admitted only through trusted-key
	// signature verification, exactly like any third-party package.
	holds, err := execution.HoldsLocalLineage(ctx)
	if err != nil {
		return err
	}
	if !holds {
		return nil
	}
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

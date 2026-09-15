package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// MigrationDescriptor is the immutable identity of one embedded migration.
// The SQL bytes, not the filename alone, participate in the identity.
type MigrationDescriptor struct {
	Name       string `json:"name"`
	FromSchema int    `json:"from_schema"`
	ToSchema   int    `json:"to_schema"`
	Digest     string `json:"digest"`
}

type Status struct {
	CurrentSchema  int                   `json:"current_schema"`
	Applied        []string              `json:"applied"`
	Pending        []MigrationDescriptor `json:"pending"`
	MigrationSetID string                `json:"migration_set_id"`
	LatestSchema   int                   `json:"latest_schema"`
}

type Plan struct {
	ID               string                `json:"id"`
	Version          string                `json:"version"`
	BootstrapDigest  string                `json:"bootstrap_digest"`
	OwnerID          string                `json:"owner_id"`
	InstallationRoot string                `json:"installation_root"`
	RootDigest       string                `json:"root_digest"`
	CurrentSchema    int                   `json:"current_schema"`
	TargetSchema     int                   `json:"target_schema"`
	MigrationSetID   string                `json:"migration_set_id"`
	Migrations       []MigrationDescriptor `json:"migrations"`
	SnapshotRequired bool                  `json:"snapshot_required"`
	CreatedAt        time.Time             `json:"created_at"`
	PlanDigest       string                `json:"digest"`
}

type Journal struct {
	PlanDigest       string    `json:"plan_digest"`
	State            string    `json:"state"`
	SnapshotPath     string    `json:"snapshot_path,omitempty"`
	SnapshotDigest   string    `json:"snapshot_digest,omitempty"`
	AppliedMigration string    `json:"applied_migration,omitempty"`
	UpdatedAt        time.Time `json:"updated_at"`
}

const journalPrefix = "schema-migration:"

func migrationDescriptors() ([]MigrationDescriptor, map[string][]byte, error) {
	entries, err := fs.ReadDir(migrationFS, ".")
	if err != nil {
		return nil, nil, err
	}
	descriptors := make([]MigrationDescriptor, 0, len(entries))
	bodies := make(map[string][]byte, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		body, err := migrationFS.ReadFile(entry.Name())
		if err != nil {
			return nil, nil, err
		}
		to, err := migrationTarget(entry.Name(), body)
		if err != nil {
			return nil, nil, fmt.Errorf("migration %s: %w", entry.Name(), err)
		}
		digest := sha256.Sum256(body)
		descriptors = append(descriptors, MigrationDescriptor{Name: entry.Name(), ToSchema: to, Digest: "sha256:" + hex.EncodeToString(digest[:])})
		bodies[entry.Name()] = body
	}
	sort.Slice(descriptors, func(i, j int) bool { return descriptors[i].Name < descriptors[j].Name })
	previous := 0
	for i := range descriptors {
		if i == 0 {
			previous = descriptors[i].ToSchema - 1
		}
		descriptors[i].FromSchema = previous
		previous = descriptors[i].ToSchema
	}
	return descriptors, bodies, nil
}

func migrationTarget(name string, body []byte) (int, error) {
	if len(name) >= 4 {
		if version, err := strconv.Atoi(name[:4]); err == nil {
			return version, nil
		}
	}
	text := string(body)
	markers := []string{"UPDATE schema_meta SET value='", "VALUES ('schema_version', '"}
	var suffix string
	for _, marker := range markers {
		if idx := strings.Index(text, marker); idx >= 0 {
			suffix = text[idx+len(marker):]
			break
		}
	}
	if suffix == "" {
		return 0, errors.New("missing schema version update")
	}
	end := strings.IndexByte(suffix, '\'')
	if end < 0 {
		return 0, errors.New("malformed schema version update")
	}
	version, err := strconv.Atoi(suffix[:end])
	if err != nil || version < 1 {
		return 0, errors.New("invalid schema version update")
	}
	return version, nil
}

func migrationSetID(descriptors []MigrationDescriptor) (string, error) {
	b, err := json.Marshal(descriptors)
	if err != nil {
		return "", err
	}
	d := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(d[:]), nil
}

func StatusOf(ctx context.Context, db *sql.DB) (Status, error) {
	var current string
	if err := db.QueryRowContext(ctx, `SELECT value FROM schema_meta WHERE key='schema_version'`).Scan(&current); err != nil {
		return Status{}, fmt.Errorf("read schema version: %w", err)
	}
	currentSchema, err := strconv.Atoi(current)
	if err != nil {
		return Status{}, fmt.Errorf("invalid schema version %q", current)
	}
	descriptors, _, err := migrationDescriptors()
	if err != nil {
		return Status{}, err
	}
	setID, err := migrationSetID(descriptors)
	if err != nil {
		return Status{}, err
	}
	applied := make([]string, 0, len(descriptors))
	pending := make([]MigrationDescriptor, 0)
	for _, descriptor := range descriptors {
		var count int
		err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM praxis_schema_migrations WHERE name=?`, descriptor.Name).Scan(&count)
		if err != nil {
			return Status{}, fmt.Errorf("read migration ledger: %w", err)
		}
		if count == 1 {
			applied = append(applied, descriptor.Name)
		} else {
			pending = append(pending, descriptor)
		}
	}
	latest := descriptors[len(descriptors)-1].ToSchema
	return Status{CurrentSchema: currentSchema, Applied: applied, Pending: pending, MigrationSetID: setID, LatestSchema: latest}, nil
}

func NewPlan(ctx context.Context, db *sql.DB, bootstrapDigest, ownerID, rootRef, rootDigest string, now time.Time) (Plan, error) {
	if bootstrapDigest == "" || ownerID == "" || rootRef == "" || rootDigest == "" || now.IsZero() {
		return Plan{}, errors.New("migration plan requires installation identity and root")
	}
	status, err := StatusOf(ctx, db)
	if err != nil {
		return Plan{}, err
	}
	if len(status.Pending) == 0 {
		return Plan{}, errors.New("installation schema is already current")
	}
	plan := Plan{ID: "schema-migration:" + bootstrapDigest + ":" + strconv.Itoa(status.CurrentSchema) + "-" + strconv.Itoa(status.LatestSchema), Version: "1", BootstrapDigest: bootstrapDigest, OwnerID: ownerID, InstallationRoot: rootRef, RootDigest: rootDigest, CurrentSchema: status.CurrentSchema, TargetSchema: status.LatestSchema, MigrationSetID: status.MigrationSetID, Migrations: append([]MigrationDescriptor(nil), status.Pending...), SnapshotRequired: true, CreatedAt: now.UTC()}
	digest, err := plan.Digest()
	if err != nil {
		return Plan{}, err
	}
	plan.PlanDigest = digest
	return plan, nil
}

func (p Plan) Digest() (string, error) {
	copy := p
	copy.PlanDigest = ""
	b, err := json.Marshal(copy)
	if err != nil {
		return "", err
	}
	d := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(d[:]), nil
}

func (p Plan) Verify() error {
	if p.Version != "1" || p.BootstrapDigest == "" || p.OwnerID == "" || p.InstallationRoot == "" || p.RootDigest == "" || p.CurrentSchema < 1 || p.TargetSchema <= p.CurrentSchema || len(p.Migrations) == 0 || !p.SnapshotRequired || p.CreatedAt.IsZero() {
		return errors.New("migration plan is incomplete")
	}
	actual, err := p.Digest()
	if err != nil || actual != p.PlanDigest {
		return errors.New("migration plan digest mismatch")
	}
	return nil
}

func ApplyPlan(ctx context.Context, db *sql.DB, plan Plan, snapshotPath, actor string, now time.Time) (Journal, error) {
	if err := plan.Verify(); err != nil {
		return Journal{}, err
	}
	if actor == "" || snapshotPath == "" {
		return Journal{}, errors.New("migration execution requires actor and snapshot path")
	}
	status, err := StatusOf(ctx, db)
	if err != nil {
		return Journal{}, err
	}
	if status.MigrationSetID != plan.MigrationSetID {
		return Journal{}, errors.New("migration plan is stale or migration set changed")
	}
	if status.CurrentSchema == plan.TargetSchema && len(status.Pending) == 0 {
		journal, journalErr := ReadJournal(ctx, db, plan.PlanDigest)
		if journalErr == nil && journal.State == "committed" {
			return journal, nil
		}
		return Journal{}, errors.New("migration is complete but its committed journal is unavailable")
	}
	firstPending := len(plan.Migrations) - len(status.Pending)
	if firstPending < 0 || firstPending > len(plan.Migrations) {
		return Journal{}, errors.New("migration ledger is inconsistent with plan")
	}
	if firstPending == 0 && status.CurrentSchema != plan.CurrentSchema {
		return Journal{}, errors.New("migration plan current schema is stale")
	}
	if firstPending > 0 && status.CurrentSchema != plan.Migrations[firstPending-1].ToSchema {
		return Journal{}, errors.New("migration schema is inconsistent with ledger")
	}
	for i := firstPending; i < len(plan.Migrations); i++ {
		if status.Pending[i-firstPending] != plan.Migrations[i] {
			return Journal{}, errors.New("migration order or content does not match plan")
		}
	}
	journal, journalErr := ReadJournal(ctx, db, plan.PlanDigest)
	if journalErr != nil {
		journal = Journal{PlanDigest: plan.PlanDigest, State: "prepared", UpdatedAt: now.UTC()}
		if err := writeJournal(ctx, db, journal, plan, actor); err != nil {
			return Journal{}, err
		}
	}
	if journal.SnapshotPath != "" {
		if journal.SnapshotPath != snapshotPath {
			return Journal{}, errors.New("migration snapshot path does not match journal")
		}
		actual, err := fileDigest(snapshotPath)
		if err != nil || actual != journal.SnapshotDigest {
			return Journal{}, errors.New("migration snapshot is unavailable or tampered")
		}
	} else {
		if err := os.MkdirAll(filepath.Dir(snapshotPath), 0700); err != nil {
			return Journal{}, err
		}
		if _, err := os.Stat(snapshotPath); err == nil {
			return Journal{}, errors.New("migration snapshot already exists")
		}
		quoted := strings.ReplaceAll(snapshotPath, "'", "''")
		if _, err := db.ExecContext(ctx, "VACUUM INTO '"+quoted+"'"); err != nil {
			return Journal{}, fmt.Errorf("create migration snapshot: %w", err)
		}
		snapshotDigest, err := fileDigest(snapshotPath)
		if err != nil {
			return Journal{}, err
		}
		journal.State, journal.SnapshotPath, journal.SnapshotDigest, journal.UpdatedAt = "snapshot-complete", snapshotPath, snapshotDigest, now.UTC()
	}
	if err := writeJournal(ctx, db, journal, plan, actor); err != nil {
		return Journal{}, err
	}
	_, bodies, err := migrationDescriptors()
	if err != nil {
		return Journal{}, err
	}
	for _, migration := range plan.Migrations[firstPending:] {
		journal.State, journal.AppliedMigration, journal.UpdatedAt = "applying", migration.Name, time.Now().UTC()
		if err := writeJournal(ctx, db, journal, plan, actor); err != nil {
			return Journal{}, err
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return Journal{}, err
		}
		if _, err := tx.ExecContext(ctx, string(bodies[migration.Name])); err != nil {
			_ = tx.Rollback()
			journal.State, journal.UpdatedAt = "failed-recoverable", time.Now().UTC()
			_ = writeJournal(ctx, db, journal, plan, actor)
			return Journal{}, fmt.Errorf("apply migration %s: %w", migration.Name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO praxis_schema_migrations(name,applied_at) VALUES(?,?)`, migration.Name, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			_ = tx.Rollback()
			return Journal{}, err
		}
		if err := tx.Commit(); err != nil {
			return Journal{}, err
		}
	}
	status, err = StatusOf(ctx, db)
	if err != nil || status.CurrentSchema != plan.TargetSchema || len(status.Pending) != 0 {
		journal.State, journal.UpdatedAt = "failed-recoverable", time.Now().UTC()
		_ = writeJournal(ctx, db, journal, plan, actor)
		if err != nil {
			return Journal{}, err
		}
		return Journal{}, errors.New("migration verification failed")
	}
	journal.State, journal.UpdatedAt = "committed", time.Now().UTC()
	if err := writeJournal(ctx, db, journal, plan, actor); err != nil {
		return Journal{}, err
	}
	return journal, nil
}

func writeJournal(ctx context.Context, db *sql.DB, journal Journal, plan Plan, actor string) error {
	payload, err := json.Marshal(journal)
	if err != nil {
		return err
	}
	commandID := journalPrefix + plan.PlanDigest
	_, err = db.ExecContext(ctx, `INSERT INTO commands(command_id,command_type,command_version,actor_id,actor_kind,scope,correlation_id,payload,status,created_at) VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(command_id) DO UPDATE SET payload=excluded.payload,status=excluded.status,completed_at=CASE WHEN excluded.status='committed' THEN excluded.created_at ELSE commands.completed_at END`, commandID, "schema-migration", "1", actor, "installation-owner", "installation:"+plan.BootstrapDigest, plan.PlanDigest, payload, journal.State, journal.UpdatedAt.Format(time.RFC3339Nano))
	return err
}

func ReadJournal(ctx context.Context, db *sql.DB, planDigest string) (Journal, error) {
	var payload []byte
	err := db.QueryRowContext(ctx, `SELECT payload FROM commands WHERE command_id=?`, journalPrefix+planDigest).Scan(&payload)
	if err != nil {
		return Journal{}, err
	}
	var journal Journal
	if err := json.Unmarshal(payload, &journal); err != nil {
		return Journal{}, err
	}
	if journal.PlanDigest != planDigest {
		return Journal{}, errors.New("migration journal digest mismatch")
	}
	return journal, nil
}

func fileDigest(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	d := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(d[:]), nil
}

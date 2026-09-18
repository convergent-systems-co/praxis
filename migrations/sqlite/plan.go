package sqlite

import (
	"context"
	"crypto/rand"
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
	// Holder and LeaseExpiresAt form the installation-wide migration
	// maintenance lease. Exactly one process may execute a governed
	// migration at a time; the lease is claimed with a single immediate
	// transaction before any snapshot or DDL, is bounded so a crashed holder
	// cannot block forever, and is cleared on commit or recoverable failure.
	Holder         string     `json:"holder,omitempty"`
	LeaseExpiresAt *time.Time `json:"lease_expires_at,omitempty"`
}

// MigrationLeaseDuration bounds how long one process may hold the migration
// maintenance lease without completing. A takeover after expiry only ever
// resumes from the durable ledger, never from another process's memory.
const MigrationLeaseDuration = 10 * time.Minute

var ErrMigrationInProgress = errors.New("governed migration is in progress by another process")

func migrationHolder() string {
	host, _ := os.Hostname()
	var nonce [8]byte
	_, _ = rand.Read(nonce[:])
	return fmt.Sprintf("%s:%d:%s", host, os.Getpid(), hex.EncodeToString(nonce[:]))
}

// MaintenanceLease is the single installation-wide migration maintenance
// record. It is keyed by the installation, not by the plan digest: two
// previews of the same pending migrations carry different plan digests
// (CreatedAt is part of the plan), and both must still be excluded from
// executing at once.
type MaintenanceLease struct {
	BootstrapDigest string     `json:"bootstrap_digest"`
	PlanDigest      string     `json:"plan_digest,omitempty"`
	Holder          string     `json:"holder,omitempty"`
	LeaseExpiresAt  *time.Time `json:"lease_expires_at,omitempty"`
	State           string     `json:"state"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func maintenanceLeaseID(bootstrapDigest string) string {
	return journalPrefix + "maintenance:" + bootstrapDigest
}

// ReadMaintenanceLease returns the installation-wide migration maintenance
// record, or a released zero record when none was ever claimed.
func ReadMaintenanceLease(ctx context.Context, db *sql.DB, bootstrapDigest string) (MaintenanceLease, error) {
	var payload []byte
	err := db.QueryRowContext(ctx, `SELECT payload FROM commands WHERE command_id=?`, maintenanceLeaseID(bootstrapDigest)).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return MaintenanceLease{BootstrapDigest: bootstrapDigest, State: "released"}, nil
	}
	if err != nil {
		return MaintenanceLease{}, err
	}
	var lease MaintenanceLease
	if err := json.Unmarshal(payload, &lease); err != nil {
		return MaintenanceLease{}, err
	}
	return lease, nil
}

func writeMaintenanceLeaseTx(ctx context.Context, db sqlExecer, lease MaintenanceLease, actor string) error {
	payload, err := json.Marshal(lease)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `INSERT INTO commands(command_id,command_type,command_version,actor_id,actor_kind,scope,correlation_id,payload,status,created_at) VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(command_id) DO UPDATE SET payload=excluded.payload,status=excluded.status,created_at=excluded.created_at`, maintenanceLeaseID(lease.BootstrapDigest), "schema-migration-maintenance", "1", actor, "installation-owner", "installation:"+lease.BootstrapDigest, lease.PlanDigest, payload, lease.State, lease.UpdatedAt.Format(time.RFC3339Nano))
	return err
}

// claimMigrationLease claims the installation-wide maintenance lease for
// holder and reads or creates the per-plan journal inside one immediate
// write transaction, so two migrators, even with different plan digests
// for the same pending migrations, cannot both proceed. A committed journal
// is returned as-is; an unexpired lease held by another process fails
// closed with ErrMigrationInProgress before any snapshot or DDL.
func claimMigrationLease(ctx context.Context, db *sql.DB, plan Plan, actor, holder string, expectedApplied int, now time.Time) (Journal, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return Journal{}, err
	}
	defer tx.Rollback()
	// Revalidate the ledger inside the claim transaction: a migrator whose
	// pre-claim status is stale (another holder completed and released the
	// lease meanwhile) must fail closed here rather than re-apply.
	applied := 0
	for _, migration := range plan.Migrations {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM praxis_schema_migrations WHERE name=?`, migration.Name).Scan(&n); err != nil {
			return Journal{}, err
		}
		applied += n
	}
	if applied != expectedApplied {
		return Journal{}, fmt.Errorf("%w: migration ledger changed while claiming the maintenance lease; preview again", ErrMigrationInProgress)
	}
	journal := Journal{PlanDigest: plan.PlanDigest, State: "prepared"}
	var payload []byte
	switch err := tx.QueryRowContext(ctx, `SELECT payload FROM commands WHERE command_id=?`, journalPrefix+plan.PlanDigest).Scan(&payload); {
	case err == nil:
		if err := json.Unmarshal(payload, &journal); err != nil || journal.PlanDigest != plan.PlanDigest {
			return Journal{}, errors.New("migration journal digest mismatch")
		}
		if journal.State == "committed" {
			return journal, nil
		}
	case errors.Is(err, sql.ErrNoRows):
	default:
		return Journal{}, err
	}
	lease := MaintenanceLease{BootstrapDigest: plan.BootstrapDigest, State: "released"}
	switch err := tx.QueryRowContext(ctx, `SELECT payload FROM commands WHERE command_id=?`, maintenanceLeaseID(plan.BootstrapDigest)).Scan(&payload); {
	case err == nil:
		if err := json.Unmarshal(payload, &lease); err != nil {
			return Journal{}, err
		}
		if lease.State == "held" && lease.Holder != holder && lease.LeaseExpiresAt != nil && now.Before(*lease.LeaseExpiresAt) {
			return Journal{}, fmt.Errorf("%w: holder %s executing plan %s until %s", ErrMigrationInProgress, lease.Holder, lease.PlanDigest, lease.LeaseExpiresAt.UTC().Format(time.RFC3339))
		}
	case errors.Is(err, sql.ErrNoRows):
	default:
		return Journal{}, err
	}
	expires := now.UTC().Add(MigrationLeaseDuration)
	lease = MaintenanceLease{BootstrapDigest: plan.BootstrapDigest, PlanDigest: plan.PlanDigest, Holder: holder, LeaseExpiresAt: &expires, State: "held", UpdatedAt: now.UTC()}
	if err := writeMaintenanceLeaseTx(ctx, tx, lease, actor); err != nil {
		return Journal{}, err
	}
	journal.Holder, journal.LeaseExpiresAt, journal.UpdatedAt = holder, &expires, now.UTC()
	if err := writeJournalTx(ctx, tx, journal, plan, actor); err != nil {
		return Journal{}, err
	}
	if err := tx.Commit(); err != nil {
		return Journal{}, err
	}
	return journal, nil
}

// releaseMigrationLease clears the installation-wide maintenance lease and
// the journal's holder fields together with the journal's terminal or
// recoverable state, in one transaction.
func releaseMigrationLease(ctx context.Context, db *sql.DB, journal Journal, plan Plan, actor string, now time.Time) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	journal.Holder, journal.LeaseExpiresAt, journal.UpdatedAt = "", nil, now.UTC()
	if err := writeJournalTx(ctx, tx, journal, plan, actor); err != nil {
		return err
	}
	if err := writeMaintenanceLeaseTx(ctx, tx, MaintenanceLease{BootstrapDigest: plan.BootstrapDigest, PlanDigest: plan.PlanDigest, State: "released", UpdatedAt: now.UTC()}, actor); err != nil {
		return err
	}
	return tx.Commit()
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
	journal, err := claimMigrationLease(ctx, db, plan, actor, migrationHolder(), firstPending, now)
	if err != nil {
		return Journal{}, err
	}
	if journal.State == "committed" {
		return Journal{}, errors.New("migration is complete but the ledger still reports pending migrations")
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
			journal.State = "failed-recoverable"
			_ = releaseMigrationLease(ctx, db, journal, plan, actor, time.Now().UTC())
			return Journal{}, fmt.Errorf("apply migration %s: %w", migration.Name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO praxis_schema_migrations(name,applied_at) VALUES(?,?)`, migration.Name, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			_ = tx.Rollback()
			journal.State = "failed-recoverable"
			_ = releaseMigrationLease(ctx, db, journal, plan, actor, time.Now().UTC())
			return Journal{}, err
		}
		if err := tx.Commit(); err != nil {
			return Journal{}, err
		}
	}
	status, err = StatusOf(ctx, db)
	if err != nil || status.CurrentSchema != plan.TargetSchema || len(status.Pending) != 0 {
		journal.State = "failed-recoverable"
		_ = releaseMigrationLease(ctx, db, journal, plan, actor, time.Now().UTC())
		if err != nil {
			return Journal{}, err
		}
		return Journal{}, errors.New("migration verification failed")
	}
	journal.State = "committed"
	if err := releaseMigrationLease(ctx, db, journal, plan, actor, time.Now().UTC()); err != nil {
		return Journal{}, err
	}
	journal.Holder, journal.LeaseExpiresAt = "", nil
	return journal, nil
}

func writeJournal(ctx context.Context, db *sql.DB, journal Journal, plan Plan, actor string) error {
	return writeJournalTx(ctx, db, journal, plan, actor)
}

type sqlExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func writeJournalTx(ctx context.Context, db sqlExecer, journal Journal, plan Plan, actor string) error {
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

// ReadJournalLatest returns the most recently updated governed migration
// journal, for inspection after the fact.
func ReadJournalLatest(ctx context.Context, db *sql.DB) (Journal, error) {
	var payload []byte
	if err := db.QueryRowContext(ctx, `SELECT payload FROM commands WHERE command_type='schema-migration' ORDER BY created_at DESC LIMIT 1`).Scan(&payload); err != nil {
		return Journal{}, err
	}
	var journal Journal
	if err := json.Unmarshal(payload, &journal); err != nil {
		return Journal{}, err
	}
	return journal, nil
}

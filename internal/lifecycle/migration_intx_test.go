package lifecycle

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
	modernsqlite "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

func openIntxJournal(t *testing.T, installation string) (*Journal, *sql.DB) {
	t.Helper()
	ctx := context.Background()
	db, err := state.OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	journal, err := NewJournal(state.NewSQLiteEventStore(db), installation, contracts.PrincipalRef{ID: "lifecycle", Kind: "system"})
	if err != nil {
		t.Fatal(err)
	}
	return journal, db
}

type observedSQLiteWrite struct {
	table string
	op    int32
}

type sqliteWriteLog struct {
	mu     sync.Mutex
	writes []observedSQLiteWrite
}

func (l *sqliteWriteLog) reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.writes = nil
}

func (l *sqliteWriteLog) snapshot() []observedSQLiteWrite {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]observedSQLiteWrite(nil), l.writes...)
}

func (l *sqliteWriteLog) assertEventsAppendOnly(t *testing.T, wantInserts int) {
	t.Helper()
	observed := l.snapshot()
	eventInserts := 0
	for _, write := range observed {
		if write.table != "events" {
			continue
		}
		switch write.op {
		case sqlite3.SQLITE_INSERT:
			eventInserts++
		case sqlite3.SQLITE_UPDATE, sqlite3.SQLITE_DELETE:
			t.Fatalf("journal issued forbidden write against events table: op=%d log=%+v", write.op, observed)
		}
	}
	if eventInserts != wantInserts {
		t.Fatalf("events write log contains %d inserts, want exactly %d: %+v", eventInserts, wantInserts, observed)
	}
}

func openWriteLoggedLifecycleDB(t *testing.T) (*sql.DB, *sqliteWriteLog, string) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "praxis.db")
	bootstrap, err := state.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := bootstrap.Close(); err != nil {
		t.Fatal(err)
	}

	log := &sqliteWriteLog{}
	var tracedDriver modernsqlite.Driver
	tracedDriver.RegisterConnectionHook(func(conn modernsqlite.ExecQuerierContext, _ string) error {
		hooker, ok := conn.(modernsqlite.HookRegisterer)
		if !ok {
			return errors.New("sqlite connection does not expose write hooks")
		}
		hooker.RegisterPreUpdateHook(func(data modernsqlite.SQLitePreUpdateData) {
			log.mu.Lock()
			log.writes = append(log.writes, observedSQLiteWrite{table: data.TableName, op: data.Op})
			log.mu.Unlock()
		})
		return nil
	})
	driverName := "sqlite_write_log_" + t.Name()
	sql.Register(driverName, &tracedDriver)
	u := &url.URL{Scheme: "file", Path: path}
	q := u.Query()
	q.Set("_foreign_keys", "1")
	q.Set("_busy_timeout", "5000")
	q.Set("_journal_mode", "WAL")
	q.Set("_synchronous", "FULL")
	q.Set("_txlock", "immediate")
	q.Add("_pragma", "secure_delete(1)")
	u.RawQuery = q.Encode()
	db, err := sql.Open(driverName, u.String())
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	t.Cleanup(func() { db.Close() })
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	return db, log, path
}

func TestJournalAppendInTxRequiresTxCapableStore(t *testing.T) {
	journal, err := NewJournal(eventstore.NewMemoryStore(), "installation", contracts.PrincipalRef{ID: "lifecycle", Kind: "system"})
	if err != nil {
		t.Fatal(err)
	}
	if err := journal.AppendInTx(context.Background(), nil, contracts.LifecycleTransitionJournal{}); err == nil {
		t.Fatal("expected error for a store without tx-scoped append support")
	}
	if _, err := journal.LoadInTx(context.Background(), nil); err == nil {
		t.Fatal("expected error for a store without tx-scoped read support")
	}
}

func TestJournalAppendInTxCommitsOnlyWithCallerTransaction(t *testing.T) {
	journal, db := openIntxJournal(t, "installation")
	plan := migrationPlan(t, true)
	step := plan.Steps[0]
	req := RunRequest{Plan: plan, StepID: "migrate", PreconditionDigest: migrationDigest("e"), SnapshotDigest: migrationDigest("f"), Now: time.Now().UTC()}
	entry := journalEntry(req, step, 1, contracts.LifecyclePlanned, "", "", nil)

	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := journal.AppendInTx(context.Background(), tx, entry); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	history, err := journal.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 0 {
		t.Fatalf("rolled-back tx-scoped append must not be durable: %+v", history)
	}

	tx2, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := journal.AppendInTx(context.Background(), tx2, entry); err != nil {
		t.Fatal(err)
	}
	if err := tx2.Commit(); err != nil {
		t.Fatal(err)
	}
	history2, err := journal.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(history2) != 1 || history2[0].State != contracts.LifecyclePlanned {
		t.Fatalf("committed tx-scoped append must be durable: %+v", history2)
	}
}

func TestJournalRefusesInvalidExactStepTransitionBeforePersistence(t *testing.T) {
	journal, db := openIntxJournal(t, "installation")
	plan := migrationPlan(t, true)
	step := plan.Steps[0]
	req := RunRequest{Plan: plan, StepID: step.ID, PreconditionDigest: migrationDigest("e"), SnapshotDigest: migrationDigest("f"), Now: time.Now().UTC()}
	if err := journal.Append(context.Background(), journalEntry(req, step, 1, contracts.LifecyclePlanned, "", "", nil)); err != nil {
		t.Fatal(err)
	}
	// Approved->Prepared is locally legal, but this exact durable step is
	// only Planned. The journal must derive PreviousState from history rather
	// than trusting the self-consistent entry.
	invalid := journalEntry(req, step, 2, contracts.LifecyclePrepared, contracts.LifecycleApproved, "", nil)
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := journal.AppendInTx(context.Background(), tx, invalid); err == nil {
		t.Fatal("journal accepted a transition whose previous state did not match the exact plan step")
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	history, err := journal.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].State != contracts.LifecyclePlanned {
		t.Fatalf("invalid transition changed durable history: %+v", history)
	}
	outOfSequence := journalEntry(req, step, 3, contracts.LifecycleApproved, contracts.LifecyclePlanned, "", nil)
	tx2, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := journal.AppendInTx(context.Background(), tx2, outOfSequence); err == nil {
		t.Fatal("journal accepted an out-of-sequence transactional append")
	}
	_ = tx2.Rollback()
}

// atomicOutcomeDriver simulates a WU5/WU7-style StepDriver: its Apply
// opens its own SQL transaction, computes the actual result, appends the
// outcome journal entry itself via Journal.AppendInTx inside that same
// transaction, and commits once — exactly the atomic Apply boundary
// ADR-088 §13.3 requires. It reports ApplyResult.OutcomeAlreadyRecorded so
// RunStep must not append a second outcome entry.
type atomicOutcomeDriver struct {
	db         *sql.DB
	journal    *Journal
	plan       contracts.LifecyclePlan
	mismatch   bool
	err        error
	idempotent bool
	calls      int
}

func (d *atomicOutcomeDriver) Preflight(context.Context, contracts.LifecycleTransitionStep) error {
	return nil
}
func (d *atomicOutcomeDriver) Idempotent(contracts.LifecycleTransitionStep) bool { return d.idempotent }

func (d *atomicOutcomeDriver) Apply(ctx context.Context, step contracts.LifecycleTransitionStep) (ApplyResult, error) {
	d.calls++
	if d.err != nil {
		return ApplyResult{}, d.err
	}
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return ApplyResult{}, err
	}
	defer tx.Rollback()

	history, err := d.journal.LoadInTx(ctx, tx)
	if err != nil {
		return ApplyResult{}, err
	}
	var previous contracts.LifecycleTransitionState
	if len(history) > 0 {
		previous = history[len(history)-1].State
	}
	sequence := len(history) + 1

	// The driver computes its actual observed result and compares it to
	// the plan's declared target itself, before choosing which outcome to
	// commit — this is what makes the mismatch check happen pre-commit,
	// never post hoc.
	resultingDigest := step.Target.Digest
	if d.mismatch {
		resultingDigest = migrationDigest("z")
	}
	outcome, recovery := ApplyCommitted, ""
	if resultingDigest != step.Target.Digest {
		outcome, recovery = ApplyReconcileRequired, "resulting-manifest-mismatch"
	}

	entry := contracts.LifecycleTransitionJournal{
		JournalID:          "atomic:" + step.ID + ":" + strconv.Itoa(sequence),
		Version:            "1",
		PlanID:             d.plan.PlanID,
		PlanDigest:         d.plan.Digest,
		InstallationID:     d.plan.InstallationID,
		Sequence:           sequence,
		StepID:             step.ID,
		PreviousState:      previous,
		State:              lifecycleStateFor(outcome),
		RecoveryAction:     recovery,
		PreconditionDigest: migrationDigest("e"),
		SnapshotDigest:     migrationDigest("f"),
		ReadinessDigest:    d.plan.TargetManifestDigest,
		RecordedAt:         time.Now().UTC(),
	}
	if err := d.journal.AppendInTx(ctx, tx, entry); err != nil {
		return ApplyResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return ApplyResult{}, err
	}
	return ApplyResult{Outcome: outcome, RecoveryAction: recovery, OutcomeAlreadyRecorded: true}, nil
}

func TestRunStepDoesNotAppendDuplicateOutcomeAfterAtomicCommit(t *testing.T) {
	journal, db := openIntxJournal(t, "installation")
	plan := migrationPlan(t, true)
	driver := &atomicOutcomeDriver{db: db, journal: journal, plan: plan, idempotent: true}
	req := RunRequest{Plan: plan, StepID: "migrate", PreconditionDigest: migrationDigest("e"), SnapshotDigest: migrationDigest("f"), Now: time.Now().UTC()}

	if err := RunStep(context.Background(), journal, req, driver); err != nil {
		t.Fatal(err)
	}
	history, err := journal.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 5 {
		t.Fatalf("expected exactly 5 entries (planned/approved/prepared/applying/committed), got %d: %+v", len(history), history)
	}
	last := history[len(history)-1]
	if last.State != contracts.LifecycleCommitted || last.PreviousState != contracts.LifecycleApplying {
		t.Fatalf("unexpected terminal transition: %+v", last)
	}
	if driver.calls != 1 {
		t.Fatalf("expected exactly one Apply call, got %d", driver.calls)
	}

	// A subsequent RunStep call for an already-committed step must not
	// call Apply again and must not touch the journal further.
	if err := RunStep(context.Background(), journal, req, driver); err != nil {
		t.Fatal(err)
	}
	if driver.calls != 1 {
		t.Fatalf("committed step was applied twice: %d calls", driver.calls)
	}
}

func TestRunStepDoesNotAppendDuplicateOutcomeOnMismatch(t *testing.T) {
	journal, db := openIntxJournal(t, "installation")
	plan := migrationPlan(t, true)
	driver := &atomicOutcomeDriver{db: db, journal: journal, plan: plan, mismatch: true, idempotent: true}
	req := RunRequest{Plan: plan, StepID: "migrate", PreconditionDigest: migrationDigest("e"), SnapshotDigest: migrationDigest("f"), Now: time.Now().UTC()}

	if err := RunStep(context.Background(), journal, req, driver); err != nil {
		t.Fatal(err)
	}
	history, err := journal.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	assertReconcileEntriesValidate(t, history)
	last := history[len(history)-1]
	// The mismatch is caught and routed to ReconcileRequired entirely
	// inside the driver's own pre-commit path — RunStep never sees a
	// Committed outcome to re-check post hoc, and never appends a second
	// entry for it.
	if last.State != contracts.LifecycleReconcileRequired || last.RecoveryAction != "resulting-manifest-mismatch" {
		t.Fatalf("unexpected mismatch routing: %+v", last)
	}
	if len(history) != 5 {
		t.Fatalf("expected exactly 5 entries, got %d: %+v", len(history), history)
	}
}

type contradictoryDriver struct{}

func (d *contradictoryDriver) Preflight(context.Context, contracts.LifecycleTransitionStep) error {
	return nil
}
func (d *contradictoryDriver) Idempotent(contracts.LifecycleTransitionStep) bool { return true }
func (d *contradictoryDriver) Apply(context.Context, contracts.LifecycleTransitionStep) (ApplyResult, error) {
	return ApplyResult{Outcome: ApplyCommitted, OutcomeAlreadyRecorded: true}, errors.New("boom")
}

func TestRunStepRejectsErrorAlongsideAlreadyRecordedOutcome(t *testing.T) {
	journal, _ := openIntxJournal(t, "installation")
	plan := migrationPlan(t, true)
	req := RunRequest{Plan: plan, StepID: "migrate", PreconditionDigest: migrationDigest("e"), SnapshotDigest: migrationDigest("f"), Now: time.Now().UTC()}
	if err := RunStep(context.Background(), journal, req, &contradictoryDriver{}); err == nil {
		t.Fatal("expected RunStep to reject a driver reporting an error alongside OutcomeAlreadyRecorded")
	}
}

func TestStorageSchemaDriverJournalIsAppendOnlyUnderConcurrentWriterContention(t *testing.T) {
	ctx := context.Background()
	db, writeLog, path := openWriteLoggedLifecycleDB(t)
	contenderDB, err := state.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer contenderDB.Close()

	if err := revertInvocationRuntimeBindingsToHistorical(ctx, db); err != nil {
		t.Fatal(err)
	}
	seedInvocationRuntimeBindingsFixture(t, db, []bindingRowFixture{{
		entryPointID: "goal", packageID: "pkg", packageVersion: "1",
		contentDigest: "sha256:" + repeatHex("a"), active: true,
		executableBinding: []byte(`{"kind":"client-adapter"}`),
	}})

	target := canonicalDigest(t)
	step := storageSchemaStep(t, target)
	plan := storageSchemaPlan(t, step)
	store := state.NewSQLiteEventStore(db)
	contenderStore := state.NewSQLiteEventStore(contenderDB)
	journal, err := NewJournal(store, plan.InstallationID, contracts.PrincipalRef{ID: "lifecycle", Kind: "system"})
	if err != nil {
		t.Fatal(err)
	}

	started := make(chan struct{})
	contentionDone := make(chan error, 1)
	var startContention sync.Once
	driver := &StorageSchemaDriver{
		DB: db, Journal: journal, Guard: &allowingGuard{},
		applyCheckpoint: func(stage string) error {
			if stage != "after-predicate-revalidation" {
				return nil
			}
			startContention.Do(func() {
				go func() {
					close(started)
					_, appendErr := contenderStore.Append(ctx, lifecycleAggregate(plan.InstallationID), 4, []eventstore.Event{{
						ID: "contention-event", AggregateType: lifecycleJournalAggregateType,
						Type: "lifecycle.transition", Version: "v1",
						Actor:     contracts.PrincipalRef{ID: "contender", Kind: "system"},
						CommandID: "contention-command", CorrelationID: plan.PlanID,
						Trust: contracts.TrustPolicy, Payload: []byte(`{}`), CreatedAt: time.Now().UTC(),
					}})
					contentionDone <- appendErr
				}()
			})
			<-started
			select {
			case err := <-contentionDone:
				return fmt.Errorf("contending writer completed before holder transaction released its lock: %w", err)
			case <-time.After(50 * time.Millisecond):
				// The independent connection is actively blocked on this
				// transaction's writer lock. Returning lets the holder commit.
			}
			return nil
		},
	}
	preparedStorageDriver(t, db, driver)

	writeLog.reset()
	req := storageSchemaRequest(t, plan, step, driver)
	if err := RunStep(ctx, journal, req, driver); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-contentionDone:
		if !errors.Is(err, eventstore.ErrVersionConflict) {
			t.Fatalf("conflicting writer error = %v, want version conflict", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("conflicting writer neither completed nor failed cleanly")
	}

	history, err := journal.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 5 || history[4].Sequence != 5 || history[4].State != contracts.LifecycleCommitted {
		t.Fatalf("contention corrupted journal sequence: %+v", history)
	}
	writeLog.assertEventsAppendOnly(t, 5)
}

func TestStorageSchemaAlreadyCanonicalPathIssuesZeroDomainWrites(t *testing.T) {
	ctx := context.Background()
	db, writeLog, _ := openWriteLoggedLifecycleDB(t)
	canonical, err := CanonicalInvocationRuntimeBindingsShape().Digest()
	if err != nil {
		t.Fatal(err)
	}
	step := storageSchemaStep(t, canonical)
	step.Current.Digest = canonical
	plan := storageSchemaPlan(t, step)
	journal, err := NewJournal(state.NewSQLiteEventStore(db), plan.InstallationID, contracts.PrincipalRef{ID: "lifecycle", Kind: "system"})
	if err != nil {
		t.Fatal(err)
	}
	driver := preparedStorageDriver(t, db, &StorageSchemaDriver{DB: db, Journal: journal, Guard: &allowingGuard{}})
	writeLog.reset()
	if err := RunStep(ctx, journal, storageSchemaRequest(t, plan, step, driver), driver); err != nil {
		t.Fatal(err)
	}
	for _, write := range writeLog.snapshot() {
		if write.table == InvocationRuntimeBindingsTable {
			t.Fatalf("already-canonical Apply issued domain mutating SQL: %+v", writeLog.snapshot())
		}
	}
	writeLog.assertEventsAppendOnly(t, 5)
}

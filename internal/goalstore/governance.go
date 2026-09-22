package goalstore

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/convergent-systems-co/praxis/internal/faa"
	"github.com/convergent-systems-co/praxis/internal/state"
)

// Forward Authority Anchor integration (I13 temporal authority, I14
// subject-governance continuity).
//
// The mutable governance store is a rollback domain: anyone who can write the
// SQLite file can restore an earlier state of it, and every predicate that reads
// only that file evaluates the restored state exactly as it did then. Repository
// therefore keeps a chain of governance FACTS in the store (retirements,
// classification, re-anchoring: the transitions whose loss or replay would
// broaden permission) and a forward-only anchor OUTSIDE it holding the length
// and head of that chain. Governance state is consumable only when the verified
// chain ends exactly at the anchor. A store that is behind (rollback,
// truncation, erasure), ahead (anchor reset, interrupted write), unrelated
// (fork, substitution) or whose anchor is missing, unreadable or unavailable is
// refused; nothing is silently recreated.

// ErrGovernanceNotFresh is the base refusal: the governance store cannot be
// shown to be current against the forward authority anchor. Every consumer that
// depends on current governance state refuses on it.
var ErrGovernanceNotFresh = errors.New("governance state is not consumable: it is not current against the forward authority anchor")

var (
	ErrGovernanceAnchorMissing     = fmt.Errorf("%w: the anchor has no state for this installation (an installation that predates the anchor, or a lost anchor, needs a governed re-anchor)", ErrGovernanceNotFresh)
	ErrGovernanceAnchorUnavailable = fmt.Errorf("%w: the anchor is unavailable, unreadable or corrupt", ErrGovernanceNotFresh)
	ErrGovernanceRolledBack        = fmt.Errorf("%w: the store is BEHIND the anchor (rollback, truncation or erasure of governance facts)", ErrGovernanceNotFresh)
	ErrGovernanceAhead             = fmt.Errorf("%w: the store is AHEAD of the anchor (anchor reset, or an interrupted write)", ErrGovernanceNotFresh)
	ErrGovernanceUnrelated         = fmt.Errorf("%w: the store's fact chain is not the anchor's chain (fork or substitution)", ErrGovernanceNotFresh)
	ErrGovernanceChainInvalid      = fmt.Errorf("%w: the store's fact chain is not a valid authenticated chain", ErrGovernanceNotFresh)
)

// governanceOrphanNamespace holds fact rows a governed re-anchor found beyond the
// last valid link; they are preserved for inspection and never consulted.
const governanceOrphanNamespace = "governance_fact_orphan"

// ErrAuthorityRetired marks a decision or generation that a fact retires. It is
// also ErrAuthorityDecisionRevoked so every caller that refuses a revoked
// decision refuses it.
var ErrAuthorityRetired = fmt.Errorf("authority is retired by an anchored governance fact: %w", ErrAuthorityDecisionRevoked)

// governanceSnapshot is one verified observation of store and anchor.
type governanceSnapshot struct {
	Anchored bool
	Anchor   faa.State
	View     faa.View
}

func (r Repository) anchorEnabled() bool { return r.FAA != nil }

func (r Repository) openFacts(ctx context.Context, records []state.SecureBlobRecord) ([]faa.Fact, error) {
	facts := make([]faa.Fact, 0, len(records))
	for _, record := range records {
		payload, err := r.Crypto.Open(ctx, record.Envelope, state.SecureBlobAAD(record.Namespace, record.ObjectID, record.ObjectVersion, record.ObjectDigest))
		if err != nil {
			return nil, fmt.Errorf("%w: fact %s is not authentic: %v", ErrGovernanceChainInvalid, record.ObjectID, err)
		}
		if payloadDigest(payload) != record.ObjectDigest {
			return nil, fmt.Errorf("%w: fact %s digest mismatch", ErrGovernanceChainInvalid, record.ObjectID)
		}
		var fact faa.Fact
		if err := json.Unmarshal(payload, &fact); err != nil {
			return nil, fmt.Errorf("%w: fact %s is malformed: %v", ErrGovernanceChainInvalid, record.ObjectID, err)
		}
		if record.ObjectID != state.GovernanceFactID(fact.Seq) || record.ObjectVersion != "1" {
			return nil, fmt.Errorf("%w: fact row %s does not carry the fact it names", ErrGovernanceChainInvalid, record.ObjectID)
		}
		facts = append(facts, fact)
	}
	return facts, nil
}

func (r Repository) classifyAnchorError(err error) error {
	switch {
	case errors.Is(err, faa.ErrMissing):
		return ErrGovernanceAnchorMissing
	default:
		return fmt.Errorf("%w: %v", ErrGovernanceAnchorUnavailable, err)
	}
}

func (r Repository) relate(view faa.View, anchor faa.State) error {
	switch faa.Compare(view, anchor) {
	case faa.Consistent:
		return nil
	case faa.Behind:
		return fmt.Errorf("%w (store sequence %d, anchor sequence %d)", ErrGovernanceRolledBack, view.Seq, anchor.Seq)
	case faa.Ahead:
		return fmt.Errorf("%w (store sequence %d, anchor sequence %d)", ErrGovernanceAhead, view.Seq, anchor.Seq)
	default:
		return fmt.Errorf("%w (sequence %d)", ErrGovernanceUnrelated, view.Seq)
	}
}

// governanceSnapshotOnce reads the store and THEN the anchor, once. The order is
// load-bearing: a writer advances the anchor before it commits, so at every
// instant the anchor is at or past the last committed fact. Reading the store
// first therefore can never observe the store AHEAD of the anchor because of a
// concurrent write (the reverse order can, and produced spurious refusals under
// concurrent appenders); the only transient disagreement left is the anchor one
// write ahead of a store whose writer has not committed yet, which the caller
// waits out.
func (r Repository) governanceSnapshotOnce(ctx context.Context) (governanceSnapshot, error) {
	if r.InstallationDigest == "" {
		return governanceSnapshot{}, fmt.Errorf("%w: the installation identity is required", ErrGovernanceAnchorUnavailable)
	}
	records, err := r.Store.ListGovernanceFactRecords(ctx)
	if err != nil {
		return governanceSnapshot{}, fmt.Errorf("%w: %v", ErrGovernanceChainInvalid, err)
	}
	anchor, err := r.FAA.Load(ctx, r.InstallationDigest)
	if err != nil {
		return governanceSnapshot{}, r.classifyAnchorError(err)
	}
	if anchor.Installation != r.InstallationDigest {
		return governanceSnapshot{}, fmt.Errorf("%w: the anchor names another installation", ErrGovernanceAnchorUnavailable)
	}
	facts, err := r.openFacts(ctx, records)
	if err != nil {
		return governanceSnapshot{}, err
	}
	view, err := faa.VerifyChain(r.InstallationDigest, facts)
	if err != nil {
		return governanceSnapshot{}, fmt.Errorf("%w: %v", ErrGovernanceChainInvalid, err)
	}
	if err := r.relate(view, anchor); err != nil {
		return governanceSnapshot{Anchored: true, Anchor: anchor, View: view}, err
	}
	return governanceSnapshot{Anchored: true, Anchor: anchor, View: view}, nil
}

// governanceSnapshot verifies the store against the anchor. A writer advances
// the anchor inside its transaction and before the commit, so while a write is in
// flight a reader can see the anchor one transition ahead of the store. A refusal
// that is a plain lag (behind or ahead) is therefore not trusted until it has been
// re-observed BEHIND THE WRITER LOCK: inside a write transaction no write is in
// flight, so what remains is a real disagreement (a rollback, a truncation, an
// interrupted write, an anchor reset) and is refused at once. A reader that cannot
// take the lock (a read-only connection) falls back to waiting out the lag with
// backoff. An invalid chain, an unrelated head or an unreadable anchor is refused
// without any of this.
func (r Repository) governanceSnapshot(ctx context.Context) (governanceSnapshot, error) {
	if !r.anchorEnabled() {
		return governanceSnapshot{}, nil
	}
	snap, err := r.governanceSnapshotOnce(ctx)
	if err == nil || !(errors.Is(err, ErrGovernanceRolledBack) || errors.Is(err, ErrGovernanceAhead)) {
		return snap, err
	}
	var barrierSnap governanceSnapshot
	barrierErr := r.Store.WithWriteLock(ctx, func(tx *sql.Tx) error {
		var innerErr error
		barrierSnap, innerErr = r.governanceSnapshotTx(ctx, tx)
		return innerErr
	})
	if barrierErr == nil {
		return barrierSnap, nil
	}
	if errors.Is(barrierErr, ErrGovernanceNotFresh) {
		return governanceSnapshot{}, barrierErr
	}
	// The lock could not be taken (read-only connection): wait out the lag.
	delay, attempts := 5*time.Millisecond, 5
	if r.faaLagAttempts > 0 {
		attempts = r.faaLagAttempts
	}
	if r.faaLagDelay > 0 {
		delay = r.faaLagDelay
	}
	for attempt := 0; attempt < attempts; attempt++ {
		select {
		case <-ctx.Done():
			return snap, err
		case <-time.After(delay):
		}
		snap, err = r.governanceSnapshotOnce(ctx)
		if err == nil || !(errors.Is(err, ErrGovernanceRolledBack) || errors.Is(err, ErrGovernanceAhead)) {
			return snap, err
		}
		delay *= 2
	}
	return snap, err
}

// admissionStamp is the anchor sequence stamped on a new decision or generation
// admission. It also refuses the admission when the store is not current.
func (r Repository) admissionStamp(ctx context.Context) (uint64, error) {
	if !r.anchorEnabled() {
		return 0, nil
	}
	snap, err := r.governanceSnapshot(ctx)
	if err != nil {
		return 0, err
	}
	return snap.Anchor.Seq, nil
}

// appendGovernanceFact records one transition in the chain and advances the
// anchor inside the write lock, before the commit. done reports that the
// transition is already recorded (idempotence). Unanchored repositories record
// nothing.
func (r Repository) appendGovernanceFact(ctx context.Context, now time.Time, done func(faa.View) bool, fill func(*faa.Fact)) error {
	if !r.anchorEnabled() {
		return nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return r.Store.AppendGovernanceFact(ctx, func(ctx context.Context, tx *sql.Tx, existing []state.SecureBlobRecord) (state.SecureBlobRecord, func(context.Context), error) {
		anchor, err := r.FAA.Load(ctx, r.InstallationDigest)
		if err != nil {
			return state.SecureBlobRecord{}, nil, r.classifyAnchorError(err)
		}
		facts, err := r.openFacts(ctx, existing)
		if err != nil {
			return state.SecureBlobRecord{}, nil, err
		}
		view, err := faa.VerifyChain(r.InstallationDigest, facts)
		if err != nil {
			return state.SecureBlobRecord{}, nil, fmt.Errorf("%w: %v", ErrGovernanceChainInvalid, err)
		}
		if err := r.relate(view, anchor); err != nil {
			return state.SecureBlobRecord{}, nil, err
		}
		if done != nil && done(view) {
			return state.SecureBlobRecord{}, nil, nil
		}
		fact := faa.Fact{Seq: view.Seq + 1, Installation: r.InstallationDigest, Prev: view.Head, At: now.UTC()}
		fill(&fact)
		fact, err = faa.Seal(fact)
		if err != nil {
			return state.SecureBlobRecord{}, nil, err
		}
		payload, err := json.Marshal(fact)
		if err != nil {
			return state.SecureBlobRecord{}, nil, err
		}
		record, err := r.workPlanSecureRecord(ctx, state.GovernanceFactNamespace, state.GovernanceFactID(fact.Seq), "1", payload, now, nil)
		if err != nil {
			return state.SecureBlobRecord{}, nil, err
		}
		next := faa.State{Installation: r.InstallationDigest, Seq: fact.Seq, Head: fact.Head}
		if err := r.FAA.Set(ctx, r.InstallationDigest, &anchor, next); err != nil {
			return state.SecureBlobRecord{}, nil, r.classifyAnchorError(err)
		}
		undo := func(ctx context.Context) { _ = r.FAA.Revert(ctx, r.InstallationDigest, next, anchor) }
		return record, undo, nil
	})
}

// anchorDecisionRetired records, before any effect, that a decision is retired.
func (r Repository) anchorDecisionRetired(ctx context.Context, requestID, requestVersion, decisionDigest, reason string, now time.Time) error {
	key := faa.DecisionKey(requestID, requestVersion, decisionDigest)
	return r.appendGovernanceFact(ctx, now, func(v faa.View) bool { _, ok := v.RetiredDecisions[key]; return ok }, func(f *faa.Fact) {
		f.Kind, f.RequestID, f.RequestVersion, f.DecisionDigest, f.Reason = faa.KindDecisionRetired, requestID, requestVersion, decisionDigest, reason
	})
}

// anchorGenerationRetired records, before any effect, that a generation is
// retired (invalidated, revoked or superseded).
func (r Repository) anchorGenerationRetired(ctx context.Context, ref, version, digest, reason string, now time.Time) error {
	key := faa.GenerationKey(ref, version, digest)
	return r.appendGovernanceFact(ctx, now, func(v faa.View) bool { _, ok := v.RetiredGenerations[key]; return ok }, func(f *faa.Fact) {
		f.Kind, f.GenerationRef, f.GenerationVersion, f.GenerationDigest, f.Reason = faa.KindGenerationRetired, ref, version, digest, reason
	})
}

// anchorGoalClassified records, before the classification row, that a Goal has
// entered the governed safety domain.
func (r Repository) anchorGoalClassified(ctx context.Context, goalID, kernelVersion string, now time.Time) error {
	return r.appendGovernanceFact(ctx, now, func(v faa.View) bool { _, ok := v.Classified[goalID]; return ok }, func(f *faa.Fact) {
		f.Kind, f.GoalID, f.KernelVersion = faa.KindGoalClassified, goalID, kernelVersion
	})
}

// InitializeGovernanceAnchor creates the chain and the anchor for a fresh
// installation: a random-nonce genesis fact is committed to the store and its
// head is stored in the anchor, together, under the store's write lock. It
// refuses to do so when the store already holds any governance state, because an
// anchor that appears next to an existing store is indistinguishable from an
// erased one; that case is a governed re-anchor. An anchor left at sequence 0 by
// a crash before the store committed is replaced, since nothing yet depends on it.
func (r Repository) InitializeGovernanceAnchor(ctx context.Context) error {
	if !r.anchorEnabled() {
		return nil
	}
	if r.InstallationDigest == "" {
		return errors.New("the installation identity is required to initialize the forward authority anchor")
	}
	current, anchorErr := r.FAA.Load(ctx, r.InstallationDigest)
	replace := false
	switch {
	case anchorErr == nil:
		if current.Installation != r.InstallationDigest {
			return ErrGovernanceAnchorUnavailable
		}
		if _, err := r.governanceSnapshotOnce(ctx); err == nil {
			return nil
		}
		if current.Seq != 0 {
			return ErrGovernanceRolledBack
		}
		replace = true
	case errors.Is(anchorErr, faa.ErrMissing):
	default:
		return r.classifyAnchorError(anchorErr)
	}
	facts, err := r.Store.ListGovernanceFactRecords(ctx)
	if err != nil {
		return err
	}
	generations, _, _, err := r.listAuthorityGenerationsSnapshot(ctx, time.Now().UTC())
	if err != nil {
		return err
	}
	if len(facts) != 0 || len(generations) != 0 {
		if replace {
			return ErrGovernanceRolledBack
		}
		return ErrGovernanceAnchorMissing
	}
	nonceBytes := make([]byte, 32)
	if _, err := rand.Read(nonceBytes); err != nil {
		return err
	}
	now := time.Now().UTC()
	return r.Store.AppendGovernanceFact(ctx, func(ctx context.Context, tx *sql.Tx, existing []state.SecureBlobRecord) (state.SecureBlobRecord, func(context.Context), error) {
		if len(existing) != 0 {
			return state.SecureBlobRecord{}, nil, ErrGovernanceAnchorMissing
		}
		fact, next, err := faa.NewGenesis(r.InstallationDigest, hex.EncodeToString(nonceBytes), now)
		if err != nil {
			return state.SecureBlobRecord{}, nil, err
		}
		payload, err := json.Marshal(fact)
		if err != nil {
			return state.SecureBlobRecord{}, nil, err
		}
		record, err := r.workPlanSecureRecord(ctx, state.GovernanceFactNamespace, state.GovernanceFactID(0), "1", payload, now, nil)
		if err != nil {
			return state.SecureBlobRecord{}, nil, err
		}
		if replace {
			err = r.FAA.Reset(ctx, r.InstallationDigest, next)
		} else {
			err = r.FAA.Set(ctx, r.InstallationDigest, nil, next)
		}
		if err != nil {
			return state.SecureBlobRecord{}, nil, r.classifyAnchorError(err)
		}
		return record, nil, nil
	})
}

// GovernanceStatus is the inspectable relation of store to anchor. It is
// available even when the store is not consumable.
type GovernanceStatus struct {
	Anchored        bool
	Relation        string
	AnchorSeq       uint64
	AnchorHead      string
	StoreSeq        uint64
	StoreHead       string
	StoreChainError string
	AnchorError     string
	Reanchors       []faa.Fact
	OrphanFacts     int
}

func (r Repository) GovernanceStatus(ctx context.Context) (GovernanceStatus, error) {
	status := GovernanceStatus{Anchored: r.anchorEnabled()}
	if !status.Anchored {
		status.Relation = "unanchored"
		return status, nil
	}
	anchor, anchorErr := r.FAA.Load(ctx, r.InstallationDigest)
	if anchorErr != nil {
		status.AnchorError = anchorErr.Error()
	} else {
		status.AnchorSeq, status.AnchorHead = anchor.Seq, anchor.Head
	}
	records, err := r.Store.ListGovernanceFactRecords(ctx)
	if err != nil {
		return status, err
	}
	facts, openErr := r.openFacts(ctx, records)
	if openErr != nil {
		status.StoreChainError = openErr.Error()
		status.Relation = "invalid"
		return status, nil
	}
	view, chainErr := faa.VerifyChain(r.InstallationDigest, facts)
	if chainErr != nil {
		status.StoreChainError = chainErr.Error()
		status.Relation = "invalid"
		return status, nil
	}
	status.StoreSeq, status.StoreHead, status.Reanchors = view.Seq, view.Head, view.Reanchors
	orphans, err := r.Store.ListSecureBlobs(ctx, governanceOrphanNamespace, time.Unix(0, 0).UTC())
	if err == nil {
		status.OrphanFacts = len(orphans)
	}
	switch {
	case anchorErr != nil && errors.Is(anchorErr, faa.ErrMissing):
		status.Relation = "missing"
	case anchorErr != nil:
		status.Relation = "unreadable"
	default:
		status.Relation = string(faa.Compare(view, anchor))
	}
	return status, nil
}

// checkGovernedCurrent is the shared currentness predicate for a decision or a
// generation whose liveness record carries the given admission stamp.
func (s governanceSnapshot) admissionVoid(stamp uint64) bool {
	return s.Anchored && stamp < s.View.ReanchorSeq
}

func retiredKind(namespace string) string {
	if namespace == state.AuthorityDecisionLiveNamespace {
		return "decision"
	}
	return "generation"
}

func (s governanceSnapshot) retired(namespace, id, version, digest string) bool {
	if !s.Anchored {
		return false
	}
	if namespace == state.AuthorityDecisionLiveNamespace {
		_, ok := s.View.RetiredDecisions[faa.DecisionKey(id, version, digest)]
		return ok
	}
	_, ok := s.View.RetiredGenerations[faa.GenerationKey(id, version, digest)]
	return ok
}

func mustNotBeBlank(values ...string) error {
	for _, v := range values {
		if strings.TrimSpace(v) == "" {
			return errors.New("re-anchor input is incomplete")
		}
	}
	return nil
}

// governanceSnapshotTx is governanceSnapshot for a check made inside a
// caller-owned transaction (durable admission of an effect): it reads the same
// chain the surrounding write sees. It does not retry; inside a transaction a
// disagreement is a refusal.
func (r Repository) governanceSnapshotTx(ctx context.Context, tx *sql.Tx) (governanceSnapshot, error) {
	if !r.anchorEnabled() {
		return governanceSnapshot{}, nil
	}
	anchor, err := r.FAA.Load(ctx, r.InstallationDigest)
	if err != nil {
		return governanceSnapshot{}, r.classifyAnchorError(err)
	}
	records, err := state.ListGovernanceFactRecordsTx(ctx, tx)
	if err != nil {
		return governanceSnapshot{}, fmt.Errorf("%w: %v", ErrGovernanceChainInvalid, err)
	}
	facts, err := r.openFacts(ctx, records)
	if err != nil {
		return governanceSnapshot{}, err
	}
	view, err := faa.VerifyChain(r.InstallationDigest, facts)
	if err != nil {
		return governanceSnapshot{}, fmt.Errorf("%w: %v", ErrGovernanceChainInvalid, err)
	}
	if err := r.relate(view, anchor); err != nil {
		return governanceSnapshot{}, err
	}
	return governanceSnapshot{Anchored: true, Anchor: anchor, View: view}, nil
}

// requireLiveInTx is requireLiveIdentity inside a transaction, against a
// snapshot taken in the same transaction.
func (r Repository) requireLiveInTx(ctx context.Context, tx *sql.Tx, snap governanceSnapshot, namespace, id, version string) error {
	record, err := r.Store.GetSecureBlobInTx(ctx, tx, namespace, id, version, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("%w: %s %s/%s: %v", ErrLivenessMissing, namespace, id, version, err)
	}
	payload, err := r.Crypto.Open(ctx, record.Envelope, state.SecureBlobAAD(record.Namespace, record.ObjectID, record.ObjectVersion, record.ObjectDigest))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrLivenessMissing, err)
	}
	var stored livenessRecord
	if err := json.Unmarshal(payload, &stored); err != nil || stored.Namespace != namespace || stored.ID != id || stored.Version != version || stored.Digest == "" {
		return fmt.Errorf("%w: %s %s/%s names another identity", ErrLivenessMissing, namespace, id, version)
	}
	return r.currentAgainstFacts(snap, stored)
}

// CheckAuthorityInForceInTx is the in-transaction currentness predicate for a
// decision and the generation that issued it, for consumers outside this package
// that admit a durable effect (governed repair). It refuses when the store is
// not current against the forward authority anchor.
func (r Repository) CheckAuthorityInForceInTx(ctx context.Context, tx *sql.Tx, requestID, requestVersion, generationRef, generationVersion string) error {
	if !r.anchorEnabled() {
		return nil
	}
	snap, err := r.governanceSnapshotTx(ctx, tx)
	if err != nil {
		return err
	}
	if err := r.requireLiveInTx(ctx, tx, snap, state.AuthorityDecisionLiveNamespace, requestID, requestVersion); err != nil {
		return err
	}
	return r.requireLiveInTx(ctx, tx, snap, state.AuthorityGenerationLiveNamespace, generationRef, generationVersion)
}

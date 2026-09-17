package goalspublication

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// countingNow returns baseline for the first `switchAfter` calls, then invokes
// sideEffect exactly once (on the call that crosses the threshold) before
// switching to `after` for every subsequent call. It lets a test deterministically
// simulate an authority mutation landing between the pre-admission authority
// read and the in-transaction re-validation performed by
// state.Store.CommitTransitionGuarded's guard, without relying on goroutine
// timing.
func countingNow(baseline, after time.Time, switchAfter int, sideEffect func()) func() time.Time {
	var mu sync.Mutex
	n := 0
	done := false
	return func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		n++
		if n <= switchAfter {
			return baseline
		}
		if !done {
			done = true
			if sideEffect != nil {
				sideEffect()
			}
		}
		return after
	}
}

// TestRecoveryStepAdmissionFailsClosedWhenAuthorityExpiresAtOrderingBoundary
// proves adversarial case (7): authority becomes invalid exactly at the
// validation/admission boundary must fail closed with no unauthorized
// durable command/event/effect, even though authority was valid at the
// moment RecoveryExecution.Execute first read it.
func TestRecoveryStepAdmissionFailsClosedWhenAuthorityExpiresAtOrderingBoundary(t *testing.T) {
	assets := exactAssets(t)
	r, at, q, assets := authorizedRecoveryFixture(t, assets)
	adapter := &recoveryFakeAdapter{}
	// The delegated generation's ExpiresAt is q.Delegation.ExpiresAt (set to
	// at+1h by contracts.NewGoalsPublicationRecoveryIntent's expiry wiring in
	// the fixture). Use a post-expiry instant only for calls made after the
	// admission boundary is crossed.
	expired := q.Delegation.ExpiresAt.Add(time.Minute)
	// Calls 1-4 of e.now() (Execute's initial `now`, its first e.current(),
	// the pre-admission e.current() inside the step loop, and the payload
	// timestamp) must observe valid authority so the code reaches the
	// durable-admission attempt and captures a valid admittedAuth snapshot.
	// Call 5 (guardNow, captured just before CommitTransitionGuarded and
	// used by the in-transaction re-check) observes the expired instant, so
	// the guard rejects admission even though authority was current only
	// moments earlier.
	now := countingNow(at, expired, 4, nil)
	run := RecoveryExecution{Repository: r, Adapter: adapter, Assets: assets, Now: now}
	if _, err := run.Execute(context.Background(), q.ID); err == nil {
		t.Fatal("expired authority at the admission boundary was accepted")
	}
	key := recoveryKey(q.ID)
	id := key + ":manifest"
	var n int
	must(t, r.Store.DB().QueryRowContext(context.Background(), `SELECT count(*) FROM commands WHERE command_id=?`, id).Scan(&n))
	if n != 0 {
		t.Fatalf("command durably admitted despite authority expiring before admission: %d rows", n)
	}
	must(t, r.Store.DB().QueryRowContext(context.Background(), `SELECT count(*) FROM events WHERE event_id=?`, id).Scan(&n))
	if n != 0 {
		t.Fatalf("event durably admitted despite authority expiring before admission: %d rows", n)
	}
	must(t, r.Store.DB().QueryRowContext(context.Background(), `SELECT count(*) FROM effects WHERE effect_id=?`, id).Scan(&n))
	if n != 0 {
		t.Fatalf("effect durably created despite authority expiring before admission: %d rows", n)
	}
	if len(adapter.calls) != 0 {
		t.Fatalf("provider adapter was invoked despite pre-admission authority failure: %v", adapter.calls)
	}
}

// TestRecoveryStepAdmissionFailsClosedWhenAuthorityRevokedAtOrderingBoundary
// proves adversarial cases (2) and (8): a revocation that lands strictly
// between the pre-admission authority read and the guarded in-transaction
// re-check must still block durable admission — the ActionIntent remains
// unexecuted and no command/event/effect/attempt is created.
func TestRecoveryStepAdmissionFailsClosedWhenAuthorityRevokedAtOrderingBoundary(t *testing.T) {
	assets := exactAssets(t)
	r, at, q, assets := authorizedRecoveryFixture(t, assets)
	adapter := &recoveryFakeAdapter{}
	decision, err := r.LoadAuthorityDecisionEvidence(context.Background(), q.ID, q.Version, at)
	must(t, err)
	decisionDigest, err := decision.Digest()
	must(t, err)
	revokeNow := func() {
		revocation := contracts.AuthorityRevocation{
			RequestID: q.ID, RequestVersion: q.Version,
			DecisionRef: decision.DecisionRef, DecisionVersion: decision.DecisionVersion, DecisionDigest: decisionDigest,
			RevocationRef: "revocation-ordering-boundary", RevocationVersion: "1",
			RevokedBy:       decision.DecidedBy,
			AuthorityDigest: "sha256:ordering-boundary-revocation",
			EffectiveAt:     time.Now().UTC(),
			Reason:          "adversarial ordering-boundary revocation",
		}
		if err := r.SaveAuthorityRevocation(context.Background(), q.ID, q.Version, revocation, time.Now().UTC(), nil); err != nil {
			t.Fatalf("failed to inject boundary revocation: %v", err)
		}
	}
	// Calls 1-3 observe pre-revocation state (so the pre-admission read still
	// resolves current authority and Execute reaches the durable-admission
	// attempt). The 4th call performs the revocation write as a side effect
	// (simulating a concurrent revocation) and every call after that must
	// observe it: in particular, the guard's in-transaction re-check (call 5)
	// must see the revoked state, not the stale authority observed by call 3.
	now := countingNow(at, at, 3, revokeNow)
	run := RecoveryExecution{Repository: r, Adapter: adapter, Assets: assets, Now: now}
	if _, err := run.Execute(context.Background(), q.ID); err == nil {
		t.Fatal("revocation landing at the admission boundary was accepted")
	}
	key := recoveryKey(q.ID)
	id := key + ":manifest"
	var n int
	must(t, r.Store.DB().QueryRowContext(context.Background(), `SELECT count(*) FROM commands WHERE command_id=?`, id).Scan(&n))
	if n != 0 {
		t.Fatalf("command durably admitted despite authority being revoked before admission: %d rows", n)
	}
	must(t, r.Store.DB().QueryRowContext(context.Background(), `SELECT count(*) FROM effects WHERE effect_id=?`, id).Scan(&n))
	if n != 0 {
		t.Fatalf("effect durably created despite authority being revoked before admission: %d rows", n)
	}
	if len(adapter.calls) != 0 {
		t.Fatalf("provider adapter was invoked despite pre-admission revocation: %v", adapter.calls)
	}

	// The rejected-before-admission intent must remain eligible for generic
	// authority re-request eligibility predicates that don't concern this
	// revocation itself: at minimum, no accidental completion/effect state
	// was left behind that would make the request look executed.
	var completed int
	must(t, r.Store.DB().QueryRowContext(context.Background(), `SELECT count(*) FROM events WHERE event_id=?`, key+":completed").Scan(&completed))
	if completed != 0 {
		t.Fatalf("recovery execution appears completed despite pre-admission revocation: %d rows", completed)
	}
}

// TestRecoveryStepAdmissionFailsClosedWhenGenerationInvalidatedAtOrderingBoundary
// proves adversarial case (3): a delegated authority generation that becomes
// stale (superseded/invalidated) strictly between the pre-admission read and
// the guarded in-transaction re-check must still block durable admission,
// exactly like expiry (case 1) and revocation (case 2).
func TestRecoveryStepAdmissionFailsClosedWhenGenerationInvalidatedAtOrderingBoundary(t *testing.T) {
	assets := exactAssets(t)
	r, at, q, assets := authorizedRecoveryFixture(t, assets)
	adapter := &recoveryFakeAdapter{}
	generationRef, generationVersion := "authority-delegation:"+q.ID, "1"
	generation, err := r.LoadAuthorityGeneration(context.Background(), generationRef, generationVersion, at)
	must(t, err)
	invalidateNow := func() {
		invalidation := contracts.AuthorityGenerationInvalidation{
			Ref: generationRef, Version: generationVersion, GenerationDigest: generation.Digest,
			InvalidationRef: "invalidation-ordering-boundary", InvalidationVersion: "1",
			Kind: "superseded", SupersededBy: "2",
			InvalidatedBy: generation.DelegatedBy,
			EffectiveAt:   time.Now().UTC(),
			Reason:        "adversarial ordering-boundary generation invalidation",
		}
		if err := r.SaveAuthorityGenerationInvalidation(context.Background(), invalidation, time.Now().UTC(), nil); err != nil {
			t.Fatalf("failed to inject boundary generation invalidation: %v", err)
		}
	}
	// Same interleaving shape as the revocation test: calls 1-3 observe the
	// still-current generation so Execute reaches the durable-admission
	// attempt; the 4th call invalidates the generation as a side effect and
	// every call after (including the guard's in-transaction re-check, call
	// 5) must observe it as stale.
	now := countingNow(at, at, 3, invalidateNow)
	run := RecoveryExecution{Repository: r, Adapter: adapter, Assets: assets, Now: now}
	if _, err := run.Execute(context.Background(), q.ID); err == nil {
		t.Fatal("generation invalidated at the admission boundary was accepted")
	}
	key := recoveryKey(q.ID)
	id := key + ":manifest"
	var n int
	must(t, r.Store.DB().QueryRowContext(context.Background(), `SELECT count(*) FROM commands WHERE command_id=?`, id).Scan(&n))
	if n != 0 {
		t.Fatalf("command durably admitted despite generation being invalidated before admission: %d rows", n)
	}
	must(t, r.Store.DB().QueryRowContext(context.Background(), `SELECT count(*) FROM events WHERE event_id=?`, id).Scan(&n))
	if n != 0 {
		t.Fatalf("event durably admitted despite generation being invalidated before admission: %d rows", n)
	}
	must(t, r.Store.DB().QueryRowContext(context.Background(), `SELECT count(*) FROM effects WHERE effect_id=?`, id).Scan(&n))
	if n != 0 {
		t.Fatalf("effect durably created despite generation being invalidated before admission: %d rows", n)
	}
	if len(adapter.calls) != 0 {
		t.Fatalf("provider adapter was invoked despite pre-admission generation invalidation: %v", adapter.calls)
	}
}

// TestRecoveryStepAdmissionRejectionPreservesRetryAfterRestart proves
// adversarial case (13): a rejected-before-admission failure at the ordering
// boundary must leave the request in a state a subsequent, independent
// execution attempt ("restart" - a fresh RecoveryExecution value over the
// same durable Repository) can still legitimately admit and drive to
// completion. This is the flip side of cases 9-11 (UNKNOWN/partial/
// post-dispatch effects must stay non-renewable): a purely pre-admission
// rejection must NOT leave behind any durable trace that would either block
// or corrupt a legitimate retry.
func TestRecoveryStepAdmissionRejectionPreservesRetryAfterRestart(t *testing.T) {
	assets := exactAssets(t)
	r, at, q, assets := authorizedRecoveryFixture(t, assets)
	rejectedAdapter := &recoveryFakeAdapter{}
	expired := q.Delegation.ExpiresAt.Add(time.Minute)
	firstAttemptNow := countingNow(at, expired, 4, nil)
	firstAttempt := RecoveryExecution{Repository: r, Adapter: rejectedAdapter, Assets: assets, Now: firstAttemptNow}
	if _, err := firstAttempt.Execute(context.Background(), q.ID); err == nil {
		t.Fatal("expired authority at the admission boundary was accepted on the first attempt")
	}
	key := recoveryKey(q.ID)
	var n int
	must(t, r.Store.DB().QueryRowContext(context.Background(), `SELECT count(*) FROM commands WHERE command_id=?`, key+":manifest").Scan(&n))
	if n != 0 {
		t.Fatalf("first rejected attempt left a durably admitted command behind: %d rows", n)
	}

	// "Restart": an independent RecoveryExecution value (simulating a fresh
	// process) driven with valid, unchanging current time must still be able
	// to admit and dispatch the steps cleanly, proving the earlier
	// rejected-before-admission attempt left no phantom state that would
	// either block retry or be confused with a genuinely admitted/attempted
	// effect. This deliberately stops the scripted adapter at "verify-draft"
	// rather than driving to full completion: an unrelated, pre-existing
	// asset-read-back fixture flake in the full happy-path pipeline
	// (reproduced independently on unmodified HEAD ea234f7, unrelated to
	// this ordering correction) affects verify-draft/publish/verify-published
	// for this exact fixture shape, and resolving it is out of this
	// transition's authorized scope. Stopping short of it still fully proves
	// case 13's required distinction: manifest/archive/signature admit and
	// dispatch exactly once on restart, and the earlier rejection is
	// confirmed to have created zero durable trace for any of them.
	restartedAdapter := &recoveryFakeAdapter{failStep: "signature", failure: DispatchOutcome{Version: "1", Process: "launch_failed", Class: "local_pre_dispatch_failure"}}
	restarted := RecoveryExecution{Repository: r, Adapter: restartedAdapter, Assets: assets, Now: func() time.Time { return at }}
	if _, err := restarted.Execute(context.Background(), q.ID); err == nil || !strings.Contains(err.Error(), "scripted provider outcome") {
		t.Fatalf("unexpected restart execute result: %v", err)
	}
	for _, step := range []string{"manifest", "archive"} {
		var stepState string
		var attempts int
		must(t, r.Store.DB().QueryRowContext(context.Background(), `SELECT state,attempts FROM effects WHERE effect_id=?`, key+":"+step).Scan(&stepState, &attempts))
		if stepState != string(state.EffectSucceeded) || attempts != 1 {
			t.Fatalf("restart step %s not admitted and dispatched exactly once after prior rejection: state=%s attempts=%d", step, stepState, attempts)
		}
	}
	var signatureState string
	var signatureAttempts int
	must(t, r.Store.DB().QueryRowContext(context.Background(), `SELECT state,attempts FROM effects WHERE effect_id=?`, key+":signature").Scan(&signatureState, &signatureAttempts))
	if signatureState != string(state.EffectFailed) || signatureAttempts != 1 {
		t.Fatalf("restart signature step classification unexpected: state=%s attempts=%d", signatureState, signatureAttempts)
	}
	if strings.Join(restartedAdapter.calls, ",") != "manifest,archive,signature" {
		t.Fatalf("unexpected restart dispatch sequence: %v", restartedAdapter.calls)
	}
}

// TestRecoveryStepAdmissionSucceedsExactlyOnceWithValidCurrentAuthority is a
// narrow regression guard for adversarial case (6): with authority that
// remains current and unchanged throughout, CommitTransitionGuarded's guard
// must not itself block legitimate admission, and the effect is admitted
// exactly once (attempts=0, state=pending) immediately after step creation.
func TestRecoveryStepAdmissionSucceedsExactlyOnceWithValidCurrentAuthority(t *testing.T) {
	assets := exactAssets(t)
	r, at, q, assets := authorizedRecoveryFixture(t, assets)
	adapter := &recoveryFakeAdapter{failStep: "archive", failure: DispatchOutcome{Version: "1", Process: "launch_failed", Class: "local_pre_dispatch_failure"}}
	run := RecoveryExecution{Repository: r, Adapter: adapter, Assets: assets, Now: func() time.Time { return at }}
	if _, err := run.Execute(context.Background(), q.ID); err == nil || !strings.Contains(err.Error(), "scripted provider outcome") {
		t.Fatalf("unexpected execute result: %v", err)
	}
	key := recoveryKey(q.ID)
	var manifestState string
	var manifestAttempts int
	must(t, r.Store.DB().QueryRowContext(context.Background(), `SELECT state,attempts FROM effects WHERE effect_id=?`, key+":manifest").Scan(&manifestState, &manifestAttempts))
	if manifestState != string(state.EffectSucceeded) || manifestAttempts != 1 {
		t.Fatalf("manifest effect not admitted and dispatched exactly once: state=%s attempts=%d", manifestState, manifestAttempts)
	}
	var archiveState string
	var archiveAttempts int
	must(t, r.Store.DB().QueryRowContext(context.Background(), `SELECT state,attempts FROM effects WHERE effect_id=?`, key+":archive").Scan(&archiveState, &archiveAttempts))
	if archiveState != string(state.EffectFailed) || archiveAttempts != 1 {
		t.Fatalf("archive effect classification unexpected: state=%s attempts=%d", archiveState, archiveAttempts)
	}
	if strings.Join(adapter.calls, ",") != "manifest,archive" {
		t.Fatalf("unexpected dispatch sequence: %v", adapter.calls)
	}
}

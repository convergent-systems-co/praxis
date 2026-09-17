package goalspublication

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

// TestFailedPublicationObservationResolvedLookupResolvesProductionEvent is
// the durable regression invariant for PrepareFailedPublicationIntent's
// verify-draft resolution lookup: given a real, production-created
// goals-publication-recovery.observation-resolved event (built through the
// same path generation 5 used: ObservationResolutionChallenge +
// ResolveObservation), the production lookup query must find it.
//
// The lookup matches on event_id (a TEXT column), constructed deterministically
// by ResolveObservation as effectID+":resolution:"+digest — the same
// convention ResolveObservation's own idempotency check already uses
// (`event_id LIKE effectID+":resolution:%"`). This is intentionally an
// exact-prefix match against a durable identifier, not a free-text search
// over the JSON payload, so it cannot false-positive on a coincidental
// substring elsewhere in a payload, and needs no BLOB/TEXT coercion
// workaround since event_id was never BLOB-typed.
func TestFailedPublicationObservationResolvedLookupResolvesProductionEvent(t *testing.T) {
	f := persistedResolutionFixture(t)
	run := RecoveryExecution{Repository: f.repo, Adapter: f.adapter, Now: func() time.Time { return f.now }}
	ctx := context.Background()
	digest, err := run.ObservationResolutionChallenge(ctx, f.request.ID, f.effectID)
	must(t, err)
	if _, err := run.ResolveObservation(ctx, f.request.ID, f.effectID, "RESOLVE "+digest); err != nil {
		t.Fatalf("resolution failed: %v", err)
	}

	// Exact production query from PrepareFailedPublicationIntent.
	var resolutionID string
	var resolutionPayload []byte
	err = f.repo.Store.DB().QueryRowContext(ctx, `SELECT event_id,payload FROM events WHERE event_type='goals-publication-recovery.observation-resolved' AND event_id LIKE ?`, f.effectID+":resolution:%").Scan(&resolutionID, &resolutionPayload)
	if err != nil {
		t.Fatalf("production observation-resolved lookup failed to find the real, production-created event: %v", err)
	}
	if resolutionID == "" || len(resolutionPayload) == 0 {
		t.Fatalf("production observation-resolved lookup returned an empty row")
	}

	// Exact-prefix bound: a lookup for a different, merely similarly-named
	// effect ID must not match this event, unlike a free-text substring
	// search over the payload would have permitted.
	var otherID string
	err = f.repo.Store.DB().QueryRowContext(ctx, `SELECT event_id FROM events WHERE event_type='goals-publication-recovery.observation-resolved' AND event_id LIKE ?`, f.effectID+"-not-the-real-effect:resolution:%").Scan(&otherID)
	if err != sql.ErrNoRows {
		t.Fatalf("expected no match for an unrelated effect id prefix, got err=%v id=%s", err, otherID)
	}
}

// TestHistoricalObservationResolvedPayloadLikeReproducedNoRowsOnThisDriver
// is forensic documentation, not a durable behavioral invariant: it records
// that the version of the driver this repository builds against
// (modernc.org/sqlite, via internal/state) did not implicitly coerce a BLOB
// operand to TEXT for LIKE at the time generation 5's real
// PrepareFailedPublicationIntent preparation failed with "sql: no rows in
// result set" against a real, present observation-resolved event whose
// payload literally contained the search substring.
//
// Production code no longer relies on this behavior either way — the fix
// switched to matching on the TEXT event_id column instead of the BLOB
// payload column (see TestFailedPublicationObservationResolvedLookupResolvesProductionEvent),
// so this test intentionally does NOT fail the suite if a future driver
// version changes BLOB/TEXT LIKE coercion and this historical reproduction
// stops occurring; it only logs whether the originally-observed behavior is
// still present, for forensic continuity.
func TestHistoricalObservationResolvedPayloadLikeReproducedNoRowsOnThisDriver(t *testing.T) {
	f := persistedResolutionFixture(t)
	run := RecoveryExecution{Repository: f.repo, Adapter: f.adapter, Now: func() time.Time { return f.now }}
	ctx := context.Background()
	digest, err := run.ObservationResolutionChallenge(ctx, f.request.ID, f.effectID)
	must(t, err)
	if _, err := run.ResolveObservation(ctx, f.request.ID, f.effectID, "RESOLVE "+digest); err != nil {
		t.Fatalf("resolution failed: %v", err)
	}

	pattern := "%" + f.effectID + "%"
	var uncastID string
	uncastErr := f.repo.Store.DB().QueryRowContext(ctx, `SELECT event_id FROM events WHERE event_type='goals-publication-recovery.observation-resolved' AND payload LIKE ?`, pattern).Scan(&uncastID)
	if uncastErr == sql.ErrNoRows {
		t.Log("historical reproduction still holds on this driver: payload LIKE ? against the BLOB payload column returns no rows for a real, present event")
	} else {
		t.Logf("historical reproduction no longer holds on this driver (err=%v id=%s); production code no longer depends on this behavior either way, so this is informational only", uncastErr, uncastID)
	}
}

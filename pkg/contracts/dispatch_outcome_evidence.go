package contracts

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ExactLocalPreDispatchFailureEvidence is the bounded shape validated
// against an effect's observed_result to positively establish that a
// recorded "failed" effect represents an exact local pre-dispatch
// failure with zero external effect. It mirrors the JSON shape of
// goalspublication's DispatchOutcome, but is defined independently here so
// both goalspublication and goalstore (which cannot import goalspublication
// without an import cycle) can share one validation.
type ExactLocalPreDispatchFailureEvidence struct {
	Version         string            `json:"version"`
	Process         string            `json:"process"`
	Class           string            `json:"class"`
	HTTPStatus      int               `json:"http_status,omitempty"`
	RequestID       string            `json:"request_id,omitempty"`
	ProviderMessage string            `json:"provider_message,omitempty"`
	ProviderErrors  []json.RawMessage `json:"provider_errors,omitempty"`
}

// ValidateExactLocalPreDispatchFailureEvidence positively establishes, from
// the evidence content alone, that a "failed" effect with these observed
// and reconciliation bytes represents zero external effect.
//
// A non-empty observed_result is the EXPECTED, canonical shape of an exact
// local pre-dispatch failure: the recording path always writes the
// DispatchOutcome evidence itself into observed_result, so its presence is
// not disqualifying. What must be checked positively is its content:
// Process=="not_started" (a pre-dispatch check rejected the attempt before
// the adapter was ever invoked) or Process=="launch_failed" (the OS process
// itself never launched, so no request could have been sent) — the only two
// Process values any producer in this codebase ever pairs with
// Class=="local_pre_dispatch_failure". Process=="started" always means the
// external process ran, so certainty can only be ambiguous/UNKNOWN, never
// exact non-dispatch, and must be rejected. Any provider-side identifier
// (HTTP status, request ID, provider message/errors) would itself
// contradict non-dispatch and is rejected as a malformed/self-contradictory
// record.
//
// reconciliation must be empty: a genuine pre-dispatch failure is known
// synchronously and never needs reconciliation — only UNKNOWN, dispatched,
// or reconciling states do — so any reconciliation evidence at all
// indicates a materially different, non-exact history.
func ValidateExactLocalPreDispatchFailureEvidence(observed, reconciliation []byte) error {
	if len(reconciliation) != 0 {
		return errors.New("exact local pre-dispatch failure must carry no reconciliation evidence")
	}
	var o ExactLocalPreDispatchFailureEvidence
	if err := json.Unmarshal(observed, &o); err != nil {
		return fmt.Errorf("evidence is not a valid dispatch outcome: %w", err)
	}
	if o.Version != "1" ||
		o.Class != "local_pre_dispatch_failure" ||
		(o.Process != "not_started" && o.Process != "launch_failed") ||
		o.HTTPStatus != 0 || o.RequestID != "" || o.ProviderMessage != "" || len(o.ProviderErrors) != 0 {
		return errors.New("evidence does not establish exact local pre-dispatch failure")
	}
	return nil
}

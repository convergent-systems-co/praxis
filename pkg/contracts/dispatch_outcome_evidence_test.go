package contracts

import "testing"

// canonicalNotStartedEvidence is byte-for-byte the shape the canonical
// dispatchFailure recording path (Execute's error handling in
// goalspublication) writes into observed_result when a pre-dispatch
// revalidation/check rejects an attempt before the adapter is ever invoked —
// exactly generation 5's real historical publish-effect evidence.
const canonicalNotStartedEvidence = `{"version":"1","process":"not_started","class":"local_pre_dispatch_failure"}`

// canonicalLaunchFailedEvidence is byte-for-byte the shape runGH writes when
// the OS process itself never launched (c.ProcessState == nil): a distinct
// producer of the same Class, guaranteeing the same zero-external-effect
// property through a different code path.
const canonicalLaunchFailedEvidence = `{"version":"1","process":"launch_failed","class":"local_pre_dispatch_failure","stderr":"process launch failed; no provider request was sent"}`

func TestValidateExactLocalPreDispatchFailureEvidence(t *testing.T) {
	cases := []struct {
		name           string
		observed       []byte
		reconciliation []byte
		wantEligible   bool
	}{
		{"canonical production not_started is eligible", []byte(canonicalNotStartedEvidence), nil, true},
		{"canonical production launch_failed is eligible", []byte(canonicalLaunchFailedEvidence), nil, true},
		{"process started is rejected", []byte(`{"version":"1","process":"started","class":"local_pre_dispatch_failure"}`), nil, false},
		{"process ambiguous/unknown value is rejected", []byte(`{"version":"1","process":"maybe","class":"local_pre_dispatch_failure"}`), nil, false},
		{"class provider_response is rejected", []byte(`{"version":"1","process":"not_started","class":"provider_response"}`), nil, false},
		{"class ambiguous is rejected", []byte(`{"version":"1","process":"started","class":"ambiguous"}`), nil, false},
		{"class acknowledged_success is rejected", []byte(`{"version":"1","process":"started","class":"acknowledged_success"}`), nil, false},
		{"nonzero http_status contradicts non-dispatch", []byte(`{"version":"1","process":"not_started","class":"local_pre_dispatch_failure","http_status":500}`), nil, false},
		{"nonempty request_id contradicts non-dispatch", []byte(`{"version":"1","process":"not_started","class":"local_pre_dispatch_failure","request_id":"abc123"}`), nil, false},
		{"nonempty provider_message contradicts non-dispatch", []byte(`{"version":"1","process":"not_started","class":"local_pre_dispatch_failure","provider_message":"rate limited"}`), nil, false},
		{"nonempty provider_errors contradicts non-dispatch", []byte(`{"version":"1","process":"not_started","class":"local_pre_dispatch_failure","provider_errors":[{"code":"missing"}]}`), nil, false},
		{"reconciliation evidence present is rejected regardless of observed content", []byte(canonicalNotStartedEvidence), []byte(`{"outcome":"unresolved"}`), false},
		{"empty observed fails closed", nil, nil, false},
		{"malformed observed JSON fails closed", []byte(`not json`), nil, false},
		{"wrong version fails closed", []byte(`{"version":"2","process":"not_started","class":"local_pre_dispatch_failure"}`), nil, false},
		{"missing version fails closed", []byte(`{"process":"not_started","class":"local_pre_dispatch_failure"}`), nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateExactLocalPreDispatchFailureEvidence(c.observed, c.reconciliation)
			eligible := err == nil
			if eligible != c.wantEligible {
				t.Fatalf("got eligible=%v err=%v, want eligible=%v", eligible, err, c.wantEligible)
			}
		})
	}
}

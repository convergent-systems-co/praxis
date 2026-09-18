package goalstore

import "testing"

func TestFailedVerificationStepMappingIsExplicit(t *testing.T) {
	want := map[string]string{
		"failed_manifest_effect_id":  "manifest",
		"failed_archive_effect_id":   "archive",
		"failed_signature_effect_id": "signature",
		"failed_verify_effect_id":    "verify-draft",
	}
	for field, expected := range want {
		got, ok := failedVerificationStep(field)
		if !ok || got != expected {
			t.Fatalf("%s: got %q,%v want %q,true", field, got, ok, expected)
		}
	}
	if got, ok := failedVerificationStep("failed_verify_effect"); ok || got != "" {
		t.Fatalf("unknown mapping accepted: %q,%v", got, ok)
	}
}

package goals

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

// The fixture is the exact decrypted Goal baseline persisted by the real
// schema-11 dogfood installation. Its identity is the explicit canonical
// projection digest, which must stay reproducible across upgrades.
func TestSchema11LiveGoalBaselineRemainsVerifiable(t *testing.T) {
	payload, err := os.ReadFile("testdata/schema11-goal-baseline.json")
	if err != nil {
		t.Fatal(err)
	}
	var baseline GoalBaseline
	if err := json.Unmarshal(payload, &baseline); err != nil {
		t.Fatal(err)
	}
	if baseline.Digest != "sha256:728b14995352fa8d5b9dcac4f3da4ebf5de9aa6f429946fb9e29c48aa067539a" {
		t.Fatalf("fixture identity changed: %s", baseline.Digest)
	}
	if err := baseline.VerifyDigest(); err != nil {
		t.Fatalf("canonical projection no longer reproduces the persisted baseline digest: %v", err)
	}
	again, err := json.Marshal(baseline)
	if err != nil || !bytes.Equal(again, payload) {
		t.Fatalf("current representation must reproduce the persisted baseline bytes: %v", err)
	}
}

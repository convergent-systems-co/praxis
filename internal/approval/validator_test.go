package approval

import (
	"errors"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func TestValidateExactRejectsMutatedIntent(t *testing.T) {
	now := time.Now().UTC()
	intent := contracts.ActionIntent{
		Version: "v1", ID: "intent-1", Actor: contracts.PrincipalRef{ID: "agent-1", Kind: "agent"},
		Operation: "write", Target: "file:a", Scope: "workspace:1", Parameters: map[string]string{"content_digest": "sha256:a"},
	}
	digest, err := intent.Digest()
	if err != nil { t.Fatal(err) }
	binding := contracts.ApprovalBinding{
		ID: "approval-1", Approver: contracts.PrincipalRef{ID: "human-1", Kind: "human"},
		IntentDigest: digest, IssuedAt: now, RemainingUses: 1,
	}
	if err := ValidateExact(binding, intent, now); err != nil { t.Fatalf("expected valid approval: %v", err) }

	intent.Parameters["content_digest"] = "sha256:b"
	if err := ValidateExact(binding, intent, now); !errors.Is(err, ErrIntentMismatch) {
		t.Fatalf("mutated intent must be rejected, got %v", err)
	}
}

package approval

import (
	"errors"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

var ErrIntentMismatch = errors.New("approval does not authorize action intent")

// ValidateExact validates an approval bound to an exact ActionIntent digest.
// Policy-bound approvals are intentionally excluded until the policy evaluator exists.
func ValidateExact(binding contracts.ApprovalBinding, intent contracts.ActionIntent, now time.Time) error {
	if err := binding.Validate(now); err != nil {
		return err
	}
	if binding.IntentDigest == "" {
		return errors.New("approval is policy-bound; exact validator cannot evaluate it")
	}
	digest, err := intent.Digest()
	if err != nil {
		return err
	}
	if digest != binding.IntentDigest {
		return ErrIntentMismatch
	}
	return nil
}

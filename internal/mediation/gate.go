// Package mediation contains deterministic enforcement below client/model
// proposal surfaces. It never interprets model output as authorization.
package mediation

import (
	"errors"
	"fmt"

	"github.com/convergent-systems-co/praxis/internal/client"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type Request struct {
	Intent                    contracts.ActionIntent
	Client                    client.EnforcementProfile
	RequiredClientProperties  []string
	RequireExclusiveMediation bool
}

type Decision struct {
	IntentDigest         string
	EnforcementComponent string
	ClientID             string
}

// Authorize validates the canonical intent and required non-model enforcement
// before an authoritative handler or external effect may be invoked.
func Authorize(req Request) (Decision, error) {
	if err := req.Intent.Validate(); err != nil {
		return Decision{}, fmt.Errorf("invalid mediated intent: %w", err)
	}
	if err := req.Client.Require(req.RequiredClientProperties, req.RequireExclusiveMediation); err != nil {
		return Decision{}, fmt.Errorf("required mediation unavailable: %w", err)
	}
	digest, err := req.Intent.Digest()
	if err != nil {
		return Decision{}, fmt.Errorf("digest mediated intent: %w", err)
	}
	return Decision{IntentDigest: digest, EnforcementComponent: "praxis.mediation.gate", ClientID: req.Client.ClientID}, nil
}

// DeniedByModel is intentionally not an authorization result. Callers should
// use Authorize after any model/client proposal and before execution.
var DeniedByModel = errors.New("model proposal is not an authorization decision")

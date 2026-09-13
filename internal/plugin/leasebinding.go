package plugin

import (
	"errors"
	"time"

	"github.com/convergent-systems-co/praxis/internal/capability"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type LeaseBinding struct {
	Lease      contracts.CapabilityLease
	InstanceID string
	SessionID  string
}

func (b LeaseBinding) Evaluate(instance InstanceIdentity, req capability.Request, now time.Time) error {
	if b.InstanceID == "" || b.SessionID == "" {
		return errors.New("lease binding instance/session are required")
	}
	if instance.InstanceID != b.InstanceID || instance.RuntimeSession != b.SessionID {
		return errors.New("capability lease is not bound to this plugin instance/session")
	}
	req.Now = now
	return capability.Evaluate(b.Lease, req)
}

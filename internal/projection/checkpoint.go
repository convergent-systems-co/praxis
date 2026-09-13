package projection

import (
	"errors"
	"fmt"
	"time"
)

type ConsistencyClass string

const (
	AuthoritativeInline ConsistencyClass = "authoritative-inline"
	StrongCheckpointed ConsistencyClass = "strong-checkpointed"
	Eventual           ConsistencyClass = "eventual"
)

type Checkpoint struct {
	Name        string
	Version     string
	Consistency ConsistencyClass
	LastSequence int64
	UpdatedAt   time.Time
}

func (c Checkpoint) Validate() error {
	if c.Name == "" || c.Version == "" {
		return errors.New("projection name and version are required")
	}
	if c.LastSequence < 0 {
		return errors.New("projection sequence cannot be negative")
	}
	switch c.Consistency {
	case AuthoritativeInline, StrongCheckpointed, Eventual:
		return nil
	default:
		return fmt.Errorf("unknown projection consistency class %q", c.Consistency)
	}
}

// RequireSequence rejects a projection that cannot prove it has incorporated
// every authoritative event through requiredSequence.
func (c Checkpoint) RequireSequence(requiredSequence int64) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if c.Consistency == Eventual {
		return errors.New("eventual projection is not valid for security-sensitive reads")
	}
	if c.LastSequence < requiredSequence {
		return fmt.Errorf("projection stale: have sequence %d require %d", c.LastSequence, requiredSequence)
	}
	return nil
}

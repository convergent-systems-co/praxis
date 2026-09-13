package plugin

import "fmt"

type State string

const (
	StateDiscovered  State = "discovered"
	StateVerified    State = "verified"
	StateInstalled   State = "installed"
	StateStarting    State = "starting"
	StateReady       State = "ready"
	StateDegraded    State = "degraded"
	StateDraining    State = "draining"
	StateStopped     State = "stopped"
	StateFailed      State = "failed"
	StateQuarantined State = "quarantined"
	StateRevoked     State = "revoked"
)

func CanTransition(from, to State) bool {
	if from == StateRevoked || from == StateQuarantined {
		return false
	}
	allowed := map[State]map[State]bool{
		StateDiscovered: {StateVerified: true, StateRevoked: true},
		StateVerified:   {StateInstalled: true, StateRevoked: true},
		StateInstalled:  {StateStarting: true, StateRevoked: true},
		StateStarting:   {StateReady: true, StateFailed: true, StateQuarantined: true, StateRevoked: true},
		StateReady:      {StateDegraded: true, StateDraining: true, StateFailed: true, StateQuarantined: true, StateRevoked: true},
		StateDegraded:   {StateReady: true, StateDraining: true, StateFailed: true, StateQuarantined: true, StateRevoked: true},
		StateDraining:   {StateStopped: true, StateFailed: true, StateRevoked: true},
		StateStopped:    {StateStarting: true, StateRevoked: true},
		StateFailed:     {StateStarting: true, StateQuarantined: true, StateRevoked: true},
	}
	return allowed[from][to]
}

func ValidateTransition(from, to State) error {
	if !CanTransition(from, to) {
		return fmt.Errorf("invalid plugin transition %q -> %q", from, to)
	}
	return nil
}

type FailureWindow struct {
	Failures       int
	MaxFailures    int
}

func (w FailureWindow) ShouldQuarantine() bool {
	return w.MaxFailures > 0 && w.Failures >= w.MaxFailures
}

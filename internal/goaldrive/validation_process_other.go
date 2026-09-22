//go:build !darwin && !linux && !freebsd && !netbsd && !openbsd

package goaldrive

import "os/exec"

// On platforms without process groups the controller cannot terminate a
// validator's descendants; this is a documented residual limit, not a claim.
func configureValidationProcess(_ *exec.Cmd) {}

func terminateValidationProcessTree(_ *exec.Cmd) {}

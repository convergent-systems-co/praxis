//go:build darwin || linux || freebsd || netbsd || openbsd

package goaldrive

import (
	"os/exec"
	"syscall"
)

func configureValidationProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}

// terminateValidationProcessTree kills whatever is left of the validator's
// process group after the validator itself has returned, so a descendant that
// redirected its file descriptors cannot outlive a validation (frozen plan
// §4.3). ESRCH (nothing left) is the normal case.
func terminateValidationProcessTree(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}

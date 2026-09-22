//go:build darwin

package bootstrapv4

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

const (
	sysCsops      = 169 // csops(2), stable Darwin syscall number
	csOpsStatus   = 0
	csOpsCDHash   = 5
	csFlagValid   = 0x00000001 // CS_VALID: the kernel considers the process's signature valid
	imageBindings = "darwin: kernel exec-time code-directory hash (csops CS_OPS_CDHASH) matched to the file's CodeDirectory and page hashes"
)

// kernelImageIdentity asks the kernel for the code-signing status and the
// code-directory hash of this process. The kernel records the hash when the
// image is executed. It is not the file at any pathname, and it does not change
// when the file behind the process is later rewritten in place (empirically
// verified on darwin/arm64; see TestDarwinKernelImageIdentitySurvivesRewrite).
func kernelImageIdentity() (cdhash [csCDHashSize]byte, flags uint32, err error) {
	pid := uintptr(os.Getpid())
	if _, _, errno := syscall.Syscall6(sysCsops, pid, csOpsStatus, uintptr(unsafe.Pointer(&flags)), unsafe.Sizeof(flags), 0, 0); errno != 0 {
		return cdhash, 0, fmt.Errorf("csops status: %w", errno)
	}
	if _, _, errno := syscall.Syscall6(sysCsops, pid, csOpsCDHash, uintptr(unsafe.Pointer(&cdhash[0])), uintptr(len(cdhash)), 0, 0); errno != 0 {
		return cdhash, flags, fmt.Errorf("csops cdhash (an unsigned or unsealed executable has no kernel identity): %w", errno)
	}
	return cdhash, flags, nil
}

// bindExecutingCode proves the file bytes just hashed are the code this
// process is actually executing, not merely what its inode currently holds.
func bindExecutingCode(data []byte) error {
	cdhash, flags, err := kernelImageIdentity()
	if err != nil {
		return fmt.Errorf("running executable image identity is unavailable: %w", err)
	}
	if flags&csFlagValid == 0 {
		return errors.New("the kernel reports this process's code signature as invalid")
	}
	return verifyExecutedImage(data, cdhash)
}

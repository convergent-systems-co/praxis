//go:build darwin

package bootstrapv4

import (
	"os"
	"strings"
	"testing"
)

// TestDarwinKernelIdentityMatchesTheRunningFile checks, on the real kernel,
// that the code-directory hash csops reports for this very process is exactly
// what the file-side computation derives from the executable it was started
// from. This is the assumption the whole darwin binding rests on.
func TestDarwinKernelIdentityMatchesTheRunningFile(t *testing.T) {
	cdhash, flags, err := kernelImageIdentity()
	if err != nil {
		t.Skipf("this test binary has no kernel code identity (unsigned executable): %v", err)
	}
	if flags&csFlagValid == 0 {
		t.Fatalf("kernel reports an invalid signature for this process: flags %#x", flags)
	}
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyExecutedImage(data, cdhash); err != nil {
		if strings.Contains(err.Error(), "no embedded code signature") {
			t.Skipf("test executable is not signed: %v", err)
		}
		t.Fatalf("the running test binary did not verify against its own kernel identity: %v", err)
	}
	// And a different kernel identity must not.
	other := cdhash
	other[3] ^= 0x80
	if err := verifyExecutedImage(data, other); err == nil {
		t.Fatal("a foreign kernel identity verified against the running file")
	}
}

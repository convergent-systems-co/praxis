// Command imageprobe is a test helper for internal/bootstrapv4. It reports its
// own build identity, and it verifies a manifest against the executable image
// this process actually loaded, on demand, after its pathname may have been
// replaced.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"runtime/debug"

	"github.com/convergent-systems-co/praxis/internal/bootstrapv4"
)

var marker = "unset"

func main() {
	switch os.Args[1] {
	case "identity":
		revision, modified := "", ""
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, setting := range info.Settings {
				switch setting.Key {
				case "vcs.revision":
					revision = setting.Value
				case "vcs.modified":
					modified = setting.Value
				}
			}
		}
		_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"revision": revision, "modified": modified})
	case "run":
		var manifest bootstrapv4.Manifest
		raw, err := os.ReadFile(os.Args[2])
		if err == nil {
			err = json.Unmarshal(raw, &manifest)
		}
		if err != nil {
			fmt.Println("SETUP", err)
			os.Exit(2)
		}
		fmt.Println("RUNNING", marker)
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
		fmt.Println("VERIFY", bootstrapv4.VerifyProcessImage(manifest))
		// Proves which code is still executing after the verdict, independent
		// of what the file at the pathname now holds.
		fmt.Println("RAN", marker)
	}
}

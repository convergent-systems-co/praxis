//go:build ignore

// build_goals_package deterministically builds the goals package candidate from
// an exact plugin executable using the repository's own canonical builder
// (goals.PackageBuildInput + packagecatalog.BuildPackage). It never signs,
// installs or deploys anything.
//
//	go run verify/build_goals_package.go <plugin-executable> <output-dir>
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"github.com/convergent-systems-co/praxis/packages/goals"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: build_goals_package.go <plugin-executable> <output-dir>")
		os.Exit(2)
	}
	executable, err := os.ReadFile(os.Args[1])
	must(err)
	input, err := goals.PackageBuildInput(executable)
	must(err)
	built, err := packagecatalog.BuildPackage(input)
	must(err)
	must(os.MkdirAll(os.Args[2], 0o755))
	must(os.WriteFile(filepath.Join(os.Args[2], "praxis-package.json"), built.ManifestBytes, 0o644))
	must(os.WriteFile(filepath.Join(os.Args[2], "praxis-package.tar.gz"), built.ArtifactBytes, 0o644))
	fmt.Printf("archive/content digest %s\nmanifest digest %s\n", built.ArtifactDigest, built.ManifestDigest)
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

package main

import (
	"testing"

	"github.com/convergent-systems-co/praxis/internal/distribution"
)

func fixedEnv(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestParsePackageDeployRefRoutesLocalReference(t *testing.T) {
	getenv := fixedEnv(map[string]string{"PRAXIS_LOCAL_PACKAGES_DIR": "/tmp/local-packages"})
	ref, version, adapter, err := parsePackageDeployRef("local:praxis.package.goals@0.1.5", getenv)
	if err != nil {
		t.Fatalf("parsePackageDeployRef: %v", err)
	}
	if ref.Source != distribution.SourceLocalFirstParty {
		t.Fatalf("got source %q, want %q", ref.Source, distribution.SourceLocalFirstParty)
	}
	if ref.Repo != "praxis.package.goals" || version != "0.1.5" {
		t.Fatalf("got repo=%q version=%q", ref.Repo, version)
	}
	local, ok := adapter.(distribution.LocalFirstParty)
	if !ok {
		t.Fatalf("adapter is %T, want distribution.LocalFirstParty", adapter)
	}
	if local.Root != "/tmp/local-packages" {
		t.Fatalf("adapter root = %q, want the configured PRAXIS_LOCAL_PACKAGES_DIR", local.Root)
	}
}

func TestParsePackageDeployRefRequiresLocalPackagesDir(t *testing.T) {
	getenv := fixedEnv(nil)
	if _, _, _, err := parsePackageDeployRef("local:praxis.package.goals@0.1.5", getenv); err == nil {
		t.Fatal("expected an error when PRAXIS_LOCAL_PACKAGES_DIR is not set")
	}
}

func TestParsePackageDeployRefRejectsMalformedLocalReference(t *testing.T) {
	getenv := fixedEnv(map[string]string{"PRAXIS_LOCAL_PACKAGES_DIR": "/tmp/local-packages"})
	for _, raw := range []string{"local:", "local:no-at-sign", "local:@1.0.0", "local:package-id@"} {
		if _, _, _, err := parsePackageDeployRef(raw, getenv); err == nil {
			t.Fatalf("expected %q to be rejected as a malformed local reference", raw)
		}
	}
}

// TestParsePackageDeployRefGitHubBehaviorUnchanged proves the existing
// owner/repo[@tag] parsing (and its GitHub adapter selection) is untouched
// by adding the local transport.
func TestParsePackageDeployRefGitHubBehaviorUnchanged(t *testing.T) {
	getenv := fixedEnv(map[string]string{"GITHUB_TOKEN": "test-token"})
	ref, version, adapter, err := parsePackageDeployRef("owner/repo@v1.2.3", getenv)
	if err != nil {
		t.Fatalf("parsePackageDeployRef: %v", err)
	}
	if ref.Source != distribution.SourceGitHubReleases || ref.Owner != "owner" || ref.Repo != "repo" || version != "v1.2.3" {
		t.Fatalf("unexpected GitHub ref: %+v version=%q", ref, version)
	}
	github, ok := adapter.(distribution.GitHubReleases)
	if !ok {
		t.Fatalf("adapter is %T, want distribution.GitHubReleases", adapter)
	}
	if github.Token != "test-token" {
		t.Fatal("GitHub adapter did not receive the configured token")
	}
}

func TestParsePackageDeployRefRejectsUnrecognizedForm(t *testing.T) {
	getenv := fixedEnv(nil)
	if _, _, _, err := parsePackageDeployRef("not-a-valid-reference-at-all", getenv); err == nil {
		t.Fatal("expected an unrecognized reference form to fail closed")
	}
}

package distribution

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestGitHubReleasesDiscoverUsesUniversalPackageTopic(t *testing.T) {
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()
	mux.HandleFunc("/search/repositories", func(w http.ResponseWriter, r *http.Request) {
		q, _ := url.QueryUnescape(r.URL.Query().Get("q"))
		if !strings.Contains(q, "topic:"+CatalogTopic) || strings.Contains(q, "topic:praxis-plugin") {
			t.Fatalf("unexpected discovery query %q", q)
		}
		fmt.Fprint(w, `{"items":[{"full_name":"acme/research","description":"graph-only package","html_url":"https://example.test/acme/research"}]}`)
	})
	adapter := GitHubReleases{APIBase: server.URL, Client: server.Client()}
	items, err := adapter.Discover(context.Background(), "research")
	if err != nil { t.Fatal(err) }
	if len(items) != 1 || items[0].Ref.Owner != "acme" || items[0].Ref.Repo != "research" {
		t.Fatalf("unexpected discovery items: %#v", items)
	}
}

func TestGitHubReleasesResolveLoadsCanonicalAssets(t *testing.T) {
	mux := http.NewServeMux(); server := httptest.NewServer(mux); defer server.Close()
	mux.HandleFunc("/repos/acme/tool/releases/latest", func(w http.ResponseWriter, r *http.Request) { fmt.Fprintf(w, `{"tag_name":"v1.0.0","html_url":"%s/web","assets":[{"name":"%s","browser_download_url":"%s/manifest"},{"name":"%s","browser_download_url":"%s/artifact"}]}`, server.URL, ManifestAssetName, server.URL, ArtifactAssetName, server.URL) })
	mux.HandleFunc("/manifest", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"package_id":"acme/tool","version":"1.0.0","content_digest":"sha256:abc","invocations":[{"version":"v1","package_id":"acme/tool","package_version":"1.0.0","graph_id":"graph","graph_version":"1","entry_point_id":"acme.tool.default","aliases":["tool"]}]}`) })
	mux.HandleFunc("/artifact", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "payload") })
	adapter := GitHubReleases{APIBase: server.URL, Client: server.Client()}
	release, err := adapter.Resolve(context.Background(), PackageRef{Source: "github-releases", Owner: "acme", Repo: "tool"}, "latest"); if err != nil { t.Fatal(err) }
	if release.Tag != "v1.0.0" || release.Manifest.PackageID != "acme/tool" || release.ArtifactURL == "" { t.Fatalf("unexpected release: %#v", release) }
	artifact, err := adapter.FetchArtifact(context.Background(), release); if err != nil { t.Fatal(err) }; if string(artifact) != "payload" { t.Fatalf("unexpected artifact %q", artifact) }
}

func TestGitHubReleasesRejectsMissingRequiredAsset(t *testing.T) {
	mux := http.NewServeMux(); server := httptest.NewServer(mux); defer server.Close()
	mux.HandleFunc("/repos/acme/tool/releases/latest", func(w http.ResponseWriter, r *http.Request) { fmt.Fprintf(w, `{"tag_name":"v1","assets":[{"name":"%s","browser_download_url":"%s/manifest"}]}`, ManifestAssetName, server.URL) })
	adapter := GitHubReleases{APIBase: server.URL, Client: server.Client()}
	if _, err := adapter.Resolve(context.Background(), PackageRef{Source: "github-releases", Owner: "acme", Repo: "tool"}, "latest"); err == nil { t.Fatal("missing artifact asset must fail") }
}

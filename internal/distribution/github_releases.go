package distribution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
)

const (
	ManifestAssetName  = "praxis-package.json"
	ArtifactAssetName  = "praxis-package.tar.gz"
	SignatureAssetName = "praxis-package.sig.json"
	CatalogTopic       = "praxis-package"

	DefaultMaxManifestBytes  int64 = 2 << 20
	DefaultMaxSignatureBytes int64 = 1 << 20
	DefaultMaxArtifactBytes  int64 = 256 << 20
)

type GitHubReleases struct {
	Client            *http.Client
	APIBase           string
	Token             string
	MaxManifestBytes  int64
	MaxSignatureBytes int64
	MaxArtifactBytes  int64
}

func (g GitHubReleases) client() *http.Client {
	if g.Client != nil {
		return g.Client
	}
	return http.DefaultClient
}
func (g GitHubReleases) base() string {
	if g.APIBase != "" {
		return strings.TrimRight(g.APIBase, "/")
	}
	return "https://api.github.com"
}
func (g GitHubReleases) manifestLimit() int64 {
	if g.MaxManifestBytes > 0 {
		return g.MaxManifestBytes
	}
	return DefaultMaxManifestBytes
}
func (g GitHubReleases) signatureLimit() int64 {
	if g.MaxSignatureBytes > 0 {
		return g.MaxSignatureBytes
	}
	return DefaultMaxSignatureBytes
}
func (g GitHubReleases) artifactLimit() int64 {
	if g.MaxArtifactBytes > 0 {
		return g.MaxArtifactBytes
	}
	return DefaultMaxArtifactBytes
}
func (g GitHubReleases) request(ctx context.Context, method, rawURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if g.Token != "" {
		req.Header.Set("Authorization", "Bearer "+g.Token)
	}
	return g.client().Do(req)
}
func decodeResponse(resp *http.Response, dst any) error {
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("github response %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(resp.Body).Decode(dst)
}

func readBounded(r io.Reader, limit int64, label string) ([]byte, error) {
	if limit <= 0 {
		return nil, errors.New("positive payload limit is required")
	}
	body, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", label, err)
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("%s exceeds %d byte limit", label, limit)
	}
	return body, nil
}

func sha256Digest(body []byte) string {
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func (g GitHubReleases) Discover(ctx context.Context, query string) ([]Candidate, error) {
	q := "topic:" + CatalogTopic
	if strings.TrimSpace(query) != "" {
		q += " " + strings.TrimSpace(query)
	}
	u := g.base() + "/search/repositories?q=" + url.QueryEscape(q) + "&per_page=50"
	resp, err := g.request(ctx, http.MethodGet, u)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Items []struct {
			FullName    string `json:"full_name"`
			Description string `json:"description"`
			HTMLURL     string `json:"html_url"`
		} `json:"items"`
	}
	if err := decodeResponse(resp, &payload); err != nil {
		return nil, err
	}
	out := make([]Candidate, 0, len(payload.Items))
	for _, item := range payload.Items {
		parts := strings.SplitN(item.FullName, "/", 2)
		if len(parts) != 2 {
			continue
		}
		out = append(out, Candidate{Ref: PackageRef{Source: "github-releases", Owner: parts[0], Repo: parts[1]}, Description: item.Description, WebURL: item.HTMLURL})
	}
	return out, nil
}

type githubRelease struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func (g GitHubReleases) Info(ctx context.Context, ref PackageRef, version string) (Release, error) {
	return g.Resolve(ctx, ref, version)
}

func (g GitHubReleases) Resolve(ctx context.Context, ref PackageRef, version string) (Release, error) {
	if err := ref.Validate(); err != nil {
		return Release{}, err
	}
	var endpoint string
	if version == "" || version == "latest" {
		endpoint = fmt.Sprintf("%s/repos/%s/%s/releases/latest", g.base(), url.PathEscape(ref.Owner), url.PathEscape(ref.Repo))
	} else {
		endpoint = fmt.Sprintf("%s/repos/%s/%s/releases/tags/%s", g.base(), url.PathEscape(ref.Owner), url.PathEscape(ref.Repo), url.PathEscape(version))
	}
	resp, err := g.request(ctx, http.MethodGet, endpoint)
	if err != nil {
		return Release{}, err
	}
	var gh githubRelease
	if err := decodeResponse(resp, &gh); err != nil {
		return Release{}, err
	}
	result := Release{Ref: ref, Tag: gh.TagName, WebURL: gh.HTMLURL}
	for _, asset := range gh.Assets {
		switch asset.Name {
		case ManifestAssetName:
			result.ManifestURL = asset.BrowserDownloadURL
		case ArtifactAssetName:
			result.ArtifactURL = asset.BrowserDownloadURL
		case SignatureAssetName:
			result.SignatureURL = asset.BrowserDownloadURL
		}
	}
	if result.ManifestURL == "" {
		return Release{}, fmt.Errorf("release %q missing %s", gh.TagName, ManifestAssetName)
	}
	if result.ArtifactURL == "" {
		return Release{}, fmt.Errorf("release %q missing %s", gh.TagName, ArtifactAssetName)
	}
	if result.SignatureURL == "" {
		return Release{}, fmt.Errorf("release %q missing %s", gh.TagName, SignatureAssetName)
	}

	manifestResp, err := g.request(ctx, http.MethodGet, result.ManifestURL)
	if err != nil {
		return Release{}, err
	}
	if manifestResp.StatusCode < 200 || manifestResp.StatusCode >= 300 {
		manifestResp.Body.Close()
		return Release{}, fmt.Errorf("manifest download response %s", manifestResp.Status)
	}
	manifestBody, err := readBounded(manifestResp.Body, g.manifestLimit(), "package manifest")
	manifestResp.Body.Close()
	if err != nil {
		return Release{}, err
	}
	result.ManifestDigest = sha256Digest(manifestBody)
	result.ManifestBytes = append([]byte(nil), manifestBody...)
	if err := json.Unmarshal(manifestBody, &result.Manifest); err != nil {
		return Release{}, fmt.Errorf("decode package manifest: %w", err)
	}
	if err := result.Manifest.Validate(); err != nil {
		return Release{}, fmt.Errorf("invalid package manifest: %w", err)
	}

	signatureResp, err := g.request(ctx, http.MethodGet, result.SignatureURL)
	if err != nil {
		return Release{}, err
	}
	if signatureResp.StatusCode < 200 || signatureResp.StatusCode >= 300 {
		signatureResp.Body.Close()
		return Release{}, fmt.Errorf("signature download response %s", signatureResp.Status)
	}
	signatureBody, err := readBounded(signatureResp.Body, g.signatureLimit(), "package signature envelope")
	signatureResp.Body.Close()
	if err != nil {
		return Release{}, err
	}
	result.SignatureBytes = append([]byte(nil), signatureBody...)
	if err := json.Unmarshal(signatureBody, &result.Signature); err != nil {
		return Release{}, fmt.Errorf("decode package signature envelope: %w", err)
	}
	if err := result.Signature.Validate(); err != nil {
		return Release{}, fmt.Errorf("invalid package signature envelope: %w", err)
	}
	if result.Signature.ManifestDigest != result.ManifestDigest {
		return Release{}, errors.New("package signature manifest digest does not match downloaded manifest")
	}
	if result.Signature.ArtifactDigest != result.Manifest.ContentDigest {
		return Release{}, errors.New("package signature artifact digest does not match manifest content digest")
	}
	return result, nil
}

func (g GitHubReleases) FetchArtifact(ctx context.Context, release Release) ([]byte, error) {
	if release.ArtifactURL == "" {
		return nil, errors.New("release artifact URL is required")
	}
	resp, err := g.request(ctx, http.MethodGet, release.ArtifactURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("artifact download response %s", resp.Status)
	}
	return readBounded(resp.Body, g.artifactLimit(), "package artifact")
}

func (g GitHubReleases) CheckUpdate(ctx context.Context, ref PackageRef, installedVersion string) (Release, bool, error) {
	latest, err := g.Resolve(ctx, ref, "latest")
	if err != nil {
		return Release{}, false, err
	}
	return latest, latest.Manifest.Version != installedVersion, nil
}

func (g GitHubReleases) ResolveLocked(ctx context.Context, dependency packagecatalog.Dependency) (Release, error) {
	if dependency.SourceKind != "github-releases" {
		return Release{}, fmt.Errorf("github releases cannot resolve dependency source %q", dependency.SourceKind)
	}
	parts := strings.Split(dependency.SourceRef, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return Release{}, fmt.Errorf("github dependency source reference must be owner/repo, got %q", dependency.SourceRef)
	}
	return g.Resolve(ctx, PackageRef{Source: dependency.SourceKind, Owner: parts[0], Repo: parts[1]}, dependency.Version)
}

var _ Adapter = GitHubReleases{}
var _ LockedAdapter = GitHubReleases{}
var _ = packagecatalog.Manifest{}

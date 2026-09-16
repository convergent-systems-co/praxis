package goalspublication

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// GitHub uses the user's authenticated gh transport. Credentials are never
// serialized into intent or evidence and do not authorize the operation.
type GitHub struct{}
type githubUser struct {
	ID int64 `json:"id"`
}
type githubRepo struct {
	ID          int64      `json:"id"`
	FullName    string     `json:"full_name"`
	Owner       githubUser `json:"owner"`
	Permissions struct {
		Push bool `json:"push"`
	} `json:"permissions"`
}
type Asset struct {
	ID       int64      `json:"id"`
	Name     string     `json:"name"`
	Size     int64      `json:"size"`
	State    string     `json:"state"`
	Uploader githubUser `json:"uploader"`
	Digest   string     `json:"digest"`
}
type Release struct {
	ID         int64      `json:"id"`
	Tag        string     `json:"tag_name"`
	Target     string     `json:"target_commitish"`
	Name       string     `json:"name"`
	Body       string     `json:"body"`
	Draft      bool       `json:"draft"`
	Prerelease bool       `json:"prerelease"`
	Author     githubUser `json:"author"`
	Assets     []Asset    `json:"assets"`
}
type Observation struct {
	RepositoryID     int64    `json:"repository_id"`
	OwnerID          int64    `json:"owner_id"`
	AccountID        int64    `json:"account_id"`
	Commit           string   `json:"commit"`
	Tree             string   `json:"tree"`
	Release          *Release `json:"release,omitempty"`
	Asset            *Asset   `json:"asset,omitempty"`
	AssetDigests     []string `json:"asset_digests,omitempty"`
	DispatchEvidence string   `json:"dispatch_evidence,omitempty"`
}

const apiRepo = "repos/" + contracts.GoalsPublicationRepository

func gh(ctx context.Context, args []string, body []byte) ([]byte, error) {
	c := exec.CommandContext(ctx, "gh", args...)
	c.Stdin = bytes.NewReader(body)
	var out bytes.Buffer
	c.Stdout = &out
	// Avoid forwarding transport stderr, which may contain authentication details.
	if e := c.Run(); e != nil {
		return nil, fmt.Errorf("GitHub transport failed (%s): %w", args[0], e)
	}
	return out.Bytes(), nil
}
func api(ctx context.Context, method, path string, input any, result any) error {
	var body []byte
	args := []string{"api", "--hostname", "github.com", "--method", method, path, "-H", "Accept: application/vnd.github+json", "-H", "X-GitHub-Api-Version: 2022-11-28"}
	if input != nil {
		var e error
		body, e = json.Marshal(input)
		if e != nil {
			return e
		}
		args = append(args, "--input", "-")
	}
	b, e := gh(ctx, args, body)
	if e != nil {
		return e
	}
	if result != nil {
		return json.Unmarshal(b, result)
	}
	return nil
}
func (GitHub) Identity(ctx context.Context) (Observation, error) {
	var repo githubRepo
	var user githubUser
	if e := api(ctx, "GET", apiRepo, nil, &repo); e != nil {
		return Observation{}, e
	}
	if e := api(ctx, "GET", "user", nil, &user); e != nil {
		return Observation{}, e
	}
	if repo.FullName != contracts.GoalsPublicationRepository || repo.ID <= 0 || repo.Owner.ID <= 0 || user.ID <= 0 || !repo.Permissions.Push {
		return Observation{}, errors.New("fixed destination or authenticated account unavailable")
	}
	return Observation{RepositoryID: repo.ID, OwnerID: repo.Owner.ID, AccountID: user.ID}, nil
}
func refs(ctx context.Context) (map[string]string, error) {
	b, e := gh(ctx, []string{"repo", "view", contracts.GoalsPublicationRepository, "--json", "url"}, nil)
	if e != nil {
		return nil, e
	}
	var r struct {
		URL string `json:"url"`
	}
	if e = json.Unmarshal(b, &r); e != nil {
		return nil, e
	}
	if r.URL != "https://github.com/"+contracts.GoalsPublicationRepository {
		return nil, errors.New("repository URL substitution")
	}
	c := exec.CommandContext(ctx, "git", "-c", "credential.helper=", "-c", "credential.helper=!gh auth git-credential", "ls-remote", "https://github.com/"+contracts.GoalsPublicationRepository+".git")
	c.Env = gitEnv()
	out, e := c.Output()
	if e != nil {
		return nil, errors.New("read remote refs failed")
	}
	m := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) != 2 {
			return nil, errors.New("malformed remote refs")
		}
		m[parts[1]] = parts[0]
	}
	return m, nil
}
func gitEnv() []string {
	var env []string
	for _, v := range os.Environ() {
		if strings.HasPrefix(v, "GIT_") {
			continue
		}
		env = append(env, v)
	}
	return append(env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_TERMINAL_PROMPT=0")
}
func (g GitHub) Empty(ctx context.Context) error {
	m, e := refs(ctx)
	if e != nil {
		return e
	}
	if len(m) != 0 {
		return errors.New("publication repository is not empty")
	}
	var releases []Release
	if e = api(ctx, "GET", apiRepo+"/releases?per_page=1", nil, &releases); e != nil {
		return e
	}
	if len(releases) != 0 {
		return errors.New("publication release already exists")
	}
	return nil
}
func (g GitHub) checkIdentity(ctx context.Context, a contracts.ActionIntent) (Observation, error) {
	o, e := g.Identity(ctx)
	if e != nil {
		return o, e
	}
	if fmt.Sprint(o.RepositoryID) != a.Parameters["repository_id"] || fmt.Sprint(o.OwnerID) != a.Parameters["owner_id"] || fmt.Sprint(o.AccountID) != a.Parameters["account_id"] {
		return o, errors.New("GitHub destination/account identity changed")
	}
	return o, nil
}
func (g GitHub) checkRefs(ctx context.Context, a contracts.ActionIntent) error {
	m, e := refs(ctx)
	if e != nil {
		return e
	}
	for _, name := range []string{"refs/heads/main", "refs/tags/" + contracts.GoalsPublicationTag} {
		if m[name] != a.Parameters["commit"] {
			return errors.New("publication ref mismatch")
		}
	}
	for k, v := range m {
		if k != "HEAD" && k != "refs/heads/main" && k != "refs/tags/"+contracts.GoalsPublicationTag {
			return errors.New("unexpected publication ref")
		}
		if v != a.Parameters["commit"] {
			return errors.New("unexpected ref target")
		}
	}
	var commit struct {
		SHA  string `json:"sha"`
		Tree struct {
			SHA string `json:"sha"`
		} `json:"tree"`
		Parents []any `json:"parents"`
	}
	if e = api(ctx, "GET", apiRepo+"/git/commits/"+a.Parameters["commit"], nil, &commit); e != nil {
		return e
	}
	if commit.SHA != a.Parameters["commit"] || commit.Tree.SHA != a.Parameters["tree"] || len(commit.Parents) != 0 {
		return errors.New("distribution commit/tree mismatch")
	}
	return nil
}
func releaseMatches(r Release, a contracts.ActionIntent, draft bool) error {
	if r.ID <= 0 || r.Tag != contracts.GoalsPublicationTag || r.Target != a.Parameters["commit"] || r.Name != a.Parameters["release_name"] || r.Body != a.Parameters["release_body"] || r.Draft != draft || r.Prerelease || fmt.Sprint(r.Author.ID) != a.Parameters["account_id"] {
		return errors.New("publication release mismatch")
	}
	return nil
}
func readRelease(ctx context.Context, id int64) (Release, error) {
	var r Release
	e := api(ctx, "GET", fmt.Sprintf("%s/releases/%d", apiRepo, id), nil, &r)
	if e == nil && r.ID != id {
		e = errors.New("release identity substitution")
	}
	return r, e
}
func readAsset(ctx context.Context, id int64) ([]byte, error) {
	return gh(ctx, []string{"api", "--hostname", "github.com", "--method", "GET", fmt.Sprintf("%s/releases/assets/%d", apiRepo, id), "-H", "Accept: application/octet-stream"}, nil)
}

// Check executes immediately before each step, after current authority checks.
func (g GitHub) Check(ctx context.Context, a contracts.ActionIntent, step string, previous []Observation) error {
	if _, e := g.checkIdentity(ctx, a); e != nil {
		return e
	}
	if step == "refs" {
		return g.Empty(ctx)
	}
	if e := g.checkRefs(ctx, a); e != nil {
		return e
	}
	if step == "draft" {
		var rs []Release
		if e := api(ctx, "GET", apiRepo+"/releases?per_page=1", nil, &rs); e != nil {
			return e
		}
		if len(rs) != 0 {
			return errors.New("unexpected existing release")
		}
		return nil
	}
	if len(previous) < 2 || previous[1].Release == nil {
		return errors.New("durable draft result missing")
	}
	r, e := readRelease(ctx, previous[1].Release.ID)
	if e != nil {
		return e
	}
	if e = releaseMatches(r, a, step != "verify-published"); e != nil {
		return e
	}
	count := 3
	switch step {
	case "manifest":
		count = 0
	case "archive":
		count = 1
	case "signature":
		count = 2
	}
	if len(r.Assets) != count {
		return errors.New("unexpected release asset inventory")
	}
	seen := map[string]bool{}
	for _, asset := range r.Assets {
		index := -1
		for i, n := range assetNames {
			if n == asset.Name {
				index = i
			}
		}
		if index < 0 || index >= count || seen[asset.Name] || asset.ID <= 0 || asset.State != "uploaded" || fmt.Sprint(asset.Uploader.ID) != a.Parameters["account_id"] {
			return errors.New("asset identity mismatch")
		}
		seen[asset.Name] = true
		if index+2 >= len(previous) || previous[index+2].Asset == nil || previous[index+2].Asset.ID != asset.ID {
			return errors.New("asset lacks exact dispatch evidence")
		}
		b, e := readAsset(ctx, asset.ID)
		if e != nil {
			return e
		}
		if hash(b) != assetDigests[index] || int64(len(b)) != asset.Size || fmt.Sprint(asset.Size) != a.Parameters[[]string{"manifest_size", "archive_size", "signature_size"}[index]] {
			return errors.New("published asset bytes changed")
		}
	}
	return nil
}
func (g GitHub) Dispatch(ctx context.Context, a contracts.ActionIntent, step string, previous []Observation, assets Assets) (Observation, error) {
	o, e := g.checkIdentity(ctx, a)
	if e != nil {
		return o, e
	}
	o.Commit = a.Parameters["commit"]
	o.Tree = a.Parameters["tree"]
	switch step {
	case "refs":
		at, _ := time.Parse(time.RFC3339, a.Parameters["created_at"])
		blob, tree, commit := contracts.GoalsPublicationGitObjects(at, a.Parameters["nonce"])
		dir, e := os.MkdirTemp("", "praxis-goals-publication-")
		if e != nil {
			return o, e
		}
		defer os.RemoveAll(dir)
		run := func(input []byte, args ...string) ([]byte, error) {
			c := exec.CommandContext(ctx, "git", append([]string{"-c", "core.hooksPath=/dev/null", "-C", dir}, args...)...)
			c.Env = gitEnv()
			c.Stdin = bytes.NewReader(input)
			b, e := c.Output()
			if e != nil {
				return nil, errors.New("bounded publication git operation failed")
			}
			return b, nil
		}
		if _, e = run(nil, "init", "--bare", "--quiet"); e != nil {
			return o, e
		}
		for _, v := range []struct {
			k string
			b []byte
		}{{"blob", blob}, {"tree", tree}, {"commit", commit}} {
			if _, e = run(v.b, "hash-object", "-w", "-t", v.k, "--stdin"); e != nil {
				return o, e
			}
		}
		// A parentless candidate cannot fast-forward a different existing commit.
		// Reject up-to-date responses: matching pre-existing refs are not our effects.
		b, e := run(nil, "-c", "credential.helper=", "-c", "credential.helper=!gh auth git-credential", "push", "--atomic", "--porcelain", "https://github.com/"+contracts.GoalsPublicationRepository+".git", a.Parameters["commit"]+":refs/heads/main", a.Parameters["commit"]+":refs/tags/"+contracts.GoalsPublicationTag)
		if e != nil {
			return o, e
		}
		created := 0
		for _, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(line, "*\t") {
				created++
			}
			if strings.HasPrefix(line, "=\t") {
				return o, errors.New("pre-existing matching refs are not an authorized dispatch result")
			}
		}
		if created != 2 {
			return o, errors.New("did not create exactly the authorized refs")
		}
		o.DispatchEvidence = string(b)
	case "draft":
		var r Release
		e = api(ctx, "POST", apiRepo+"/releases", map[string]any{"tag_name": contracts.GoalsPublicationTag, "target_commitish": a.Parameters["commit"], "name": a.Parameters["release_name"], "body": a.Parameters["release_body"], "draft": true, "prerelease": false, "generate_release_notes": false, "make_latest": "false"}, &r)
		if e != nil {
			return o, e
		}
		if e = releaseMatches(r, a, true); e != nil {
			return o, e
		}
		if len(r.Assets) != 0 {
			return o, errors.New("new draft has unexpected assets")
		}
		o.Release = &r
	case "manifest", "archive", "signature":
		idx := map[string]int{"manifest": 0, "archive": 1, "signature": 2}[step]
		releaseID := previous[1].Release.ID
		path := fmt.Sprintf("https://uploads.github.com/%s/releases/%d/assets?name=%s", apiRepo, releaseID, assetNames[idx])
		b, e := gh(ctx, []string{"api", "--hostname", "github.com", "--method", "POST", path, "-H", "Content-Type: application/octet-stream", "--input", "-"}, assets[idx])
		if e != nil {
			return o, e
		}
		var asset Asset
		if e = json.Unmarshal(b, &asset); e != nil {
			return o, e
		}
		if asset.ID <= 0 || asset.Name != assetNames[idx] || asset.Size != int64(len(assets[idx])) || asset.State != "uploaded" || fmt.Sprint(asset.Uploader.ID) != a.Parameters["account_id"] {
			return o, errors.New("uploaded asset mismatch")
		}
		o.Asset = &asset
	case "verify", "verify-published":
		r, e := readRelease(ctx, previous[1].Release.ID)
		if e != nil {
			return o, e
		}
		if e = releaseMatches(r, a, step == "verify"); e != nil {
			return o, e
		}
		if e = validateInventory(a, r, previous, false); e != nil {
			return o, e
		}
		for _, name := range assetNames {
			for _, asset := range r.Assets {
				if asset.Name == name {
					b, err := readAsset(ctx, asset.ID)
					if err != nil {
						return o, err
					}
					o.AssetDigests = append(o.AssetDigests, hash(b))
				}
			}
		}
		o.Release = &r
	case "publish":
		var r Release
		e = api(ctx, "PATCH", fmt.Sprintf("%s/releases/%d", apiRepo, previous[1].Release.ID), map[string]any{"draft": false, "make_latest": "false"}, &r)
		if e != nil {
			return o, e
		}
		if e = releaseMatches(r, a, false); e != nil {
			return o, e
		}
		o.Release = &r
	default:
		return o, errors.New("unrecognized publication step")
	}
	return o, nil
}

// Reconcile is read-only. Matching remote bytes cannot establish who performed
// an ambiguous mutation. Without a durable successful response, preserve unknown
// outcome rather than guessing, replaying or claiming someone else's objects.
func (g GitHub) Reconcile(ctx context.Context, a contracts.ActionIntent, step string, previous []Observation) (Observation, error) {
	o, err := g.checkIdentity(ctx, a)
	if err != nil {
		return o, err
	}
	m, err := refs(ctx)
	if err != nil {
		return o, err
	}
	// This observation does not attribute the refs to our attempted push.
	o.Commit = m["refs/heads/main"]
	if len(previous) >= 2 && previous[1].Release != nil {
		r, err := readRelease(ctx, previous[1].Release.ID)
		if err != nil {
			return o, err
		}
		o.Release = &r
	}
	return o, errors.New("publication outcome remains indeterminate: exact dispatch result unavailable; no mutation retried")
}

// The release inventory must be the exact assets attributable to earlier steps.
func validateInventory(a contracts.ActionIntent, r Release, previous []Observation, draft bool) error {
	if draft {
		if len(r.Assets) != 0 {
			return errors.New("new draft contains assets")
		}
		return nil
	}
	if len(r.Assets) != 3 || len(previous) < 5 {
		return errors.New("incomplete release inventory evidence")
	}
	seen := map[string]bool{}
	for _, asset := range r.Assets {
		idx := -1
		for i, name := range assetNames {
			if asset.Name == name {
				idx = i
			}
		}
		if idx < 0 || seen[asset.Name] {
			return errors.New("unexpected or duplicate asset")
		}
		seen[asset.Name] = true
		want := previous[idx+2].Asset
		size, _ := strconv.ParseInt(a.Parameters[[]string{"manifest_size", "archive_size", "signature_size"}[idx]], 10, 64)
		if want == nil || asset.ID != want.ID || asset.Size != size || asset.State != "uploaded" || fmt.Sprint(asset.Uploader.ID) != a.Parameters["account_id"] {
			return errors.New("release asset identity changed")
		}
	}
	return nil
}

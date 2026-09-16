package goalspublication

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// Test the production subprocess adapter with local scripted transports. The
// stubs cannot contact GitHub; every unlisted API request is a test failure.
func scriptedTransport(t *testing.T, responses map[string]any, refOutput, pushOutput string) string {
	t.Helper()
	dir := t.TempDir()
	data, _ := json.Marshal(responses)
	must(t, os.WriteFile(filepath.Join(dir, "responses"), data, 0600))
	must(t, os.WriteFile(filepath.Join(dir, "refs"), []byte(refOutput), 0600))
	must(t, os.WriteFile(filepath.Join(dir, "push"), []byte(pushOutput), 0600))
	script := `#!/usr/bin/python3
import sys,os,json,hashlib
root=os.environ['GOALS_TEST_TRANSPORT'];args=sys.argv[1:];name=os.path.basename(sys.argv[0]);body=sys.stdin.buffer.read()
with open(root+'/calls','a') as f: f.write(json.dumps({'tool':name,'args':args,'body_sha256':hashlib.sha256(body).hexdigest(),'body':body.decode(errors='replace') if len(body)<4096 else None})+'\n')
if name=='git':
 if 'ls-remote' in args: print(open(root+'/refs').read(),end='')
 elif 'push' in args: print(open(root+'/push').read(),end='')
 elif 'init' in args or 'hash-object' in args: pass
 else: sys.exit(29)
else:
 if args[0]=='repo': key='repo-view'
 else:
  method=args[args.index('--method')+1];path=args[args.index('--method')+2];key=method+' '+path
 data=json.load(open(root+'/responses'))
 if key not in data: sys.exit(30)
 value=data[key]
 if isinstance(value,dict) and '__raw_file' in value: sys.stdout.buffer.write(open(value['__raw_file'],'rb').read())
 else: print(json.dumps(value),end='')
`
	for _, n := range []string{"gh", "git"} {
		must(t, os.WriteFile(filepath.Join(dir, n), []byte(script), 0700))
	}
	t.Setenv("GOALS_TEST_TRANSPORT", dir)
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	return dir
}
func identityResponses() map[string]any {
	return map[string]any{"GET " + apiRepo: map[string]any{"id": 123, "owner": map[string]any{"id": 456}, "full_name": "convergent-systems-co/praxis-packages", "permissions": map[string]bool{"push": true}}, "GET user": githubUser{789}, "repo-view": map[string]string{"url": "https://github.com/convergent-systems-co/praxis-packages"}}
}
func TestGitHubEmptyAndStableDestination(t *testing.T) {
	a := intent(t, time.Now(), Assets{})
	ctx := context.Background()
	for _, tc := range []struct {
		name, refs string
		existing   bool
		repo       any
		wantError  bool
	}{
		{name: "empty"}, {name: "nonempty", refs: "abc\trefs/heads/other\n", wantError: true}, {name: "existing-release", existing: true, wantError: true}, {name: "recreated-same-name", repo: map[string]any{"id": 124, "owner": map[string]any{"id": 456}, "full_name": "convergent-systems-co/praxis-packages", "permissions": map[string]bool{"push": true}}, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rs := identityResponses()
			if tc.repo != nil {
				rs["GET "+apiRepo] = tc.repo
			}
			rs["GET "+apiRepo+"/releases?per_page=1"] = []Release{}
			if tc.existing {
				rs["GET "+apiRepo+"/releases?per_page=1"] = []Release{{ID: 1}}
			}
			scriptedTransport(t, rs, tc.refs, "")
			err := (GitHub{}).Check(ctx, a, "refs", nil)
			if (err != nil) != tc.wantError {
				t.Fatalf("err=%v", err)
			}
		})
	}
}
func TestGitHubBoundedRefCreationAndNoMatchingObjectAdoption(t *testing.T) {
	a := intent(t, time.Now(), Assets{})
	for _, tc := range []struct {
		name, push string
		bad        bool
	}{{"two-new", "*\tnew-main\n*\tnew-tag\n", false}, {"one-matching", "=\texisting-main\n*\tnew-tag\n", true}, {"none-created", "", true}} {
		t.Run(tc.name, func(t *testing.T) {
			dir := scriptedTransport(t, identityResponses(), "", tc.push)
			_, err := (GitHub{}).Dispatch(context.Background(), a, "refs", nil, Assets{})
			if (err != nil) != tc.bad {
				t.Fatalf("err=%v", err)
			}
			b, e := os.ReadFile(filepath.Join(dir, "calls"))
			must(t, e)
			if !strings.Contains(string(b), "--atomic") || strings.Contains(string(b), "--force") || strings.Contains(string(b), "--mirror") {
				t.Fatal("unsafe push")
			}
		})
	}
}
func TestGitHubDraftAndPublicationSettings(t *testing.T) {
	a := intent(t, time.Now(), Assets{})
	p := []Observation{observation(a, "refs", nil)}
	draft := observation(a, "draft", p)
	p = append(p, draft)
	for _, s := range []string{"manifest", "archive", "signature"} {
		p = append(p, observation(a, s, p))
	}
	for _, s := range []string{"draft", "publish"} {
		t.Run(s, func(t *testing.T) {
			rs := identityResponses()
			path := "POST " + apiRepo + "/releases"
			if s == "publish" {
				path = "PATCH " + apiRepo + "/releases/10"
			}
			rs[path] = observation(a, s, p).Release
			dir := scriptedTransport(t, rs, "", "")
			_, err := (GitHub{}).Dispatch(context.Background(), a, s, p, Assets{})
			must(t, err)
			b, err := os.ReadFile(filepath.Join(dir, "calls"))
			must(t, err)
			lines := strings.Split(strings.TrimSpace(string(b)), "\n")
			var call struct{ Body string }
			must(t, json.Unmarshal([]byte(lines[len(lines)-1]), &call))
			var payload map[string]any
			must(t, json.Unmarshal([]byte(call.Body), &payload))
			if payload["make_latest"] != "false" || payload["draft"] != (s == "draft") {
				t.Fatal("ambient release defaults")
			}
			if s == "draft" && (payload["target_commitish"] != a.Parameters["commit"] || payload["prerelease"] != false || payload["generate_release_notes"] != false) {
				t.Fatal("unbound release settings")
			}
		})
	}
}
func TestGitHubReadbackActualBytesAndAssetIdentity(t *testing.T) {
	assets := exactAssets(t)
	a := intent(t, time.Now(), assets)
	prior := []Observation{}
	for _, step := range steps[:5] {
		prior = append(prior, observation(a, step, prior))
	}
	for _, tc := range []string{"valid", "wrong-bytes", "replaced-asset", "wrong-size", "moved-tag", "extra-ref"} {
		t.Run(tc, func(t *testing.T) {
			rs := identityResponses()
			r := *observation(a, "verify", prior).Release
			if tc == "replaced-asset" {
				r.Assets[0].ID++
			}
			if tc == "wrong-size" {
				r.Assets[0].Size++
			}
			rs["GET "+apiRepo+"/releases/10"] = r
			rs["GET "+apiRepo+"/git/commits/"+a.Parameters["commit"]] = map[string]any{"sha": a.Parameters["commit"], "tree": map[string]string{"sha": a.Parameters["tree"]}, "parents": []string{}}
			for i, n := range assetNames {
				file := filepath.Join(t.TempDir(), n)
				b := assets[i]
				if tc == "wrong-bytes" && i == 0 {
					b = []byte("substituted")
				}
				must(t, os.WriteFile(file, b, 0600))
				rs["GET "+apiRepo+"/releases/assets/"+[]string{"100", "101", "102"}[i]] = map[string]string{"__raw_file": file}
			}
			tag := a.Parameters["commit"]
			if tc == "moved-tag" {
				tag = strings.Repeat("f", 40)
			}
			refs := a.Parameters["commit"] + "\trefs/heads/main\n" + tag + "\trefs/tags/goals/v0.1.0\n"
			if tc == "extra-ref" {
				refs += tag + "\trefs/heads/extra\n"
			}
			scriptedTransport(t, rs, refs, "")
			err := (GitHub{}).Check(context.Background(), a, "verify", prior)
			if tc == "valid" {
				must(t, err)
				o, e := (GitHub{}).Dispatch(context.Background(), a, "verify", prior, assets)
				must(t, e)
				must(t, validateObservation(a, "verify", o, prior))
			} else if err == nil {
				t.Fatal("readback substitution accepted")
			}
		})
	}
}

func TestDispatchDiagnosticsAreBoundedAndSanitized(t *testing.T) {
	got := safeDiagnostic("HTTP 503 x-github-request-id: ABCDEF12 ghp_1234567890 secret=hunter2")
	if strings.Contains(got, "ghp_") || strings.Contains(got, "hunter2") || len(got) > maxDiagnosticBytes {
		t.Fatalf("unsafe diagnostic: %q", got)
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("secret was not redacted: %q", got)
	}
	if m := statusPattern.FindStringSubmatch(got); len(m) < 2 || m[1] != "503" {
		t.Fatalf("status not extractable from safe evidence: %q", got)
	}
}

func TestGitHubProcessOutcomeClassificationIsSafeAndBounded(t *testing.T) {
	t.Run("launch-failure", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("PATH", dir)
		_, err := gh(context.Background(), []string{"api", "--method", "POST", "repos/fixed/assets"}, []byte("protected-body"))
		var failure dispatchFailure
		if !errors.As(err, &failure) || failure.outcome.Process != "launch_failed" || failure.outcome.Class != "local_pre_dispatch_failure" {
			t.Fatalf("launch result not distinguished: %#v %v", failure, err)
		}
	})
	t.Run("provider-response", func(t *testing.T) {
		dir := t.TempDir()
		script := "#!/bin/sh\nprintf '%s' 'HTTP 422 x-github-request-id: ABCDEF12 token=ghp_1234567890 body=protected-data' >&2\nexit 1\n"
		must(t, os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0700))
		t.Setenv("PATH", dir)
		_, err := gh(context.Background(), []string{"api", "--method", "POST", "repos/fixed/assets"}, []byte("protected-request-body"))
		var failure dispatchFailure
		if !errors.As(err, &failure) || failure.outcome.Class != "provider_response" || failure.outcome.HTTPStatus != 422 || failure.outcome.RequestID != "ABCDEF12" {
			t.Fatalf("provider response missing: %+v %v", failure, err)
		}
		evidence := string(failure.Evidence())
		for _, secret := range []string{"ghp_", "protected-data", "protected-request-body", "token="} {
			if strings.Contains(evidence, secret) {
				t.Fatalf("sensitive transport content persisted: %s", evidence)
			}
		}
		if len(evidence) > 2048 {
			t.Fatal("dispatch evidence exceeded bound")
		}
	})
	t.Run("lost-response", func(t *testing.T) {
		dir := t.TempDir()
		must(t, os.WriteFile(filepath.Join(dir, "gh"), []byte("#!/bin/sh\nprintf '%s' 'connection reset after request secret=hidden' >&2\nexit 1\n"), 0700))
		t.Setenv("PATH", dir)
		_, err := gh(context.Background(), []string{"api", "--method", "POST", "repos/fixed/assets"}, nil)
		var failure dispatchFailure
		if !errors.As(err, &failure) || failure.outcome.Class != "ambiguous" || failure.outcome.Process != "started" || strings.Contains(string(failure.Evidence()), "hidden") {
			t.Fatalf("lost response was not safely ambiguous: %+v", failure)
		}
	})
}

func TestProviderDiagnosticsStructuredAndSanitized(t *testing.T) {
	raw := `{"message":"Validation Failed","documentation_url":"https://docs.github.com/rest?token=secret","errors":[{"resource":"ReleaseAsset","field":"name","code":"already_exists","value":"ghp_supersecretvalue"}]}`
	message, doc, errs := providerDiagnostics(raw)
	if message != "Validation Failed" {
		t.Fatalf("message not retained: %q", message)
	}
	if doc != "https://docs.github.com/rest" {
		t.Fatalf("documentation URL not normalized: %q", doc)
	}
	if len(errs) != 1 || errs[0].Resource != "ReleaseAsset" || errs[0].Field != "name" || errs[0].Code != "already_exists" {
		t.Fatalf("structured errors not retained safely: %+v", errs)
	}
	if strings.Contains(message+doc, "secret") || strings.Contains(message+doc, "ghp_") {
		t.Fatal("secret survived provider diagnostic sanitization")
	}
}

func TestProviderDiagnosticsBoundedMalformedAndCardinality(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"message":"`)
	b.WriteString(strings.Repeat("x", 5000))
	b.WriteString(`","errors":[`)
	for i := 0; i < 20; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`{"resource":"r","field":"f","code":"c"}`)
	}
	b.WriteString(`]}`)
	message, doc, errs := providerDiagnostics(b.String())
	if len(message) > maxDiagnosticBytes || len(errs) > 4 {
		t.Fatalf("provider diagnostics exceeded bounds: message=%d errors=%d", len(message), len(errs))
	}
	message, doc, errs = providerDiagnostics("not-json\x00with token=ghp_1234567890")
	if message != "" || doc != "" || len(errs) != 0 {
		t.Fatalf("malformed output produced structured diagnostics: %q %q %+v", message, doc, errs)
	}
}

func TestRecoveryGitHubRejectsEstablishedStateSubstitutionReadOnly(t *testing.T) {
	at := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	a, err := contracts.NewGoalsPublicationRecoveryIntent(contracts.GoalsRecoveryInput{CreatedAt: at, ExpiresAt: at.Add(time.Hour), Identity: strings.Repeat("e", 64), AccountID: 789, Sizes: [3]int64{1, 2, 3}, PredecessorRequestID: "goals-publication-request:old", PredecessorRequestDigest: "sha256:" + strings.Repeat("1", 64), PredecessorIntentID: "goals-initial-publication:old", PredecessorIntentDigest: "sha256:" + strings.Repeat("2", 64), AbandonmentEventID: "goals-publication-abandoned:old", AbandonmentDigest: "sha256:" + strings.Repeat("3", 64)})
	must(t, err)
	for _, tc := range []string{"valid", "repository", "owner", "main", "tag", "commit-tree", "release-id", "release-settings", "nonempty-inventory", "extra-ref"} {
		t.Run(tc, func(t *testing.T) {
			responses := identityResponses()
			responses["GET "+apiRepo] = map[string]any{"id": 1372388187, "owner": map[string]any{"id": 263966243}, "full_name": contracts.GoalsPublicationRepository, "permissions": map[string]bool{"push": true}}
			responses["GET user"] = githubUser{789}
			commit := map[string]any{"sha": contracts.GoalsRecoveryCommit, "tree": map[string]string{"sha": contracts.GoalsRecoveryTree}, "parents": []any{}}
			if tc == "commit-tree" {
				commit["tree"] = map[string]string{"sha": strings.Repeat("f", 40)}
			}
			responses["GET "+apiRepo+"/git/commits/"+contracts.GoalsRecoveryCommit] = commit
			refText := contracts.GoalsRecoveryCommit + "\trefs/heads/main\n" + contracts.GoalsRecoveryCommit + "\trefs/tags/" + contracts.GoalsPublicationTag + "\n"
			if tc == "main" {
				refText = strings.Repeat("a", 40) + "\trefs/heads/main\n" + contracts.GoalsRecoveryCommit + "\trefs/tags/" + contracts.GoalsPublicationTag + "\n"
			}
			if tc == "tag" {
				refText = contracts.GoalsRecoveryCommit + "\trefs/heads/main\n" + strings.Repeat("a", 40) + "\trefs/tags/" + contracts.GoalsPublicationTag + "\n"
			}
			if tc == "extra-ref" {
				refText += contracts.GoalsRecoveryCommit + "\trefs/heads/other\n"
			}
			if tc == "repository" {
				responses["GET "+apiRepo] = map[string]any{"id": 999, "owner": map[string]any{"id": 263966243}, "full_name": contracts.GoalsPublicationRepository, "permissions": map[string]bool{"push": true}}
			}
			if tc == "owner" {
				responses["GET "+apiRepo] = map[string]any{"id": 1372388187, "owner": map[string]any{"id": 999}, "full_name": contracts.GoalsPublicationRepository, "permissions": map[string]bool{"push": true}}
			}
			release := Release{ID: 389997269, Tag: contracts.GoalsPublicationTag, Target: contracts.GoalsRecoveryCommit, Name: contracts.GoalsPublicationPackage, Body: "Exact signed Goals initial publication; intent " + contracts.GoalsRecoveryNonce, Draft: true, Author: githubUser{789}}
			if tc == "release-id" {
				release.ID++
			}
			if tc == "release-settings" {
				release.Prerelease = true
			}
			if tc == "nonempty-inventory" {
				release.Assets = []Asset{{ID: 100, Name: "unexpected", Size: 10, State: "uploaded", Uploader: githubUser{789}}}
			}
			responses["GET "+apiRepo+"/releases/389997269"] = release
			dir := scriptedTransport(t, responses, refText, "")
			err := (RecoveryGitHub{}).Check(context.Background(), a, "manifest", nil)
			if tc == "valid" {
				must(t, err)
			} else if err == nil {
				t.Fatal("mutated established state accepted")
			}
			calls, _ := os.ReadFile(filepath.Join(dir, "calls"))
			if strings.Contains(string(calls), "POST") || strings.Contains(string(calls), "PATCH") {
				t.Fatalf("precondition check mutated GitHub: %s", calls)
			}
		})
	}
}

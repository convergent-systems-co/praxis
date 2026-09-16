package goalspublication

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

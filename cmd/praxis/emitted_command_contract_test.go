package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

var placeholderPattern = regexp.MustCompile(`<[^>]*>|\.\.\.|\|`)

// executeEmitted runs one emitted next-action command exactly as the public
// CLI would dispatch it: `praxis goals-lifecycle ...` goes through the
// dynamic installed-entry-point dispatcher and `praxis authority decide`
// through the interactive owner decision with its typed confirmation. The
// command text is a product contract: it must be a complete shell command
// with full identities and no placeholders.
func executeEmitted(t *testing.T, ctx context.Context, governed func(string) string, command string) ([]byte, error) {
	t.Helper()
	if placeholderPattern.MatchString(command) {
		t.Fatalf("emitted command carries a placeholder: %s", command)
	}
	fields := strings.Fields(command)
	if len(fields) < 2 || fields[0] != "praxis" {
		t.Fatalf("emitted command is not a praxis invocation: %s", command)
	}
	if fields[1] == "authority" && len(fields) > 2 && fields[2] == "decide" {
		var digest, outcome string
		for i := 3; i+1 < len(fields); i++ {
			switch fields[i] {
			case "--request":
				digest = fields[i+1]
			case "--outcome":
				outcome = fields[i+1]
			}
		}
		var out bytes.Buffer
		err := runAuthorityDecideWithTerminal([]string{"--request", digest, "--outcome", outcome}, governed, strings.NewReader("DECIDE-"+strings.ToUpper(outcome)+" "+digest+"\n"), &out, true)
		return out.Bytes(), err
	}
	var runErr error
	out := captureStdout(t, func() { runErr = runDynamicInvocation(ctx, fields[1:], governed) })
	return out, runErr
}

func inspectGoal(t *testing.T, ctx context.Context, governed func(string) string, goalID, version string) map[string]any {
	t.Helper()
	out := captureStdout(t, func() {
		if err := runDynamicInvocation(ctx, []string{"goals-lifecycle", "--operation=inspect", "--goal-id=" + goalID, "--goal-version=" + version}, governed); err != nil {
			t.Fatalf("inspect %s/%s: %v", goalID, version, err)
		}
	})
	var result map[string]any
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("inspect output: %v %s", err, out)
	}
	return result
}

func emitted(t *testing.T, value any, path ...string) string {
	t.Helper()
	current := value
	for _, key := range path {
		switch node := current.(type) {
		case map[string]any:
			current = node[key]
		case []any:
			if len(node) != 1 {
				t.Fatalf("expected exactly one entry at %v, got %d", path, len(node))
			}
			current = node[0].(map[string]any)[key]
		default:
			t.Fatalf("unexpected node at %v: %T", path, current)
		}
	}
	command, ok := current.(string)
	if !ok || command == "" {
		t.Fatalf("emitted command missing at %v", path)
	}
	return command
}

// TestEmittedNextActionsAreExecutableProductContracts drives a Goal from
// import to a drivable generation by executing only the next-action commands
// the product emits, then proves the adversarial properties of the option
// selectors. This is the regression test for the class of defect where
// inspect names a next step the public CLI cannot run as rendered.
func TestEmittedNextActionsAreExecutableProductContracts(t *testing.T) {
	ctx := context.Background()
	governed, _, _, dir := lifecycleFixture(t, ctx)
	const goalID = "goal:pending-surface"
	state := inspectGoal(t, ctx, governed, goalID, "1")
	if state["drivable"] != false || !strings.Contains(fmt.Sprint(state["next_step"]), "propose") {
		t.Fatalf("fresh Goal must name propose as the next step: %v", state)
	}
	// The planner's decomposition is the one intent-bearing input.
	proposeFixture(t, ctx, governed, dir, "emitted")
	state = inspectGoal(t, ctx, governed, goalID, "1")
	reviewCmd := emitted(t, state, "proposals", "review_accept_with")
	reviewOut, err := executeEmitted(t, ctx, governed, reviewCmd)
	if err != nil {
		t.Fatalf("emitted review command failed: %s: %v", reviewCmd, err)
	}
	var reviewed map[string]any
	if err := json.Unmarshal(reviewOut, &reviewed); err != nil {
		t.Fatal(err)
	}
	if by := reviewed["review"].(map[string]any)["reviewed_by"].(map[string]any); by["kind"] != "human" || !strings.HasPrefix(by["id"].(string), "installation-owner:") {
		t.Fatalf("default reviewer must be the installation owner: %v", by)
	}
	state = inspectGoal(t, ctx, governed, goalID, "1")
	requestCmd := emitted(t, state, "reviews", "request_with")
	if requestCmd != emitted(t, reviewed, "request_with") {
		t.Fatalf("review output and inspect must emit the same request command")
	}
	requestOut, err := executeEmitted(t, ctx, governed, requestCmd)
	if err != nil {
		t.Fatalf("emitted request command failed: %s: %v", requestCmd, err)
	}
	var requested map[string]any
	if err := json.Unmarshal(requestOut, &requested); err != nil {
		t.Fatal(err)
	}
	requestDigest := requested["request_digest"].(string)
	state = inspectGoal(t, ctx, governed, goalID, "1")
	if state["pending_authority"] == nil {
		t.Fatalf("request must be pending: %v", state)
	}
	decideCmd := emitted(t, state, "authority_requests", "resolve_with")
	if !strings.Contains(decideCmd, requestDigest) || decideCmd != emitted(t, requested, "resolve_with") {
		t.Fatalf("resolve command must carry the full request digest: %s", decideCmd)
	}
	// The proposer (a model) cannot execute the decision: only the owner can,
	// interactively. Executing the emitted decide command is the owner's act.
	if _, err := executeEmitted(t, ctx, governed, decideCmd); err != nil {
		t.Fatalf("emitted decide command failed: %s: %v", decideCmd, err)
	}
	state = inspectGoal(t, ctx, governed, goalID, "1")
	acceptCmd := emitted(t, state, "authority_requests", "accept_with")
	if !strings.Contains(acceptCmd, "--request-digest="+requestDigest) {
		t.Fatalf("accept command must carry the full request digest: %s", acceptCmd)
	}
	acceptOut, err := executeEmitted(t, ctx, governed, acceptCmd)
	if err != nil {
		t.Fatalf("emitted accept command failed: %s: %v", acceptCmd, err)
	}
	var accepted map[string]any
	if err := json.Unmarshal(acceptOut, &accepted); err != nil {
		t.Fatal(err)
	}
	// Acceptance replay is idempotent and truthful.
	replayOut, err := executeEmitted(t, ctx, governed, acceptCmd)
	if err != nil || !bytes.Contains(replayOut, []byte(accepted["acceptance_ref"].(string))) {
		t.Fatalf("acceptance replay must return the same durable acceptance: %v %s", err, replayOut)
	}
	state = inspectGoal(t, ctx, governed, goalID, "1")
	attachCmd := emitted(t, state, "acceptances", "attach_with")
	if attachCmd != emitted(t, accepted, "attach_with") {
		t.Fatal("accept output and inspect must emit the same attach command")
	}
	attachOut, err := executeEmitted(t, ctx, governed, attachCmd)
	if err != nil {
		t.Fatalf("emitted attach command failed: %s: %v", attachCmd, err)
	}
	var attached map[string]any
	if err := json.Unmarshal(attachOut, &attached); err != nil {
		t.Fatal(err)
	}
	if attached["goal_version"] != "2" {
		t.Fatalf("attach must create generation 2: %v", attached)
	}
	// Attachment replay is idempotent: same generation, no third generation.
	replayOut, err = executeEmitted(t, ctx, governed, attachCmd)
	if err != nil || !bytes.Contains(replayOut, []byte(`"replay": true`)) || !bytes.Contains(replayOut, []byte(`"goal_version": "2"`)) {
		t.Fatalf("attach replay must report the existing generation: %v %s", err, replayOut)
	}
	if _, err := executeEmitted(t, ctx, governed, "praxis goals-lifecycle --operation=inspect --goal-id="+goalID+" --goal-version=3"); err == nil {
		t.Fatal("attach replay must not create a third generation")
	}
	state = inspectGoal(t, ctx, governed, goalID, "2")
	if state["drivable"] != true {
		t.Fatalf("generation 2 must be drivable: %v", state)
	}
	template := state["drive_template"].(map[string]any)
	if !strings.Contains(template["command"].(string), "--goal-version=2") {
		t.Fatalf("drive template must name the exact generation: %v", template)
	}

	// Adversarial selectors.
	wrong := "sha256:" + strings.Repeat("0", 64)
	if _, err := executeEmitted(t, ctx, governed, acceptCommand(wrong)); err == nil {
		t.Fatal("accept with an unknown request digest must fail closed")
	}
	if _, err := executeEmitted(t, ctx, governed, attachCommand(goalID, "1", "acceptance:unknown")); err == nil {
		t.Fatal("attach with an unknown acceptance must fail closed")
	}
	if _, err := executeEmitted(t, ctx, governed, attachCommand(goalID, "2", accepted["acceptance_ref"].(string))); err == nil {
		t.Fatal("attach onto a generation that already carries a plan must fail closed")
	}
	// A second proposal reviewed and requested, then rejected: never accepted,
	// and the first Goal's lineage is untouched.
	second := proposeFixture(t, ctx, governed, dir, "second")
	secondReviewOut, err := executeEmitted(t, ctx, governed, reviewCommand(second, string(contracts.ReviewAcceptableForAuthority)))
	if err != nil {
		t.Fatal(err)
	}
	var secondReview map[string]any
	if err := json.Unmarshal(secondReviewOut, &secondReview); err != nil {
		t.Fatal(err)
	}
	secondRequestOut, err := executeEmitted(t, ctx, governed, emitted(t, secondReview, "request_with"))
	if err != nil {
		t.Fatal(err)
	}
	var secondRequest map[string]any
	if err := json.Unmarshal(secondRequestOut, &secondRequest); err != nil {
		t.Fatal(err)
	}
	if _, err := executeEmitted(t, ctx, governed, emitted(t, secondRequest, "reject_with")); err != nil {
		t.Fatalf("emitted reject command failed: %v", err)
	}
	if _, err := executeEmitted(t, ctx, governed, acceptCommand(secondRequest["request_digest"].(string))); err == nil {
		t.Fatal("a rejected request must not be accepted")
	}
	// The review selector refuses a digest the reviewer cannot bind.
	if _, err := executeEmitted(t, ctx, governed, reviewCommand(wrong, string(contracts.ReviewAcceptableForAuthority))); err == nil {
		t.Fatal("review of an unknown proposal must fail closed")
	}
	// Legacy full documents remain accepted but are never emitted.
	for _, entry := range state["proposals"].([]any) {
		for key, value := range entry.(map[string]any) {
			if strings.HasSuffix(key, "_with") && placeholderPattern.MatchString(value.(string)) {
				t.Fatalf("inspect emitted a placeholder in %s: %s", key, value)
			}
		}
	}
}

package goaldrive

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWorkerContextCarriesTheExecutionEnvelope is the #170 contract: the
// worker is told the non-interactive execution envelope it actually runs
// under (tool patterns, one command per shell call, denials without a
// prompt), so it plans within it instead of discovering denials mid-turn
// (Weather II turns 3, 4, 6 and 7 each reported auto-denied commands and
// manual re-runs).
func TestWorkerContextCarriesTheExecutionEnvelope(t *testing.T) {
	req := contractRequest("unit:one")
	envelope := ClaudeSubscriptionEnvelope()
	if envelope == nil || envelope.Interactive || len(envelope.Tools) == 0 {
		t.Fatalf("the claude profile declares a non-interactive envelope with tool patterns: %+v", envelope)
	}
	ctx, err := BuildWorkerContext(req.GoalBaseline, req.WorkCandidates, nil, "unit:one", WorkerRepositoryContext{Path: "/work", Branch: "main", StartHead: "abc123"}, ClaudeSubscriptionCapabilities(), false, "", nil, envelope)
	if err != nil {
		t.Fatal(err)
	}
	if ctx.Authority.Envelope == nil || ctx.Authority.Envelope.Interactive || !containsString(ctx.Authority.Envelope.Tools, "Bash(git commit:*)") {
		t.Fatalf("RED #170: the worker context must carry the execution envelope: %+v", ctx.Authority)
	}
	prompt, err := providerPrompt(WorkerRequest{GoalID: "goal:contract", GoalVersion: "2", TurnID: "t", ChildObjective: "unit:one", GraphID: "g", GraphVersion: "1", StartHead: "abc123", Context: ctx})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"## Execution envelope", "Bash(git commit:*)", "one allowed command per shell call", "denied without a prompt", "Bash(./.praxis/validate:*)"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt lacks %q:\n%s", want, prompt)
		}
	}
	if codex := CodexSubscriptionEnvelope(); codex == nil || codex.Interactive {
		t.Fatalf("the codex profile declares its sandbox envelope: %+v", codex)
	}
}

// TestDispatchRecordsTheExecutionEnvelope: the envelope a turn ran under is
// durable evidence (activity execution.envelope before the provider
// starts), and the worker actually receives it in its prompt.
func TestDispatchRecordsTheExecutionEnvelope(t *testing.T) {
	ctx := context.Background()
	root, workDir := contractRepo(t)
	promptPath := filepath.Join(root, "prompt.txt")
	worker := ProviderCLIWorker{ProviderID: "local", Dir: workDir, Command: []string{"/bin/sh", "-c", "cat > " + promptPath + " && printf 'ok\\n' > ok.txt && git add ok.txt && git commit -q -m ok"}, Envelope: &WorkerExecutionEnvelope{Interactive: false, Tools: []string{"Bash(git commit:*)", "Bash(sh:*)"}, ShellPolicy: "one allowed command per shell call; pipes, loops and compound commands are denied", DenialPolicy: "a call outside the envelope is denied without a prompt; plan within it"}}
	controller, activity := contractController(t, root, worker)
	record, err := controller.ExecuteTurnWithRepository(ctx, contractRequest(""), GitRepository{Dir: workDir, Remote: "origin", Branch: "main"})
	if err != nil || !record.Progress {
		t.Fatalf("turn: %+v %v", record, err)
	}
	types := strings.Join(activityTypesFor(t, activity, "inv-contract", "inv-contract:turn:1"), ",")
	if !strings.Contains(types, "work.selected,execution.envelope,action.started") {
		t.Fatalf("RED #170: the execution envelope must be durable before the provider starts: %s", types)
	}
	events, err := activity.Load(ctx, "inv-contract", "inv-contract:turn:1", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.Type == ActivityExecutionEnvelope {
			if event.Data["interactive"] != "false" || !strings.Contains(event.Data["tools"], "Bash(git commit:*)") {
				t.Fatalf("envelope evidence: %v", event.Data)
			}
		}
	}
	prompt, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(prompt), "## Execution envelope") || !strings.Contains(string(prompt), "Bash(sh:*)") {
		t.Fatalf("the worker must receive its envelope:\n%s", prompt)
	}
}

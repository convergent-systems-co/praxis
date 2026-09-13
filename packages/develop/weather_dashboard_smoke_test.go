package develop

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/convergent-systems-co/praxis/internal/client"
	"github.com/convergent-systems-co/praxis/internal/kernel"
)

type smokeTraceObserver struct{}

func (smokeTraceObserver) ObserveRun(_ context.Context, observation kernel.RunObservation) error {
	body, _ := json.Marshal(observation)
	fmt.Printf("PRAXIS_TRACE %s\n", body)
	return nil
}

type weatherDashboardSmokeExecutor struct {
	artifactPath string
}

func (e *weatherDashboardSmokeExecutor) ExecuteNode(_ context.Context, graph kernel.GraphDef, node kernel.NodeDef, _ *kernel.RunExecution) (kernel.NodeResult, error) {
	var result kernel.NodeResult
	if graph.ID == "praxis.package.goals.default" {
		switch node.ID {
		case "intent": result = kernel.NodeResult{Outcome: "captured", Evidence: []string{"intent:build-simple-weather-dashboard"}}
		case "frame": result = kernel.NodeResult{Outcome: "rigorous", Evidence: []string{"rigor:standard"}}
		case "discover": result = kernel.NodeResult{Outcome: "ready", Evidence: []string{"evidence:mock-weather-data"}}
		case "variance": result = kernel.NodeResult{Outcome: "decide", Evidence: []string{"variance:none-material"}}
		case "decide": result = kernel.NodeResult{Outcome: "ready", Evidence: []string{"decision:single-page-static-dashboard"}}
		case "model": result = kernel.NodeResult{Outcome: "ready", Evidence: []string{"model:city-temp-condition-humidity-wind"}}
		case "specify": result = kernel.NodeResult{Outcome: "ready", Evidence: []string{"spec:responsive-html-no-external-api"}}
		case "plan": result = kernel.NodeResult{Outcome: "ready", Evidence: []string{"plan:generate-validate-integrate"}}
		case "baseline": result = kernel.NodeResult{Outcome: "stored", Evidence: []string{"goal-baseline:sha256:weather-dashboard-smoke"}}
		default: result = kernel.NodeResult{Outcome: "blocked"}
		}
	} else {
		switch node.ID {
		case "discover": result = kernel.NodeResult{Outcome: "ready", Evidence: []string{"workspace:temp-weather-dashboard"}}
		case "classify": result = kernel.NodeResult{Outcome: "goals", Evidence: []string{"classification:architected"}}
		case "materialize": result = kernel.NodeResult{Outcome: "ready", Evidence: []string{"materialized:weather-dashboard-spec"}}
		case "plan": result = kernel.NodeResult{Outcome: "ready", Evidence: []string{"local-plan:index.html"}}
		case "prepare": result = kernel.NodeResult{Outcome: "ready", Evidence: []string{"capability:workspace-write-prepared"}}
		case "implement":
			html := `<!doctype html><html><head><meta charset="utf-8"><title>Weather Dashboard</title></head><body><main><h1>Shenandoah Weather</h1><section id="current"><strong>72°F</strong><span>Sunny</span><span>Humidity 41%</span><span>Wind NW 8 mph</span></section><small>Deterministic smoke-test data</small></main></body></html>`
			if err := os.WriteFile(e.artifactPath, []byte(html), 0o644); err != nil { return kernel.NodeResult{}, err }
			sum := sha256.Sum256([]byte(html))
			result = kernel.NodeResult{Outcome: "done", Evidence: []string{"artifact:" + e.artifactPath, "artifact-sha256:" + hex.EncodeToString(sum[:])}}
		case "validate":
			body, err := os.ReadFile(e.artifactPath); if err != nil { return kernel.NodeResult{}, err }
			if len(body) == 0 { return kernel.NodeResult{Outcome: "repair"}, nil }
			result = kernel.NodeResult{Outcome: "pass", Evidence: []string{"validation:artifact-readable", "validation:non-empty-html"}}
		case "review": result = kernel.NodeResult{Outcome: "pass", Evidence: []string{"review:requirements-satisfied"}}
		case "integrate": result = kernel.NodeResult{Outcome: "done", Evidence: []string{"integration:local-artifact-complete"}}
		default: result = kernel.NodeResult{Outcome: "blocked"}
		}
	}
	body, _ := json.Marshal(map[string]any{"graph": graph.ID, "node": node.ID, "outcome": result.Outcome, "evidence": result.Evidence})
	fmt.Printf("PRAXIS_EXECUTOR %s\n", body)
	return result, nil
}

func TestSmokeWeatherDashboardFullTrace(t *testing.T) {
	input := `/praxis develop weather dashboard --dashboard --mode=auto --workspace=/tmp/weather-dashboard`
	inv, err := client.ParseSlashInvocation(input); if err != nil { t.Fatal(err) }
	invBody, _ := json.Marshal(inv); fmt.Printf("PRAXIS_INVOCATION %s\n", invBody)

	registry, err := client.NewRegistry([]contracts.InvocationContract{InvocationContract()}); if err != nil { t.Fatal(err) }
	contract, options, err := registry.Resolve(inv); if err != nil { t.Fatal(err) }
	resolved, _ := json.Marshal(map[string]any{"entry_point": contract.EntryPointID, "graph_id": contract.GraphID, "graph_version": contract.GraphVersion, "options": options, "arguments": inv.Arguments})
	fmt.Printf("PRAXIS_RESOLUTION %s\n", resolved)

	artifact := filepath.Join(t.TempDir(), "index.html")
	delegate := &weatherDashboardSmokeExecutor{artifactPath: artifact}
	executor, err := NewExecutor(delegate); if err != nil { t.Fatal(err) }
	run := &kernel.RunExecution{RunID: "smoke-weather-dashboard-001", State: kernel.RunQueued}
	if err := kernel.RunObserved(context.Background(), Graph(), run, executor, smokeTraceObserver{}); err != nil { t.Fatal(err) }
	if run.State != kernel.RunSucceeded { t.Fatalf("expected succeeded, got %s", run.State) }
	body, err := os.ReadFile(artifact); if err != nil { t.Fatal(err) }
	final, _ := json.Marshal(run); fmt.Printf("PRAXIS_FINAL_RUN %s\n", final)
	fmt.Printf("PRAXIS_ARTIFACT_PATH %s\n", artifact)
	fmt.Printf("PRAXIS_ARTIFACT_BEGIN\n%s\nPRAXIS_ARTIFACT_END\n", string(body))
}

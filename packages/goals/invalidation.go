package goals

import (
	"errors"
	"sort"
)

type Dependency struct {
	From string
	To   string
}

type ArtifactGraph struct {
	Artifacts    map[string]ArtifactRef
	Dependencies []Dependency
}

func (g ArtifactGraph) Validate() error {
	for id, artifact := range g.Artifacts {
		if id == "" || artifact.ID == "" || id != artifact.ID {
			return errors.New("artifact graph keys must match non-empty artifact ids")
		}
	}
	for _, d := range g.Dependencies {
		if d.From == "" || d.To == "" { return errors.New("dependency endpoints are required") }
		if _, ok := g.Artifacts[d.From]; !ok { return errors.New("dependency source is not in artifact graph") }
		if _, ok := g.Artifacts[d.To]; !ok { return errors.New("dependency target is not in artifact graph") }
	}
	return nil
}

// Invalidate returns the deterministic dependency closure that must be
// reconsidered when any starting artifact/assumption changes. Only downstream
// dependents are invalidated; unrelated reasoning remains reusable.
func (g ArtifactGraph) Invalidate(changed []string) ([]string, error) {
	if err := g.Validate(); err != nil { return nil, err }
	adj := map[string][]string{}
	for _, d := range g.Dependencies { adj[d.From] = append(adj[d.From], d.To) }
	invalid := map[string]struct{}{}
	queue := make([]string, 0, len(changed))
	for _, id := range changed {
		if _, ok := g.Artifacts[id]; !ok { return nil, errors.New("changed artifact is not in artifact graph") }
		if _, seen := invalid[id]; seen { continue }
		invalid[id] = struct{}{}; queue = append(queue,id)
	}
	for len(queue) > 0 {
		id := queue[0]; queue = queue[1:]
		for _, next := range adj[id] {
			if _, seen := invalid[next]; seen { continue }
			invalid[next] = struct{}{}; queue = append(queue,next)
		}
	}
	out := make([]string,0,len(invalid))
	for id := range invalid { out = append(out,id) }
	sort.Strings(out)
	return out,nil
}

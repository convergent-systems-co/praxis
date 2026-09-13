package goals

import (
	"reflect"
	"testing"
)

func TestArtifactInvalidationOnlyPropagatesToDependents(t *testing.T) {
	g := ArtifactGraph{
		Artifacts: map[string]ArtifactRef{
			"decision-auth": {ID:"decision-auth",Role:"decision"},
			"model-auth": {ID:"model-auth",Role:"model"},
			"spec-auth": {ID:"spec-auth",Role:"specification"},
			"plan-auth": {ID:"plan-auth",Role:"plan"},
			"decision-ui": {ID:"decision-ui",Role:"decision"},
			"spec-ui": {ID:"spec-ui",Role:"specification"},
		},
		Dependencies: []Dependency{
			{From:"decision-auth",To:"model-auth"},
			{From:"model-auth",To:"spec-auth"},
			{From:"spec-auth",To:"plan-auth"},
			{From:"decision-ui",To:"spec-ui"},
		},
	}
	got, err := g.Invalidate([]string{"model-auth"})
	if err != nil { t.Fatal(err) }
	want := []string{"model-auth","plan-auth","spec-auth"}
	if !reflect.DeepEqual(got,want) { t.Fatalf("got %v want %v",got,want) }
}

func TestArtifactInvalidationHandlesCyclesWithoutExplosion(t *testing.T) {
	g := ArtifactGraph{Artifacts:map[string]ArtifactRef{"a":{ID:"a"},"b":{ID:"b"}},Dependencies:[]Dependency{{From:"a",To:"b"},{From:"b",To:"a"}}}
	got,err:=g.Invalidate([]string{"a"}); if err!=nil { t.Fatal(err) }
	want:=[]string{"a","b"}; if !reflect.DeepEqual(got,want) { t.Fatalf("got %v want %v",got,want) }
}

func TestUnknownChangedArtifactFailsClosed(t *testing.T) {
	g:=ArtifactGraph{Artifacts:map[string]ArtifactRef{"a":{ID:"a"}}}
	if _,err:=g.Invalidate([]string{"missing"}); err==nil { t.Fatal("unknown changed artifact must fail") }
}

package kernel

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

type GraphRegistry struct {
	mu     sync.RWMutex
	graphs map[string]GraphDef
}

func NewGraphRegistry(graphs ...GraphDef) (*GraphRegistry, error) {
	r := &GraphRegistry{graphs: map[string]GraphDef{}}
	for _, g := range graphs {
		if err := r.Register(g); err != nil { return nil, err }
	}
	return r,nil
}

func graphRegistryKey(id,version string) string { return id+"@"+version }

func (r *GraphRegistry) Register(g GraphDef) error {
	if r==nil { return errors.New("graph registry is required") }
	if err:=g.Validate(); err!=nil { return err }
	key:=graphRegistryKey(g.ID,g.Version)
	r.mu.Lock(); defer r.mu.Unlock()
	if _,exists:=r.graphs[key]; exists { return fmt.Errorf("graph %s already registered",key) }
	r.graphs[key]=g
	return nil
}

func (r *GraphRegistry) Resolve(ref SubgraphRef) (GraphDef,error) {
	if r==nil { return GraphDef{},errors.New("graph registry is required") }
	if err:=ref.Validate(); err!=nil { return GraphDef{},err }
	r.mu.RLock(); defer r.mu.RUnlock()
	g,ok:=r.graphs[graphRegistryKey(ref.GraphID,ref.GraphVersion)]
	if !ok { return GraphDef{},fmt.Errorf("graph %s@%s is not registered",ref.GraphID,ref.GraphVersion) }
	return g,nil
}

// LocalSubgraphRuntime executes exact registered child graphs in-process. It is
// intentionally limited to child runs that complete or suspend within the call;
// durable suspended-child recovery can be supplied by another SubgraphRuntime
// without changing parent graph semantics.
type LocalSubgraphRuntime struct {
	Registry *GraphRegistry
	Executor NodeExecutor
	Observer RunObserver
}

func (r LocalSubgraphRuntime) ExecuteSubgraph(ctx context.Context, _ GraphDef, parentNode NodeDef, parentRun *RunExecution, ref SubgraphRef) (RunState,[]string,*Suspension,error) {
	if r.Registry==nil || r.Executor==nil { return "",nil,nil,errors.New("local subgraph registry and executor are required") }
	if parentRun==nil || parentRun.RunID=="" { return "",nil,nil,errors.New("parent run identity is required") }
	childGraph,err:=r.Registry.Resolve(ref); if err!=nil { return "",nil,nil,err }
	childID:=parentRun.RunID+"/"+parentNode.ID+"/"+ref.GraphID+"@"+ref.GraphVersion
	child:=&RunExecution{RunID:childID,GraphID:childGraph.ID,GraphVersion:childGraph.Version,State:RunQueued}
	err=RunObserved(ctx,childGraph,child,r.Executor,r.Observer)
	if errors.Is(err,ErrRunSuspended) {
		if child.PendingWait==nil { return "",nil,nil,errors.New("suspended child run has no wait reference") }
		wait:=*child.PendingWait
		return child.State,append([]string(nil),child.Evidence...),&wait,nil
	}
	if err!=nil { return child.State,append([]string(nil),child.Evidence...),nil,err }
	if !child.State.Terminal() { return child.State,nil,nil,fmt.Errorf("child graph returned nonterminal state %q",child.State) }
	return child.State,append([]string(nil),child.Evidence...),nil,nil
}

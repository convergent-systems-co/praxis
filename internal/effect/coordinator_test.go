package effect

import (
	"context"
	"errors"
	"testing"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type fakeRevalidator struct{ err error }
func (f fakeRevalidator) Revalidate(context.Context, contracts.ActionIntent, AuthorizationSnapshot) error { return f.err }

type fakePreconditions struct{ err error }
func (f fakePreconditions) Check(context.Context, contracts.ActionIntent) error { return f.err }

type fakeDispatcher struct{ called bool }
func (f *fakeDispatcher) Dispatch(context.Context, contracts.ActionIntent, string) (Result, error) {
	f.called = true
	return Result{ObservedState: "ok"}, nil
}

func testIntent() contracts.ActionIntent {
	return contracts.ActionIntent{Version: "v1", ID: "i1", Actor: contracts.PrincipalRef{ID: "a1", Kind: "agent"}, Operation: "write", Target: "file:x", Scope: "workspace:1"}
}

func TestCommitRefusesMutatedIntent(t *testing.T) {
	intent := testIntent()
	d, _ := intent.Digest()
	dispatcher := &fakeDispatcher{}
	c := Coordinator{Revalidator: fakeRevalidator{}, Preconditions: fakePreconditions{}, Dispatcher: dispatcher}
	intent.Target = "file:y"
	_, err := c.Commit(context.Background(), intent, AuthorizationSnapshot{IntentDigest: d}, "k1")
	if err == nil || dispatcher.called { t.Fatal("mutated intent must not dispatch") }
}

func TestCommitRevalidatesImmediatelyBeforeDispatch(t *testing.T) {
	intent := testIntent()
	d, _ := intent.Digest()
	dispatcher := &fakeDispatcher{}
	c := Coordinator{Revalidator: fakeRevalidator{err: errors.New("revoked")}, Preconditions: fakePreconditions{}, Dispatcher: dispatcher}
	_, err := c.Commit(context.Background(), intent, AuthorizationSnapshot{IntentDigest: d}, "k1")
	if err == nil || dispatcher.called { t.Fatal("revoked authority must not dispatch") }
}

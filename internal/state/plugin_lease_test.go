package state

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/capability"
	"github.com/convergent-systems-co/praxis/internal/plugin"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func insertBoundPluginLease(t *testing.T, ctx context.Context, db execer, now time.Time) {
	t.Helper()
	ops, _ := json.Marshal([]string{"read"})
	_, err := db.ExecContext(ctx, `INSERT INTO capability_leases(lease_id,principal_id,principal_kind,capability,operations_json,scope,issued_at,remaining_uses,bound_instance_id,bound_session_id) VALUES(?,?,?,?,?,?,?,?,?,?)`, "lease-1", "plugin-1", "plugin", "workspace.search.text", ops, "workspace:1", now.Format(time.RFC3339Nano), 1, "instance-1", "session-1")
	if err != nil { t.Fatal(err) }
}

type execer interface { ExecContext(context.Context, string, ...any) (sql.Result, error) }

func TestConsumePluginLeaseBindsInstanceAndConsumesUse(t *testing.T) {
	ctx:=context.Background(); db,err:=OpenSQLite(ctx,filepath.Join(t.TempDir(),"praxis.db")); if err!=nil { t.Fatal(err) }; defer db.Close()
	now:=time.Now().UTC(); insertBoundPluginLease(t,ctx,db,now)
	lease:=contracts.CapabilityLease{ID:"lease-1",Principal:contracts.PrincipalRef{ID:"plugin-1",Kind:"plugin"},Capability:"workspace.search.text",Operations:[]string{"read"},Scope:"workspace:1"}; one:=uint64(1); lease.RemainingUses=&one
	binding:=plugin.LeaseBinding{Lease:lease,InstanceID:"instance-1",SessionID:"session-1"}; instance:=plugin.InstanceIdentity{InstanceID:"instance-1",PluginID:"plugin-1",PluginVersion:"1",ArtifactDigest:"sha256:x",RuntimeSession:"session-1"}; req:=capability.Request{Principal:lease.Principal,Capability:lease.Capability,Operation:"read",Scope:lease.Scope,Now:now}; store:=New(db)
	if err:=store.ConsumePluginLease(ctx,binding,instance,req,now); err!=nil { t.Fatalf("first consumption: %v",err) }
	if err:=store.ConsumePluginLease(ctx,binding,instance,req,now); err==nil { t.Fatal("second consumption must fail") }
}

func TestConsumePluginLeaseRejectsDifferentRuntimeInstance(t *testing.T) {
	ctx:=context.Background(); db,err:=OpenSQLite(ctx,filepath.Join(t.TempDir(),"praxis.db")); if err!=nil { t.Fatal(err) }; defer db.Close()
	now:=time.Now().UTC(); insertBoundPluginLease(t,ctx,db,now)
	lease:=contracts.CapabilityLease{ID:"lease-1",Principal:contracts.PrincipalRef{ID:"plugin-1",Kind:"plugin"},Capability:"workspace.search.text",Operations:[]string{"read"},Scope:"workspace:1"}; one:=uint64(1); lease.RemainingUses=&one
	binding:=plugin.LeaseBinding{Lease:lease,InstanceID:"instance-1",SessionID:"session-1"}; wrong:=plugin.InstanceIdentity{InstanceID:"instance-2",PluginID:"plugin-1",PluginVersion:"1",ArtifactDigest:"sha256:x",RuntimeSession:"session-2"}; req:=capability.Request{Principal:lease.Principal,Capability:lease.Capability,Operation:"read",Scope:lease.Scope,Now:now}
	if err:=New(db).ConsumePluginLease(ctx,binding,wrong,req,now); err==nil { t.Fatal("different plugin runtime instance must be denied") }
}

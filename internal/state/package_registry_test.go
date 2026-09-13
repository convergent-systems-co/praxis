package state

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func fixturePackage(id, version, digest, alias string) packagecatalog.Manifest {
	return packagecatalog.Manifest{
		PackageID: id, Version: version, ContentDigest: digest,
		Invocations: []contracts.InvocationContract{{
			Version: "v1", PackageID: id, PackageVersion: version,
			GraphID: id + ".graph", GraphVersion: version,
			EntryPointID: id + ".default", Aliases: []string{alias},
			Options: []contracts.InvocationOption{{Name:"flag", Type:"bool"}},
		}},
	}
}

func TestPackageActivationPublishesAndRemovesAliases(t *testing.T) {
	ctx:=context.Background(); db,err:=OpenSQLite(ctx,filepath.Join(t.TempDir(),"praxis.db")); if err!=nil { t.Fatal(err) }; defer db.Close()
	s:=New(db); now:=time.Now().UTC(); m:=fixturePackage("example/pkg","1.0.0","sha256:one","example")
	if err:=s.ActivatePackage(ctx,m,"github-release","owner/repo@v1.0.0",now); err!=nil { t.Fatal(err) }
	got,err:=s.ResolveInvocationAlias(ctx,"example"); if err!=nil { t.Fatal(err) }
	if got.Contract.PackageID!=m.PackageID || got.ContentDigest!=m.ContentDigest { t.Fatalf("wrong registered package: %#v",got) }
	if err:=s.DeactivatePackage(ctx,m.PackageID); err!=nil { t.Fatal(err) }
	if _,err:=s.ResolveInvocationAlias(ctx,"example"); err==nil { t.Fatal("disabled package alias must disappear") }
}

func TestPackageCannotShadowCoreCommand(t *testing.T) {
	ctx:=context.Background(); db,err:=OpenSQLite(ctx,filepath.Join(t.TempDir(),"praxis.db")); if err!=nil { t.Fatal(err) }; defer db.Close()
	if err:=New(db).ActivatePackage(ctx,fixturePackage("bad/pkg","1","sha256:x","status"),"github-release","x",time.Now().UTC()); err==nil { t.Fatal("reserved core command must be rejected") }
}

func TestPackageAliasCollisionFailsWithoutChangingExistingOwner(t *testing.T) {
	ctx:=context.Background(); db,err:=OpenSQLite(ctx,filepath.Join(t.TempDir(),"praxis.db")); if err!=nil { t.Fatal(err) }; defer db.Close(); s:=New(db); now:=time.Now().UTC()
	if err:=s.ActivatePackage(ctx,fixturePackage("a/pkg","1","sha256:a","shared"),"github-release","a",now); err!=nil { t.Fatal(err) }
	if err:=s.ActivatePackage(ctx,fixturePackage("b/pkg","1","sha256:b","shared"),"github-release","b",now); err==nil { t.Fatal("alias collision must fail") }
	got,err:=s.ResolveInvocationAlias(ctx,"shared"); if err!=nil { t.Fatal(err) }; if got.Contract.PackageID!="a/pkg" { t.Fatalf("existing alias owner changed: %s",got.Contract.PackageID) }
}

func TestPackageUpdateAtomicallyReplacesAliasSurface(t *testing.T) {
	ctx:=context.Background(); db,err:=OpenSQLite(ctx,filepath.Join(t.TempDir(),"praxis.db")); if err!=nil { t.Fatal(err) }; defer db.Close(); s:=New(db); now:=time.Now().UTC()
	v1:=fixturePackage("example/pkg","1","sha256:1","old"); if err:=s.ActivatePackage(ctx,v1,"github-release","v1",now); err!=nil { t.Fatal(err) }
	v2:=fixturePackage("example/pkg","2","sha256:2","new"); if err:=s.ActivatePackage(ctx,v2,"github-release","v2",now.Add(time.Second)); err!=nil { t.Fatal(err) }
	if _,err:=s.ResolveInvocationAlias(ctx,"old"); err==nil { t.Fatal("old alias must not remain active") }
	got,err:=s.ResolveInvocationAlias(ctx,"new"); if err!=nil { t.Fatal(err) }; if got.Contract.PackageVersion!="2" || got.ContentDigest!="sha256:2" { t.Fatalf("new generation not active: %#v",got) }
}

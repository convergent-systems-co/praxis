package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/convergent-systems-co/praxis/internal/distribution"
	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"github.com/convergent-systems-co/praxis/internal/state"
)

func runPackageCommand(command string, args []string) error {
	ctx := context.Background()
	adapter := distribution.GitHubReleases{Token: os.Getenv("GITHUB_TOKEN")}
	switch command {
	case "discover":
		query := strings.Join(args, " ")
		items, err := adapter.Discover(ctx, query)
		if err != nil { return err }
		return printJSON(items)
	case "info":
		if len(args) != 1 { return errors.New("usage: praxis info <owner/repo[@tag]>") }
		ref, version, err := parseGitHubPackageRef(args[0]); if err != nil { return err }
		release, err := adapter.Info(ctx, ref, version); if err != nil { return err }
		return printJSON(release)
	case "list":
		db, err := openPackageDB(ctx); if err != nil { return err }; defer db.Close()
		items, err := state.New(db).InstalledPackages(ctx); if err != nil { return err }
		return printJSON(items)
	case "install":
		if len(args) != 1 { return errors.New("usage: praxis install <owner/repo[@tag]>") }
		ref, version, err := parseGitHubPackageRef(args[0]); if err != nil { return err }
		release, err := adapter.Resolve(ctx, ref, version); if err != nil { return err }
		artifact, err := adapter.FetchArtifact(ctx, release); if err != nil { return err }
		if err := verifyReleaseArtifact(release, artifact); err != nil { return err }
		db, err := openPackageDB(ctx); if err != nil { return err }; defer db.Close()
		if err := state.New(db).ActivatePackage(ctx, release.Manifest, "github-release", release.Ref.String()+"@"+release.Tag, time.Now().UTC()); err != nil { return err }
		return printJSON(map[string]any{"installed":release.Manifest.PackageID,"version":release.Manifest.Version,"digest":release.Manifest.ContentDigest,"entry_points":release.Manifest.Invocations})
	case "update":
		if len(args) < 1 || len(args) > 2 { return errors.New("usage: praxis update <package-id> [--accept-permission-changes]") }
		acceptChanges := len(args)==2 && args[1]=="--accept-permission-changes"
		db, err := openPackageDB(ctx); if err != nil { return err }; defer db.Close(); store:=state.New(db)
		installed, err := store.ActivePackage(ctx,args[0]); if err != nil { return fmt.Errorf("active package %q: %w",args[0],err) }
		ref, _, err := parseGitHubPackageRef(installed.SourceRef); if err != nil { return fmt.Errorf("installed source: %w",err) }
		latest, changed, err := adapter.CheckUpdate(ctx,ref,installed.Manifest.Version); if err != nil { return err }
		if !changed { return printJSON(map[string]any{"package_id":args[0],"up_to_date":true,"version":installed.Manifest.Version}) }
		review := packagecatalog.ReviewUpdate(installed.Manifest.Capabilities,latest.Manifest.Capabilities, !sameStrings(installed.Manifest.RequiredEnforcement,latest.Manifest.RequiredEnforcement), installed.Manifest.CryptoProfile!=latest.Manifest.CryptoProfile)
		if review.RequiresReauthorization && !acceptChanges { return fmt.Errorf("update requires explicit review/reauthorization: added_capabilities=%v enforcement_changed=%v crypto_changed=%v; rerun with --accept-permission-changes after review",review.AddedCapabilities,review.EnforcementChanged,review.CryptoProfileChanged) }
		artifact, err := adapter.FetchArtifact(ctx,latest); if err != nil { return err }; if err:=verifyReleaseArtifact(latest,artifact);err!=nil{return err}
		if err:=store.ActivatePackage(ctx,latest.Manifest,"github-release",latest.Ref.String()+"@"+latest.Tag,time.Now().UTC());err!=nil{return err}
		return printJSON(map[string]any{"updated":latest.Manifest.PackageID,"from":installed.Manifest.Version,"to":latest.Manifest.Version,"review":review})
	case "uninstall":
		if len(args)!=1{return errors.New("usage: praxis uninstall <package-id>")}
		db,err:=openPackageDB(ctx);if err!=nil{return err};defer db.Close();if err:=state.New(db).RemovePackage(ctx,args[0]);err!=nil{return err}
		return printJSON(map[string]any{"uninstalled":args[0],"durable_history_preserved":true})
	default:
		return fmt.Errorf("unknown package command %q",command)
	}
}

func openPackageDB(ctx context.Context) (*sql.DB,error) {
	path:=os.Getenv("PRAXIS_DB"); if path=="" { return nil,errors.New("PRAXIS_DB is required for package lifecycle commands") }
	return state.OpenSQLite(ctx,path)
}

func parseGitHubPackageRef(raw string) (distribution.PackageRef,string,error) {
	if raw=="" { return distribution.PackageRef{},"",errors.New("package reference is required") }
	base,version:=raw,""
	if at:=strings.LastIndex(raw,"@");at>0{base,version=raw[:at],raw[at+1:]}
	parts:=strings.Split(base,"/"); if len(parts)!=2||parts[0]==""||parts[1]=="" { return distribution.PackageRef{},"",fmt.Errorf("GitHub package reference must be owner/repo[@tag], got %q",raw) }
	return distribution.PackageRef{Source:"github-releases",Owner:parts[0],Repo:parts[1]},version,nil
}

func verifyReleaseArtifact(release distribution.Release, artifact []byte) error {
	if len(artifact)==0{return errors.New("empty release artifact")}
	if !strings.HasPrefix(release.Manifest.ContentDigest,"sha256:"){return fmt.Errorf("unsupported package content digest %q",release.Manifest.ContentDigest)}
	sum:=sha256.Sum256(artifact); actual:="sha256:"+hex.EncodeToString(sum[:])
	if actual!=release.Manifest.ContentDigest{return fmt.Errorf("package artifact digest mismatch: manifest=%s actual=%s",release.Manifest.ContentDigest,actual)}
	return nil
}

func sameStrings(a,b []string) bool {
	if len(a)!=len(b){return false}; seen:=map[string]int{};for _,v:=range a{seen[v]++};for _,v:=range b{seen[v]--};for _,n:=range seen{if n!=0{return false}};return true
}

func printJSON(v any) error { body,err:=json.MarshalIndent(v,"","  ");if err!=nil{return err};fmt.Println(string(body));return nil }

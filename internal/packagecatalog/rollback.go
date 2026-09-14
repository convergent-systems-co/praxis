package packagecatalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// RollbackRequest binds both the observed active closure and the exact retained
// target closure. Target order is dependency-first and ends with RootPackageID.
type RollbackRequest struct {
	RootPackageID string                 `json:"root_package_id"`
	Current       []PackageIdentity      `json:"current"`
	Targets       []PackageIdentity      `json:"targets"`
	Intent        contracts.ActionIntent `json:"intent"`
	ApprovalID    string                 `json:"approval_id"`
}

func NewRollbackRequest(rootPackageID string, current, targets []PackageIdentity, actor contracts.PrincipalRef, approvalID string) (RollbackRequest, error) {
	if rootPackageID == "" || approvalID == "" || len(current) == 0 || len(targets) == 0 {
		return RollbackRequest{}, errors.New("package rollback requires root, current closure, target closure, and approval")
	}
	current = append([]PackageIdentity(nil), current...)
	sort.Slice(current, func(i, j int) bool { return current[i].PackageID < current[j].PackageID })
	targets = append([]PackageIdentity(nil), targets...)
	request := RollbackRequest{RootPackageID: rootPackageID, Current: current, Targets: targets, ApprovalID: approvalID}
	intent, err := rollbackIntent(request, actor)
	if err != nil {
		return RollbackRequest{}, err
	}
	request.Intent = intent
	return request, request.Validate()
}

func (r RollbackRequest) Validate() error {
	if r.RootPackageID == "" || r.ApprovalID == "" || len(r.Current) == 0 || len(r.Targets) == 0 {
		return errors.New("package rollback requires root, current closure, target closure, and approval")
	}
	wantCurrent := append([]PackageIdentity(nil), r.Current...)
	sort.Slice(wantCurrent, func(i, j int) bool { return wantCurrent[i].PackageID < wantCurrent[j].PackageID })
	if !reflect.DeepEqual(wantCurrent, r.Current) {
		return errors.New("package rollback current closure must use canonical package order")
	}
	seenCurrent, seenTargets := map[string]bool{}, map[string]bool{}
	for _, identity := range r.Current {
		if err := identity.Validate(); err != nil {
			return err
		}
		if seenCurrent[identity.PackageID] {
			return fmt.Errorf("duplicate current package %q", identity.PackageID)
		}
		seenCurrent[identity.PackageID] = true
	}
	for _, identity := range r.Targets {
		if err := identity.Validate(); err != nil {
			return err
		}
		if seenTargets[identity.PackageID] {
			return fmt.Errorf("duplicate rollback target package %q", identity.PackageID)
		}
		seenTargets[identity.PackageID] = true
		if !seenCurrent[identity.PackageID] {
			return fmt.Errorf("rollback target %q has no current-generation precondition", identity.PackageID)
		}
	}
	if r.Targets[len(r.Targets)-1].PackageID != r.RootPackageID {
		return errors.New("package rollback target closure must end with root package")
	}
	expected, err := rollbackIntent(RollbackRequest{RootPackageID: r.RootPackageID, Current: wantCurrent, Targets: r.Targets, ApprovalID: r.ApprovalID}, r.Intent.Actor)
	if err != nil {
		return err
	}
	wantDigest, err := expected.Digest()
	if err != nil {
		return err
	}
	gotDigest, err := r.Intent.Digest()
	if err != nil || gotDigest != wantDigest {
		return errors.New("package rollback intent does not bind current and target closures")
	}
	return nil
}

func rollbackIntent(request RollbackRequest, actor contracts.PrincipalRef) (contracts.ActionIntent, error) {
	if err := actor.Validate(); err != nil {
		return contracts.ActionIntent{}, err
	}
	currentBody, err := json.Marshal(request.Current)
	if err != nil {
		return contracts.ActionIntent{}, err
	}
	targetBody, err := json.Marshal(request.Targets)
	if err != nil {
		return contracts.ActionIntent{}, err
	}
	root := request.Targets[len(request.Targets)-1]
	intent := contracts.ActionIntent{
		Version: packageRollbackIntentVersions.CurrentVersion(), ID: "package-rollback:" + root.PackageID + "@" + root.Version + "#" + root.ContentDigest,
		Actor: actor, Operation: "package.rollback", Target: root.PackageID + "@" + root.Version + "#" + root.ContentDigest,
		Scope:      "package:" + request.RootPackageID,
		Parameters: map[string]string{"current_closure_digest": bytesDigest(currentBody), "target_closure_digest": bytesDigest(targetBody)},
	}
	return intent, intent.Validate()
}

package packagecatalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// DeploymentRequest binds one local authorization to an exact, verified,
// dependency-first package closure. Dependencies are executable generations,
// not merely evidence attached to the root package.
type DeploymentRequest struct {
	Root       VerifiedPackage
	Packages   []VerifiedPackage
	Intent     contracts.ActionIntent
	ApprovalID string
}

func NewDeploymentRequest(root VerifiedPackage, dependencies map[string]VerifiedPackage, actor contracts.PrincipalRef, approvalID string) (DeploymentRequest, error) {
	packages, err := canonicalDeploymentOrder(root, dependencies)
	if err != nil {
		return DeploymentRequest{}, err
	}
	intent, err := NewDeploymentIntent(root, packages, actor)
	if err != nil {
		return DeploymentRequest{}, err
	}
	request := DeploymentRequest{Root: root, Packages: packages, Intent: intent, ApprovalID: approvalID}
	return request, request.Validate()
}

func NewDeploymentIntent(root VerifiedPackage, packages []VerifiedPackage, actor contracts.PrincipalRef) (contracts.ActionIntent, error) {
	if err := root.Validate(); err != nil {
		return contracts.ActionIntent{}, err
	}
	if err := actor.Validate(); err != nil {
		return contracts.ActionIntent{}, err
	}
	closure, err := deploymentClosureIdentity(packages)
	if err != nil {
		return contracts.ActionIntent{}, err
	}
	body, err := json.Marshal(closure)
	if err != nil {
		return contracts.ActionIntent{}, err
	}
	evidence := root.Evidence()
	intent := contracts.ActionIntent{
		Version: deploymentIntentVersions.CurrentVersion(),
		ID:      "package-deployment:" + evidence.ID,
		Actor:   actor, Operation: "package.deploy",
		Target: root.Manifest().PackageID + "@" + root.Manifest().Version + "#" + root.Manifest().ContentDigest,
		Parameters: map[string]string{
			"root_verification_id": evidence.ID,
			"closure_digest":       bytesDigest(body),
		},
		Scope: "package:" + root.Manifest().PackageID, CryptoProfile: evidence.SignatureProfile,
	}
	return intent, intent.Validate()
}

func (r DeploymentRequest) Validate() error {
	if r.ApprovalID == "" {
		return errors.New("package deployment approval id is required")
	}
	dependencies := make(map[string]VerifiedPackage, len(r.Packages))
	rootID := r.Root.Manifest().PackageID
	for _, pkg := range r.Packages {
		if err := pkg.Validate(); err != nil {
			return err
		}
		id := pkg.Manifest().PackageID
		if _, exists := dependencies[id]; exists {
			return fmt.Errorf("duplicate deployment package %q", id)
		}
		if id != rootID {
			dependencies[id] = pkg
		}
	}
	expected, err := canonicalDeploymentOrder(r.Root, dependencies)
	if err != nil {
		return err
	}
	gotIdentity, err := deploymentClosureIdentity(r.Packages)
	if err != nil {
		return err
	}
	wantIdentity, err := deploymentClosureIdentity(expected)
	if err != nil || !reflect.DeepEqual(gotIdentity, wantIdentity) {
		return errors.New("package deployment closure is incomplete, contains extras, or is not dependency-first")
	}
	expectedIntent, err := NewDeploymentIntent(r.Root, expected, r.Intent.Actor)
	if err != nil {
		return err
	}
	wantDigest, err := expectedIntent.Digest()
	if err != nil {
		return err
	}
	gotDigest, err := r.Intent.Digest()
	if err != nil || gotDigest != wantDigest {
		return errors.New("package deployment intent does not bind the exact verified closure")
	}
	return nil
}

type deploymentIdentity struct {
	PackageID      string `json:"package_id"`
	PackageVersion string `json:"package_version"`
	ContentDigest  string `json:"content_digest"`
	VerificationID string `json:"verification_id"`
}

func deploymentClosureIdentity(packages []VerifiedPackage) ([]deploymentIdentity, error) {
	out := make([]deploymentIdentity, 0, len(packages))
	for _, pkg := range packages {
		if err := pkg.Validate(); err != nil {
			return nil, err
		}
		manifest, evidence := pkg.Manifest(), pkg.Evidence()
		out = append(out, deploymentIdentity{PackageID: manifest.PackageID, PackageVersion: manifest.Version, ContentDigest: manifest.ContentDigest, VerificationID: evidence.ID})
	}
	return out, nil
}

func canonicalDeploymentOrder(root VerifiedPackage, dependencies map[string]VerifiedPackage) ([]VerifiedPackage, error) {
	if err := root.Validate(); err != nil {
		return nil, err
	}
	all := make(map[string]VerifiedPackage, len(dependencies)+1)
	for id, pkg := range dependencies {
		if err := pkg.Validate(); err != nil {
			return nil, fmt.Errorf("deployment dependency %q: %w", id, err)
		}
		if id != pkg.Manifest().PackageID || id == root.Manifest().PackageID {
			return nil, fmt.Errorf("deployment dependency key %q does not identify a distinct package", id)
		}
		all[id] = pkg
	}
	all[root.Manifest().PackageID] = root
	visiting, visited := map[string]bool{}, map[string]bool{}
	order := make([]VerifiedPackage, 0, len(all))
	var visit func(string) error
	visit = func(id string) error {
		if visiting[id] {
			return fmt.Errorf("deployment dependency cycle at %q", id)
		}
		if visited[id] {
			return nil
		}
		pkg, ok := all[id]
		if !ok {
			return fmt.Errorf("deployment dependency %q is missing from verified closure", id)
		}
		visiting[id] = true
		locks := append([]Dependency(nil), pkg.Manifest().Dependencies...)
		sort.Slice(locks, func(i, j int) bool { return locks[i].PackageID < locks[j].PackageID })
		for _, lock := range locks {
			dependency, ok := all[lock.PackageID]
			if !ok {
				return fmt.Errorf("deployment dependency %q is missing from verified closure", lock.PackageID)
			}
			manifest := dependency.Manifest()
			if manifest.Version != lock.Version || manifest.ContentDigest != lock.Digest {
				return fmt.Errorf("deployment dependency %q does not match immutable lock", lock.PackageID)
			}
			if err := visit(lock.PackageID); err != nil {
				return err
			}
		}
		visiting[id] = false
		visited[id] = true
		order = append(order, pkg)
		return nil
	}
	if err := visit(root.Manifest().PackageID); err != nil {
		return nil, err
	}
	if len(visited) != len(all) {
		return nil, errors.New("verified deployment closure contains package not reachable from root")
	}
	for id, pkg := range all {
		expectedEvidence := map[string]bool{}
		var collect func(string)
		collect = func(packageID string) {
			for _, lock := range all[packageID].Manifest().Dependencies {
				dependency := all[lock.PackageID]
				expectedEvidence[dependency.Evidence().ID] = true
				collect(lock.PackageID)
			}
		}
		collect(id)
		var want []string
		for evidenceID := range expectedEvidence {
			want = append(want, evidenceID)
		}
		sort.Strings(want)
		if !reflect.DeepEqual(canonicalStrings(pkg.Evidence().DependencyEvidenceIDs), want) {
			return nil, fmt.Errorf("deployment package %q verification lineage does not bind the supplied closure", id)
		}
	}
	return order, nil
}

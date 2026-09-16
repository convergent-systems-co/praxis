package goalspublication

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// RecoveryGitHub is intentionally separate from the empty-repository adapter:
// its only mutations are the three missing assets and publishing the exact
// existing draft release.
type RecoveryGitHub struct{ GitHub }

func (g RecoveryGitHub) Check(ctx context.Context, a contracts.ActionIntent, step string, previous []Observation) error {
	if err := contracts.ValidateGoalsPublicationRecoveryIntent(a); err != nil {
		return err
	}
	o, err := g.checkIdentity(ctx, a)
	if err != nil {
		return err
	}
	_ = o
	if err = g.checkRefs(ctx, a); err != nil {
		return err
	}
	r, err := readRelease(ctx, 389997269)
	if err != nil {
		return err
	}
	if err = matchRecoveryRelease(r, a, step != "verify-published"); err != nil {
		return err
	}
	counts := map[string]int{"manifest": 0, "archive": 1, "signature": 2, "verify-draft": 3, "publish": 3, "verify-published": 3}
	expect, ok := counts[step]
	if !ok {
		return errors.New("unknown recovery step")
	}
	if len(r.Assets) != expect {
		return errors.New("recovery release asset inventory changed")
	}
	for i, asset := range r.Assets {
		if asset.Name != assetNames[i] || asset.State != "uploaded" || asset.Size <= 0 || fmt.Sprint(asset.Uploader.ID) != a.Parameters["account_id"] {
			return errors.New("recovery asset inventory identity mismatch")
		}
		if i >= len(previous) || previous[i].Asset == nil || previous[i].Asset.ID != asset.ID {
			return errors.New("recovery assets are not attributed to this successor intent")
		}
		b, err := readAsset(ctx, asset.ID)
		if err != nil {
			return err
		}
		if hash(b) != assetDigests[i] || int64(len(b)) != asset.Size || fmt.Sprint(asset.Size) != a.Parameters[[]string{"manifest_size", "archive_size", "signature_size"}[i]] {
			return errors.New("recovery asset bytes differ from signed Goals")
		}
	}
	return nil
}
func matchRecoveryRelease(r Release, a contracts.ActionIntent, draft bool) error {
	if r.ID != 389997269 || r.Tag != contracts.GoalsPublicationTag || r.Target != contracts.GoalsRecoveryCommit || r.Name != contracts.GoalsPublicationPackage || r.Body != "Exact signed Goals initial publication; intent "+contracts.GoalsRecoveryNonce || r.Draft != draft || r.Prerelease || fmt.Sprint(r.Author.ID) != a.Parameters["account_id"] {
		return errors.New("established draft release identity/settings changed")
	}
	return nil
}
func (g RecoveryGitHub) Dispatch(ctx context.Context, a contracts.ActionIntent, step string, previous []Observation, assets Assets) (Observation, error) {
	if err := g.Check(ctx, a, step, previous); err != nil {
		return Observation{}, err
	}
	o, err := g.checkIdentity(ctx, a)
	if err != nil {
		return o, err
	}
	o.Commit = contracts.GoalsRecoveryCommit
	o.Tree = contracts.GoalsRecoveryTree
	switch step {
	case "manifest", "archive", "signature":
		i := map[string]int{"manifest": 0, "archive": 1, "signature": 2}[step]
		path := fmt.Sprintf("https://uploads.github.com/%s/releases/%s/assets?name=%s", apiRepo, contracts.GoalsRecoveryReleaseID, assetNames[i])
		b, err := ghFile(ctx, []string{"api", "--hostname", "github.com", "--method", "POST", path, "-H", "Content-Type: application/octet-stream"}, assets[i])
		if err != nil {
			return o, err
		}
		var asset Asset
		if err = json.Unmarshal(b, &asset); err != nil {
			return o, dispatchFailure{outcome: DispatchOutcome{Version: "1", Process: "started", Class: "ambiguous"}, err: errors.New("upload response was not valid asset metadata")}
		}
		if asset.Name != assetNames[i] || asset.Size != int64(len(assets[i])) || asset.State != "uploaded" || asset.ID <= 0 || fmt.Sprint(asset.Uploader.ID) != a.Parameters["account_id"] {
			return o, dispatchFailure{outcome: DispatchOutcome{Version: "1", Process: "started", Class: "acknowledged_success"}, err: errors.New("successor upload response mismatch")}
		}
		o.Asset = &asset
		o.DispatchOutcome = &DispatchOutcome{Version: "1", Process: "started", Class: "acknowledged_success"}
	case "verify-draft", "verify-published":
		r, err := readRelease(ctx, 389997269)
		if err != nil {
			var failure dispatchFailure
			if !errors.As(err, &failure) {
				err = dispatchFailure{outcome: DispatchOutcome{Version: "1", Process: "started", Class: "ambiguous"}, err: err}
			}
			return o, err
		}
		if err = matchRecoveryRelease(r, a, step == "verify-draft"); err != nil {
			return o, err
		}
		o.Release = &r
		for _, asset := range r.Assets {
			b, err := readAsset(ctx, asset.ID)
			if err != nil {
				return o, err
			}
			o.AssetDigests = append(o.AssetDigests, hash(b))
		}
	case "publish":
		var r Release
		err = api(ctx, "PATCH", fmt.Sprintf("%s/releases/%s", apiRepo, contracts.GoalsRecoveryReleaseID), map[string]any{"draft": false, "make_latest": "false"}, &r)
		if err != nil {
			var failure dispatchFailure
			if !errors.As(err, &failure) {
				err = dispatchFailure{outcome: DispatchOutcome{Version: "1", Process: "started", Class: "ambiguous"}, err: err}
			}
			return o, err
		}
		if err = matchRecoveryRelease(r, a, false); err != nil {
			return o, dispatchFailure{outcome: DispatchOutcome{Version: "1", Process: "started", Class: "acknowledged_success"}, err: err}
		}
		o.Release = &r
		o.DispatchOutcome = &DispatchOutcome{Version: "1", Process: "started", Class: "acknowledged_success"}
	default:
		return o, errors.New("unrecognized Goals successor effect")
	}
	return o, nil
}
func (g RecoveryGitHub) Reconcile(ctx context.Context, a contracts.ActionIntent, step string, previous []Observation) (Observation, error) {
	o, err := g.checkIdentity(ctx, a)
	if err != nil {
		return o, err
	}
	o.Commit = contracts.GoalsRecoveryCommit
	o.Tree = contracts.GoalsRecoveryTree
	if err = g.checkRefs(ctx, a); err != nil {
		return o, err
	}
	r, err := readRelease(ctx, 389997269)
	if err != nil {
		return o, err
	}
	o.Release = &r
	for _, asset := range r.Assets {
		b, e := readAsset(ctx, asset.ID)
		if e != nil {
			return o, e
		}
		o.AssetDigests = append(o.AssetDigests, hash(b))
	}
	if err = g.Check(ctx, a, step, previous); err != nil {
		return o, err
	}
	return o, nil
}

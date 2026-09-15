package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// StagedPointer is the durable hand-off between a verified staged artifact
// and the active component pointer. It contains no authority; the journal and
// lifecycle plan remain the source of transition permission.
type StagedPointer struct {
	PlanID         string                          `json:"plan_id"`
	PlanDigest     string                          `json:"plan_digest"`
	StepID         string                          `json:"step_id"`
	InstallationID string                          `json:"installation_id"`
	Target         contracts.LifecycleComponentRef `json:"target"`
	Artifact       contracts.LifecycleArtifactRef  `json:"artifact"`
	StageDigest    string                          `json:"stage_digest"`
}

func (p StagedPointer) Validate() error {
	if p.PlanID == "" || p.StepID == "" || p.InstallationID == "" || p.StageDigest == "" {
		return errors.New("staged pointer identity is required")
	}
	if err := contracts.ValidateSHA256Digest(p.PlanDigest); err != nil {
		return err
	}
	if err := p.Target.Validate(); err != nil {
		return fmt.Errorf("staged target: %w", err)
	}
	if err := p.Artifact.Validate(); err != nil {
		return fmt.Errorf("staged artifact: %w", err)
	}
	if p.Artifact.Digest != p.StageDigest || p.Target.Digest != p.StageDigest {
		return errors.New("staged pointer target and artifact digest differ")
	}
	return nil
}

// StagedArtifactStore uses an explicit directory supplied by the caller. It
// never follows a caller-provided executable path and only activates bytes
// whose canonical digest matches the immutable target.
type StagedArtifactStore struct{ Root string }

func (s StagedArtifactStore) validate() error {
	if s.Root == "" {
		return errors.New("staged artifact root is required")
	}
	return nil
}

func (s StagedArtifactStore) Stage(ctx context.Context, pointer StagedPointer, contents []byte) error {
	if err := s.validate(); err != nil {
		return err
	}
	if err := pointer.Validate(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	digest := contentDigest(contents)
	if digest != pointer.StageDigest {
		return errors.New("staged artifact content digest mismatch")
	}
	dir := filepath.Join(s.Root, "staged")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(dir, digestHex(digest))
	f, err := os.OpenFile(path+".tmp", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err = f.Write(contents); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(path+".tmp", path)
}

func (s StagedArtifactStore) Activate(ctx context.Context, pointer StagedPointer) error {
	if err := s.validate(); err != nil {
		return err
	}
	if err := pointer.Validate(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	contents, err := os.ReadFile(filepath.Join(s.Root, "staged", digestHex(pointer.StageDigest)))
	if err != nil {
		return fmt.Errorf("read staged artifact: %w", err)
	}
	digest := contentDigest(contents)
	if digest != pointer.StageDigest {
		return errors.New("staged artifact failed activation digest verification")
	}
	encoded, err := json.Marshal(pointer)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.Root, 0o700); err != nil {
		return err
	}
	tmp := filepath.Join(s.Root, "active.pointer.tmp")
	if err := os.WriteFile(tmp, encoded, 0o600); err != nil {
		return err
	}
	f, err := os.OpenFile(tmp, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	err = f.Sync()
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(tmp, filepath.Join(s.Root, "active.pointer"))
}

func (s StagedArtifactStore) Recover(ctx context.Context) (StagedPointer, error) {
	if err := s.validate(); err != nil {
		return StagedPointer{}, err
	}
	if err := ctx.Err(); err != nil {
		return StagedPointer{}, err
	}
	encoded, err := os.ReadFile(filepath.Join(s.Root, "active.pointer"))
	if err != nil {
		return StagedPointer{}, err
	}
	var pointer StagedPointer
	if err := json.Unmarshal(encoded, &pointer); err != nil {
		return StagedPointer{}, fmt.Errorf("decode active pointer: %w", err)
	}
	if err := pointer.Validate(); err != nil {
		return StagedPointer{}, err
	}
	contents, err := os.ReadFile(filepath.Join(s.Root, "staged", digestHex(pointer.StageDigest)))
	if err != nil {
		return StagedPointer{}, fmt.Errorf("recover staged artifact: %w", err)
	}
	digest := contentDigest(contents)
	if digest != pointer.StageDigest {
		return StagedPointer{}, errors.New("recovered pointer references invalid artifact")
	}
	return pointer, nil
}

func digestHex(digest string) string {
	return hex.EncodeToString([]byte(strings.TrimPrefix(digest, "sha256:")))
}

func contentDigest(contents []byte) string {
	sum := sha256.Sum256(contents)
	return "sha256:" + hex.EncodeToString(sum[:])
}

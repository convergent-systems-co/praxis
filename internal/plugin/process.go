package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

// LaunchSpec is the only input from which a host process may be started. The
// executable bytes must be the exact bytes verified by package/state authority;
// a manifest path or caller-selected executable name is not sufficient.
type LaunchSpec struct {
	Provider          Provider
	Executable        []byte
	Entrypoint        string
	SocketPath        string
	RequiredIsolation []IsolationProperty
}

func (spec LaunchSpec) Validate() error {
	if err := spec.Provider.Identity.ValidateAgainst(spec.Provider.Manifest); err != nil {
		return fmt.Errorf("launch provider: %w", err)
	}
	if len(spec.Executable) == 0 {
		return errors.New("verified executable bytes are required")
	}
	if spec.Entrypoint == "" || filepath.IsAbs(spec.Entrypoint) {
		return errors.New("safe package-relative executable entrypoint is required")
	}
	if spec.SocketPath == "" {
		return errors.New("plugin socket path is required")
	}
	digest := sha256.Sum256(spec.Executable)
	if got := "sha256:" + hex.EncodeToString(digest[:]); got != spec.Provider.Manifest.ArtifactDigest {
		return fmt.Errorf("executable digest %q does not match manifest %q", got, spec.Provider.Manifest.ArtifactDigest)
	}
	required := append([]IsolationProperty(nil), spec.Provider.Manifest.RequiredIsolation...)
	required = append(required, spec.RequiredIsolation...)
	if err := spec.Provider.Isolation.Satisfies(uniqueIsolation(required)); err != nil {
		return err
	}
	return nil
}

// LocalProcessControl materializes only verified bytes and never inherits the
// caller environment. It intentionally refuses required isolation until a
// platform enforcer is supplied; process separation alone is not sandboxing.
type LocalProcessControl struct {
	mu      sync.Mutex
	process map[string]*exec.Cmd
	paths   map[string]string
}

func NewLocalProcessControl() *LocalProcessControl {
	return &LocalProcessControl{process: map[string]*exec.Cmd{}, paths: map[string]string{}}
}

func (control *LocalProcessControl) Start(ctx context.Context, spec LaunchSpec) error {
	if control == nil {
		return errors.New("local process control is required")
	}
	if err := spec.Validate(); err != nil {
		return err
	}
	required := append([]IsolationProperty(nil), spec.Provider.Manifest.RequiredIsolation...)
	required = append(required, spec.RequiredIsolation...)
	if len(uniqueIsolation(required)) != 0 {
		return errors.New("required plugin isolation has no host enforcer")
	}
	directory, err := os.MkdirTemp("", "praxis-plugin-")
	if err != nil {
		return fmt.Errorf("materialize plugin directory: %w", err)
	}
	path := filepath.Join(directory, filepath.Base(spec.Entrypoint))
	if err := os.WriteFile(path, spec.Executable, 0o700); err != nil {
		_ = os.RemoveAll(directory)
		return fmt.Errorf("materialize verified plugin: %w", err)
	}
	command := exec.CommandContext(ctx, path)
	command.Env = []string{"PRAXIS_PLUGIN_SOCKET=" + spec.SocketPath}
	if err := command.Start(); err != nil {
		_ = os.RemoveAll(directory)
		return fmt.Errorf("start verified plugin: %w", err)
	}
	control.mu.Lock()
	control.process[spec.Provider.Identity.InstanceID] = command
	control.paths[spec.Provider.Identity.InstanceID] = directory
	control.mu.Unlock()
	go func() {
		_ = command.Wait()
		control.mu.Lock()
		delete(control.process, spec.Provider.Identity.InstanceID)
		dir := control.paths[spec.Provider.Identity.InstanceID]
		delete(control.paths, spec.Provider.Identity.InstanceID)
		control.mu.Unlock()
		_ = os.RemoveAll(dir)
	}()
	return nil
}

func (control *LocalProcessControl) Terminate(_ context.Context, instance InstanceIdentity) error {
	if control == nil {
		return errors.New("local process control is required")
	}
	control.mu.Lock()
	command, ok := control.process[instance.InstanceID]
	control.mu.Unlock()
	if !ok {
		return errors.New("plugin process is not running")
	}
	if command.Process == nil {
		return errors.New("plugin process has no process identity")
	}
	if err := command.Process.Kill(); err != nil {
		return fmt.Errorf("terminate plugin process: %w", err)
	}
	return nil
}

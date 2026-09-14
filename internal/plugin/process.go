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

// ProcessExitObserver lets a supervisor turn an unexpected child exit into
// authoritative lifecycle state. Implementations may invoke the handler
// after the child has already been reaped; the supervisor must not assume the
// process can still be terminated at that point.
type ProcessExitObserver interface {
	SetExitHandler(instance InstanceIdentity, handler func(error))
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
	mu       sync.Mutex
	process  map[string]*exec.Cmd
	paths    map[string]string
	handlers map[string]func(error)
}

func NewLocalProcessControl() *LocalProcessControl {
	return &LocalProcessControl{process: map[string]*exec.Cmd{}, paths: map[string]string{}, handlers: map[string]func(error){}}
}

func (control *LocalProcessControl) SetExitHandler(instance InstanceIdentity, handler func(error)) {
	control.mu.Lock()
	defer control.mu.Unlock()
	key := processKey(instance)
	if handler == nil {
		delete(control.handlers, key)
		return
	}
	control.handlers[key] = handler
}

func processKey(instance InstanceIdentity) string {
	return instance.InstanceID + "\x00" + instance.RuntimeSession
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
	// Identity is authoritative launch context, not caller-controlled plugin
	// content. Supplying it explicitly lets the child prove the exact binding
	// during handshake without inheriting ambient host environment.
	command.Env = []string{
		"PRAXIS_PLUGIN_SOCKET=" + spec.SocketPath,
		"PRAXIS_PLUGIN_ID=" + spec.Provider.Identity.PluginID,
		"PRAXIS_PLUGIN_VERSION=" + spec.Provider.Identity.PluginVersion,
		"PRAXIS_PLUGIN_INSTANCE=" + spec.Provider.Identity.InstanceID,
		"PRAXIS_PLUGIN_DIGEST=" + spec.Provider.Identity.ArtifactDigest,
		"PRAXIS_PLUGIN_SESSION=" + spec.Provider.Identity.RuntimeSession,
	}
	if err := command.Start(); err != nil {
		_ = os.RemoveAll(directory)
		return fmt.Errorf("start verified plugin: %w", err)
	}
	control.mu.Lock()
	key := processKey(spec.Provider.Identity)
	control.process[key] = command
	control.paths[key] = directory
	control.mu.Unlock()
	go func() {
		err := command.Wait()
		control.mu.Lock()
		key := processKey(spec.Provider.Identity)
		// A stale reaper must never remove or report against a newer process
		// using the same map slot. Runtime session is part of the authoritative
		// process identity, and the command pointer closes the remaining race.
		current, currentOK := control.process[key]
		if currentOK && current == command {
			delete(control.process, key)
			dir := control.paths[key]
			delete(control.paths, key)
			handler := control.handlers[key]
			delete(control.handlers, key)
			control.mu.Unlock()
			_ = os.RemoveAll(dir)
			if handler != nil {
				handler(err)
			}
			return
		}
		control.mu.Unlock()
		// The replacement owns the directory and handler now. The stale
		// process has no lifecycle authority after its slot was replaced.
	}()
	return nil
}

func (control *LocalProcessControl) Terminate(_ context.Context, instance InstanceIdentity) error {
	if control == nil {
		return errors.New("local process control is required")
	}
	control.mu.Lock()
	command, ok := control.process[processKey(instance)]
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

// Package boundexec owns the generic package-owned invocation boundary.
//
// Callers identify an invocation by its durable alias. They do not provide an
// executable path, plugin metadata, or a reconstructed binding.
package boundexec

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/convergent-systems-co/praxis/internal/capability"
	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/internal/plugin"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

var (
	ErrMissingExecutableBinding = errors.New("bound invocation has no executable binding")
	ErrAuthorityRequired        = errors.New("bound invocation authority is required")
)

// Request contains only invocation input and the actor making the request.
// Package/runtime relationships are recovered from the authoritative store.
type Request struct {
	Alias     string
	Actor     contracts.PrincipalRef
	Operation string
	Scope     string
	Payload   []byte
	Now       time.Time
}

// Authority authorizes the exact runtime instance before launch. The returned
// binding is then consumed again by plugin.Gateway at the transport boundary.
type Authority interface {
	Authorize(context.Context, contracts.InvocationContract, plugin.InstanceIdentity, capability.Request, time.Time) (plugin.LeaseBinding, error)
}

// Repository is the authoritative durable package/invocation resolver. The
// production state.Store implements it; the narrow interface also makes the
// controller boundary independently testable without a second resolver.
type Repository interface {
	ResolveInvocationAlias(context.Context, string) (state.RegisteredInvocation, error)
	ResolvePlugin(context.Context, string, string) (state.ResolvedPlugin, error)
}

// Runtime is the lifecycle/transport boundary. Implementations must launch
// only the supplied verified package content and must not call an in-process
// handler for a bound invocation.
type Runtime interface {
	Execute(context.Context, state.ResolvedPlugin, contracts.InvocationContract, plugin.LeaseBinding, Request, plugin.InstanceIdentity) ([]byte, error)
}

// Result is controller-owned attribution for one completed bound invocation.
type Result struct {
	Alias                  string
	PackageID              string
	PackageVersion         string
	PackageDigest          string
	ContractDigest         string
	ExecutableBinding      contracts.ExecutableBinding
	PluginDefinitionDigest string
	ExecutableDigest       string
	RuntimeID              string
	RuntimeVersion         string
	RuntimeDigest          string
	AuthorityLeaseID       string
	Payload                []byte
}

// Executor resolves and validates all durable relationships before delegating
// lifecycle/transport work to the existing supervised plugin runtime.
type Executor struct {
	Store     Repository
	Authority Authority
	Runtime   Runtime
	Events    eventstore.Store
}

func (e Executor) Execute(ctx context.Context, req Request) (Result, error) {
	if e.Store == nil || e.Authority == nil || e.Runtime == nil {
		return Result{}, errors.New("bound invocation store, authority, and runtime are required")
	}
	if err := req.Actor.Validate(); err != nil {
		return Result{}, fmt.Errorf("invocation actor: %w", err)
	}
	if req.Alias == "" || req.Operation == "" || req.Scope == "" {
		return Result{}, errors.New("invocation alias, operation, and scope are required")
	}
	now := req.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	invocation, err := e.Store.ResolveInvocationAlias(ctx, req.Alias)
	if err != nil {
		return Result{}, fmt.Errorf("resolve registered invocation: %w", err)
	}
	binding := invocation.RuntimeBinding.Executable
	if binding == nil {
		return Result{}, ErrMissingExecutableBinding
	}
	if err := binding.Validate(); err != nil {
		return Result{}, fmt.Errorf("validate executable binding: %w", err)
	}
	if binding.EntryPointID != invocation.Contract.EntryPointID {
		return Result{}, errors.New("executable binding entry point does not match invocation contract")
	}
	if invocation.RuntimeBinding.PackageID != invocation.Contract.PackageID || invocation.RuntimeBinding.PackageVersion != invocation.Contract.PackageVersion || invocation.RuntimeBinding.PackageDigest != invocation.ContentDigest {
		return Result{}, errors.New("runtime binding does not identify the active package generation")
	}
	if invocation.RuntimeBinding.ContractDigest != invocation.ContractDigest {
		return Result{}, errors.New("runtime binding does not identify the exact invocation contract")
	}
	if invocation.RuntimeBinding.RuntimeID != binding.RuntimeID || invocation.RuntimeBinding.RuntimeVersion != binding.RuntimeVersion || invocation.RuntimeBinding.RuntimeDigest != binding.RuntimeDigest {
		return Result{}, errors.New("runtime binding does not identify the exact executable runtime")
	}
	resolved, err := e.Store.ResolvePlugin(ctx, binding.PluginID, binding.PluginVersion)
	if err != nil {
		return Result{}, fmt.Errorf("resolve bound plugin: %w", err)
	}
	if resolved.Definition.PackageID != invocation.Contract.PackageID || resolved.Definition.PackageVersion != invocation.Contract.PackageVersion || resolved.Definition.PackageDigest != invocation.ContentDigest {
		return Result{}, errors.New("plugin content is not owned by the active invocation package generation")
	}
	if resolved.Manifest.ID != binding.PluginID || resolved.Manifest.Version != binding.PluginVersion || resolved.Manifest.ArtifactDigest != binding.ExecutableDigest || resolved.Executable.Content.Digest != binding.ExecutableDigest {
		return Result{}, errors.New("resolved plugin does not match executable binding")
	}
	if resolved.Manifest.Entrypoint != resolved.Executable.Content.Artifact {
		return Result{}, errors.New("resolved plugin entrypoint does not match executable binding")
	}
	instance := plugin.InstanceIdentity{InstanceID: executionID(req.Alias), PluginID: binding.PluginID, PluginVersion: binding.PluginVersion, ArtifactDigest: binding.ExecutableDigest, RuntimeSession: executionID(req.Alias + ":session")}
	capabilityName := firstRequiredCapability(invocation.Contract)
	if capabilityName == "" {
		return Result{}, ErrAuthorityRequired
	}
	authorityRequest := capability.Request{Principal: req.Actor, Capability: capabilityName, Operation: req.Operation, Scope: req.Scope, Now: now}
	lease, err := e.Authority.Authorize(ctx, invocation.Contract, instance, authorityRequest, now)
	if err != nil {
		return Result{}, fmt.Errorf("authorize bound invocation before launch: %w", err)
	}
	payload, err := e.Runtime.Execute(ctx, resolved, invocation.Contract, lease, req, instance)
	if err != nil {
		return Result{}, err
	}
	result := Result{Alias: req.Alias, PackageID: invocation.Contract.PackageID, PackageVersion: invocation.Contract.PackageVersion, PackageDigest: invocation.ContentDigest, ContractDigest: invocation.ContractDigest, ExecutableBinding: *binding, PluginDefinitionDigest: binding.PluginDefinitionDigest, ExecutableDigest: binding.ExecutableDigest, RuntimeID: binding.RuntimeID, RuntimeVersion: binding.RuntimeVersion, RuntimeDigest: binding.RuntimeDigest, AuthorityLeaseID: lease.Lease.ID, Payload: append([]byte(nil), payload...)}
	if e.Events != nil {
		if err := appendResult(ctx, e.Events, req.Actor, result, now); err != nil {
			return Result{}, fmt.Errorf("persist bound invocation result: %w", err)
		}
	}
	return result, nil
}

func firstRequiredCapability(contract contracts.InvocationContract) string {
	if len(contract.RequiredCapabilities) == 0 {
		return ""
	}
	return contract.RequiredCapabilities[0]
}

func executionID(seed string) string {
	var nonce [12]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "bound-invocation-" + hex.EncodeToString([]byte(seed))
	}
	return "bound-invocation-" + hex.EncodeToString(nonce[:])
}

func appendResult(ctx context.Context, events eventstore.Store, actor contracts.PrincipalRef, result Result, now time.Time) error {
	aggregate := "bound-invocation:" + result.Alias
	history, err := events.LoadAggregate(ctx, aggregate, 0)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(struct {
		PackageID, PackageVersion, PackageDigest, ContractDigest                                             string
		ExecutableBinding                                                                                    contracts.ExecutableBinding
		PluginDefinitionDigest, ExecutableDigest, RuntimeID, RuntimeVersion, RuntimeDigest, AuthorityLeaseID string
		ResultDigest                                                                                         string
	}{result.PackageID, result.PackageVersion, result.PackageDigest, result.ContractDigest, result.ExecutableBinding, result.PluginDefinitionDigest, result.ExecutableDigest, result.RuntimeID, result.RuntimeVersion, result.RuntimeDigest, result.AuthorityLeaseID, digest(result.Payload)})
	if err != nil {
		return err
	}
	_, err = events.Append(ctx, aggregate, int64(len(history)), []eventstore.Event{{ID: aggregate + ":" + fmt.Sprint(len(history)+1), AggregateType: "bound_invocation", Type: "bound_invocation.completed", Version: "v1", Actor: actor, CommandID: aggregate + ":command:" + fmt.Sprint(len(history)+1), CorrelationID: aggregate, Trust: contracts.TrustObserved, Payload: payload, CreatedAt: now}})
	return err
}

func digest(payload []byte) string {
	// The event stores the result digest, not arbitrary result bytes.
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// SupervisedRuntime is the production adapter for the existing Supervisor and
// GRPCTransport. It intentionally has no Goals-specific behavior.
type SupervisedRuntime struct {
	Store             *state.Store
	Process           plugin.ProcessControl
	Policy            plugin.SupervisorPolicy
	CoreProtocol      plugin.ProtocolRange
	Isolation         plugin.IsolationProfile
	RequiredIsolation []plugin.IsolationProperty
}

func (r SupervisedRuntime) Execute(ctx context.Context, resolved state.ResolvedPlugin, _ contracts.InvocationContract, lease plugin.LeaseBinding, req Request, instance plugin.InstanceIdentity) ([]byte, error) {
	if r.Store == nil || r.Process == nil {
		return nil, errors.New("supervised runtime store and process control are required")
	}
	directory, err := os.MkdirTemp("", "praxis-bound-invocation-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(directory)
	socket := filepath.Join(directory, "plugin.sock")
	spec, err := resolved.LaunchSpec(instance, r.Isolation, socket, r.RequiredIsolation)
	if err != nil {
		return nil, err
	}
	registry := plugin.NewRegistry()
	supervisor, err := plugin.NewPersistentSupervisor(ctx, registry, r.Process, r.Policy, r.Store)
	if err != nil {
		return nil, err
	}
	if err := supervisor.RegisterLaunch(spec); err != nil {
		return nil, err
	}
	if err := supervisor.Start(ctx, instance.InstanceID, req.Now); err != nil {
		return nil, fmt.Errorf("launch bound plugin: %w", err)
	}
	defer supervisor.Stop(context.Background(), instance.InstanceID)
	transport, handshake, err := plugin.DialGRPCPlugin(ctx, socket, r.CoreProtocol, resolved.Manifest, instance, r.Isolation, req.Alias)
	if err != nil {
		return nil, fmt.Errorf("bound plugin handshake: %w", err)
	}
	defer transport.Close()
	if err := supervisor.MarkReady(instance.InstanceID, handshake); err != nil {
		return nil, err
	}
	return plugin.Gateway{Registry: registry, Transport: transport, LeaseConsumer: r.Store}.Dispatch(ctx, plugin.DispatchRequest{Principal: lease.Lease.Principal, Capability: lease.Lease.Capability, Operation: req.Operation, Scope: req.Scope, RequiredIsolation: r.RequiredIsolation, Binding: lease, Payload: req.Payload, Now: req.Now})
}

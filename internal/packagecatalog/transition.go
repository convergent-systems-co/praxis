package packagecatalog

import (
	"errors"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type TransitionOperation string

const (
	TransitionDisable TransitionOperation = "package.disable"
	TransitionRemove  TransitionOperation = "package.remove"
)

type PackageIdentity struct {
	PackageID     string `json:"package_id"`
	Version       string `json:"version"`
	ContentDigest string `json:"content_digest"`
}

func (i PackageIdentity) Validate() error {
	if i.PackageID == "" || i.Version == "" || i.ContentDigest == "" {
		return errors.New("complete package generation identity is required")
	}
	return nil
}

type TransitionRequest struct {
	Identity   PackageIdentity        `json:"identity"`
	Operation  TransitionOperation    `json:"operation"`
	Intent     contracts.ActionIntent `json:"intent"`
	ApprovalID string                 `json:"approval_id"`
}

func NewTransitionRequest(identity PackageIdentity, operation TransitionOperation, actor contracts.PrincipalRef, approvalID string) (TransitionRequest, error) {
	request, err := newTransitionRequest(identity, operation, actor, approvalID)
	if err != nil {
		return TransitionRequest{}, err
	}
	return request, request.Validate()
}

func (r TransitionRequest) Validate() error {
	if r.ApprovalID == "" {
		return errors.New("package transition approval id is required")
	}
	expected, err := newTransitionRequest(r.Identity, r.Operation, r.Intent.Actor, r.ApprovalID)
	if err != nil {
		return err
	}
	want, err := expected.Intent.Digest()
	if err != nil {
		return err
	}
	got, err := r.Intent.Digest()
	if err != nil {
		return err
	}
	if got != want {
		return errors.New("package transition intent does not bind exact generation and operation")
	}
	return nil
}

func newTransitionRequest(identity PackageIdentity, operation TransitionOperation, actor contracts.PrincipalRef, approvalID string) (TransitionRequest, error) {
	if err := identity.Validate(); err != nil {
		return TransitionRequest{}, err
	}
	if err := actor.Validate(); err != nil {
		return TransitionRequest{}, err
	}
	if operation != TransitionDisable && operation != TransitionRemove {
		return TransitionRequest{}, errors.New("unsupported governed package transition")
	}
	intent := contracts.ActionIntent{Version: packageTransitionIntentVersions.CurrentVersion(), ID: string(operation) + ":" + identity.PackageID + "@" + identity.Version + "#" + identity.ContentDigest, Actor: actor, Operation: string(operation), Target: identity.PackageID + "@" + identity.Version + "#" + identity.ContentDigest, Scope: "package:" + identity.PackageID}
	return TransitionRequest{Identity: identity, Operation: operation, Intent: intent, ApprovalID: approvalID}, nil
}

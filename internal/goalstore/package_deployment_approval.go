package goalstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// DerivePackageDeploymentApproval is the only production approval boundary
// for package installation. It derives the compatibility row from the exact
// durable request, decision, and installation-local package-manager
// generation; callers cannot supply authority semantics as arguments.
func (r Repository) DerivePackageDeploymentApproval(ctx context.Context, requestID, requestVersion string, intent contracts.ActionIntent, now time.Time) (string, error) {
	request, err := r.LoadAuthorityRequest(ctx, requestID, requestVersion, now)
	if err != nil {
		return "", err
	}
	requestDigest, err := request.Digest()
	if err != nil {
		return "", err
	}
	intentDigest, err := intent.Digest()
	if err != nil {
		return "", err
	}
	if request.RequestedAuthority != contracts.GovernedPackageDeploy || request.Delegation != nil || request.RequestedScope == "" || request.IntentDigest != intentDigest || request.InstallationDigest == "" || request.ClosureDigest != intent.Parameters["closure_digest"] || request.VerificationEvidenceDigest == "" || intent.Parameters["verification_evidence_digest"] != request.VerificationEvidenceDigest || intent.Actor != contracts.PackageManagerPrincipal() || intent.Operation != contracts.GovernedPackageDeploy {
		return "", errors.New("package-deploy request does not bind the exact package intent")
	}
	evidence, err := r.Store.LoadVerificationEvidence(ctx, request.VerificationEvidenceDigest)
	if err != nil || evidence.InstallationDigest != request.InstallationDigest || evidence.ClosureDigest != request.ClosureDigest || intent.Parameters["verification_evidence_digest"] != request.VerificationEvidenceDigest {
		return "", errors.New("package-deploy approval does not bind exact durable verification evidence")
	}
	decision, err := r.LoadAuthorityDecision(ctx, requestID, requestVersion, now)
	if err != nil {
		return "", err
	}
	if err := decision.Validate(request, now); err != nil {
		return "", err
	}
	if decision.Outcome != contracts.AuthorityApprove || decision.AuthorityRef == "" || decision.AuthorityGenerationDigest == "" || decision.OperationalAuthorityRef == "" || decision.OperationalAuthorityVersion == "" || decision.OperationalAuthorityGenerationDigest == "" {
		return "", errors.New("package-deploy approval requires an approved exact authority decision")
	}
	if err := r.ValidateAuthorityGeneration(ctx, decision, now); err != nil {
		return "", err
	}
	if err := r.validatePackageDeploymentDecision(ctx, request, decision, now); err != nil {
		return "", err
	}
	generation, err := r.LoadAuthorityGeneration(ctx, decision.OperationalAuthorityRef, decision.OperationalAuthorityVersion, now)
	if err != nil || generation.Digest != decision.OperationalAuthorityGenerationDigest {
		return "", errors.New("package-deploy decision does not bind its exact operational authority generation")
	}
	// Approval derivation is the last gate before a bearer approval exists: the
	// package-manager generation must be current with its whole lineage (N17
	// equivalent path).
	if err := r.requireCurrentLineage(ctx, generation, now); err != nil {
		return "", fmt.Errorf("package-deploy operational authority is not current: %w", err)
	}
	if generation.Principal != contracts.PackageManagerPrincipal() || generation.DelegationProfile != contracts.DelegationProfilePackageDeploy || !containsAuthority(generation.Authorities, contracts.GovernedPackageDeploy) || generation.ParentDigest != request.InstallationDigest {
		return "", errors.New("package-deploy authority is not the exact installation-bound package manager")
	}
	decisionDigest, err := decision.Digest()
	if err != nil {
		return "", err
	}
	approvalID := "package-approval:" + decisionDigest
	expiresAt := generation.ExpiresAt
	if decision.ExpiresAt != nil && (expiresAt == nil || decision.ExpiresAt.Before(*expiresAt)) {
		expiresAt = decision.ExpiresAt
	}
	binding := contracts.ApprovalBinding{ID: approvalID, Approver: generation.Principal, IntentDigest: intentDigest, IssuedAt: now.UTC(), ExpiresAt: expiresAt, RemainingUses: 1, DecisionRef: decision.DecisionRef, DecisionVersion: decision.DecisionVersion, DecisionDigest: decisionDigest, DecisionAuthorityRef: decision.AuthorityRef, DecisionAuthorityVersion: decision.AuthorityVersion, DecisionAuthorityGenerationDigest: decision.AuthorityGenerationDigest, OperationalAuthorityRef: decision.OperationalAuthorityRef, OperationalAuthorityVersion: decision.OperationalAuthorityVersion, OperationalAuthorityGenerationDigest: decision.OperationalAuthorityGenerationDigest, VerificationEvidenceDigest: request.VerificationEvidenceDigest, InstallationDigest: request.InstallationDigest}
	if err := binding.Validate(now); err != nil {
		return "", err
	}
	if r.Store == nil {
		return "", errors.New("state store is required")
	}
	// expires_at is read back as RFC3339Nano text; a bare time value would be
	// stored in the driver's own text form and never parse again.
	var expiresText any
	if expiresAt != nil {
		expiresText = expiresAt.UTC().Format(time.RFC3339Nano)
	}
	_, err = r.Store.DB().ExecContext(ctx, `INSERT INTO approvals(approval_id,approver_id,approver_kind,intent_digest,issued_at,expires_at,remaining_uses,authority_request_id,authority_request_version,authority_request_digest,authority_decision_ref,authority_decision_version,authority_decision_digest,authority_generation_ref,authority_generation_version,authority_generation_digest,authority_model_version,authority_model_digest,installation_digest,closure_digest,decision_authority_ref,decision_authority_version,decision_authority_generation_digest,operational_authority_ref,operational_authority_version,operational_authority_generation_digest,verification_evidence_digest) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(approval_id) DO NOTHING`, approvalID, generation.Principal.ID, generation.Principal.Kind, intentDigest, now.UTC().Format(time.RFC3339Nano), expiresText, 1, request.ID, request.Version, requestDigest, decision.DecisionRef, decision.DecisionVersion, decisionDigest, generation.Ref, generation.Version, generation.Digest, generation.AuthorityModelVersion, generation.AuthorityModelDigest, request.InstallationDigest, request.ClosureDigest, decision.AuthorityRef, decision.AuthorityVersion, decision.AuthorityGenerationDigest, generation.Ref, generation.Version, generation.Digest, request.VerificationEvidenceDigest)
	if err != nil {
		return "", fmt.Errorf("persist derived package approval: %w", err)
	}
	var approverID, approverKind, storedIntent, storedRequestID, storedRequestVersion, storedRequestDigest string
	var storedDecisionRef, storedDecisionVersion, storedDecisionDigest, storedDecisionAuthorityRef, storedDecisionAuthorityVersion, storedDecisionAuthorityDigest string
	var storedOperationalRef, storedOperationalVersion, storedOperationalDigest, storedEvidenceDigest, storedInstallationDigest, storedClosureDigest string
	if err := r.Store.DB().QueryRowContext(ctx, `SELECT approver_id,approver_kind,intent_digest,authority_request_id,authority_request_version,authority_request_digest,authority_decision_ref,authority_decision_version,authority_decision_digest,decision_authority_ref,decision_authority_version,decision_authority_generation_digest,operational_authority_ref,operational_authority_version,operational_authority_generation_digest,verification_evidence_digest,installation_digest,closure_digest FROM approvals WHERE approval_id=?`, approvalID).Scan(&approverID, &approverKind, &storedIntent, &storedRequestID, &storedRequestVersion, &storedRequestDigest, &storedDecisionRef, &storedDecisionVersion, &storedDecisionDigest, &storedDecisionAuthorityRef, &storedDecisionAuthorityVersion, &storedDecisionAuthorityDigest, &storedOperationalRef, &storedOperationalVersion, &storedOperationalDigest, &storedEvidenceDigest, &storedInstallationDigest, &storedClosureDigest); err != nil {
		return "", fmt.Errorf("reload derived package approval: %w", err)
	}
	if approverID != binding.Approver.ID || approverKind != binding.Approver.Kind || storedIntent != binding.IntentDigest || storedRequestID != request.ID || storedRequestVersion != request.Version || storedRequestDigest != requestDigest || storedDecisionRef != binding.DecisionRef || storedDecisionVersion != binding.DecisionVersion || storedDecisionDigest != binding.DecisionDigest || storedDecisionAuthorityRef != binding.DecisionAuthorityRef || storedDecisionAuthorityVersion != binding.DecisionAuthorityVersion || storedDecisionAuthorityDigest != binding.DecisionAuthorityGenerationDigest || storedOperationalRef != binding.OperationalAuthorityRef || storedOperationalVersion != binding.OperationalAuthorityVersion || storedOperationalDigest != binding.OperationalAuthorityGenerationDigest || storedEvidenceDigest != binding.VerificationEvidenceDigest || storedInstallationDigest != binding.InstallationDigest || storedClosureDigest != request.ClosureDigest {
		return "", errors.New("existing package approval does not match canonical governed lineage")
	}
	return approvalID, nil
}

// SavePackageDeploymentRequest creates the canonical exact request after
// package verification. It is not an approval and does not mutate installed
// package state.
func (r Repository) SavePackageDeploymentRequest(ctx context.Context, intent contracts.ActionIntent, installationDigest, closureDigest, evidenceDigest string, now time.Time) (contracts.AuthorityRequest, string, error) {
	if intent.Actor != contracts.PackageManagerPrincipal() || intent.Operation != contracts.GovernedPackageDeploy || installationDigest == "" || closureDigest == "" || evidenceDigest == "" || intent.Parameters["closure_digest"] != closureDigest || intent.Parameters["verification_evidence_digest"] != evidenceDigest {
		return contracts.AuthorityRequest{}, "", errors.New("package-deploy intent is not canonical")
	}
	evidence, err := r.Store.LoadVerificationEvidence(ctx, evidenceDigest)
	if err != nil || evidence.InstallationDigest != installationDigest || evidence.ClosureDigest != closureDigest {
		return contracts.AuthorityRequest{}, "", errors.New("package-deploy request does not bind exact durable verification evidence")
	}
	intentDigest, err := intent.Digest()
	if err != nil {
		return contracts.AuthorityRequest{}, "", err
	}
	manager, err := r.ResolvePackageManagerAuthority(ctx, installationDigest, now)
	if err != nil {
		return contracts.AuthorityRequest{}, "", err
	}
	root, err := r.LoadAuthorityGeneration(ctx, manager.ParentRef, manager.ParentVersion, now)
	if err != nil || root.Digest != installationDigest {
		return contracts.AuthorityRequest{}, "", errors.New("package-deploy request installation root mismatch")
	}
	request := contracts.AuthorityRequest{ID: "package-deploy-request:" + intentDigest, Version: "1", RequestedAuthority: contracts.GovernedPackageDeploy, RequestedScope: root.Scope, Reason: "deploy exact verified package closure", Status: contracts.AuthorityRequestPending, IntentDigest: intentDigest, InstallationDigest: installationDigest, ClosureDigest: closureDigest, VerificationEvidenceDigest: evidenceDigest, Intent: &intent}
	digest, err := r.SaveAuthorityRequest(ctx, request, now, manager.ExpiresAt)
	if err != nil {
		return contracts.AuthorityRequest{}, "", err
	}
	return request, digest, nil
}

// DB is intentionally narrow and read/write scoped to the repository store.
func (r Repository) DB() *sql.DB { return r.Store.DB() }

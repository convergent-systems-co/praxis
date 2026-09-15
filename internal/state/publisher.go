package state

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type PublisherGenerationRecord struct {
	Generation contracts.PublisherGeneration `json:"generation"`
	Digest     string                        `json:"digest"`
	State      string                        `json:"state"`
}

// EnrollPublisherGeneration durably records an owner-authorized publisher
// generation. It stores no private key material; signing remains delegated to
// the protected key backend named by the owner-controlled ceremony.
func (s *Store) EnrollPublisherGeneration(ctx context.Context, generation contracts.PublisherGeneration, intent contracts.ActionIntent, approvalID string, now time.Time) (string, error) {
	if s == nil || s.db == nil {
		return "", errors.New("state store is required")
	}
	if err := generation.Validate(); err != nil {
		return "", err
	}
	if err := intent.Validate(); err != nil {
		return "", err
	}
	if approvalID == "" || now.IsZero() {
		return "", errors.New("publisher enrollment approval and time are required")
	}
	digest, err := generation.Digest()
	if err != nil {
		return "", err
	}
	intentDigest, err := intent.Digest()
	if err != nil {
		return "", err
	}
	if intent.Operation != "publisher.enroll" || intent.Target != digest || intent.Scope != "package:"+generation.PackageNamespace {
		return "", errors.New("publisher enrollment intent does not bind the exact generation and namespace")
	}
	body, err := json.Marshal(generation)
	if err != nil {
		return "", err
	}
	commandID := "publisher-enrollment:" + digest
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if err := insertCommand(ctx, tx, CommandRecord{ID: commandID, Type: "publisher.enrollment", Version: "v1", Actor: intent.Actor, Scope: intent.Scope, CorrelationID: commandID, Payload: body, CreatedAt: now}); err != nil {
		return "", err
	}
	if err := compareAndAdvanceAggregate(ctx, tx, digest, "publisher_generation", 0, 1); err != nil {
		return "", err
	}
	if err := consumePackageApproval(ctx, tx, approvalID, intent.Actor, intentDigest, now); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO publisher_generations(publisher_generation_digest,principal_id,principal_kind,generation,record_json,state,enrollment_approval_id,enrolled_at) VALUES(?,?,?,?,?,'active',?,?)`, digest, generation.Principal.ID, generation.Principal.Kind, generation.Generation, body, approvalID, now.UTC().Format(time.RFC3339Nano)); err != nil {
		return "", err
	}
	eventPayload, _ := json.Marshal(map[string]string{"publisher_generation_digest": digest, "enrollment_intent_digest": intentDigest})
	event := EventRecord{ID: commandID + ":event", AggregateID: digest, AggregateType: "publisher_generation", AggregateVersion: 1, Type: "publisher_generation.enrolled", Version: "v1", Actor: intent.Actor, CommandID: commandID, CorrelationID: commandID, TrustClass: contracts.TrustPolicy, Payload: eventPayload, CreatedAt: now}
	if err := insertEvent(ctx, tx, event); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE commands SET status='committed',completed_at=? WHERE command_id=?`, now.UTC().Format(time.RFC3339Nano), commandID); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return digest, nil
}

func (s *Store) PublisherGeneration(ctx context.Context, digest string) (PublisherGenerationRecord, error) {
	if s == nil || s.db == nil || digest == "" {
		return PublisherGenerationRecord{}, errors.New("state store and publisher digest are required")
	}
	var record PublisherGenerationRecord
	var body []byte
	if err := s.db.QueryRowContext(ctx, `SELECT record_json,state FROM publisher_generations WHERE publisher_generation_digest=?`, digest).Scan(&body, &record.State); err != nil {
		return record, err
	}
	if err := json.Unmarshal(body, &record.Generation); err != nil {
		return record, fmt.Errorf("decode publisher generation: %w", err)
	}
	actual, err := record.Generation.Digest()
	if err != nil || actual != digest {
		return record, errors.New("publisher generation digest mismatch")
	}
	record.Digest = digest
	return record, nil
}

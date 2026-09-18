package contracts

import (
	"testing"
	"time"
)

func TestActionIntentDigestDeterministicAcrossMapOrder(t *testing.T) {
	a := ActionIntent{Version:"v1", ID:"a1", Actor:PrincipalRef{ID:"p1",Kind:"agent"}, Operation:"write", Target:"file:x", Scope:"workspace:1", Parameters:map[string]string{"b":"2","a":"1"}}
	b := a
	b.Parameters = map[string]string{"a":"1","b":"2"}
	da, err := a.Digest(); if err != nil { t.Fatal(err) }
	db, err := b.Digest(); if err != nil { t.Fatal(err) }
	if da != db { t.Fatalf("canonical digest differs: %s != %s", da, db) }
}

func TestApprovalRejectsMutatedIntent(t *testing.T) {
	now := time.Now().UTC()
	intent := ActionIntent{Version:"v1", ID:"a1", Actor:PrincipalRef{ID:"p1",Kind:"agent"}, Operation:"write", Target:"file:x", Scope:"workspace:1", Parameters:map[string]string{"content":"one"}}
	d, err := intent.Digest(); if err != nil { t.Fatal(err) }
	approval := ApprovalBinding{ID:"ap1", Approver:PrincipalRef{ID:"u1",Kind:"human"}, IntentDigest:d, IssuedAt:now, RemainingUses:1}
	if err := approval.AuthorizesIntent(intent, now); err != nil { t.Fatalf("bound intent rejected: %v", err) }
	intent.Parameters["content"] = "two"
	if err := approval.AuthorizesIntent(intent, now); err == nil { t.Fatal("mutated intent must invalidate approval") }
}

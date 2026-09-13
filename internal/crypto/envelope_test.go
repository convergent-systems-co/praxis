package crypto

import (
	"context"
	"errors"
	"testing"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type fakeWrapper struct {
	caps Capabilities
	selectedOverride contracts.CryptoProfile
	key []byte
}

func (f *fakeWrapper) Capabilities(_ context.Context, _ string) (Capabilities,error) { return f.caps,nil }
func (f *fakeWrapper) Wrap(_ context.Context, keyRef string, profile contracts.CryptoProfile, plaintextKey []byte) (WrappedKey,error) {
	selected:=profile; if f.selectedOverride!="" { selected=f.selectedOverride }
	f.key=append([]byte(nil),plaintextKey...)
	return WrappedKey{Ciphertext:[]byte("wrapped"),SuiteID:"test-suite",KeyRef:keyRef,KeyVersion:"1",SelectedProfile:selected},nil
}
func (f *fakeWrapper) Unwrap(_ context.Context, _ WrappedKey) ([]byte,error) { if len(f.key)==0 { return nil,errors.New("no key") }; return append([]byte(nil),f.key...),nil }

func TestEnvelopeRoundTripWithPQRequired(t *testing.T) {
	w:=&fakeWrapper{caps:Capabilities{PQ:true}}
	s:=EnvelopeService{Wrapper:w}
	env,err:=s.Seal(context.Background(),"key:1",contracts.CryptoPQRequired,[]byte("secret baseline"),[]byte("goal:g1:v1")); if err!=nil { t.Fatal(err) }
	if string(env.Ciphertext)=="secret baseline" { t.Fatal("plaintext must not be stored as ciphertext") }
	if env.WrappedDEK.SelectedProfile!=contracts.CryptoPQRequired { t.Fatalf("unexpected selected profile %s",env.WrappedDEK.SelectedProfile) }
	plain,err:=s.Open(context.Background(),env,[]byte("goal:g1:v1")); if err!=nil { t.Fatal(err) }
	if string(plain)!="secret baseline" { t.Fatalf("unexpected plaintext %q",plain) }
}

func TestEnvelopePQRequiredFailsBeforeClassicalFallback(t *testing.T) {
	w:=&fakeWrapper{caps:Capabilities{Classical:true}}
	_,err:= (EnvelopeService{Wrapper:w}).Seal(context.Background(),"key:1",contracts.CryptoPQRequired,[]byte("x"),nil)
	if err==nil { t.Fatal("pq-required must fail when PQ protection is unavailable") }
}

func TestEnvelopePQPreferredFallbackRequiresExplicitPolicy(t *testing.T) {
	w:=&fakeWrapper{caps:Capabilities{Classical:true}}
	_,err:= (EnvelopeService{Wrapper:w}).Seal(context.Background(),"key:1",contracts.CryptoPQPreferred,[]byte("x"),nil)
	if err==nil { t.Fatal("pq-preferred must not silently downgrade") }
	env,err:= (EnvelopeService{Wrapper:w,Policy:EnvelopePolicy{AllowPQPreferredFallback:true}}).Seal(context.Background(),"key:1",contracts.CryptoPQPreferred,[]byte("x"),nil); if err!=nil { t.Fatal(err) }
	if env.WrappedDEK.SelectedProfile!=contracts.CryptoClassicalCompatible { t.Fatalf("expected explicit classical fallback, got %s",env.WrappedDEK.SelectedProfile) }
}

func TestEnvelopeRejectsWrapperLyingAboutSelectedProfile(t *testing.T) {
	w:=&fakeWrapper{caps:Capabilities{PQ:true},selectedOverride:contracts.CryptoClassicalCompatible}
	_,err:= (EnvelopeService{Wrapper:w}).Seal(context.Background(),"key:1",contracts.CryptoPQRequired,[]byte("x"),nil)
	if err==nil { t.Fatal("wrapper cannot silently substitute a weaker selected profile") }
}

func TestEnvelopeAADBindsCiphertextToRecordIdentity(t *testing.T) {
	w:=&fakeWrapper{caps:Capabilities{PQ:true}}
	s:=EnvelopeService{Wrapper:w}
	env,err:=s.Seal(context.Background(),"key:1",contracts.CryptoPQRequired,[]byte("x"),[]byte("goal:g1")); if err!=nil { t.Fatal(err) }
	if _,err:=s.Open(context.Background(),env,[]byte("goal:g2")); err==nil { t.Fatal("different record identity must fail authentication boundary") }
}

func TestOpenRejectsPersistedPQDowngradeMetadata(t *testing.T) {
	w:=&fakeWrapper{caps:Capabilities{PQ:true}}
	s:=EnvelopeService{Wrapper:w}
	env,err:=s.Seal(context.Background(),"key:1",contracts.CryptoPQRequired,[]byte("x"),nil); if err!=nil { t.Fatal(err) }
	env.WrappedDEK.SelectedProfile=contracts.CryptoClassicalCompatible
	if _,err:=s.Open(context.Background(),env,nil); err==nil { t.Fatal("persisted pq-required downgrade must fail") }
}

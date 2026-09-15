//go:build darwin

package crypto

/*
#cgo darwin LDFLAGS: -framework Security -framework CoreFoundation
#include <CoreFoundation/CoreFoundation.h>
#include <Security/Security.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
static CFStringRef ps_string(const char *v) { return CFStringCreateWithCString(NULL, v, kCFStringEncodingUTF8); }
static OSStatus ps_add(const char *service, const char *account, const uint8_t *bytes, size_t length) {
 CFStringRef s=ps_string(service), a=ps_string(account); CFDataRef d=CFDataCreate(NULL,bytes,(CFIndex)length);
 const void *k[]={kSecClass,kSecAttrService,kSecAttrAccount,kSecAttrAccessible,kSecValueData};
 const void *v[]={kSecClassGenericPassword,s,a,kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly,d};
 CFDictionaryRef q=CFDictionaryCreate(NULL,k,v,5,&kCFTypeDictionaryKeyCallBacks,&kCFTypeDictionaryValueCallBacks); OSStatus st=SecItemAdd(q,NULL);
 CFRelease(q); CFRelease(d); CFRelease(a); CFRelease(s); return st;
}
static OSStatus ps_copy(const char *service, const char *account, uint8_t **bytes, size_t *length) {
 CFStringRef s=ps_string(service), a=ps_string(account); const void *k[]={kSecClass,kSecAttrService,kSecAttrAccount,kSecReturnData,kSecMatchLimit}; const void *v[]={kSecClassGenericPassword,s,a,kCFBooleanTrue,kSecMatchLimitOne};
 CFDictionaryRef q=CFDictionaryCreate(NULL,k,v,5,&kCFTypeDictionaryKeyCallBacks,&kCFTypeDictionaryValueCallBacks); CFTypeRef r=NULL; OSStatus st=SecItemCopyMatching(q,&r);
 if(st==errSecSuccess && r && CFGetTypeID(r)==CFDataGetTypeID()){ CFIndex n=CFDataGetLength((CFDataRef)r); *bytes=malloc((size_t)n); if(*bytes){*length=(size_t)n; memcpy(*bytes,CFDataGetBytePtr((CFDataRef)r),(size_t)n);} else st=errSecAllocate; }
 if(r)CFRelease(r); CFRelease(q); CFRelease(a); CFRelease(s); return st;
}
static OSStatus ps_delete(const char *service, const char *account) { CFStringRef s=ps_string(service),a=ps_string(account); const void *k[]={kSecClass,kSecAttrService,kSecAttrAccount}; const void *v[]={kSecClassGenericPassword,s,a}; CFDictionaryRef q=CFDictionaryCreate(NULL,k,v,3,&kCFTypeDictionaryKeyCallBacks,&kCFTypeDictionaryValueCallBacks); OSStatus st=SecItemDelete(q); CFRelease(q);CFRelease(a);CFRelease(s);return st; }
*/
import "C"

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"unsafe"
)

const publisherSigningService = "com.convergent-systems-co.praxis.publisher-signing"

type MacOSKeychainPublisherBackend struct{}

func NewMacOSKeychainPublisherBackend() *MacOSKeychainPublisherBackend {
	return &MacOSKeychainPublisherBackend{}
}
func (MacOSKeychainPublisherBackend) Generate(ctx context.Context, keyID string) (PublisherSigner, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if keyID == "" {
		return nil, errors.New("publisher signing key id is required")
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	account := C.CString(keyID)
	service := C.CString(publisherSigningService)
	defer C.free(unsafe.Pointer(account))
	defer C.free(unsafe.Pointer(service))
	if status := C.ps_add(service, account, (*C.uint8_t)(unsafe.Pointer(&priv[0])), C.size_t(len(priv))); status != C.errSecSuccess {
		return nil, fmt.Errorf("store publisher signing key in Keychain: status %d", int(status))
	}
	return macOSPublisherSigner{keyID: keyID, public: append([]byte(nil), pub...)}, nil
}
func (MacOSKeychainPublisherBackend) Open(ctx context.Context, keyID string) (PublisherSigner, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if keyID == "" {
		return nil, errors.New("publisher signing key id is required")
	}
	return macOSPublisherSigner{keyID: keyID}, nil
}
func (MacOSKeychainPublisherBackend) Delete(_ context.Context, keyID string) error {
	a := C.CString(keyID)
	s := C.CString(publisherSigningService)
	defer C.free(unsafe.Pointer(a))
	defer C.free(unsafe.Pointer(s))
	if status := C.ps_delete(s, a); status != C.errSecSuccess {
		return fmt.Errorf("delete publisher signing key: status %d", int(status))
	}
	return nil
}

type macOSPublisherSigner struct {
	keyID  string
	public []byte
}

func (s macOSPublisherSigner) KeyID() string   { return s.keyID }
func (macOSPublisherSigner) Algorithm() string { return "ed25519" }
func (s macOSPublisherSigner) load() ([]byte, error) {
	a := C.CString(s.keyID)
	svc := C.CString(publisherSigningService)
	defer C.free(unsafe.Pointer(a))
	defer C.free(unsafe.Pointer(svc))
	var p *C.uint8_t
	var n C.size_t
	if status := C.ps_copy(svc, a, &p, &n); status != C.errSecSuccess {
		return nil, fmt.Errorf("load publisher signing key: status %d", int(status))
	}
	defer C.free(unsafe.Pointer(p))
	return C.GoBytes(unsafe.Pointer(p), C.int(n)), nil
}
func (s macOSPublisherSigner) PublicKey(ctx context.Context) ([]byte, error) {
	if len(s.public) == ed25519.PublicKeySize {
		return append([]byte(nil), s.public...), nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	priv, err := s.load()
	if err != nil {
		return nil, err
	}
	defer zero(priv)
	if len(priv) != ed25519.PrivateKeySize {
		return nil, errors.New("publisher signing key has invalid size")
	}
	return append([]byte(nil), priv[32:]...), nil
}
func (s macOSPublisherSigner) Sign(ctx context.Context, msg []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	priv, err := s.load()
	if err != nil {
		return nil, err
	}
	defer zero(priv)
	if len(priv) != ed25519.PrivateKeySize {
		return nil, errors.New("publisher signing key has invalid size")
	}
	return ed25519.Sign(ed25519.PrivateKey(priv), msg), nil
}

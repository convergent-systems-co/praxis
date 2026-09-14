//go:build darwin

package crypto

/*
#cgo darwin LDFLAGS: -framework Security -framework CoreFoundation
#include <CoreFoundation/CoreFoundation.h>
#include <Security/Security.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

static CFStringRef praxis_string(const char *value) {
	return CFStringCreateWithCString(NULL, value, kCFStringEncodingUTF8);
}

static OSStatus praxis_keychain_add(const char *service, const char *account, const uint8_t *bytes, size_t length) {
	CFStringRef service_ref = praxis_string(service);
	CFStringRef account_ref = praxis_string(account);
	CFDataRef data_ref = CFDataCreate(NULL, bytes, (CFIndex)length);
	const void *keys[] = { kSecClass, kSecAttrService, kSecAttrAccount, kSecAttrAccessible, kSecValueData };
	const void *values[] = { kSecClassGenericPassword, service_ref, account_ref, kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly, data_ref };
	CFDictionaryRef query = CFDictionaryCreate(NULL, keys, values, 5, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	OSStatus status = SecItemAdd(query, NULL);
	CFRelease(query); CFRelease(data_ref); CFRelease(account_ref); CFRelease(service_ref);
	return status;
}

static OSStatus praxis_keychain_copy(const char *service, const char *account, uint8_t **bytes, size_t *length) {
	CFStringRef service_ref = praxis_string(service);
	CFStringRef account_ref = praxis_string(account);
	const void *keys[] = { kSecClass, kSecAttrService, kSecAttrAccount, kSecReturnData, kSecMatchLimit };
	const void *values[] = { kSecClassGenericPassword, service_ref, account_ref, kCFBooleanTrue, kSecMatchLimitOne };
	CFDictionaryRef query = CFDictionaryCreate(NULL, keys, values, 5, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	CFTypeRef result = NULL;
	OSStatus status = SecItemCopyMatching(query, &result);
	if (status == errSecSuccess) {
		if (result == NULL || CFGetTypeID(result) != CFDataGetTypeID()) {
			status = errSecDecode;
		} else {
			CFIndex count = CFDataGetLength((CFDataRef)result);
			uint8_t *copy = (uint8_t *)malloc((size_t)count);
			if (copy == NULL) {
				status = errSecAllocate;
			} else {
				memcpy(copy, CFDataGetBytePtr((CFDataRef)result), (size_t)count);
				*bytes = copy; *length = (size_t)count;
			}
		}
	}
	if (result != NULL) CFRelease(result);
	CFRelease(query); CFRelease(account_ref); CFRelease(service_ref);
	return status;
}

static OSStatus praxis_keychain_delete(const char *service, const char *account) {
	CFStringRef service_ref = praxis_string(service);
	CFStringRef account_ref = praxis_string(account);
	const void *keys[] = { kSecClass, kSecAttrService, kSecAttrAccount };
	const void *values[] = { kSecClassGenericPassword, service_ref, account_ref };
	CFDictionaryRef query = CFDictionaryCreate(NULL, keys, values, 3, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	OSStatus status = SecItemDelete(query);
	CFRelease(query); CFRelease(account_ref); CFRelease(service_ref);
	return status;
}
*/
import "C"

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"runtime"
	"time"
	"unsafe"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

const (
	MacOSKeychainProviderID = "macos-keychain"
	macOSKeychainService    = "com.convergent-systems-co.praxis"
	macOSKeychainSuite      = "AES-256-GCM-KEYCHAIN-V1"
)

var (
	ErrKeychainUnavailable = errors.New("macOS Keychain is unavailable")
	ErrKeychainItemMissing = errors.New("macOS Keychain item is missing")
	ErrKeychainDuplicate   = errors.New("macOS Keychain item already exists")
)

// MacOSKeychainBackend protects a Praxis KEK in a user Keychain item. The
// Secure Enclave is deliberately not required: Keychain works on Intel and
// Apple Silicon, while a future hardware-backed backend can use this same
// BootstrapBackend contract.
type MacOSKeychainBackend struct{ Service string }

func NewMacOSKeychainBackend() *MacOSKeychainBackend {
	return &MacOSKeychainBackend{Service: macOSKeychainService}
}

func (b *MacOSKeychainBackend) ProviderID() string { return MacOSKeychainProviderID }

func (b *MacOSKeychainBackend) SecurityLevel(context.Context) (SecurityLevel, error) {
	if b == nil || b.Service == "" {
		return "", ErrKeychainUnavailable
	}
	return SecurityPlatformProtected, nil
}

func (b *MacOSKeychainBackend) Available(ctx context.Context) error {
	if b == nil || b.Service == "" {
		return ErrKeychainUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func (b *MacOSKeychainBackend) Bootstrap(ctx context.Context, request BootstrapRequest) (BootstrapRecord, KeyWrapper, error) {
	if err := b.Available(ctx); err != nil {
		return BootstrapRecord{}, nil, err
	}
	if err := request.Validate(); err != nil {
		return BootstrapRecord{}, nil, err
	}
	if request.Profile != contracts.CryptoClassicalCompatible {
		return BootstrapRecord{}, nil, errors.New("macOS Keychain backend supports only classical-compatible wrapping")
	}
	rootKey := make([]byte, 32)
	defer zero(rootKey)
	if _, err := io.ReadFull(rand.Reader, rootKey); err != nil {
		return BootstrapRecord{}, nil, fmt.Errorf("generate Praxis Keychain wrapping key: %w", err)
	}
	account := keychainAccount(request.KeyID, "1")
	serviceRef := C.CString(b.Service)
	accountRef := C.CString(account)
	defer C.free(unsafe.Pointer(serviceRef))
	defer C.free(unsafe.Pointer(accountRef))
	status := C.praxis_keychain_add(serviceRef, accountRef, (*C.uint8_t)(unsafe.Pointer(&rootKey[0])), C.size_t(len(rootKey)))
	if status == C.errSecDuplicateItem {
		return BootstrapRecord{}, nil, ErrKeychainDuplicate
	}
	if status != C.errSecSuccess {
		return BootstrapRecord{}, nil, keychainStatus("add", status)
	}
	record := BootstrapRecord{Version: BootstrapRecordVersion, ProviderID: b.ProviderID(), KeyID: request.KeyID, KeyVersion: "1", KeyMaterialHash: keyMaterialHash(rootKey), Owner: request.Owner, Purpose: request.Purpose, Profile: request.Profile, SecurityLevel: SecurityPlatformProtected, Platform: runtime.GOOS, Architecture: runtime.GOARCH, CreatedAt: time.Now().UTC()}
	return record, &macOSKeychainWrapper{backend: b, keyID: request.KeyID, keyVersion: "1", expectedHash: record.KeyMaterialHash}, nil
}

func (b *MacOSKeychainBackend) Open(ctx context.Context, record BootstrapRecord) (KeyWrapper, error) {
	if err := b.Available(ctx); err != nil {
		return nil, err
	}
	if err := record.Validate(); err != nil {
		return nil, err
	}
	if record.ProviderID != b.ProviderID() || record.Platform != runtime.GOOS || record.Architecture != runtime.GOARCH {
		return nil, errors.New("macOS Keychain bootstrap binding does not match this provider or platform")
	}
	wrapper := &macOSKeychainWrapper{backend: b, keyID: record.KeyID, keyVersion: record.KeyVersion, expectedHash: record.KeyMaterialHash}
	if _, err := wrapper.rootKey(); err != nil {
		return nil, err
	}
	return wrapper, nil
}

type macOSKeychainWrapper struct {
	backend           *MacOSKeychainBackend
	keyID, keyVersion string
	expectedHash      string
}

func (w *macOSKeychainWrapper) Capabilities(ctx context.Context, keyRef string) (Capabilities, error) {
	if keyRef != w.keyID {
		return Capabilities{}, errors.New("Keychain key reference does not match bootstrap identity")
	}
	if _, err := w.rootKey(); err != nil {
		return Capabilities{}, err
	}
	return Capabilities{Classical: true}, nil
}

func (w *macOSKeychainWrapper) Wrap(ctx context.Context, keyRef string, profile contracts.CryptoProfile, plaintextKey []byte) (WrappedKey, error) {
	if keyRef != w.keyID {
		return WrappedKey{}, errors.New("Keychain key reference does not match bootstrap identity")
	}
	if profile != contracts.CryptoClassicalCompatible || len(plaintextKey) != 32 {
		return WrappedKey{}, errors.New("macOS Keychain wrapping requires classical 256-bit key material")
	}
	root, err := w.rootKey()
	if err != nil {
		return WrappedKey{}, err
	}
	defer zero(root)
	block, err := aes.NewCipher(root)
	if err != nil {
		return WrappedKey{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return WrappedKey{}, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return WrappedKey{}, err
	}
	ciphertext := gcm.Seal(nonce, nonce, plaintextKey, keychainAAD(keyRef, w.keyVersion, profile))
	return WrappedKey{Ciphertext: ciphertext, SuiteID: macOSKeychainSuite, KeyRef: keyRef, KeyVersion: w.keyVersion, SelectedProfile: profile}, nil
}

func (w *macOSKeychainWrapper) Unwrap(ctx context.Context, wrapped WrappedKey) ([]byte, error) {
	if wrapped.KeyRef != w.keyID || wrapped.KeyVersion != w.keyVersion || wrapped.SuiteID != macOSKeychainSuite || wrapped.SelectedProfile != contracts.CryptoClassicalCompatible {
		return nil, errors.New("Keychain wrapped-key binding mismatch")
	}
	root, err := w.rootKey()
	if err != nil {
		return nil, err
	}
	defer zero(root)
	block, err := aes.NewCipher(root)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(wrapped.Ciphertext) < gcm.NonceSize() {
		return nil, errors.New("Keychain wrapped-key ciphertext is truncated")
	}
	nonce, ciphertext := wrapped.Ciphertext[:gcm.NonceSize()], wrapped.Ciphertext[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ciphertext, keychainAAD(wrapped.KeyRef, wrapped.KeyVersion, wrapped.SelectedProfile))
}

func (w *macOSKeychainWrapper) rootKey() ([]byte, error) {
	if w == nil || w.backend == nil {
		return nil, ErrKeychainUnavailable
	}
	account := keychainAccount(w.keyID, w.keyVersion)
	var bytes *C.uint8_t
	var length C.size_t
	serviceRef := C.CString(w.backend.Service)
	accountRef := C.CString(account)
	defer C.free(unsafe.Pointer(serviceRef))
	defer C.free(unsafe.Pointer(accountRef))
	status := C.praxis_keychain_copy(serviceRef, accountRef, &bytes, &length)
	if status == C.errSecItemNotFound {
		return nil, ErrKeychainItemMissing
	}
	if status != C.errSecSuccess {
		return nil, keychainStatus("read", status)
	}
	defer C.free(unsafe.Pointer(bytes))
	key := C.GoBytes(unsafe.Pointer(bytes), C.int(length))
	if len(key) != 32 {
		zero(key)
		return nil, errors.New("Keychain wrapping key has invalid length")
	}
	if w.expectedHash != "" && keyMaterialHash(key) != w.expectedHash {
		zero(key)
		return nil, errors.New("Keychain item does not match bootstrap binding")
	}
	return key, nil
}

func (b *MacOSKeychainBackend) deleteForTest(keyID, version string) error {
	if b == nil || b.Service == "" {
		return ErrKeychainUnavailable
	}
	serviceRef := C.CString(b.Service)
	accountRef := C.CString(keychainAccount(keyID, version))
	defer C.free(unsafe.Pointer(serviceRef))
	defer C.free(unsafe.Pointer(accountRef))
	status := C.praxis_keychain_delete(serviceRef, accountRef)
	if status == C.errSecItemNotFound {
		return ErrKeychainItemMissing
	}
	if status != C.errSecSuccess {
		return keychainStatus("delete", status)
	}
	return nil
}

func keychainAccount(keyID, version string) string { return keyID + "@" + version }
func keyMaterialHash(key []byte) string {
	digest := sha256.Sum256(key)
	return "sha256:" + hex.EncodeToString(digest[:])
}
func keychainAAD(keyID, version string, profile contracts.CryptoProfile) []byte {
	return []byte(keyID + "\x00" + version + "\x00" + string(profile))
}

func keychainStatus(operation string, status C.OSStatus) error {
	if status == C.errSecItemNotFound {
		return fmt.Errorf("%w: %s", ErrKeychainItemMissing, operation)
	}
	return fmt.Errorf("%w: %s operation failed (status %d)", ErrKeychainUnavailable, operation, int(status))
}

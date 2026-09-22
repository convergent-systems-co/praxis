//go:build darwin

package crypto

/*
#cgo darwin LDFLAGS: -framework Security -framework CoreFoundation
#include <CoreFoundation/CoreFoundation.h>
#include <Security/Security.h>
#include <dlfcn.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

#pragma clang diagnostic ignored "-Wdeprecated-declarations"

static CFStringRef faa_string(const char *value) {
	return CFStringCreateWithCString(NULL, value, kCFStringEncodingUTF8);
}

// A NULL keychain means the user's default (login) keychain; otherwise the item
// is added to, and matched only within, the given dedicated keychain.
static CFMutableDictionaryRef faa_query(SecKeychainRef kc, CFStringRef service, CFStringRef account, int add) {
	CFMutableDictionaryRef q = CFDictionaryCreateMutable(NULL, 0, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	CFDictionarySetValue(q, kSecClass, kSecClassGenericPassword);
	CFDictionarySetValue(q, kSecAttrService, service);
	CFDictionarySetValue(q, kSecAttrAccount, account);
	if (kc != NULL) {
		if (add) {
			CFDictionarySetValue(q, kSecUseKeychain, kc);
		} else {
			CFArrayRef list = CFArrayCreate(NULL, (const void **)&kc, 1, &kCFTypeArrayCallBacks);
			CFDictionarySetValue(q, kSecMatchSearchList, list);
			CFRelease(list);
		}
	}
	return q;
}

static OSStatus faa_keychain_add(SecKeychainRef kc, const char *service, const char *account, const uint8_t *bytes, size_t length) {
	CFStringRef service_ref = faa_string(service);
	CFStringRef account_ref = faa_string(account);
	CFDataRef data_ref = CFDataCreate(NULL, bytes, (CFIndex)length);
	CFMutableDictionaryRef query = faa_query(kc, service_ref, account_ref, 1);
	CFDictionarySetValue(query, kSecValueData, data_ref);
	OSStatus status = SecItemAdd(query, NULL);
	CFRelease(query); CFRelease(data_ref); CFRelease(account_ref); CFRelease(service_ref);
	return status;
}

static OSStatus faa_keychain_update(SecKeychainRef kc, const char *service, const char *account, const uint8_t *bytes, size_t length) {
	CFStringRef service_ref = faa_string(service);
	CFStringRef account_ref = faa_string(account);
	CFDataRef data_ref = CFDataCreate(NULL, bytes, (CFIndex)length);
	CFMutableDictionaryRef query = faa_query(kc, service_ref, account_ref, 0);
	const void *akeys[] = { kSecValueData };
	const void *avalues[] = { data_ref };
	CFDictionaryRef attrs = CFDictionaryCreate(NULL, akeys, avalues, 1, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	OSStatus status = SecItemUpdate(query, attrs);
	CFRelease(attrs); CFRelease(query); CFRelease(data_ref); CFRelease(account_ref); CFRelease(service_ref);
	return status;
}

static OSStatus faa_keychain_copy(SecKeychainRef kc, const char *service, const char *account, uint8_t **bytes, size_t *length) {
	CFStringRef service_ref = faa_string(service);
	CFStringRef account_ref = faa_string(account);
	CFMutableDictionaryRef query = faa_query(kc, service_ref, account_ref, 0);
	CFDictionarySetValue(query, kSecReturnData, kCFBooleanTrue);
	CFDictionarySetValue(query, kSecMatchLimit, kSecMatchLimitOne);
	CFTypeRef result = NULL;
	OSStatus status = SecItemCopyMatching(query, &result);
	if (status == errSecSuccess) {
		if (result == NULL || CFGetTypeID(result) != CFDataGetTypeID()) {
			status = errSecDecode;
		} else {
			CFIndex count = CFDataGetLength((CFDataRef)result);
			uint8_t *copy = (uint8_t *)malloc((size_t)count + 1);
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

static OSStatus faa_keychain_delete(SecKeychainRef kc, const char *service, const char *account) {
	CFStringRef service_ref = faa_string(service);
	CFStringRef account_ref = faa_string(account);
	CFMutableDictionaryRef query = faa_query(kc, service_ref, account_ref, 0);
	OSStatus status = SecItemDelete(query);
	CFRelease(query); CFRelease(account_ref); CFRelease(service_ref);
	return status;
}

// faa_kc_create makes a dedicated keychain file protected by a password. It
// locks itself after a few seconds of inactivity so that a crashed process
// cannot leave it unlocked.
static OSStatus faa_kc_create(const char *path, const char *password, SecKeychainRef *out) {
	SecKeychainRef kc = NULL;
	OSStatus status = SecKeychainCreate(path, (UInt32)strlen(password), password, false, NULL, &kc);
	if (status != errSecSuccess) return status;
	SecKeychainSettings settings;
	settings.version = SEC_KEYCHAIN_SETTINGS_VERS1;
	settings.lockOnSleep = true;
	settings.useLockInterval = true;
	settings.lockInterval = 5;
	status = SecKeychainSetSettings(kc, &settings);
	if (status != errSecSuccess) { SecKeychainLock(kc); CFRelease(kc); return status; }
	*out = kc;
	return errSecSuccess;
}

// faa_kc_open opens an existing dedicated keychain and unlocks it with the password.
static OSStatus faa_kc_open(const char *path, const char *password, SecKeychainRef *out) {
	SecKeychainRef kc = NULL;
	OSStatus status = SecKeychainOpen(path, &kc);
	if (status != errSecSuccess) return status;
	status = SecKeychainUnlock(kc, (UInt32)strlen(password), password, true);
	if (status != errSecSuccess) { CFRelease(kc); return status; }
	*out = kc;
	return errSecSuccess;
}

// SecKeychainChangePassword is exported by Security.framework (it backs
// `security set-keychain-password`) but is neither in the public headers nor
// documented as stable. It is therefore resolved at run time with dlsym rather
// than linked: if a future macOS removes it, the core still starts, every re-key
// reports errSecUnimplemented, and anchor advances are refused (fail closed)
// instead of the executable failing to launch.
typedef OSStatus (*faa_change_password_fn)(SecKeychainRef, UInt32, const void *, UInt32, const void *);

static faa_change_password_fn faa_change_password_symbol(const char *name) {
	return (faa_change_password_fn)dlsym(RTLD_DEFAULT, name);
}

static int faa_change_password_available(const char *name) { return faa_change_password_symbol(name) != NULL; }

// faa_kc_change_password re-keys an unlocked dedicated keychain file, so that
// an earlier copy of the file no longer opens with the current password. When
// no_interaction is set, user interaction is disabled around the call so a
// re-key that would need a prompt fails at once instead of waiting (used by
// qualification to show that the re-key needs no prompt). The symbol name is a
// parameter so qualification can show what a missing entry point does.
static OSStatus faa_kc_change_password(const char *symbol, SecKeychainRef kc, const char *old_password, const char *new_password, int no_interaction) {
	faa_change_password_fn fn = faa_change_password_symbol(symbol);
	if (fn == NULL) return errSecUnimplemented;
	Boolean before = true;
	if (no_interaction) {
		SecKeychainGetUserInteractionAllowed(&before);
		SecKeychainSetUserInteractionAllowed(false);
	}
	OSStatus status = fn(kc, (UInt32)strlen(old_password), old_password, (UInt32)strlen(new_password), new_password);
	if (no_interaction) SecKeychainSetUserInteractionAllowed(before);
	return status;
}

static void faa_kc_release(SecKeychainRef kc) {
	if (kc == NULL) return;
	SecKeychainLock(kc);
	CFRelease(kc);
}

// faa_kc_destroy deletes the keychain file (the reference is consumed).
static OSStatus faa_kc_destroy(SecKeychainRef kc) {
	if (kc == NULL) return errSecSuccess;
	OSStatus status = SecKeychainDelete(kc);
	CFRelease(kc);
	return status;
}

static OSStatus faa_kc_open_only(const char *path, SecKeychainRef *out) {
	return SecKeychainOpen(path, out);
}

// faa_foreign_attempt performs, without the keychain password and with user
// interaction disabled (so it fails at once instead of prompting), the
// operation another process of the same user would try against the dedicated
// keychain: 0 read, 1 overwrite, 2 add another item, 3 delete.
static OSStatus faa_foreign_attempt(const char *path, const char *service, const char *account, int op) {
	SecKeychainRef kc = NULL;
	OSStatus status = SecKeychainOpen(path, &kc);
	if (status != errSecSuccess) return status;
	Boolean interaction_before = true;
	SecKeychainGetUserInteractionAllowed(&interaction_before);
	SecKeychainSetUserInteractionAllowed(false);
	CFStringRef service_ref = faa_string(service);
	CFStringRef account_ref = faa_string(account);
	CFDataRef data_ref = CFDataCreate(NULL, (const UInt8 *)"foreign", 7);
	CFMutableDictionaryRef query = faa_query(kc, service_ref, account_ref, op == 2);
	if (op == 0) {
		CFDictionarySetValue(query, kSecReturnData, kCFBooleanTrue);
		CFTypeRef result = NULL;
		status = SecItemCopyMatching(query, &result);
		if (result != NULL) CFRelease(result);
	} else if (op == 1) {
		const void *akeys[] = { kSecValueData };
		const void *avalues[] = { data_ref };
		CFDictionaryRef attrs = CFDictionaryCreate(NULL, akeys, avalues, 1, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
		status = SecItemUpdate(query, attrs);
		CFRelease(attrs);
	} else if (op == 2) {
		CFDictionarySetValue(query, kSecValueData, data_ref);
		status = SecItemAdd(query, NULL);
	} else {
		status = SecItemDelete(query);
	}
	SecKeychainSetUserInteractionAllowed(interaction_before);
	CFRelease(query); CFRelease(data_ref); CFRelease(account_ref); CFRelease(service_ref); CFRelease(kc);
	return status;
}
*/
import "C"

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
	"sync"
	"syscall"
	"unsafe"

	"github.com/convergent-systems-co/praxis/internal/faa"
)

const macOSFAAService = "com.convergent-systems-co.praxis.faa"

// Statuses that decide how a Keychain failure is classified.
const (
	statusNotFound        = -25300 // errSecItemNotFound
	statusAuthFailed      = -25293 // errSecAuthFailed: wrong password, or a change that would need a prompt
	statusNoSuchKeychain  = -25294
	statusInvalidKeychain = -25295
	statusDuplicate       = -25299 // errSecDuplicateItem
)

// KeychainAnchor is the first Forward Authority Anchor backend.
//
// The anchor state lives in a dedicated, password-protected Keychain file, one
// per installation. The password is 32 random bytes held in a separate item of
// the user's login Keychain, whose access list trusts only the code identity
// that created it, so another process of the same user cannot read it without
// a prompt. The dedicated file is kept LOCKED except for the few milliseconds
// of one operation, and it also locks itself after five seconds. A locked
// Keychain refuses another process's attempt to update or add an item in it:
// the platform demands the password, or the user, for that. This is what an
// item access list alone cannot do. Measured on macOS, an item whose own
// access list restricted every entry to the creating binary was still silently
// overwritten and deleted by another process of the same user.
//
// What this does NOT prevent, and the platform does not let any Keychain
// backend prevent: silent DELETION of the item (or of the file, or of the
// password item), which the anchor reports as missing or corrupt and which
// therefore fails closed; and an overwrite during the brief unlocked window of
// a genuine operation, which produces a value that cannot be a valid anchor
// state (its head is not computable) and therefore also fails closed. The item
// access list still forbids another binary from READING the anchor.
//
// The Keychain has no native compare-and-set; Set reads, compares, writes and
// reads back inside one unlocked session, an in-process mutex and an advisory
// file lock serialise sessions, and callers serialise writers with the
// governance store's write lock. Availability failures are reported, never
// masked.
type KeychainAnchor struct {
	Service string
	// Dir holds the dedicated Keychain files; empty means ~/Library/Keychains.
	Dir string
}

func NewKeychainAnchor() *KeychainAnchor { return &KeychainAnchor{Service: macOSFAAService} }

func (k *KeychainAnchor) account(installation string) string { return "faa:" + installation }

// layout names the dedicated keychain file, its advisory lock file, and the
// login Keychain item that holds the file's password.
func (k *KeychainAnchor) layout(installation string) (file, lock, pwService, pwAccount string, err error) {
	dir := k.Dir
	if dir == "" {
		home, herr := os.UserHomeDir()
		if herr != nil {
			return "", "", "", "", fmt.Errorf("%w: %v", faa.ErrUnavailable, herr)
		}
		dir = filepath.Join(home, "Library", "Keychains")
	}
	sum := sha256.Sum256([]byte(k.Service + "\x00" + installation))
	name := "praxis-faa-" + hex.EncodeToString(sum[:8]) + ".keychain-db"
	return filepath.Join(dir, name), filepath.Join(dir, name+".lock"), k.Service + ".keychain-password", "faa-keychain:" + installation, nil
}

// DisableUserInteraction makes every later Keychain call in this process fail at
// once (errSecInteractionNotAllowed, -25308) instead of showing a prompt and
// waiting for a person. Qualification uses it so that a test that would need a
// human, such as the access prompt for an item another binary created, reports
// the Keychain as unavailable and is skipped rather than blocking until someone
// cancels it. Production never calls it: there the prompt is the intended
// behaviour and a non-interactive session simply fails closed as unavailable.
//
// It returns a function that restores the interaction setting it OBSERVED, not an
// assumed default, so a qualification run leaves the process as it found it.
func DisableUserInteraction() (restore func()) {
	var before C.Boolean = 1
	C.SecKeychainGetUserInteractionAllowed(&before)
	C.SecKeychainSetUserInteractionAllowed(0)
	return func() { C.SecKeychainSetUserInteractionAllowed(before) }
}

// keychainRef names the framework reference type for tests, which cannot use cgo.
type keychainRef = C.SecKeychainRef

var keychainSessions sync.Mutex // one unlocked session at a time in this process

type sessionMode int

const (
	sessionExisting sessionMode = iota // the keychain must exist
	sessionCreate                      // create the keychain (and its password) when absent
	sessionRecreate                    // tear down whatever exists and create afresh (governed re-anchor)
)

// pendingAccount names the login Keychain item that holds the password a
// re-key is moving to. It exists only between the start of a re-key and its
// completion, so that a crash in between never strands the file.
func pendingAccount(pwAccount string) string { return pwAccount + ":pending" }

func statusErr(what string, status C.OSStatus) error {
	return fmt.Errorf("%w: keychain %s status %d", faa.ErrUnavailable, what, int(status))
}

func corruptErr(what string) error { return fmt.Errorf("%w: %s", faa.ErrCorrupt, what) }

var errPasswordMissing = corruptErr("the dedicated keychain exists but the item holding its password is missing")

func readPassword(pwService, pwAccount string) (string, error) {
	service, account := C.CString(pwService), C.CString(pwAccount)
	defer C.free(unsafe.Pointer(service))
	defer C.free(unsafe.Pointer(account))
	var bytes *C.uint8_t
	var length C.size_t
	status := C.faa_keychain_copy(0, service, account, &bytes, &length)
	if status == statusNotFound {
		return "", errPasswordMissing
	}
	if status != C.errSecSuccess {
		return "", statusErr("password read", status)
	}
	raw := C.GoBytes(unsafe.Pointer(bytes), C.int(length))
	C.free(unsafe.Pointer(bytes))
	if len(raw) != 64 {
		return "", corruptErr("the dedicated keychain's password item is malformed")
	}
	return string(raw), nil
}

func storePassword(pwService, pwAccount, password string) error {
	service, account := C.CString(pwService), C.CString(pwAccount)
	defer C.free(unsafe.Pointer(service))
	defer C.free(unsafe.Pointer(account))
	// Replacing a stale password item is part of creating a fresh keychain.
	if status := C.faa_keychain_delete(0, service, account); status != C.errSecSuccess && status != statusNotFound {
		return statusErr("password item removal", status)
	}
	body := []byte(password)
	if status := C.faa_keychain_add(0, service, account, (*C.uint8_t)(unsafe.Pointer(&body[0])), C.size_t(len(body))); status != C.errSecSuccess {
		return statusErr("password item write", status)
	}
	return nil
}

func removePassword(pwService, pwAccount string) {
	service, account := C.CString(pwService), C.CString(pwAccount)
	defer C.free(unsafe.Pointer(service))
	defer C.free(unsafe.Pointer(account))
	_ = C.faa_keychain_delete(0, service, account)
	pending := C.CString(pendingAccount(pwAccount))
	defer C.free(unsafe.Pointer(pending))
	_ = C.faa_keychain_delete(0, service, pending)
}

// readOptionalPassword reads a password item, reporting whether it exists.
func readOptionalPassword(pwService, pwAccount string) (string, bool, error) {
	password, err := readPassword(pwService, pwAccount)
	if err != nil {
		if errors.Is(err, errPasswordMissing) {
			return "", false, nil
		}
		return "", false, err
	}
	return password, true, nil
}

func destroyKeychainFile(file string) {
	path := C.CString(file)
	defer C.free(unsafe.Pointer(path))
	var kc C.SecKeychainRef
	if C.faa_kc_open_only(path, &kc) == C.errSecSuccess {
		_ = C.faa_kc_destroy(kc)
	}
	_ = os.Remove(file) // a file the framework could not delete, or a non-keychain file put in its place
}

// session runs fn against the installation's dedicated keychain, unlocked, and
// locks it again before returning. Nothing is created unless the mode asks.
func (k *KeychainAnchor) session(ctx context.Context, installation string, mode sessionMode, fn func(kc C.SecKeychainRef, rotate func() error) error) error {
	if k == nil || k.Service == "" || installation == "" {
		return faa.ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	file, lockFile, pwService, pwAccount, err := k.layout(installation)
	if err != nil {
		return err
	}
	keychainSessions.Lock()
	defer keychainSessions.Unlock()
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return fmt.Errorf("%w: %v", faa.ErrUnavailable, err)
	}
	lock, err := os.OpenFile(lockFile, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("%w: %v", faa.ErrUnavailable, err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("%w: %v", faa.ErrUnavailable, err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)

	cfile := C.CString(file)
	defer C.free(unsafe.Pointer(cfile))
	_, statErr := os.Stat(file)
	exists := statErr == nil
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("%w: %v", faa.ErrUnavailable, statErr)
	}
	if mode == sessionRecreate {
		if exists {
			destroyKeychainFile(file)
		}
		removePassword(pwService, pwAccount)
		exists = false
	}
	if !exists && mode == sessionExisting {
		return faa.ErrMissing
	}

	var kc C.SecKeychainRef
	var password string
	created := false
	if exists {
		kc, password, err = openWithRecovery(cfile, pwService, pwAccount)
		if err != nil {
			return err
		}
	} else {
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			return fmt.Errorf("%w: %v", faa.ErrUnavailable, err)
		}
		password = hex.EncodeToString(raw)
		if err := storePassword(pwService, pwAccount, password); err != nil {
			return err
		}
		cpw := C.CString(password)
		status := C.faa_kc_create(cfile, cpw, &kc)
		C.free(unsafe.Pointer(cpw))
		if status != C.errSecSuccess {
			removePassword(pwService, pwAccount)
			destroyKeychainFile(file)
			return statusErr("create", status)
		}
		created = true
	}
	err = fn(kc, func() error { return rotatePassword(kc, password, pwService, pwAccount) })
	C.faa_kc_release(kc)
	if err != nil && created {
		// A keychain this call made and could not populate is not left behind.
		destroyKeychainFile(file)
		removePassword(pwService, pwAccount)
	}
	return err
}

func unlockFailed(status C.OSStatus) bool {
	return status == statusAuthFailed || status == statusNoSuchKeychain || status == statusInvalidKeychain
}

var noKeychain C.SecKeychainRef

// openWithRecovery unlocks the existing dedicated keychain. The file is re-keyed
// after every write (rotatePassword), so an earlier copy of the file cannot be
// opened with the current password. A crash in the middle of a re-key leaves the
// new password in the pending item, which is promoted here.
func openWithRecovery(cfile *C.char, pwService, pwAccount string) (C.SecKeychainRef, string, error) {
	pendingName := pendingAccount(pwAccount)
	current, haveCurrent, err := readOptionalPassword(pwService, pwAccount)
	if err != nil {
		return noKeychain, "", err
	}
	pending, havePending, err := readOptionalPassword(pwService, pendingName)
	junkPending := false
	if err != nil {
		if !errors.Is(err, faa.ErrCorrupt) {
			return noKeychain, "", err
		}
		junkPending = true // present but malformed: never a password, removed once the current one works
	}
	if !haveCurrent && !havePending {
		return noKeychain, "", errPasswordMissing
	}
	if haveCurrent {
		kc, status := func() (C.SecKeychainRef, C.OSStatus) {
			var kc C.SecKeychainRef
			cpw := C.CString(current)
			defer C.free(unsafe.Pointer(cpw))
			return kc, C.faa_kc_open(cfile, cpw, &kc)
		}()
		switch {
		case status == C.errSecSuccess:
			if havePending || junkPending {
				removePendingPassword(pwService, pwAccount) // a re-key that never reached the file, or junk
			}
			return kc, current, nil
		case !unlockFailed(status) || !havePending:
			if unlockFailed(status) {
				return noKeychain, "", corruptErr(fmt.Sprintf("the dedicated keychain cannot be unlocked with its password (status %d)", int(status)))
			}
			return noKeychain, "", statusErr("unlock", status)
		}
	}
	// The re-key reached the file but not the current password item.
	var kc C.SecKeychainRef
	cpw := C.CString(pending)
	status := C.faa_kc_open(cfile, cpw, &kc)
	C.free(unsafe.Pointer(cpw))
	switch {
	case status == C.errSecSuccess:
		if err := setCurrentPassword(pwService, pwAccount, pending); err != nil {
			C.faa_kc_release(kc)
			return noKeychain, "", err
		}
		removePendingPassword(pwService, pwAccount)
		return kc, pending, nil
	case unlockFailed(status):
		return noKeychain, "", corruptErr(fmt.Sprintf("the dedicated keychain cannot be unlocked with its password (status %d)", int(status)))
	default:
		return noKeychain, "", statusErr("unlock", status)
	}
}

func removePendingPassword(pwService, pwAccount string) {
	service, account := C.CString(pwService), C.CString(pendingAccount(pwAccount))
	defer C.free(unsafe.Pointer(service))
	defer C.free(unsafe.Pointer(account))
	_ = C.faa_keychain_delete(0, service, account)
}

// setCurrentPassword replaces the login Keychain item holding the file's password.
func setCurrentPassword(pwService, pwAccount, password string) error {
	service, account := C.CString(pwService), C.CString(pwAccount)
	defer C.free(unsafe.Pointer(service))
	defer C.free(unsafe.Pointer(account))
	body := []byte(password)
	status := C.faa_keychain_update(0, service, account, (*C.uint8_t)(unsafe.Pointer(&body[0])), C.size_t(len(body)))
	if status == statusNotFound {
		status = C.faa_keychain_add(0, service, account, (*C.uint8_t)(unsafe.Pointer(&body[0])), C.size_t(len(body)))
	}
	if status != C.errSecSuccess {
		return statusErr("password item update", status)
	}
	return nil
}

// rotatePassword re-keys the unlocked dedicated keychain to a fresh random
// password, so that any earlier copy of the file (which a same-user process can
// take as opaque bytes) stops opening with the current password item. The new
// password is recorded as pending before the file changes, and becomes the
// current password only after the file has it; openWithRecovery finishes an
// interrupted re-key from either side.
func rotatePassword(kc C.SecKeychainRef, old, pwService, pwAccount string) error {
	if err := rotationStep("start"); err != nil {
		return err
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return fmt.Errorf("%w: %v", faa.ErrUnavailable, err)
	}
	next := hex.EncodeToString(raw)
	if err := storePassword(pwService, pendingAccount(pwAccount), next); err != nil {
		return err
	}
	if err := rotationStep("pending-stored"); err != nil {
		return err
	}
	status := changePassword(kc, old, next)
	if status != 0 {
		removePendingPassword(pwService, pwAccount)
		return statusErr("re-key", C.OSStatus(status))
	}
	if err := rotationStep("file-rekeyed"); err != nil {
		return err
	}
	if err := setCurrentPassword(pwService, pwAccount, next); err != nil {
		return err // the pending item survives; the next open completes the re-key
	}
	if err := rotationStep("current-updated"); err != nil {
		return err
	}
	removePendingPassword(pwService, pwAccount)
	return nil
}

// changePassword re-keys the file; qualification replaces it to model a platform
// refusal (a missing symbol, a prompt that cannot be shown). It returns the
// OSStatus, 0 on success.
var changePassword = func(kc C.SecKeychainRef, old, next string) int {
	cold, cnext := C.CString(old), C.CString(next)
	defer C.free(unsafe.Pointer(cold))
	defer C.free(unsafe.Pointer(cnext))
	symbol := C.CString(changePasswordSymbol)
	defer C.free(unsafe.Pointer(symbol))
	interaction := C.int(0)
	if rekeyWithoutInteraction {
		interaction = 1
	}
	return int(C.faa_kc_change_password(symbol, kc, cold, cnext, interaction))
}

// changePasswordSymbol is the Security.framework entry point that re-keys a
// keychain file. It is exported but undocumented and not in the public headers;
// qualification points it at a name that does not exist to show what a macOS
// without it does (every re-key is refused, so no anchor advance completes).
var changePasswordSymbol = "SecKeychainChangePassword"

// rekeyWithoutInteraction is set only by qualification.
var rekeyWithoutInteraction bool

// rekeySupported reports whether this OS exports the re-key entry point.
func rekeySupported() bool {
	symbol := C.CString(changePasswordSymbol)
	defer C.free(unsafe.Pointer(symbol))
	return C.faa_change_password_available(symbol) != 0
}

// rotationStep lets qualification stop a re-key between its steps, which is how
// a crash is modelled. It is nil in production.
var rotationHook func(step string) error

func rotationStep(step string) error {
	if rotationHook == nil {
		return nil
	}
	return rotationHook(step)
}

func (k *KeychainAnchor) loadIn(kc C.SecKeychainRef, installation string) (faa.State, error) {
	service, account := C.CString(k.Service), C.CString(k.account(installation))
	defer C.free(unsafe.Pointer(service))
	defer C.free(unsafe.Pointer(account))
	var bytes *C.uint8_t
	var length C.size_t
	status := C.faa_keychain_copy(kc, service, account, &bytes, &length)
	if status == statusNotFound {
		return faa.State{}, faa.ErrMissing
	}
	if status != C.errSecSuccess {
		return faa.State{}, statusErr("read", status)
	}
	raw := C.GoBytes(unsafe.Pointer(bytes), C.int(length))
	C.free(unsafe.Pointer(bytes))
	var s faa.State
	if err := json.Unmarshal(raw, &s); err != nil || s.Validate() != nil || s.Installation != installation {
		return faa.State{}, faa.ErrCorrupt
	}
	return s, nil
}

func (k *KeychainAnchor) Load(ctx context.Context, installation string) (faa.State, error) {
	var out faa.State
	err := k.session(ctx, installation, sessionExisting, func(kc C.SecKeychainRef, _ func() error) error {
		var err error
		out, err = k.loadIn(kc, installation)
		return err
	})
	if err != nil {
		return faa.State{}, err
	}
	return out, nil
}

// writeIn writes the state into the keychain: an add when create, else an update.
func (k *KeychainAnchor) writeIn(kc C.SecKeychainRef, installation string, next faa.State, create bool) error {
	body, err := json.Marshal(next)
	if err != nil {
		return err
	}
	service, account := C.CString(k.Service), C.CString(k.account(installation))
	defer C.free(unsafe.Pointer(service))
	defer C.free(unsafe.Pointer(account))
	var status C.OSStatus
	if create {
		status = C.faa_keychain_add(kc, service, account, (*C.uint8_t)(unsafe.Pointer(&body[0])), C.size_t(len(body)))
		if status == statusDuplicate {
			return faa.ErrConflict
		}
	} else {
		status = C.faa_keychain_update(kc, service, account, (*C.uint8_t)(unsafe.Pointer(&body[0])), C.size_t(len(body)))
	}
	if status != C.errSecSuccess {
		return statusErr("write", status)
	}
	return nil
}

// write places a state directly; tests use it.
func (k *KeychainAnchor) write(installation string, next faa.State, create bool) error {
	mode := sessionExisting
	if create {
		mode = sessionCreate
	}
	return k.session(context.Background(), installation, mode, func(kc C.SecKeychainRef, _ func() error) error {
		return k.writeIn(kc, installation, next, create)
	})
}

func (k *KeychainAnchor) Set(ctx context.Context, installation string, expect *faa.State, next faa.State) error {
	if next.Installation != installation || next.Validate() != nil {
		return faa.ErrCorrupt
	}
	mode := sessionExisting
	if expect == nil {
		mode = sessionCreate
	}
	return k.session(ctx, installation, mode, func(kc C.SecKeychainRef, rotate func() error) error {
		cur, err := k.loadIn(kc, installation)
		switch {
		case expect == nil && err == nil:
			return faa.ErrConflict
		case expect == nil && !errors.Is(err, faa.ErrMissing):
			return err
		case expect != nil && (err != nil || cur != *expect || next.Seq <= cur.Seq):
			if err != nil {
				return err
			}
			return faa.ErrConflict
		}
		if err := k.writeIn(kc, installation, next, expect == nil); err != nil {
			return err
		}
		got, err := k.loadIn(kc, installation)
		if err != nil || got != next {
			return fmt.Errorf("%w: keychain read-back differs from what was written", faa.ErrUnavailable)
		}
		if expect == nil {
			return nil // a freshly created file has a fresh password and no earlier copy
		}
		// Re-key so that a copy of the file taken before this step cannot be
		// replayed. When the re-key fails the step is undone, so that a refused Set
		// leaves the anchor where it was.
		if err := rotate(); err != nil {
			if uerr := rotationStep("undo"); uerr != nil || k.writeIn(kc, installation, *expect, false) != nil {
				// The advance could not be taken back: the anchor is one step ahead of
				// the store, which is the stranded fail-closed state, never a wrong one.
				return fmt.Errorf("%w: the anchor advanced to sequence %d but its re-key failed and the step could not be undone: %v", faa.ErrUnavailable, next.Seq, err)
			}
			return fmt.Errorf("%w: the anchor was not advanced because its re-key failed: %v", faa.ErrUnavailable, err)
		}
		return nil
	})
}

func (k *KeychainAnchor) Revert(ctx context.Context, installation string, from, to faa.State) error {
	return k.session(ctx, installation, sessionExisting, func(kc C.SecKeychainRef, rotate func() error) error {
		cur, err := k.loadIn(kc, installation)
		if err != nil || cur != from {
			return faa.ErrConflict
		}
		if err := k.writeIn(kc, installation, to, false); err != nil {
			return err
		}
		if err := rotate(); err != nil {
			return fmt.Errorf("%w: the anchor was restored but its re-key failed: %v", faa.ErrUnavailable, err)
		}
		return nil
	})
}

// Reset replaces whatever anchor exists, including a damaged one, with a fresh
// keychain holding next. It is reached only through the governed re-anchor
// ceremony and the crashed-initialisation replacement.
func (k *KeychainAnchor) Reset(ctx context.Context, installation string, next faa.State) error {
	if next.Installation != installation || next.Validate() != nil {
		return faa.ErrCorrupt
	}
	return k.session(ctx, installation, sessionRecreate, func(kc C.SecKeychainRef, _ func() error) error {
		if err := k.writeIn(kc, installation, next, true); err != nil {
			return err
		}
		got, err := k.loadIn(kc, installation)
		if err != nil || got != next {
			return fmt.Errorf("%w: keychain read-back differs from what was written", faa.ErrUnavailable)
		}
		return nil
	})
}

// deleteForTest removes the installation's keychain file and password item;
// production code has no delete path.
func (k *KeychainAnchor) deleteForTest(installation string) error {
	file, lockFile, pwService, pwAccount, err := k.layout(installation)
	if err != nil {
		return err
	}
	keychainSessions.Lock()
	defer keychainSessions.Unlock()
	destroyKeychainFile(file)
	removePassword(pwService, pwAccount)
	_ = os.Remove(lockFile)
	return nil
}

// foreignAttempt reports the OSStatus of an operation attempted without the
// keychain password. It exists for qualification.
func (k *KeychainAnchor) foreignAttempt(installation string, op int, account string) (int, error) {
	file, _, _, _, err := k.layout(installation)
	if err != nil {
		return 0, err
	}
	cfile, service, acct := C.CString(file), C.CString(k.Service), C.CString(account)
	defer C.free(unsafe.Pointer(cfile))
	defer C.free(unsafe.Pointer(service))
	defer C.free(unsafe.Pointer(acct))
	return int(C.faa_foreign_attempt(cfile, service, acct, C.int(op))), nil
}

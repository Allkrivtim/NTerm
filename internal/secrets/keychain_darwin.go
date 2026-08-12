//go:build darwin

package secrets

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <Security/SecItem.h>
#include <CoreFoundation/CoreFoundation.h>
#include <stdlib.h>
#include <string.h>

static void nterm_secure_free(void *value, CFIndex length) {
    if (value == NULL) return;
    volatile unsigned char *bytes = (volatile unsigned char *)value;
    while (length-- > 0) *bytes++ = 0;
    free(value);
}

static CFStringRef nterm_string(const void *bytes, CFIndex length) {
    return CFStringCreateWithBytes(NULL, bytes, length, kCFStringEncodingUTF8, false);
}

static CFMutableDictionaryRef nterm_query(
    const void *service, CFIndex serviceLength,
    const void *account, CFIndex accountLength) {
    CFStringRef serviceString = nterm_string(service, serviceLength);
    CFStringRef accountString = nterm_string(account, accountLength);
    if (serviceString == NULL || accountString == NULL) {
        if (serviceString != NULL) CFRelease(serviceString);
        if (accountString != NULL) CFRelease(accountString);
        return NULL;
    }
    CFMutableDictionaryRef query = CFDictionaryCreateMutable(
        NULL, 0, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
    if (query != NULL) {
        CFDictionarySetValue(query, kSecClass, kSecClassGenericPassword);
        CFDictionarySetValue(query, kSecAttrService, serviceString);
        CFDictionarySetValue(query, kSecAttrAccount, accountString);
    }
    CFRelease(serviceString);
    CFRelease(accountString);
    return query;
}

static OSStatus nterm_keychain_set(
    const void *service, CFIndex serviceLength,
    const void *account, CFIndex accountLength,
    const void *password, CFIndex passwordLength) {
    CFMutableDictionaryRef query = nterm_query(service, serviceLength, account, accountLength);
    CFDataRef data = CFDataCreate(NULL, password, passwordLength);
    if (query == NULL || data == NULL) {
        if (query != NULL) CFRelease(query);
        if (data != NULL) CFRelease(data);
        return errSecAllocate;
    }
    const void *keys[] = { kSecValueData };
    const void *values[] = { data };
    CFDictionaryRef update = CFDictionaryCreate(
        NULL, keys, values, 1, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
    if (update == NULL) {
        CFRelease(data);
        CFRelease(query);
        return errSecAllocate;
    }
    OSStatus status = SecItemUpdate(query, update);
    if (status == errSecItemNotFound) {
        CFDictionarySetValue(query, kSecValueData, data);
        status = SecItemAdd(query, NULL);
    }
    CFRelease(update);
    CFRelease(data);
    CFRelease(query);
    return status;
}

static OSStatus nterm_keychain_get(
    const void *service, CFIndex serviceLength,
    const void *account, CFIndex accountLength,
    CFIndex *passwordLength, void **password) {
    *password = NULL;
    *passwordLength = 0;
    CFMutableDictionaryRef query = nterm_query(service, serviceLength, account, accountLength);
    if (query == NULL) return errSecAllocate;
    CFDictionarySetValue(query, kSecReturnData, kCFBooleanTrue);
    CFDictionarySetValue(query, kSecMatchLimit, kSecMatchLimitOne);
    CFTypeRef result = NULL;
    OSStatus status = SecItemCopyMatching(query, &result);
    CFRelease(query);
    if (status != errSecSuccess) return status;
    if (result == NULL || CFGetTypeID(result) != CFDataGetTypeID()) {
        if (result != NULL) CFRelease(result);
        return errSecInternalComponent;
    }
    CFDataRef data = (CFDataRef)result;
    CFIndex length = CFDataGetLength(data);
    void *copy = malloc((size_t)(length > 0 ? length : 1));
    if (copy == NULL) {
        CFRelease(data);
        return errSecAllocate;
    }
    if (length > 0) memcpy(copy, CFDataGetBytePtr(data), (size_t)length);
    *password = copy;
    *passwordLength = length;
    CFRelease(data);
    return errSecSuccess;
}

static OSStatus nterm_keychain_has(
    const void *service, CFIndex serviceLength,
    const void *account, CFIndex accountLength) {
    CFMutableDictionaryRef query = nterm_query(service, serviceLength, account, accountLength);
    if (query == NULL) return errSecAllocate;
    CFDictionarySetValue(query, kSecMatchLimit, kSecMatchLimitOne);
    OSStatus status = SecItemCopyMatching(query, NULL);
    CFRelease(query);
    return status;
}

static OSStatus nterm_keychain_delete(
    const void *service, CFIndex serviceLength,
    const void *account, CFIndex accountLength) {
    CFMutableDictionaryRef query = nterm_query(service, serviceLength, account, accountLength);
    if (query == NULL) return errSecAllocate;
    OSStatus status = SecItemDelete(query);
    CFRelease(query);
    if (status == errSecItemNotFound) return errSecSuccess;
    return status;
}
*/
import "C"

import (
	"errors"
	"fmt"
	"strings"
	"unsafe"
)

const service = "io.nterm.ssh"

const maxSecretBytes = 64 << 10

func Set(account, password string) error {
	account = strings.TrimSpace(account)
	if account == "" {
		return errors.New("credential account is empty")
	}
	if password == "" {
		return Delete(account)
	}
	if len(password) > maxSecretBytes {
		return errors.New("credential is too large")
	}
	servicePointer, serviceLength := keychainBytes(service)
	defer C.free(servicePointer)
	accountPointer, accountLength := keychainBytes(account)
	defer C.free(accountPointer)
	passwordPointer, passwordLength := keychainBytes(password)
	defer C.nterm_secure_free(passwordPointer, passwordLength)
	status := C.nterm_keychain_set(servicePointer, serviceLength, accountPointer, accountLength, passwordPointer, passwordLength)
	return keychainError("save password", status)
}

func Delete(account string) error {
	account = strings.TrimSpace(account)
	if account == "" {
		return errors.New("credential account is empty")
	}
	servicePointer, serviceLength := keychainBytes(service)
	defer C.free(servicePointer)
	accountPointer, accountLength := keychainBytes(account)
	defer C.free(accountPointer)
	return keychainError("delete password", C.nterm_keychain_delete(servicePointer, serviceLength, accountPointer, accountLength))
}

func Has(account string) bool {
	account = strings.TrimSpace(account)
	if account == "" {
		return false
	}
	servicePointer, serviceLength := keychainBytes(service)
	defer C.free(servicePointer)
	accountPointer, accountLength := keychainBytes(account)
	defer C.free(accountPointer)
	return C.nterm_keychain_has(servicePointer, serviceLength, accountPointer, accountLength) == C.errSecSuccess
}

func Get(account string) (string, error) {
	account = strings.TrimSpace(account)
	if account == "" {
		return "", errors.New("credential account is empty")
	}
	servicePointer, serviceLength := keychainBytes(service)
	defer C.free(servicePointer)
	accountPointer, accountLength := keychainBytes(account)
	defer C.free(accountPointer)
	var passwordLength C.CFIndex
	var password unsafe.Pointer
	status := C.nterm_keychain_get(servicePointer, serviceLength, accountPointer, accountLength, &passwordLength, &password)
	if err := keychainError("read password", status); err != nil {
		return "", err
	}
	defer C.nterm_secure_free(password, passwordLength)
	if passwordLength < 0 || passwordLength > maxSecretBytes {
		return "", errors.New("credential stored in Keychain is too large")
	}
	return string(C.GoBytes(password, C.int(passwordLength))), nil
}

func keychainBytes(value string) (unsafe.Pointer, C.CFIndex) {
	data := []byte(value)
	return C.CBytes(data), C.CFIndex(len(data))
}

func keychainError(operation string, status C.OSStatus) error {
	if status == C.errSecSuccess {
		return nil
	}
	return fmt.Errorf("%s in Keychain failed (OSStatus %d)", operation, int32(status))
}

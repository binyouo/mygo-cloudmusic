// Built once per local signing identity and reused byte-for-byte. A changing
// cdhash would make the file-based Keychain ask again after every app build.
#include <CoreFoundation/CoreFoundation.h>
#include <Security/Security.h>
#include <stdio.h>
#include <string.h>
#include <unistd.h>

// The build script pins both values; callers cannot select another item.
#ifndef APP_REQUIREMENT
#error APP_REQUIREMENT is required
#endif
#ifndef KEYCHAIN_SERVICE
#error KEYCHAIN_SERVICE is required
#endif

static OSStatus authenticate_parent(void) {
    pid_t pid = getppid();
    if (pid <= 1) return errSecAuthFailed;
    CFNumberRef number = CFNumberCreate(NULL, kCFNumberIntType, &pid);
    const void *keys[] = { kSecGuestAttributePid };
    const void *values[] = { number };
    CFDictionaryRef attrs = CFDictionaryCreate(NULL, keys, values, 1,
        &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
    SecCodeRef parent = NULL;
    SecRequirementRef requirement = NULL;
    OSStatus status = SecCodeCopyGuestWithAttributes(NULL, attrs, kSecCSDefaultFlags, &parent);
    if (status == errSecSuccess)
        status = SecRequirementCreateWithString(CFSTR(APP_REQUIREMENT), kSecCSDefaultFlags, &requirement);
    if (status == errSecSuccess)
        status = SecCodeCheckValidity(parent, kSecCSStrictValidate, requirement);
    // Do not send a credential to a process whose parent exited during validation.
    if (getppid() != pid) status = errSecAuthFailed;
    if (requirement) CFRelease(requirement);
    if (parent) CFRelease(parent);
    CFRelease(attrs);
    CFRelease(number);
    return status;
}

int main(int argc, char **argv) {
    if (argc < 2 || argc > 3) return 2;
    if (argc == 3) {
        if (strcmp(argv[2], "--no-ui")) return 2;
        SecKeychainSetUserInteractionAllowed(false);
    }
    OSStatus status = authenticate_parent();
    if (status != errSecSuccess) {
        fprintf(stderr, "caller rejected: %d\n", (int)status);
        return 1;
    }
    CFMutableDictionaryRef query = CFDictionaryCreateMutable(NULL, 0,
        &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
    CFDictionarySetValue(query, kSecClass, kSecClassGenericPassword);
    CFDictionarySetValue(query, kSecAttrService, CFSTR(KEYCHAIN_SERVICE));
    CFDictionarySetValue(query, kSecAttrAccount, CFSTR("netease-session"));
    if (!strcmp(argv[1], "read")) {
        CFDictionarySetValue(query, kSecReturnData, kCFBooleanTrue);
        CFTypeRef result = NULL;
        status = SecItemCopyMatching(query, &result);
        if (status == errSecSuccess) {
            if (!result || CFGetTypeID(result) != CFDataGetTypeID()) status = errSecDecode;
            else {
                size_t size = (size_t)CFDataGetLength(result);
                if (fwrite(CFDataGetBytePtr(result), 1, size, stdout) != size) status = errSecIO;
            }
        }
        if (result) CFRelease(result);
    } else if (!strcmp(argv[1], "write")) {
        unsigned char value[2561];
        size_t size = fread(value, 1, sizeof(value), stdin);
        if (ferror(stdin) || size > 2560) status = errSecParam;
        else {
            CFDataRef data = CFDataCreate(NULL, value, (CFIndex)size);
            const void *keys[] = { kSecValueData };
            const void *values[] = { data };
            CFDictionaryRef update = CFDictionaryCreate(NULL, keys, values, 1,
                &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
            status = SecItemUpdate(query, update);
            if (status == errSecItemNotFound) {
                CFDictionarySetValue(query, kSecValueData, data);
                status = SecItemAdd(query, NULL);
            }
            CFRelease(update);
            CFRelease(data);
        }
        memset(value, 0, sizeof(value));
    } else if (!strcmp(argv[1], "delete")) {
        status = SecItemDelete(query);
        if (status == errSecItemNotFound) status = errSecSuccess;
    } else status = errSecParam;
    CFRelease(query);
    if (status != errSecSuccess) fprintf(stderr, "keychain status: %d\n", (int)status);
    return status == errSecSuccess ? 0 : 1;
}

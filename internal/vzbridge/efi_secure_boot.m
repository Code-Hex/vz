#import "native.h"
#import <Foundation/Foundation.h>
#import <Security/Security.h>
#import <Virtualization/Virtualization.h>

VZBridgeOwnedObject vz_newEFICertificate(VZBridgeObject data)
{
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 270000
    if (@available(macOS 27.0, *)) {
        SecCertificateRef certificate = SecCertificateCreateWithData(NULL, (CFDataRef)data);
        if (!certificate) return NULL;
        id signature = [[VZEFISignatureX509Certificate alloc] initWithCertificate:certificate];
        CFRelease(certificate);
        return signature;
    }
#endif
    return NULL;
}

VZBridgeOwnedObject vz_efiCertificateData(VZBridgeObject signature)
{
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 270000
    if (@available(macOS 27.0, *)) {
        return (void *)SecCertificateCopyData([(VZEFISignatureX509Certificate *)signature certificate]);
    }
#endif
    return NULL;
}

bool vz_isEFICertificate(VZBridgeObject signature)
{
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 270000
    if (@available(macOS 27.0, *)) {
        return [(id)signature isKindOfClass:[VZEFISignatureX509Certificate class]];
    }
#endif
    return false;
}

void vz_enableEFISecureBoot(VZBridgeObject store, VZBridgeObject signature, VZBridgeErrorOut errorOut)
{
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 270000
    if (@available(macOS 27.0, *)) {
        NSError *error = nil;
        [(VZEFIVariableStore *)store enableSecureBootWithPlatformKey:[(VZEFISignatureX509Certificate *)signature certificate] error:&error];
        *errorOut = [error retain];
    }
#endif
}

bool vz_efiSecureBootEnabled(VZBridgeObject store, VZBridgeErrorOut errorOut)
{
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 270000
    if (@available(macOS 27.0, *)) {
        BOOL value = NO;
        NSError *error = nil;
        [(VZEFIVariableStore *)store getSecureBootEnabled:&value error:&error];
        *errorOut = [error retain];
        return value;
    }
#endif
    return false;
}

#import "custom_virtio.h"
#import <Virtualization/Virtualization.h>
#import <objc/runtime.h>
#include <stdatomic.h>

typedef uintptr_t (*VirtioCallback)(uint32_t, uint64_t, void *, void *);
static char virtioDelegateKey;
static char virtioConfigurationLengthKey;
static char virtioDeviceIDKey;
static _Atomic(uint64_t) nextVirtioDeviceID;

static void invokeVirtioBlock(uintptr_t context)
{
    ((void (^)(void))context)();
}

#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 270000
API_AVAILABLE(macos(27.0))
@interface VZBridgeCustomVirtioDelegate : NSObject <VZCustomVirtioDeviceConfigurationDelegate, VZCustomVirtioDeviceDelegate> {
@public
    VirtioCallback callback;
    uint64_t context;
}
- (void)invalidate;
@end

@implementation VZBridgeCustomVirtioDelegate
- (void)invalidate
{
    if (callback) {
        VirtioCallback old = callback;
        callback = NULL;
        old(VirtioReleased, context, NULL, NULL);
    }
}
- (void)dealloc
{
    [self invalidate];
    [super dealloc];
}
- (void)customVirtioConfiguration:(VZCustomVirtioDeviceConfiguration *)configuration didCreateDevice:(VZCustomVirtioDevice *)device
{
    vz_setObjectQueue(device, device.deviceQueue);
    uint64_t deviceID = atomic_fetch_add_explicit(&nextVirtioDeviceID, 1, memory_order_relaxed) + 1;
    objc_setAssociatedObject(device, &virtioDeviceIDKey, @(deviceID), OBJC_ASSOCIATION_RETAIN);
    objc_setAssociatedObject(device, &virtioConfigurationLengthKey, @(configuration.deviceSpecificConfiguration.configurationData.length), OBJC_ASSOCIATION_RETAIN);
    objc_setAssociatedObject(device, &virtioDelegateKey, self, OBJC_ASSOCIATION_RETAIN);
    device.delegate = self;
    void (^created)(void) = ^{
        if (callback)
            callback(VirtioCreated, context, [device retain], NULL);
    };
    vz_dispatchSyncOnObject(device, (uintptr_t)invokeVirtioBlock, (uintptr_t)created);
}
- (void)customVirtioDevice:(VZCustomVirtioDevice *)device didReceiveNotificationForQueue:(VZVirtioQueue *)queue
{
    vz_setObjectQueue(queue, device.deviceQueue);
    if (callback)
        callback(VirtioNotified, context, [device retain], [queue retain]);
}
- (void)customVirtioDeviceDidAcceptDriverOk:(VZCustomVirtioDevice *)device
{
    if (callback)
        callback(VirtioDriverReady, context, [device retain], NULL);
}
- (void)customVirtioDeviceWillStop:(VZCustomVirtioDevice *)device
{
    if (callback)
        callback(VirtioWillStop, context, [device retain], NULL);
}
- (void)customVirtioDeviceWillPause:(VZCustomVirtioDevice *)device
{
    if (callback)
        callback(VirtioWillPause, context, [device retain], NULL);
}
- (void)customVirtioDeviceWillResume:(VZCustomVirtioDevice *)device
{
    if (callback)
        callback(VirtioWillResume, context, [device retain], NULL);
}
- (void)customVirtioDeviceWillReset:(VZCustomVirtioDevice *)device
{
    if (callback)
        callback(VirtioWillReset, context, [device retain], NULL);
}
- (NSData *)customVirtioDeviceSaveStateForRestore:(VZCustomVirtioDevice *)device
{
    return callback ? [(NSData *)callback(VirtioSave, context, [device retain], NULL) autorelease] : nil;
}
- (BOOL)customVirtioDeviceShouldRestore:(VZCustomVirtioDevice *)device saveState:(NSData *)state
{
    return callback && callback(VirtioRestore, context, [device retain], [state retain]);
}
@end
#endif

void *vz_newCustomVirtioProvider(uintptr_t callback, uint64_t context)
{
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 270000
    if (@available(macOS 27.0, *)) {
        dispatch_queue_t queue = dispatch_queue_create("com.codehex.vz.custom-virtio", DISPATCH_QUEUE_SERIAL);
        VZBridgeCustomVirtioDelegate *delegate = [VZBridgeCustomVirtioDelegate new];
        delegate->callback = (VirtioCallback)callback;
        delegate->context = context;
        VZCustomVirtioDeviceDelegateProvider *provider = [[VZCustomVirtioDeviceDelegateProvider alloc] initWithDeviceQueue:queue delegate:delegate];
        vz_setObjectQueue(provider, queue);
        objc_setAssociatedObject(provider, &virtioDelegateKey, delegate, OBJC_ASSOCIATION_RETAIN);
        [delegate release];
        dispatch_release(queue);
        return provider;
    }
#endif
    return NULL;
}

void vz_closeCustomVirtioProvider(void *provider)
{
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 270000
    if (@available(macOS 27.0, *))
        [(VZBridgeCustomVirtioDelegate *)objc_getAssociatedObject((id)provider, &virtioDelegateKey) invalidate];
#endif
}

void vz_customVirtioUpdate(void *device, void *configuration, uintptr_t callback, uint64_t context)
{
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 270000
    if (@available(macOS 27.0, *)) {
        id owner = (id)device;
        [(VZCustomVirtioDevice *)device updateDeviceSpecificConfiguration:(VZVirtioDeviceSpecificConfiguration *)configuration completionHandler:^(NSError *error) {
            ((VirtioCallback)callback)(VirtioCompleted, context, [error retain], NULL);
            [owner self];
        }];
    }
#endif
}

void vz_customVirtioMap(void *region, void *memory, uint64_t offset, uint64_t size, uintptr_t callback, uint64_t context)
{
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 270000
    if (@available(macOS 27.0, *)) {
        id owner = (id)region;
        [(VZVirtioSharedMemoryRegion *)region mapMemory:memory atOffset:offset size:size completionHandler:^(NSError *error) {
            ((VirtioCallback)callback)(VirtioCompleted, context, [error retain], NULL);
            [owner self];
        }];
    }
#endif
}

void vz_customVirtioUnmap(void *region, uint64_t offset, uint64_t size, uintptr_t callback, uint64_t context)
{
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 270000
    if (@available(macOS 27.0, *)) {
        id owner = (id)region;
        [(VZVirtioSharedMemoryRegion *)region unmapMemoryAtOffset:offset size:size completionHandler:^(NSError *error) {
            ((VirtioCallback)callback)(VirtioCompleted, context, [error retain], NULL);
            [owner self];
        }];
    }
#endif
}

uint64_t vz_customVirtioConfigurationLength(void *device)
{
    return [(NSNumber *)objc_getAssociatedObject((id)device, &virtioConfigurationLengthKey) unsignedLongLongValue];
}

uint64_t vz_customVirtioDeviceID(void *device)
{
    return [(NSNumber *)objc_getAssociatedObject((id)device, &virtioDeviceIDKey) unsignedLongLongValue];
}

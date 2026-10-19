#import "native.h"
#import <Foundation/Foundation.h>
#import <Virtualization/Virtualization.h>
#import <errno.h>
#import <objc/runtime.h>
#import <stdatomic.h>
#import <unistd.h>

static dispatch_queue_t bridgeQueue;
static char bridgeQueueKey;
typedef uint64_t (*BridgeCallback)(uint32_t, uint64_t, void *, void *, uint64_t);
static _Atomic(BridgeCallback) bridgeCallback;
static char objectQueueKey;

static dispatch_queue_t commonQueue(void)
{
    static dispatch_once_t once;
    dispatch_once(&once, ^{
        bridgeQueue = dispatch_queue_create("com.codehex.vz.native", DISPATCH_QUEUE_SERIAL);
        dispatch_queue_set_specific(bridgeQueue, &bridgeQueueKey, bridgeQueue, NULL);
    });
    return bridgeQueue;
}

@interface VZBridgeQueue : NSObject {
@public
    dispatch_queue_t queue;
}
@end

@implementation VZBridgeQueue
- (instancetype)init
{
    self = [super init];
    if (self) {
        queue = dispatch_queue_create("com.codehex.vz.machine", DISPATCH_QUEUE_SERIAL);
        dispatch_queue_set_specific(queue, &bridgeQueueKey, queue, NULL);
    }
    return self;
}
- (void)dealloc
{
    dispatch_release(queue);
    [super dealloc];
}
@end

void *vz_objectQueue(void *object)
{
    @autoreleasepool {
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 260000
        if (@available(macOS 26.0, *)) {
            if ([(id)object isKindOfClass:[VZVirtualMachine class]])
                return [(VZVirtualMachine *)object queue];
        }
#endif
        VZBridgeQueue *owner = objc_getAssociatedObject((id)object, &objectQueueKey);
        return owner ? owner->queue : commonQueue();
    }
}

static void syncOnQueue(dispatch_queue_t target, void (^body)(void))
{
    if (dispatch_get_specific(&bridgeQueueKey) == target) {
        @autoreleasepool {
            body();
        }
    } else {
        dispatch_sync(target, ^{ @autoreleasepool { body(); } });
    }
}

static void syncNative(void (^body)(void))
{
    syncOnQueue(commonQueue(), body);
}

static void syncObject(id object, void (^body)(void))
{
    @autoreleasepool {
        syncOnQueue(vz_objectQueue(object), body);
    }
}

void vz_inheritObjectQueue(void *object, void *source)
{
    if (!object || !source)
        return;
    @autoreleasepool {
        VZBridgeQueue *owner = objc_getAssociatedObject((id)source, &objectQueueKey);
        if (!owner)
            return;
        if ([(id)object isKindOfClass:[NSArray class]]) {
            for (id element in (NSArray *)object)
                objc_setAssociatedObject(element, &objectQueueKey, owner, OBJC_ASSOCIATION_RETAIN);
        } else {
            objc_setAssociatedObject((id)object, &objectQueueKey, owner, OBJC_ASSOCIATION_RETAIN);
        }
    }
}

void vz_dispatchSyncOnObject(void *object, uintptr_t callback, uintptr_t context)
{
    syncObject((id)object, ^{ ((void (*)(uintptr_t))callback)(context); });
}

void *vz_dispatchQueuePointer(void) { return commonQueue(); }

static dispatch_group_t pendingReleases(void)
{
    static dispatch_group_t group;
    static dispatch_once_t once;
    dispatch_once(&once, ^{ group = dispatch_group_create(); });
    return group;
}

void vz_drain(void)
{
    syncNative(^ {});
    dispatch_group_wait(pendingReleases(), DISPATCH_TIME_FOREVER);
}

void vz_setCallback(uint64_t callback) { atomic_store(&bridgeCallback, (BridgeCallback)(uintptr_t)callback); }

void vz_releaseObject(void *object)
{
    if (!object)
        return;
    @autoreleasepool {
        dispatch_queue_t target = vz_objectQueue(object);
        dispatch_retain(target);
        // A void pointer keeps the block from taking an extra Objective-C reference.
        dispatch_group_async(pendingReleases(), target, ^{
            @autoreleasepool {
                [(id)object release];
            }
            dispatch_release(target);
        });
    }
}

void vz_setInvalidDiskModeError(void **errorOut)
{
    *errorOut = [[NSError errorWithDomain:NSPOSIXErrorDomain
                                     code:EINVAL
                                 userInfo:@ { NSLocalizedDescriptionKey : @"Invalid disk caching or synchronization mode" }] retain];
}

static uint64_t emit(uint32_t kind, uint64_t context, id first, id second, uint64_t value)
{
    BridgeCallback callback = atomic_load(&bridgeCallback);
    if (callback)
        return callback(kind, context, first, second, value);
    switch (kind) {
    case VZBridgeCallbackSocketConnected:
    case VZBridgeCallbackRestoreImageLoaded:
        [first release];
        [second release];
        break;
    case VZBridgeCallbackMachineStoppedWithError:
    case VZBridgeCallbackCompletion:
    case VZBridgeCallbackNetworkDisconnected:
    case VZBridgeCallbackSocketAccepted:
    case VZBridgeCallbackNetworkBlockDeviceError:
    case VZBridgeCallbackRosettaInstallCompleted:
    case VZBridgeCallbackMacOSInstallCompleted:
        [first release];
        break;
    }
    return 0;
}

static void fail(NSError *error, void **output)
{
    if (output)
        *output = [error retain];
}

static NSError *posixError(int code)
{
    return [NSError errorWithDomain:NSPOSIXErrorDomain code:code userInfo:nil];
}

static void unavailable(uint32_t kind, uint64_t context)
{
    NSError *error = [NSError errorWithDomain:NSPOSIXErrorDomain code:ENOTSUP userInfo:nil];
    if (kind == VZBridgeCallbackRestoreImageLoaded)
        emit(kind, context, nil, [error retain], 0);
    else
        emit(kind, context, [error retain], nil, 0);
}

static NSString *text(const char *value) { return value ? [NSString stringWithUTF8String:value] : @""; }
static NSURL *fileURL(const char *value) { return [NSURL fileURLWithPath:text(value)]; }

@interface VZBridgeMachine : VZVirtualMachine <VZVirtualMachineDelegate> {
    uint64_t _stateContext;
    uint64_t _networkContext;
    BOOL _observing;
    NSHashTable<id<VZVirtualMachineDelegate>> *_externalDelegates;
}
- (instancetype)initWithConfiguration:(VZVirtualMachineConfiguration *)configuration queueOwner:(VZBridgeQueue *)owner state:(uint64_t)state network:(uint64_t)network;
@end

@implementation VZBridgeMachine
- (instancetype)initWithConfiguration:(VZVirtualMachineConfiguration *)configuration queueOwner:(VZBridgeQueue *)owner state:(uint64_t)state network:(uint64_t)network
{
    self = [super initWithConfiguration:configuration queue:owner->queue];
    if (self) {
        objc_setAssociatedObject(self, &objectQueueKey, owner, OBJC_ASSOCIATION_RETAIN);
        _stateContext = state;
        _networkContext = network;
        _externalDelegates = [[NSHashTable weakObjectsHashTable] retain];
        [super setDelegate:self];
        [self addObserver:self forKeyPath:@"state" options:NSKeyValueObservingOptionNew context:&bridgeQueueKey];
        _observing = YES;
    }
    return self;
}
- (void)observeValueForKeyPath:(NSString *)key ofObject:(id)object change:(NSDictionary *)change context:(void *)context
{
    if (context == &bridgeQueueKey) {
        emit(VZBridgeCallbackMachineStateChanged, _stateContext, nil, nil, [change[NSKeyValueChangeNewKey] unsignedLongLongValue]);
    } else {
        [super observeValueForKeyPath:key ofObject:object change:change context:context];
    }
}
- (void)setDelegate:(id<VZVirtualMachineDelegate>)delegate
{
    if (delegate == self)
        return;
    syncObject(self, ^{
        if (delegate)
            [_externalDelegates addObject:delegate];
        else
            [_externalDelegates removeAllObjects];
    });
}
- (void)guestDidStopVirtualMachine:(VZVirtualMachine *)machine
{
    for (id<VZVirtualMachineDelegate> delegate in _externalDelegates.allObjects) {
        if ([delegate respondsToSelector:_cmd])
            [delegate guestDidStopVirtualMachine:machine];
    }
}
- (void)virtualMachine:(VZVirtualMachine *)machine didStopWithError:(NSError *)error
{
    emit(VZBridgeCallbackMachineStoppedWithError, _stateContext, [error retain], nil, 0);
    for (id<VZVirtualMachineDelegate> delegate in _externalDelegates.allObjects) {
        if ([delegate respondsToSelector:_cmd])
            [delegate virtualMachine:machine didStopWithError:error];
    }
}
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 120000
- (void)virtualMachine:(VZVirtualMachine *)machine networkDevice:(VZNetworkDevice *)device attachmentWasDisconnectedWithError:(NSError *)error API_AVAILABLE(macos(12.0))
{
    NSInteger index = [machine.networkDevices indexOfObjectIdenticalTo:device];
    emit(VZBridgeCallbackNetworkDisconnected, _networkContext, [error retain], nil, index == NSNotFound ? UINT64_MAX : (uint64_t)index);
    for (id<VZVirtualMachineDelegate> delegate in _externalDelegates.allObjects) {
        if ([delegate respondsToSelector:_cmd])
            [delegate virtualMachine:machine networkDevice:device attachmentWasDisconnectedWithError:error];
    }
}
#endif
- (void)dealloc
{
    if (_observing)
        [self removeObserver:self forKeyPath:@"state" context:&bridgeQueueKey];
    [super setDelegate:nil];
    [_externalDelegates release];
    if (_stateContext)
        emit(VZBridgeCallbackContextReleased, _stateContext, nil, nil, 0);
    if (_networkContext)
        emit(VZBridgeCallbackContextReleased, _networkContext, nil, nil, 0);
    [super dealloc];
}
@end

void *vz_newVZVirtualMachineWithDispatchQueue(void *config, uint64_t state, uint64_t network)
{
    VZBridgeQueue *owner = [[VZBridgeQueue alloc] init];
    __block VZBridgeMachine *machine;
    syncOnQueue(owner->queue, ^{
        machine = [[VZBridgeMachine alloc] initWithConfiguration:config queueOwner:owner state:state network:network];
    });
    [owner release];
    return machine;
}

static void (^completion(id owner, uint64_t context))(NSError *)
{
    return [[^(NSError *error) {
        emit(VZBridgeCallbackCompletion, context, [error retain], nil, 0);
        [owner self];
    } copy] autorelease];
}
void vz_startWithCompletionHandler(void *machine, uint64_t context)
{
    [(VZVirtualMachine *)machine startWithCompletionHandler:completion(machine, context)];
}
void vz_pauseWithCompletionHandler(void *machine, uint64_t context)
{
    [(VZVirtualMachine *)machine pauseWithCompletionHandler:completion(machine, context)];
}
void vz_resumeWithCompletionHandler(void *machine, uint64_t context)
{
    [(VZVirtualMachine *)machine resumeWithCompletionHandler:completion(machine, context)];
}
void vz_stopWithCompletionHandler(void *machine, uint64_t context)
{
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 120000
    if (@available(macOS 12, *)) {
        [(VZVirtualMachine *)machine stopWithCompletionHandler:completion(machine, context)];
        return;
    }
#endif
    unavailable(VZBridgeCallbackCompletion, context);
}
void vz_startWithOptionsCompletionHandler(void *machine, void *options, uint64_t context)
{
#if defined(__arm64__) && __MAC_OS_X_VERSION_MAX_ALLOWED >= 130000
    if (@available(macOS 13, *)) {
        [(VZVirtualMachine *)machine startWithOptions:options completionHandler:completion(machine, context)];
        return;
    }
#endif
    unavailable(VZBridgeCallbackCompletion, context);
}

@interface VZBridgeSocketListener : VZVirtioSocketListener <VZVirtioSocketListenerDelegate> {
    uint64_t _context;
}
- (instancetype)initWithContext:(uint64_t)context;
- (void)invalidate;
@end
@implementation VZBridgeSocketListener
- (instancetype)initWithContext:(uint64_t)context
{
    self = [super init];
    if (self) {
        _context = context;
        self.delegate = self;
    }
    return self;
}
- (BOOL)listener:(VZVirtioSocketListener *)listener shouldAcceptNewConnection:(VZVirtioSocketConnection *)connection fromSocketDevice:(VZVirtioSocketDevice *)device
{
    if (!_context)
        return NO;
    vz_inheritObjectQueue(connection, device);
    return emit(VZBridgeCallbackSocketAccepted, _context, [connection retain], device, 0) != 0;
}
- (void)invalidate
{
    self.delegate = nil;
    uint64_t context = _context;
    _context = 0;
    if (context)
        emit(VZBridgeCallbackContextReleased, context, nil, nil, 0);
}
- (void)dealloc
{
    [self invalidate];
    [super dealloc];
}
@end

void *vz_newVZVirtioSocketListener(void *device, uint64_t context)
{
    VZBridgeSocketListener *listener = [[VZBridgeSocketListener alloc] initWithContext:context];
    vz_inheritObjectQueue(listener, device);
    return listener;
}
void vz_invalidateVZVirtioSocketListener(void *listener) { [(VZBridgeSocketListener *)listener invalidate]; }
void vz_VZVirtioSocketDevice_connectToPort(void *pointer, uint32_t port, uint64_t context)
{
    VZVirtioSocketDevice *device = pointer;
    [device connectToPort:port
        completionHandler:^(VZVirtioSocketConnection *connection, NSError *error) {
            vz_inheritObjectQueue(connection, device);
            emit(VZBridgeCallbackSocketConnected, context, [connection retain], [error retain], 0);
            [device self];
        }];
}
int32_t vz_socketConnectionDuplicatedFileDescriptor(void *connection, void **output)
{
    int descriptor = dup([(VZVirtioSocketConnection *)connection fileDescriptor]);
    fail(descriptor < 0 ? posixError(errno) : nil, output);
    return descriptor;
}

void *vz_getUUIDUSBDevice(void *device)
{
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 150000
    if (@available(macOS 15, *))
        return [[[(id<VZUSBDevice>)device uuid] UUIDString] retain];
#endif
    return nil;
}
void vz_attachDeviceVZUSBController(void *pointer, void *devicePointer, uint64_t context)
{
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 150000
    if (@available(macOS 15, *)) {
        VZUSBController *controller = pointer;
        id<VZUSBDevice> device = devicePointer;
        [controller attachDevice:device
               completionHandler:^(NSError *error) {
                   if (!error)
                       vz_inheritObjectQueue(device, controller);
                   emit(VZBridgeCallbackCompletion, context, [error retain], nil, 0);
                   [controller self];
                   [device self];
               }];
        return;
    }
#endif
    unavailable(VZBridgeCallbackCompletion, context);
}
void vz_detachDeviceVZUSBController(void *pointer, void *devicePointer, uint64_t context)
{
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 150000
    if (@available(macOS 15, *)) {
        VZUSBController *controller = pointer;
        id<VZUSBDevice> device = devicePointer;
        [controller detachDevice:device
               completionHandler:^(NSError *error) {
                   emit(VZBridgeCallbackCompletion, context, [error retain], nil, 0);
                   [controller self];
                   [device self];
               }];
        return;
    }
#endif
    unavailable(VZBridgeCallbackCompletion, context);
}

static NSFileHandle *duplicateFile(int descriptor, void **output)
{
    int copied = dup(descriptor);
    if (copied < 0) {
        fail(posixError(errno), output);
        return nil;
    }
    return [[[NSFileHandle alloc] initWithFileDescriptor:copied closeOnDealloc:YES] autorelease];
}
void *vz_newVZFileHandleSerialPortAttachment(int32_t readFD, int32_t writeFD, void **output)
{
    if (output)
        *output = nil;
    NSFileHandle *readHandle = duplicateFile(readFD, output);
    if (!readHandle)
        return nil;
    NSFileHandle *writeHandle = duplicateFile(writeFD, output);
    if (!writeHandle)
        return nil;
    return [[VZFileHandleSerialPortAttachment alloc] initWithFileHandleForReading:readHandle fileHandleForWriting:writeHandle];
}
void *vz_newVZFileHandleNetworkDeviceAttachment(int32_t descriptor, void **output)
{
    if (output)
        *output = nil;
    NSFileHandle *handle = duplicateFile(descriptor, output);
    if (!handle)
        return nil;
    return [[VZFileHandleNetworkDeviceAttachment alloc] initWithFileHandle:handle];
}
void *vz_newVZDiskBlockDeviceStorageDeviceAttachment(int32_t descriptor, bool readOnly, int32_t mode, void **output)
{
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 140000
    if (@available(macOS 14, *)) {
        if (mode != VZDiskSynchronizationModeFull && mode != VZDiskSynchronizationModeNone) {
            fail(posixError(EINVAL), output);
            return nil;
        }
        if (output)
            *output = nil;
        NSFileHandle *handle = duplicateFile(descriptor, output);
        if (!handle)
            return nil;
        NSError *error = nil;
        id result = [[VZDiskBlockDeviceStorageDeviceAttachment alloc] initWithFileHandle:handle readOnly:readOnly synchronizationMode:mode error:&error];
        fail(error, output);
        return result;
    }
#endif
    fail(posixError(ENOTSUP), output);
    return nil;
}

#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 140000
API_AVAILABLE(macos(14.0))
@interface VZBridgeNetworkDelegate : NSObject <VZNetworkBlockDeviceStorageDeviceAttachmentDelegate> {
    uint64_t _context;
}
- (instancetype)initWithContext:(uint64_t)context;
@end
@implementation VZBridgeNetworkDelegate
- (instancetype)initWithContext:(uint64_t)context
{
    self = [super init];
    if (self)
        _context = context;
    return self;
}
- (void)attachment:(VZNetworkBlockDeviceStorageDeviceAttachment *)attachment didEncounterError:(NSError *)error
{
    emit(VZBridgeCallbackNetworkBlockDeviceError, _context, [error retain], nil, 0);
}
- (void)attachmentWasConnected:(VZNetworkBlockDeviceStorageDeviceAttachment *)attachment
{
    emit(VZBridgeCallbackNetworkBlockDeviceConnected, _context, nil, nil, 0);
}
- (void)dealloc
{
    if (_context)
        emit(VZBridgeCallbackContextReleased, _context, nil, nil, 0);
    [super dealloc];
}
@end
#endif
void *vz_newVZNetworkBlockDeviceStorageDeviceAttachment(const char *uri, double timeout, bool readOnly, int32_t mode, void **output, uint64_t context)
{
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 140000
    if (@available(macOS 14, *)) {
        NSURL *url = [NSURL URLWithString:text(uri)];
        if (!url) {
            fail([NSError errorWithDomain:NSURLErrorDomain code:NSURLErrorBadURL userInfo:nil], output);
            return nil;
        }
        if (mode != VZDiskSynchronizationModeFull && mode != VZDiskSynchronizationModeNone) {
            fail(posixError(EINVAL), output);
            return nil;
        }
        NSError *error = nil;
        VZNetworkBlockDeviceStorageDeviceAttachment *attachment = [[VZNetworkBlockDeviceStorageDeviceAttachment alloc] initWithURL:url timeout:timeout forcedReadOnly:readOnly synchronizationMode:mode error:&error];
        fail(error, output);
        if (!attachment)
            return nil;
        VZBridgeNetworkDelegate *delegate = [[VZBridgeNetworkDelegate alloc] initWithContext:context];
        attachment.delegate = delegate;
        static char associationKey;
        objc_setAssociatedObject(attachment, &associationKey, delegate, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
        [delegate release];
        return attachment;
    }
#endif
    fail(posixError(ENOTSUP), output);
    return nil;
}

#if defined(__arm64__) && __MAC_OS_X_VERSION_MAX_ALLOWED >= 120000
void vz_fetchLatestSupportedMacOSRestoreImageWithCompletionHandler(uint64_t context)
{
    if (@available(macOS 12, *)) {
        [VZMacOSRestoreImage fetchLatestSupportedWithCompletionHandler:^(VZMacOSRestoreImage *image, NSError *error) {
            emit(VZBridgeCallbackRestoreImageLoaded, context, [image retain], [error retain], 0);
        }];
        return;
    }
    unavailable(VZBridgeCallbackRestoreImageLoaded, context);
}
void vz_loadMacOSRestoreImageFile(const char *path, uint64_t context)
{
    if (@available(macOS 12, *)) {
        [VZMacOSRestoreImage loadFileURL:fileURL(path)
                       completionHandler:^(VZMacOSRestoreImage *image, NSError *error) {
                           emit(VZBridgeCallbackRestoreImageLoaded, context, [image retain], [error retain], 0);
                       }];
        return;
    }
    unavailable(VZBridgeCallbackRestoreImageLoaded, context);
}
int64_t vz_macOSRestoreImageMajorVersion(void *object)
{
    if (@available(macOS 12, *))
        return [(VZMacOSRestoreImage *)object operatingSystemVersion].majorVersion;
    return 0;
}
int64_t vz_macOSRestoreImageMinorVersion(void *object)
{
    if (@available(macOS 12, *))
        return [(VZMacOSRestoreImage *)object operatingSystemVersion].minorVersion;
    return 0;
}
int64_t vz_macOSRestoreImagePatchVersion(void *object)
{
    if (@available(macOS 12, *))
        return [(VZMacOSRestoreImage *)object operatingSystemVersion].patchVersion;
    return 0;
}

API_AVAILABLE(macos(12.0))
@interface VZBridgeInstaller : NSObject {
    VZMacOSInstaller *_installer;
    uint64_t _progressContext;
    BOOL _observing;
}
- (instancetype)initWithMachine:(VZVirtualMachine *)machine path:(const char *)path;
- (void)install:(uint64_t)completionContext progress:(uint64_t)progress;
- (void)cancel;
@end
@implementation VZBridgeInstaller
- (instancetype)initWithMachine:(VZVirtualMachine *)machine path:(const char *)path
{
    self = [super init];
    if (self) {
        vz_inheritObjectQueue(self, machine);
        _installer = [[VZMacOSInstaller alloc] initWithVirtualMachine:machine restoreImageURL:fileURL(path)];
        vz_inheritObjectQueue(_installer, machine);
    }
    return self;
}
- (void)finishProgress
{
    if (_observing) {
        [_installer.progress removeObserver:self forKeyPath:@"fractionCompleted" context:&bridgeQueueKey];
        _observing = NO;
    }
    uint64_t context = _progressContext;
    _progressContext = 0;
    if (context)
        emit(VZBridgeCallbackContextReleased, context, nil, nil, 0);
}
- (void)install:(uint64_t)completionContext progress:(uint64_t)progress
{
    if (_progressContext) {
        emit(VZBridgeCallbackContextReleased, progress, nil, nil, 0);
        emit(VZBridgeCallbackMacOSInstallCompleted, completionContext, [posixError(EBUSY) retain], nil, 0);
        return;
    }
    _progressContext = progress;
    _observing = YES;
    [_installer.progress addObserver:self forKeyPath:@"fractionCompleted" options:NSKeyValueObservingOptionInitial | NSKeyValueObservingOptionNew context:&bridgeQueueKey];
    [_installer installWithCompletionHandler:^(NSError *error) {
        syncObject(self, ^{
            [self finishProgress];
            emit(VZBridgeCallbackMacOSInstallCompleted, completionContext, [error retain], nil, 0);
        });
    }];
}
- (void)observeValueForKeyPath:(NSString *)key ofObject:(id)object change:(NSDictionary *)change context:(void *)context
{
    if (context != &bridgeQueueKey) {
        [super observeValueForKeyPath:key ofObject:object change:change context:context];
        return;
    }
    double progress = [change[NSKeyValueChangeNewKey] doubleValue];
    uint64_t bits;
    memcpy(&bits, &progress, sizeof(bits));
    syncObject(self, ^{ if (_progressContext) emit(VZBridgeCallbackMacOSInstallProgress, _progressContext, nil, nil, bits); });
}
- (void)cancel
{
    if (_installer.progress.cancellable)
        [_installer.progress cancel];
}
- (void)dealloc
{
    [self finishProgress];
    [_installer release];
    [super dealloc];
}
@end
void *vz_newVZMacOSInstaller(void *machine, const char *path)
{
    if (@available(macOS 12, *))
        return [[VZBridgeInstaller alloc] initWithMachine:machine path:path];
    return nil;
}
void vz_installByVZMacOSInstaller(void *installer, uint64_t completionContext, uint64_t progress)
{
    if (@available(macOS 12, *)) {
        [(VZBridgeInstaller *)installer install:completionContext progress:progress];
        return;
    }
    emit(VZBridgeCallbackContextReleased, progress, nil, nil, 0);
    unavailable(VZBridgeCallbackMacOSInstallCompleted, completionContext);
}
void vz_cancelInstallVZMacOSInstaller(void *installer)
{
    if (@available(macOS 12, *))
        [(VZBridgeInstaller *)installer cancel];
}
#else
void vz_fetchLatestSupportedMacOSRestoreImageWithCompletionHandler(uint64_t context) { unavailable(VZBridgeCallbackRestoreImageLoaded, context); }
void vz_loadMacOSRestoreImageFile(const char *path, uint64_t context) { unavailable(VZBridgeCallbackRestoreImageLoaded, context); }
int64_t vz_macOSRestoreImageMajorVersion(void *object) { return 0; }
int64_t vz_macOSRestoreImageMinorVersion(void *object) { return 0; }
int64_t vz_macOSRestoreImagePatchVersion(void *object) { return 0; }
void *vz_newVZMacOSInstaller(void *machine, const char *path) { return nil; }
void vz_installByVZMacOSInstaller(void *installer, uint64_t context, uint64_t progress)
{
    emit(VZBridgeCallbackContextReleased, progress, nil, nil, 0);
    unavailable(VZBridgeCallbackMacOSInstallCompleted, context);
}
void vz_cancelInstallVZMacOSInstaller(void *installer) { }
#endif

void vz_linuxInstallRosetta(uint64_t context)
{
#if defined(__arm64__) && __MAC_OS_X_VERSION_MAX_ALLOWED >= 130000
    if (@available(macOS 13, *)) {
        [VZLinuxRosettaDirectoryShare installRosettaWithCompletionHandler:^(NSError *error) { emit(VZBridgeCallbackRosettaInstallCompleted, context, [error retain], nil, 0); }];
        return;
    }
#endif
    unavailable(VZBridgeCallbackRosettaInstallCompleted, context);
}
void vz_saveMachineStateToURLWithCompletionHandler(void *machine, uint64_t context, const char *path)
{
#if defined(__arm64__) && __MAC_OS_X_VERSION_MAX_ALLOWED >= 140000
    if (@available(macOS 14, *)) {
        [(VZVirtualMachine *)machine saveMachineStateToURL:fileURL(path) completionHandler:completion(machine, context)];
        return;
    }
#endif
    unavailable(VZBridgeCallbackCompletion, context);
}
void vz_restoreMachineStateFromURLWithCompletionHandler(void *machine, uint64_t context, const char *path)
{
#if defined(__arm64__) && __MAC_OS_X_VERSION_MAX_ALLOWED >= 140000
    if (@available(macOS 14, *)) {
        [(VZVirtualMachine *)machine restoreMachineStateFromURL:fileURL(path) completionHandler:completion(machine, context)];
        return;
    }
#endif
    unavailable(VZBridgeCallbackCompletion, context);
}

#import "native.h"
#import <Foundation/Foundation.h>
#import <Virtualization/Virtualization.h>
#import <errno.h>
#import <objc/runtime.h>
#import <unistd.h>

static dispatch_queue_t bridgeQueue;
static char bridgeQueueKey;
typedef uint64_t (*BridgeCallback)(uint32_t, uint64_t, void *, void *, uint64_t);
static BridgeCallback bridgeCallback;

static dispatch_queue_t queue(void)
{
    static dispatch_once_t once;
    dispatch_once(&once, ^{
        bridgeQueue = dispatch_queue_create("com.codehex.vz.native", DISPATCH_QUEUE_SERIAL);
        dispatch_queue_set_specific(bridgeQueue, &bridgeQueueKey, &bridgeQueueKey, NULL);
    });
    return bridgeQueue;
}

static void syncNative(void (^body)(void))
{
    dispatch_queue_t target = queue();
    if (dispatch_get_specific(&bridgeQueueKey)) {
        @autoreleasepool {
            body();
        }
    } else {
        dispatch_sync(target, ^{ @autoreleasepool { body(); } });
    }
}

void vz_dispatchSync(uintptr_t callback, uintptr_t context)
{
    syncNative(^{ ((void (*)(uintptr_t))callback)(context); });
}

void *vz_dispatchQueuePointer(void) { return queue(); }
void vz_drain(void) { syncNative(^ { }); }
void vz_setCallback(uint64_t callback) { bridgeCallback = (BridgeCallback)(uintptr_t)callback; }
void vz_releaseObject(void *object)
{
    if (!object)
        return;
    // A void pointer keeps the block from taking an extra Objective-C reference.
    dispatch_async(queue(), ^{ @autoreleasepool { [(id)object release]; } });
}

void vz_setInvalidDiskModeError(void **errorOut)
{
    *errorOut = [[NSError errorWithDomain:NSPOSIXErrorDomain
                                     code:EINVAL
                                 userInfo:@ { NSLocalizedDescriptionKey : @"Invalid disk caching or synchronization mode" }] retain];
}

static uint64_t emit(uint32_t kind, uint64_t context, id first, id second, uint64_t value)
{
    __block uint64_t result = 0;
    syncNative(^{
        if (bridgeCallback) {
            result = bridgeCallback(kind, context, first, second, value);
        } else {
            if (kind == 1 || kind == 3 || kind == 5 || kind == 6 || kind == 7 || kind == 9 || kind == 10 || kind == 11)
                [first release];
            if (kind == 5 || kind == 10)
                [second release];
        }
    });
    return result;
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
    if (kind == 10)
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
- (instancetype)initWithConfiguration:(VZVirtualMachineConfiguration *)configuration state:(uint64_t)state network:(uint64_t)network;
@end

@implementation VZBridgeMachine
- (instancetype)initWithConfiguration:(VZVirtualMachineConfiguration *)configuration state:(uint64_t)state network:(uint64_t)network
{
    self = [super initWithConfiguration:configuration queue:queue()];
    if (self) {
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
        emit(2, _stateContext, nil, nil, [change[NSKeyValueChangeNewKey] unsignedLongLongValue]);
    } else {
        [super observeValueForKeyPath:key ofObject:object change:change context:context];
    }
}
- (void)setDelegate:(id<VZVirtualMachineDelegate>)delegate
{
    if (delegate == self)
        return;
    syncNative(^{
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
    for (id<VZVirtualMachineDelegate> delegate in _externalDelegates.allObjects) {
        if ([delegate respondsToSelector:_cmd])
            [delegate virtualMachine:machine didStopWithError:error];
    }
}
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 120000
- (void)virtualMachine:(VZVirtualMachine *)machine networkDevice:(VZNetworkDevice *)device attachmentWasDisconnectedWithError:(NSError *)error API_AVAILABLE(macos(12.0))
{
    NSInteger index = [machine.networkDevices indexOfObjectIdenticalTo:device];
    emit(3, _networkContext, [error retain], nil, index == NSNotFound ? UINT64_MAX : (uint64_t)index);
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
        emit(4, _stateContext, nil, nil, 0);
    if (_networkContext)
        emit(4, _networkContext, nil, nil, 0);
    [super dealloc];
}
@end

void *vz_newVZVirtualMachineWithDispatchQueue(void *config, uint64_t state, uint64_t network)
{
    return [[VZBridgeMachine alloc] initWithConfiguration:config state:state network:network];
}

static void (^completion(id owner, uint64_t context))(NSError *)
{
    return [[^(NSError *error) {
        emit(1, context, [error retain], nil, 0);
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
    unavailable(1, context);
}
void vz_startWithOptionsCompletionHandler(void *machine, void *options, uint64_t context)
{
#if defined(__arm64__) && __MAC_OS_X_VERSION_MAX_ALLOWED >= 130000
    if (@available(macOS 13, *)) {
        [(VZVirtualMachine *)machine startWithOptions:options completionHandler:completion(machine, context)];
        return;
    }
#endif
    unavailable(1, context);
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
    return emit(6, _context, [connection retain], device, 0) != 0;
}
- (void)invalidate
{
    self.delegate = nil;
    uint64_t context = _context;
    _context = 0;
    if (context)
        emit(4, context, nil, nil, 0);
}
- (void)dealloc
{
    [self invalidate];
    [super dealloc];
}
@end

void *vz_newVZVirtioSocketListener(uint64_t context) { return [[VZBridgeSocketListener alloc] initWithContext:context]; }
void vz_invalidateVZVirtioSocketListener(void *listener) { [(VZBridgeSocketListener *)listener invalidate]; }
void vz_VZVirtioSocketDevice_connectToPort(void *pointer, uint32_t port, uint64_t context)
{
    VZVirtioSocketDevice *device = pointer;
    [device connectToPort:port
        completionHandler:^(VZVirtioSocketConnection *connection, NSError *error) {
            emit(5, context, [connection retain], [error retain], 0);
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
                   emit(1, context, [error retain], nil, 0);
                   [controller self];
                   [device self];
               }];
        return;
    }
#endif
    unavailable(1, context);
}
void vz_detachDeviceVZUSBController(void *pointer, void *devicePointer, uint64_t context)
{
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 150000
    if (@available(macOS 15, *)) {
        VZUSBController *controller = pointer;
        id<VZUSBDevice> device = devicePointer;
        [controller detachDevice:device
               completionHandler:^(NSError *error) {
                   emit(1, context, [error retain], nil, 0);
                   [controller self];
                   [device self];
               }];
        return;
    }
#endif
    unavailable(1, context);
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
    emit(7, _context, [error retain], nil, 0);
}
- (void)attachmentWasConnected:(VZNetworkBlockDeviceStorageDeviceAttachment *)attachment
{
    emit(8, _context, nil, nil, 0);
}
- (void)dealloc
{
    if (_context)
        emit(4, _context, nil, nil, 0);
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
            emit(10, context, [image retain], [error retain], 0);
        }];
        return;
    }
    unavailable(10, context);
}
void vz_loadMacOSRestoreImageFile(const char *path, uint64_t context)
{
    if (@available(macOS 12, *)) {
        [VZMacOSRestoreImage loadFileURL:fileURL(path)
                       completionHandler:^(VZMacOSRestoreImage *image, NSError *error) {
                           emit(10, context, [image retain], [error retain], 0);
                       }];
        return;
    }
    unavailable(10, context);
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
    if (self)
        _installer = [[VZMacOSInstaller alloc] initWithVirtualMachine:machine restoreImageURL:fileURL(path)];
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
        emit(4, context, nil, nil, 0);
}
- (void)install:(uint64_t)completionContext progress:(uint64_t)progress
{
    if (_progressContext) {
        emit(4, progress, nil, nil, 0);
        emit(11, completionContext, [posixError(EBUSY) retain], nil, 0);
        return;
    }
    _progressContext = progress;
    _observing = YES;
    [_installer.progress addObserver:self forKeyPath:@"fractionCompleted" options:NSKeyValueObservingOptionInitial | NSKeyValueObservingOptionNew context:&bridgeQueueKey];
    [_installer installWithCompletionHandler:^(NSError *error) {
        syncNative(^{
            [self finishProgress];
            emit(11, completionContext, [error retain], nil, 0);
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
    syncNative(^{ if (_progressContext) emit(12, _progressContext, nil, nil, bits); });
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
    emit(4, progress, nil, nil, 0);
    unavailable(11, completionContext);
}
void vz_cancelInstallVZMacOSInstaller(void *installer)
{
    if (@available(macOS 12, *))
        [(VZBridgeInstaller *)installer cancel];
}
#else
void vz_fetchLatestSupportedMacOSRestoreImageWithCompletionHandler(uint64_t context) { unavailable(10, context); }
void vz_loadMacOSRestoreImageFile(const char *path, uint64_t context) { unavailable(10, context); }
int64_t vz_macOSRestoreImageMajorVersion(void *object) { return 0; }
int64_t vz_macOSRestoreImageMinorVersion(void *object) { return 0; }
int64_t vz_macOSRestoreImagePatchVersion(void *object) { return 0; }
void *vz_newVZMacOSInstaller(void *machine, const char *path) { return nil; }
void vz_installByVZMacOSInstaller(void *installer, uint64_t context, uint64_t progress)
{
    emit(4, progress, nil, nil, 0);
    unavailable(11, context);
}
void vz_cancelInstallVZMacOSInstaller(void *installer) { }
#endif

void vz_linuxInstallRosetta(uint64_t context)
{
#if defined(__arm64__) && __MAC_OS_X_VERSION_MAX_ALLOWED >= 130000
    if (@available(macOS 13, *)) {
        [VZLinuxRosettaDirectoryShare installRosettaWithCompletionHandler:^(NSError *error) { emit(9, context, [error retain], nil, 0); }];
        return;
    }
#endif
    unavailable(9, context);
}
void vz_saveMachineStateToURLWithCompletionHandler(void *machine, uint64_t context, const char *path)
{
#if defined(__arm64__) && __MAC_OS_X_VERSION_MAX_ALLOWED >= 140000
    if (@available(macOS 14, *)) {
        [(VZVirtualMachine *)machine saveMachineStateToURL:fileURL(path) completionHandler:completion(machine, context)];
        return;
    }
#endif
    unavailable(1, context);
}
void vz_restoreMachineStateFromURLWithCompletionHandler(void *machine, uint64_t context, const char *path)
{
#if defined(__arm64__) && __MAC_OS_X_VERSION_MAX_ALLOWED >= 140000
    if (@available(macOS 14, *)) {
        [(VZVirtualMachine *)machine restoreMachineStateFromURL:fileURL(path) completionHandler:completion(machine, context)];
        return;
    }
#endif
    unavailable(1, context);
}

#import "native.h"
#import "virtualization_view.h"
#import <pthread.h>

@interface VZGraphicWindowDescription : NSObject {
@public
    VZVirtualMachine *machine;
    double width;
    double height;
    NSString *title;
    BOOL enableController;
}
@end

@implementation VZGraphicWindowDescription
- (void)dealloc
{
    vz_releaseObject(machine);
    [title release];
    [super dealloc];
}
@end

API_AVAILABLE(macos(12.0))
@interface VZGraphicApplicationController : NSObject <NSApplicationDelegate> {
@public
    NSUInteger pendingAdditions;
    BOOL stopping;
}
@property (nonatomic, retain) NSMutableDictionary<NSValue *, VZGraphicWindowController *> *windows;
- (VZGraphicApplicationResult)addWindows:(NSArray *)descriptions;
- (void)windowClosed:(VZGraphicWindowController *)window;
- (void)stopIfIdle;
@end

static pthread_mutex_t applicationLock = PTHREAD_MUTEX_INITIALIZER;
static VZGraphicApplicationController *activeApplication API_AVAILABLE(macos(12.0));

@implementation VZGraphicApplicationController
- (VZGraphicApplicationResult)addWindows:(NSArray *)descriptions
{
    if (stopping) {
        return VZGraphicApplicationStopping;
    }
    NSMutableSet *machines = [NSMutableSet set];
    for (VZGraphicWindowDescription *description in descriptions) {
        NSValue *key = [NSValue valueWithPointer:description->machine];
        if ([self.windows objectForKey:key] || [machines containsObject:key]) {
            return VZGraphicApplicationDuplicateMachine;
        }
        [machines addObject:key];
    }
    for (VZGraphicWindowDescription *description in descriptions) {
        VZVirtualMachine *machine = description->machine;
        VZGraphicWindowController *window = [[VZGraphicWindowController alloc]
            initWithVirtualMachine:machine
                             queue:(dispatch_queue_t)vz_objectQueue(machine)
                       windowWidth:description->width
                      windowHeight:description->height
                       windowTitle:description->title
                  enableController:description->enableController];
        window.didClose = ^(VZGraphicWindowController *closedWindow) {
            [self windowClosed:closedWindow];
        };
        if (self.windows.count == 0) {
            [window setupMenuBar];
        }
        [self.windows setObject:window forKey:[NSValue valueWithPointer:machine]];
        [window showWindow];
        [window release];
    }
    [NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
    [NSApp activateIgnoringOtherApps:YES];
    return VZGraphicApplicationSuccess;
}
- (void)stopIfIdle
{
    pthread_mutex_lock(&applicationLock);
    BOOL done = self.windows.count == 0 && pendingAdditions == 0;
    if (done) {
        stopping = YES;
    }
    pthread_mutex_unlock(&applicationLock);
    if (done) {
        [NSApp stop:nil];
    }
}
- (void)windowClosed:(VZGraphicWindowController *)window
{
    [[window retain] autorelease];
    window.didClose = nil;
    for (NSValue *machine in self.windows.keyEnumerator) {
        if ([self.windows objectForKey:machine] == window) {
            [self.windows removeObjectForKey:machine];
            break;
        }
    }
    [self stopIfIdle];
}
- (NSApplicationTerminateReply)applicationShouldTerminate:(NSApplication *)sender
{
    pthread_mutex_lock(&applicationLock);
    stopping = YES;
    pthread_mutex_unlock(&applicationLock);
    for (VZGraphicWindowController *window in self.windows.objectEnumerator.allObjects) {
        [window close];
    }
    [self stopIfIdle];
    return NSTerminateCancel;
}
- (void)dealloc
{
    for (VZGraphicWindowController *window in _windows.objectEnumerator) {
        window.didClose = nil;
        [window close];
    }
    [_windows release];
    [super dealloc];
}
@end

void *vz_newGraphicApplication(void)
{
    return [[NSMutableArray alloc] init];
}

void vz_addGraphicWindow(void *application, void *machine, double width, double height, const char *title, bool enableController)
{
    VZGraphicWindowDescription *description = [[VZGraphicWindowDescription alloc] init];
    description->machine = [(VZVirtualMachine *)machine retain];
    description->width = width;
    description->height = height;
    description->title = [[NSString alloc] initWithUTF8String:title ? title : ""];
    if (!description->title) {
        description->title = [@"" retain];
    }
    description->enableController = enableController;
    [(NSMutableArray *)application addObject:description];
    [description release];
}

VZGraphicApplicationResult vz_runGraphicApplication(void *application)
{
    if (@available(macOS 12, *)) {
        pthread_mutex_lock(&applicationLock);
        VZGraphicApplicationController *controller = activeApplication;
        if (controller) {
            if (controller->stopping) {
                pthread_mutex_unlock(&applicationLock);
                return VZGraphicApplicationStopping;
            }
            [controller retain];
            controller->pendingAdditions++;
        }
        pthread_mutex_unlock(&applicationLock);
        if (controller) {
            __block VZGraphicApplicationResult result;
            void (^addWindows)(void) = ^{
                @autoreleasepool {
                    result = [controller addWindows:(NSArray *)application];
                    pthread_mutex_lock(&applicationLock);
                    controller->pendingAdditions--;
                    pthread_mutex_unlock(&applicationLock);
                    [controller stopIfIdle];
                    [controller release];
                }
            };
            if ([NSThread isMainThread]) {
                addWindows();
            } else {
                dispatch_sync(dispatch_get_main_queue(), addWindows);
            }
            return result;
        }
        if (![NSThread isMainThread]) {
            return VZGraphicApplicationNotMainThread;
        }
        @autoreleasepool {
            [VZApplication sharedApplication];
            controller = [[VZGraphicApplicationController alloc] init];
            controller.windows = [NSMutableDictionary dictionary];
            VZGraphicApplicationResult result;
            @autoreleasepool {
                NSApp.delegate = controller;
                result = [controller addWindows:(NSArray *)application];
            }
            if (result == VZGraphicApplicationSuccess) {
                pthread_mutex_lock(&applicationLock);
                activeApplication = controller;
                pthread_mutex_unlock(&applicationLock);
                [NSApp run];
            }
            pthread_mutex_lock(&applicationLock);
            activeApplication = nil;
            pthread_mutex_unlock(&applicationLock);
            NSApp.delegate = nil;
            [controller release];
            return result;
        }
    }
    return VZGraphicApplicationSuccess;
}

void vz_freeGraphicApplication(void *application)
{
    [(NSMutableArray *)application release];
}

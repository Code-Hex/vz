#import "native.h"
#import "virtualization_view.h"

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
    [title release];
    [super dealloc];
}
@end

API_AVAILABLE(macos(12.0))
@interface VZGraphicApplicationController : NSObject <NSApplicationDelegate>
@property (nonatomic, retain) NSMutableArray<VZGraphicWindowController *> *windows;
- (void)windowClosed:(VZGraphicWindowController *)window;
- (void)showWindows;
@end

@implementation VZGraphicApplicationController
- (void)showWindows
{
    [self.windows.firstObject setupMenuBar];
    for (VZGraphicWindowController *window in self.windows) {
        [window showWindow];
    }
    [NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
    [NSApp activateIgnoringOtherApps:YES];
}
- (void)windowClosed:(VZGraphicWindowController *)window
{
    [[window retain] autorelease];
    window.didClose = nil;
    [self.windows removeObjectIdenticalTo:window];
    if (self.windows.count == 0) {
        [NSApp stop:nil];
    }
}
- (void)dealloc
{
    for (VZGraphicWindowController *window in _windows) {
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
    description->machine = machine;
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
    static BOOL running;
    if (![NSThread isMainThread]) {
        return VZGraphicApplicationNotMainThread;
    }
    if (running) {
        return VZGraphicApplicationAlreadyRunning;
    }
    running = YES;
    if (@available(macOS 12, *)) {
        @autoreleasepool {
            [VZApplication sharedApplication];
            VZGraphicApplicationController *controller = [[VZGraphicApplicationController alloc] init];
            @autoreleasepool {
                controller.windows = [NSMutableArray array];
                for (VZGraphicWindowDescription *description in (NSArray *)application) {
                    VZVirtualMachine *machine = description->machine;
                    VZGraphicWindowController *window = [[VZGraphicWindowController alloc]
                        initWithVirtualMachine:machine
                                         queue:(dispatch_queue_t)vz_objectQueue(machine)
                                   windowWidth:description->width
                                  windowHeight:description->height
                                   windowTitle:description->title
                              enableController:description->enableController];
                    window.didClose = ^(VZGraphicWindowController *closedWindow) {
                        [controller windowClosed:closedWindow];
                    };
                    [controller.windows addObject:window];
                    [window release];
                }
                NSApp.delegate = controller;
                [controller showWindows];
            }
            [NSApp run];
            NSApp.delegate = nil;
            for (VZGraphicWindowController *window in controller.windows) {
                window.didClose = nil;
                [window close];
            }
            [controller release];
        }
    }
    running = NO;
    return VZGraphicApplicationSuccess;
}

void vz_freeGraphicApplication(void *application)
{
    [(NSMutableArray *)application release];
}

//
//  virtualization_view.m
//
//  Created by codehex.
//

#import "virtualization_view.h"

@implementation AppDelegate {
    VZVirtualMachine *_virtualMachine;
    VZVirtualMachineView *_virtualMachineView;
    CGFloat _windowWidth;
    CGFloat _windowHeight;
}

- (instancetype)initWithVirtualMachine:(VZVirtualMachine *)virtualMachine
    windowWidth:(CGFloat)windowWidth
    windowHeight:(CGFloat)windowHeight
{
    self = [super init];
    _virtualMachine = virtualMachine;
    _virtualMachine.delegate = self;

    // Setup virtual machine view configs
    VZVirtualMachineView *view = [[[VZVirtualMachineView alloc] init] autorelease];
    view.capturesSystemKeys = NO;
    view.virtualMachine = _virtualMachine;
    _virtualMachineView = view;

    // Setup some window configs
    _windowWidth = windowWidth;
    _windowHeight = windowHeight;
    return self;
}

/* IMPORTANT: delegate methods are called from VM's queue */
- (void)guestDidStopVirtualMachine:(VZVirtualMachine *)virtualMachine {
    NSLog(@"VM %@ guest stopped", virtualMachine);
    [NSApp performSelectorOnMainThread:@selector(terminate:) withObject:self waitUntilDone:NO];
}

- (void)virtualMachine:(VZVirtualMachine *)virtualMachine didStopWithError:(NSError *)error {
    NSLog(@"VM %@ didStopWithError: %@", virtualMachine, error);
    [NSApp performSelectorOnMainThread:@selector(terminate:) withObject:self waitUntilDone:NO];
}

- (void)applicationDidFinishLaunching:(NSNotification *)notification {
    [self setupMenuBar];
    [self setupGraphicWindow];

    // These methods are required to call here. Because the menubar will be not active even if
    // application is running.
    // See: https://stackoverflow.com/questions/62739862/why-doesnt-activateignoringotherapps-enable-the-menu-bar
    [NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
    [NSApp activateIgnoringOtherApps:YES];
}

- (BOOL)applicationShouldTerminateAfterLastWindowClosed:(NSApplication *)sender
{
    return YES;
}

- (void)setupGraphicWindow
{
    NSRect rect = NSMakeRect(0, 0, _windowWidth, _windowHeight);
    NSWindow *window = [[[NSWindow alloc] initWithContentRect:rect
                            styleMask:NSWindowStyleMaskTitled|NSWindowStyleMaskClosable|NSWindowStyleMaskMiniaturizable|NSWindowStyleMaskResizable//|NSTexturedBackgroundWindowMask
                            backing:NSBackingStoreBuffered defer:NO] autorelease];
    [window setOpaque:NO];
    [window setContentView:_virtualMachineView];
    // [window setDelegate:self];
    // [window setInitialFirstResponder:view];
    [window setTitleVisibility:NSWindowTitleHidden];
    [window center];

    [window makeKeyAndOrderFront:nil];
}

- (void)setupMenuBar
{
    NSMenu *menuBar = [[[NSMenu alloc] init] autorelease];
    NSMenuItem *menuBarItem = [[[NSMenuItem alloc] init] autorelease];
    [menuBar addItem:menuBarItem];
    [NSApp setMainMenu:menuBar];

    // App menu
    NSMenu *appMenu = [self setupApplicationMenu];
    [menuBarItem setSubmenu:appMenu];

    // Window menu
    NSMenu *windowMenu = [self setupWindowMenu];
    NSMenuItem *windowMenuItem = [[[NSMenuItem alloc] initWithTitle:@"Window" action:nil keyEquivalent:@""] autorelease];
    [menuBar addItem:windowMenuItem];
    [windowMenuItem setSubmenu:windowMenu];
}


- (NSMenu *)setupApplicationMenu
{
    NSMenu *appMenu = [[[NSMenu alloc] init] autorelease];
    NSString *applicationName = [[NSProcessInfo processInfo] processName];

    // Service menu
    NSMenuItem *servicesMenuItem = [[[NSMenuItem alloc] initWithTitle:@"Services" action:nil keyEquivalent:@""] autorelease];
    NSMenu *servicesMenu = [[[NSMenu alloc] initWithTitle:@"Services"] autorelease];
    [servicesMenuItem setSubmenu:servicesMenu];
    [NSApp setServicesMenu:servicesMenu];

    NSMenuItem *hideOthersItem = [[[NSMenuItem alloc]
            initWithTitle:@"Hide Others"
            action:@selector(hideOtherApplications:)
            keyEquivalent:@"h"] autorelease];
    [hideOthersItem setKeyEquivalentModifierMask:(NSEventModifierFlagOption|NSEventModifierFlagCommand)];

    NSArray *menuItems = @[
        servicesMenuItem,
        [NSMenuItem separatorItem],
        [[[NSMenuItem alloc]
            initWithTitle:[@"Hide " stringByAppendingString:applicationName]
            action:@selector(hide:)
            keyEquivalent:@"h"] autorelease],
        hideOthersItem,
        [NSMenuItem separatorItem],
        [[[NSMenuItem alloc]
            initWithTitle:[@"Quit " stringByAppendingString:applicationName]
            action:@selector(terminate:)
            keyEquivalent:@"q"] autorelease],
    ];
    for (NSMenuItem *menuItem in menuItems) {
        [appMenu addItem:menuItem];
    }
    return appMenu;
}

- (NSMenu *)setupWindowMenu
{
    NSMenu *windowMenu = [[[NSMenu alloc] initWithTitle:@"Window"] autorelease];
    NSArray *menuItems = @[
        [[[NSMenuItem alloc] initWithTitle:@"Minimize" action:@selector(performMiniaturize:) keyEquivalent:@"m"] autorelease],
        [[[NSMenuItem alloc] initWithTitle:@"Zoom" action:@selector(performZoom:) keyEquivalent:@""] autorelease],
        [[[NSMenuItem alloc] initWithTitle:@"Close Window" action:@selector(performClose:) keyEquivalent:@"w"] autorelease],
        [[[NSMenuItem alloc] initWithTitle:@"Copy" action:@selector(copy:) keyEquivalent:@"c"] autorelease],
        [[[NSMenuItem alloc] initWithTitle:@"Paste" action:@selector(paste:) keyEquivalent:@"v"] autorelease],
    ];
    for (NSMenuItem *menuItem in menuItems) {
        [windowMenu addItem:menuItem];
    }
    [NSApp setWindowsMenu:windowMenu];
    return windowMenu;
}

@end

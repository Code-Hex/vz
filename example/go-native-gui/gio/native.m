//go:build darwin

#include "native.h"
#include "_cgo_export.h"
#import <AppKit/AppKit.h>
int vzGUIGioMountView(uintptr_t host, void *child)
{
    NSView *parent = (__bridge NSView *)(void *)host;
    NSView *view = (__bridge NSView *)child;
    view.translatesAutoresizingMaskIntoConstraints = NO;
    [parent addSubview:view];
    [NSLayoutConstraint activateConstraints:@[
        [view.leadingAnchor constraintEqualToAnchor:parent.leadingAnchor],
        [view.trailingAnchor constraintEqualToAnchor:parent.trailingAnchor],
        [view.bottomAnchor constraintEqualToAnchor:parent.bottomAnchor],
        [view.topAnchor constraintEqualToAnchor:parent.topAnchor
                                       constant:60]
    ]];
    [parent.window makeFirstResponder:view];
    return [NSThread isMainThread] && view.superview == parent;
}

void vzGUIGioScheduleHostClose(uintptr_t host)
{
    NSWindow *window = ((__bridge NSView *)(void *)host).window;
    // performClose synchronously asks Gio to deliver a ClosingEvent.
    dispatch_async(dispatch_get_main_queue(), ^{ [window performClose:nil]; });
}

void vzGUIGioFocusView(uintptr_t host, void *child)
{
    NSView *parent = (__bridge NSView *)(void *)host;
    NSView *view = (__bridge NSView *)child;
    if (view.superview == parent)
        [parent.window makeFirstResponder:view];
}

void vzGUIGioRestoreHostFocus(uintptr_t host)
{
    NSView *parent = (__bridge NSView *)(void *)host;
    [parent.window makeFirstResponder:parent];
}

@interface VZGoGUIGioApplicationDelegate : NSObject <NSApplicationDelegate>
@property (nonatomic, strong) id<NSApplicationDelegate> original;
@property (nonatomic) uintptr_t callback;
@end
@implementation VZGoGUIGioApplicationDelegate
- (BOOL)respondsToSelector:(SEL)selector
{
    return [super respondsToSelector:selector] || [self.original respondsToSelector:selector];
}
- (id)forwardingTargetForSelector:(SEL)selector
{
    return self.original;
}
- (NSApplicationTerminateReply)applicationShouldTerminate:(NSApplication *)sender
{
    vzGUIGioApplicationShouldTerminate(self.callback);
    return NSTerminateCancel;
}
@end

// Gio emits DestroyEvent from GioView.dealloc, which may follow window closure.
@interface VZGoGUIGioWindowCloseObserver : NSObject
@property (nonatomic) uintptr_t callback;
@property (nonatomic, strong) VZGoGUIGioApplicationDelegate *applicationDelegate;
@end
@implementation VZGoGUIGioWindowCloseObserver
- (void)windowWillClose:(NSNotification *)notification
{
    __attribute__((objc_precise_lifetime)) VZGoGUIGioWindowCloseObserver *observer = self;
    [[NSNotificationCenter defaultCenter] removeObserver:observer];
    if (NSApp.delegate == observer.applicationDelegate)
        NSApp.delegate = observer.applicationDelegate.original;
    vzGUIGioWindowWillClose(observer.callback);
    CFRelease((__bridge CFTypeRef)observer);
}
@end

int vzGUIGioObserveWindowClose(uintptr_t host, uintptr_t callback)
{
    NSView *parent = (__bridge NSView *)(void *)host;
    if (![NSThread isMainThread] || !parent.window)
        return 0;
    VZGoGUIGioWindowCloseObserver *observer = [VZGoGUIGioWindowCloseObserver new];
    observer.callback = callback;
    observer.applicationDelegate = [VZGoGUIGioApplicationDelegate new];
    observer.applicationDelegate.original = NSApp.delegate;
    observer.applicationDelegate.callback = callback;
    NSApp.delegate = observer.applicationDelegate;
    [[NSNotificationCenter defaultCenter] addObserver:observer
                                             selector:@selector(windowWillClose:)
                                                 name:NSWindowWillCloseNotification
                                               object:parent.window];
    // The one-shot callback balances this reference after unregistering itself.
    CFBridgingRetain(observer);
    return 1;
}

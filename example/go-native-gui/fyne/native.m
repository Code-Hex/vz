//go:build darwin

#include "native.h"
#import <AppKit/AppKit.h>
int vzGUIFyneMount(uintptr_t window, void *pointer)
{
    NSWindow *host = (__bridge NSWindow *)(void *)window;
    NSView *view = (__bridge NSView *)pointer;
    NSView *parent = host.contentView;
    if (!NSThread.isMainThread || !parent || !view)
        return 0;
    view.frame = NSMakeRect(0, parent.isFlipped ? 100 : 0, parent.bounds.size.width, parent.bounds.size.height - 100);
    view.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;
    [parent addSubview:view];
    [host makeFirstResponder:view];
    return view.superview == parent;
}

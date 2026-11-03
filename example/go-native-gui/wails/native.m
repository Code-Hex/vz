//go:build darwin

#include "native.h"
#import <AppKit/AppKit.h>
bool vzGUIWailsMount(void *pointer, void *child)
{
    NSWindow *window = (NSWindow *)pointer;
    NSView *view = (NSView *)child;
    if (![NSThread isMainThread] || !window || !view)
        return false;
    NSView *content = window.contentView;
    view.translatesAutoresizingMaskIntoConstraints = NO;
    [content addSubview:view];
    [NSLayoutConstraint activateConstraints:@[
        [view.leadingAnchor constraintEqualToAnchor:content.leadingAnchor],
        [view.trailingAnchor constraintEqualToAnchor:content.trailingAnchor],
        [view.bottomAnchor constraintEqualToAnchor:content.bottomAnchor],
        [view.topAnchor constraintEqualToAnchor:content.topAnchor
                                       constant:110]
    ]];
    [content layoutSubtreeIfNeeded];
    [window makeFirstResponder:view];
    return view.superview == content;
}

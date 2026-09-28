#import "virtualization_view.h"
#import <QuartzCore/QuartzCore.h>
#import <math.h>

@interface AppDelegate (GraphicsTest)
- (void)setupGraphicWindow;
- (void)handleMouseMovement:(NSEvent *)event;
- (void)stopScrollTimer;
@end

@interface NSWindow (GraphicsAspectTest)
- (void)setDisplayAspectRatio:(NSSize)ratio;
@end

static int failures;

static void check(BOOL passed, NSString *message)
{
    printf("%s %s\n", passed ? "PASS" : "FAIL", message.UTF8String);
    if (!passed) {
        failures++;
    }
}

static void settle(NSWindow *window)
{
    NSDate *deadline = [NSDate dateWithTimeIntervalSinceNow:0.5];
    do {
        [window.contentView layoutSubtreeIfNeeded];
        [NSApp updateWindows];
        [[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:0.01]];
    } while ([deadline timeIntervalSinceNow] > 0);
}

static NSScrollView *findScrollView(NSView *view)
{
    if ([view isKindOfClass:NSScrollView.class]) {
        return (NSScrollView *)view;
    }
    for (NSView *child in view.subviews) {
        NSScrollView *scroll = findScrollView(child);
        if (scroll) {
            return scroll;
        }
    }
    return nil;
}

static NSButton *findButton(NSView *view, SEL action)
{
    if ([view isKindOfClass:NSButton.class] && ((NSButton *)view).action == action) {
        return (NSButton *)view;
    }
    for (NSView *child in view.subviews) {
        NSButton *button = findButton(child, action);
        if (button) {
            return button;
        }
    }
    return nil;
}

static void checkEntireDisplay(NSScrollView *scroll, NSString *label)
{
    NSRect document = scroll.documentView.bounds;
    NSRect visible = scroll.documentVisibleRect;
    BOOL full = document.size.width > 0 && document.size.height > 0
        && NSContainsRect(NSInsetRect(visible, -0.5, -0.5), document);
    check(full && fabs(scroll.magnification - 1.0) < 0.001,
        [NSString stringWithFormat:@"%@ entire display at 1x, document %@, visible %@, magnification %.3f",
            label, NSStringFromRect(document), NSStringFromRect(visible), scroll.magnification]);
}

static void checkCorners(NSWindow *window, NSScrollView *scroll, NSString *label)
{
    BOOL rectangular = scroll.bounds.size.width > 0 && scroll.bounds.size.height > 0;
    for (NSView *view = scroll; view; view = view.superview) {
        CALayer *layer = view.layer;
        if (!layer) {
            continue;
        }
        NSRect display = [view convertRectToLayer:[scroll convertRect:scroll.bounds toView:view]];
        if (layer.mask) {
            rectangular = NO;
        }
        if (!layer.masksToBounds) {
            continue;
        }
        rectangular &= NSContainsRect(NSInsetRect(layer.bounds, -0.5, -0.5), display);
        CGFloat radius = layer.cornerRadius;
        if (radius <= 0) {
            continue;
        }
        struct {
            CACornerMask corner;
            NSRect area;
        } corners[] = {
            { kCALayerMinXMinYCorner, NSMakeRect(NSMinX(layer.bounds), NSMinY(layer.bounds), radius, radius) },
            { kCALayerMaxXMinYCorner, NSMakeRect(NSMaxX(layer.bounds) - radius, NSMinY(layer.bounds), radius, radius) },
            { kCALayerMinXMaxYCorner, NSMakeRect(NSMinX(layer.bounds), NSMaxY(layer.bounds) - radius, radius, radius) },
            { kCALayerMaxXMaxYCorner, NSMakeRect(NSMaxX(layer.bounds) - radius, NSMaxY(layer.bounds) - radius, radius, radius) },
        };
        for (NSUInteger i = 0; i < sizeof(corners) / sizeof(corners[0]); i++) {
            if ((layer.maskedCorners & corners[i].corner) && NSIntersectsRect(display, corners[i].area)) {
                rectangular = NO;
            }
        }
    }
    check(rectangular, [label stringByAppendingString:@" display rectangle stays outside ancestor corner masks"]);
#if defined(__MAC_26_0) && __MAC_OS_X_VERSION_MAX_ALLOWED >= __MAC_26_0
    if (@available(macOS 26.0, *)) {
        NSViewLayoutRegion *region = [NSViewLayoutRegion safeAreaLayoutRegionWithCornerAdaptation:NSViewLayoutRegionAdaptivityAxisVertical];
        NSRect safe = [window.contentView rectForLayoutRegion:region];
        NSRect display = [scroll convertRect:scroll.bounds toView:window.contentView];
        check(safe.size.width > 0 && safe.size.height > 0 && display.size.width > 0 && display.size.height > 0
                && NSContainsRect(NSInsetRect(safe, -0.5, -0.5), display),
            [NSString stringWithFormat:@"%@ display avoids corners, display %@, safe %@", label,
                NSStringFromRect(display), NSStringFromRect(safe)]);
    }
#endif
}

static void checkDisplayLayout(NSWindow *window, NSScrollView *scroll, NSString *label)
{
    NSRect display = [scroll convertRect:scroll.bounds toView:nil];
    NSRect content = window.contentLayoutRect;
    CGFloat headerHeight = NSMaxY(content) - NSMaxY(display);
    BOOL fitsHeader = fabs(headerHeight) < 0.5;
    if (@available(macOS 26.0, *)) {
        fitsHeader = headerHeight >= 0 && headerHeight <= 40.5;
    }
    check(NSWidth(display) > 0 && NSHeight(display) > 0
            && fabs(NSMinX(display) - NSMinX(content)) < 0.5
            && fabs(NSMinY(display) - NSMinY(content)) < 0.5
            && fabs(NSMaxX(display) - NSMaxX(content)) < 0.5 && fitsHeader,
        [NSString stringWithFormat:@"%@ display reaches bottom and side edges below a single compact header, display %@, content %@", label,
            NSStringFromRect(display), NSStringFromRect(content)]);
    checkCorners(window, scroll, label);
}

static NSEvent *mouseMoved(NSWindow *window, NSPoint point)
{
    return [NSEvent mouseEventWithType:NSEventTypeMouseMoved
                              location:point
                         modifierFlags:0
                             timestamp:NSProcessInfo.processInfo.systemUptime
                          windowNumber:window.windowNumber
                               context:nil
                           eventNumber:0
                            clickCount:0
                              pressure:0];
}

static void checkEdgePanning(AppDelegate *delegate, NSWindow *window, NSScrollView *scroll, NSString *label) API_AVAILABLE(macos(12.0));
static void checkEdgePanning(AppDelegate *delegate, NSWindow *window, NSScrollView *scroll, NSString *label)
{
    [scroll setMagnification:2 centeredAtPoint:NSMakePoint(480, 300)];
    settle(window);
    NSClipView *clip = scroll.contentView;
    NSRect viewport = [clip convertRect:clip.bounds toView:nil];
    NSRect content = [window.contentView convertRect:window.contentView.bounds toView:nil];
    struct {
        NSString *name;
        NSPoint mouse;
        NSPoint direction;
    } cases[] = {
        { @"bottom edge", NSMakePoint(NSMidX(viewport), NSMinY(viewport) + 5), NSMakePoint(0, -1) },
        { @"top edge", NSMakePoint(NSMidX(viewport), NSMaxY(viewport) - 5), NSMakePoint(0, 1) },
        { @"left edge", NSMakePoint(NSMinX(viewport) + 5, NSMidY(viewport)), NSMakePoint(-1, 0) },
        { @"right edge", NSMakePoint(NSMaxX(viewport) - 5, NSMidY(viewport)), NSMakePoint(1, 0) },
        { @"bottom padding", NSMakePoint(NSMidX(viewport), NSMinY(content) + 5), NSZeroPoint },
        { @"header", NSMakePoint(NSMinX(viewport) + 5, NSMaxY(viewport) + 10), NSZeroPoint },
    };
    for (NSUInteger i = 0; i < sizeof(cases) / sizeof(cases[0]); i++) {
        if ([cases[i].name isEqualToString:@"bottom padding"] && NSPointInRect(cases[i].mouse, viewport)) {
            continue;
        }
        [delegate stopScrollTimer];
        [clip scrollToPoint:NSMakePoint(100, 100)];
        [scroll reflectScrolledClipView:clip];
        NSPoint before = clip.bounds.origin;
        BOOL stationary = NSEqualPoints(cases[i].direction, NSZeroPoint);
        if (stationary) {
            [delegate handleMouseMovement:mouseMoved(window, NSMakePoint(NSMaxX(viewport) - 5, NSMidY(viewport)))];
        }
        [delegate handleMouseMovement:mouseMoved(window, cases[i].mouse)];
        settle(window);
        [delegate stopScrollTimer];
        NSPoint after = clip.bounds.origin;
        CGFloat dx = after.x - before.x;
        CGFloat dy = after.y - before.y;
        BOOL xPassed = cases[i].direction.x == 0 ? fabs(dx) < 0.5 : dx * cases[i].direction.x > 1;
        BOOL yPassed = cases[i].direction.y == 0 ? fabs(dy) < 0.5 : dy * cases[i].direction.y > 1;
        check(xPassed && yPassed,
            [NSString stringWithFormat:@"%@ %@ %@, before %@, after %@", label, cases[i].name,
                stationary ? @"stops edge panning" : @"pans the display", NSStringFromPoint(before), NSStringFromPoint(after)]);
    }
    [delegate stopScrollTimer];
}

@interface CloseDecisionDelegate : NSObject <NSWindowDelegate>
@property BOOL allowsClose;
@property NSUInteger requests;
@property BOOL allowsZoom;
@property NSUInteger zoomRequests;
@end

@implementation CloseDecisionDelegate
- (BOOL)windowShouldClose:(NSWindow *)sender
{
    self.requests++;
    return self.allowsClose;
}
- (BOOL)windowShouldZoom:(NSWindow *)sender toFrame:(NSRect)newFrame
{
    self.zoomRequests++;
    return self.allowsZoom;
}
@end

static void checkDisplayResizing(NSWindow *window)
{
    if (@available(macOS 26.0, *)) {
        NSScrollView *scroll = findScrollView(window.contentView);
        for (NSValue *value in @[ [NSValue valueWithSize:NSMakeSize(1920, 1080)], [NSValue valueWithSize:NSMakeSize(600, 960)] ]) {
            NSSize ratio = value.sizeValue;
            [window setDisplayAspectRatio:ratio];
            for (NSValue *requested in @[ [NSValue valueWithSize:NSMakeSize(640, 420)], [NSValue valueWithSize:NSMakeSize(360, 600)], [NSValue valueWithSize:NSMakeSize(100, 80)] ]) {
                NSRect frame = window.frame;
                frame.size = [window.delegate windowWillResize:window toSize:requested.sizeValue];
                [window setFrame:frame display:YES];
                settle(window);
                NSSize display = scroll.documentView.bounds.size;
                check(display.width > 0 && display.height > 0
                        && fabs(display.height - display.width * ratio.height / ratio.width) < 1
                        && window.frame.size.width >= 319.5 && window.frame.size.height >= 279.5,
                    [NSString stringWithFormat:@"display %@ resize request %@ preserves ratio and minimum size, display %@, window %@",
                        NSStringFromSize(ratio), NSStringFromSize(requested.sizeValue), NSStringFromSize(display), NSStringFromRect(window.frame)]);
            }
            NSRect original = window.frame;
            [window performZoom:nil];
            settle(window);
            NSSize zoomed = scroll.documentView.bounds.size;
            check(!NSEqualRects(original, window.frame) && zoomed.width > 0 && zoomed.height > 0
                    && fabs(zoomed.height - zoomed.width * ratio.height / ratio.width) < 1,
                [NSString stringWithFormat:@"display %@ window zoom preserves guest ratio, display %@", NSStringFromSize(ratio), NSStringFromSize(zoomed)]);
            [window performZoom:nil];
            settle(window);
            check(NSEqualRects(original, window.frame),
                [NSString stringWithFormat:@"display %@ window zoom restores frame %@", NSStringFromSize(ratio), NSStringFromRect(original)]);
            checkDisplayLayout(window, scroll, NSStringFromSize(ratio));
        }
        [window setDisplayAspectRatio:NSZeroSize];
    }
}

static void checkWindowActions(void) API_AVAILABLE(macos(12.0));
static void checkWindowActions(void)
{
    NSString *title = @"VZ graphics window actions";
    dispatch_queue_t queue = dispatch_queue_create("vz.window-actions-test", DISPATCH_QUEUE_SERIAL);
    AppDelegate *controller = [[AppDelegate alloc] initWithVirtualMachine:nil
                                                                    queue:queue
                                                              windowWidth:640
                                                             windowHeight:480
                                                              windowTitle:title
                                                         enableController:NO];
    [controller setupGraphicWindow];
    NSWindow *window = nil;
    for (NSWindow *candidate in NSApp.windows) {
        if ([candidate.title isEqualToString:title]) {
            window = candidate;
            break;
        }
    }
    check(window != nil && window.visible, @"window actions use a visible application window");
    if (!window) {
        [controller release];
        dispatch_release(queue);
        return;
    }
    checkDisplayResizing(window);
    CloseDecisionDelegate *decision = [[CloseDecisionDelegate alloc] init];
    window.delegate = decision;
    [window makeKeyAndOrderFront:nil];
    settle(window);
    NSRect initialFrame = window.frame;
    [window performZoom:nil];
    settle(window);
    check(NSEqualRects(window.frame, initialFrame) && decision.zoomRequests == 1,
        @"window zoom action respects delegate rejection");
    decision.allowsZoom = YES;
    [window performZoom:nil];
    settle(window);
    check(!NSEqualRects(window.frame, initialFrame) && decision.zoomRequests == 2,
        @"window zoom action changes the window frame");
    [window performZoom:nil];
    settle(window);
    check(NSEqualRects(window.frame, initialFrame) && decision.zoomRequests == 3,
        @"window zoom action restores the original frame");
    [window performMiniaturize:nil];
    settle(window);
    check(window.miniaturized, @"window minimize action reaches the Dock");
    [window deminiaturize:nil];
    settle(window);
    check(!window.miniaturized && window.visible, @"window restores from the Dock");
    [window performClose:nil];
    settle(window);
    check(window.visible && decision.requests == 1, @"window close action respects delegate rejection");
    decision.allowsClose = YES;
    [window performClose:nil];
    settle(window);
    check(!window.visible && decision.requests == 2, @"window close action closes after delegate approval");
    window.delegate = nil;
    [window orderOut:nil];
    [decision release];
    [controller release];
    dispatch_release(queue);
}

static void exerciseWindow(NSScrollerStyle style) API_AVAILABLE(macos(12.0));
static void exerciseWindow(NSScrollerStyle style)
{
    NSString *label = style == NSScrollerStyleLegacy ? @"Legacy" : @"Overlay";
    NSString *title = [@"VZ graphics regression " stringByAppendingString:label];
    dispatch_queue_t queue = dispatch_queue_create("vz.graphics-test", DISPATCH_QUEUE_SERIAL);
    AppDelegate *delegate = [[AppDelegate alloc] initWithVirtualMachine:nil
                                                                  queue:queue
                                                            windowWidth:960
                                                           windowHeight:600
                                                            windowTitle:title
                                                       enableController:NO];
    [delegate setupGraphicWindow];
    NSWindow *window = nil;
    for (NSWindow *candidate in NSApp.windows) {
        if ([candidate.title isEqualToString:title]) {
            window = candidate;
            break;
        }
    }
    NSScrollView *scroll = findScrollView(window.contentView);
    NSButton *zoomButton = findButton(window.contentView, @selector(toggleZoomMode:));
    for (NSToolbarItem *item in window.toolbar.items) {
        if (!zoomButton) {
            zoomButton = findButton(item.view, @selector(toggleZoomMode:));
        }
    }
    check(window && scroll && zoomButton && [scroll.documentView isKindOfClass:VZVirtualMachineView.class],
        [NSString stringWithFormat:@"%@ real VM view and zoom control are available, window %@, scroll %@, zoom %@", label, window, scroll, zoomButton]);
    if (!window || !scroll || !zoomButton) {
        [delegate release];
        dispatch_release(queue);
        return;
    }
    scroll.scrollerStyle = style;
    settle(window);
    checkDisplayLayout(window, scroll, label);
    checkEntireDisplay(scroll, [label stringByAppendingString:@" initial"]);
    [zoomButton performClick:nil];
    settle(window);
    checkEntireDisplay(scroll, [label stringByAppendingString:@" zoom enabled"]);
    NSSize originalSize = scroll.documentView.bounds.size;
    for (NSNumber *scale in @[ @2, @4 ]) {
        [scroll setMagnification:scale.doubleValue centeredAtPoint:NSMakePoint(originalSize.width / 2, originalSize.height / 2)];
        settle(window);
        NSRect document = scroll.documentView.bounds;
        NSSize visibleSize = scroll.documentVisibleRect.size;
        check(fabs(visibleSize.width * scale.doubleValue - document.size.width) < 0.5
                && fabs(visibleSize.height * scale.doubleValue - document.size.height) < 0.5,
            [NSString stringWithFormat:@"%@ %@x shows the corresponding fraction of the display, document %@, visible %@", label, scale,
                NSStringFromRect(document), NSStringFromSize(visibleSize)]);
        [scroll.contentView scrollToPoint:NSZeroPoint];
        [scroll reflectScrolledClipView:scroll.contentView];
        settle(window);
        NSRect before = scroll.documentVisibleRect;
        [scroll.contentView scrollToPoint:NSMakePoint(NSMaxX(document) - visibleSize.width, NSMaxY(document) - visibleSize.height)];
        [scroll reflectScrolledClipView:scroll.contentView];
        settle(window);
        NSRect after = scroll.documentVisibleRect;
        check(fabs(scroll.magnification - scale.doubleValue) < 0.001
                && fabs(NSMinX(before) - NSMinX(document)) < 0.5
                && fabs(NSMinY(before) - NSMinY(document)) < 0.5
                && fabs(NSMaxX(after) - NSMaxX(document)) < 0.5
                && fabs(NSMaxY(after) - NSMaxY(document)) < 0.5
                && after.origin.x > before.origin.x + 1 && after.origin.y > before.origin.y + 1,
            [NSString stringWithFormat:@"%@ %@x pans to the opposite display edges, before %@, after %@", label, scale,
                NSStringFromRect(before), NSStringFromRect(after)]);
    }
    checkEdgePanning(delegate, window, scroll, label);
    [scroll setMagnification:1 centeredAtPoint:NSZeroPoint];
    settle(window);
    checkEntireDisplay(scroll, [label stringByAppendingString:@" returned from zoom"]);
    [scroll setMagnification:2 centeredAtPoint:NSMakePoint(480, 300)];
    [scroll.contentView scrollToPoint:NSMakePoint(100, 80)];
    [zoomButton performClick:nil];
    settle(window);
    checkEntireDisplay(scroll, [label stringByAppendingString:@" zoom disabled"]);
    for (NSValue *value in @[ [NSValue valueWithSize:NSMakeSize(720, 480)], [NSValue valueWithSize:NSMakeSize(1100, 700)] ]) {
        [window setContentSize:value.sizeValue];
        settle(window);
        NSString *resized = [NSString stringWithFormat:@"%@ resized %@", label, NSStringFromSize(value.sizeValue)];
        checkDisplayLayout(window, scroll, resized);
        checkEntireDisplay(scroll, resized);
    }
    [window orderOut:nil];
    if (style == NSScrollerStyleLegacy) {
        checkWindowActions();
    }
    [delegate release];
    dispatch_release(queue);
}

int main(void)
{
    @autoreleasepool {
        if (@available(macOS 12.0, *)) {
            [NSApplication sharedApplication];
            [NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
            exerciseWindow(NSScrollerStyleLegacy);
            exerciseWindow(NSScrollerStyleOverlay);
        } else {
            printf("SKIP graphics tests require macOS 12 or later\n");
        }
    }
    printf("%s graphics regression tests, %d failures\n", failures ? "FAIL" : "PASS", failures);
    return failures ? 1 : 0;
}

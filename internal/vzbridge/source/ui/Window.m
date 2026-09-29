#import "virtualization_view.h"

void VZBridgeStartWindow(void *machine, void *queue, double width, double height, const char *title, bool enableController)
{
    [VZApplication sharedApplication];
    if (@available(macOS 12, *)) {
        @autoreleasepool {
            NSString *windowTitle = [NSString stringWithUTF8String:title];
            AppDelegate *appDelegate = [[[AppDelegate alloc]
                initWithVirtualMachine:(VZVirtualMachine *)machine
                                 queue:(dispatch_queue_t)queue
                           windowWidth:(CGFloat)width
                          windowHeight:(CGFloat)height
                           windowTitle:windowTitle
                      enableController:enableController] autorelease];

            NSApp.delegate = appDelegate;
            [NSApp run];
            return;
        }
    }
    [NSException raise:NSInternalInconsistencyException format:@"Window UI requires macOS 12"];
}

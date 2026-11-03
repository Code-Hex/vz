#import "virtual_machine_view.h"
#import "native.h"
#import <objc/runtime.h>

#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 120000
static char viewMachineKey;
@interface VZViewMachineLifetime : NSObject {
@public
    void *machine;
}
@end
@implementation VZViewMachineLifetime
- (void)dealloc
{
    vz_releaseObject(machine);
    [super dealloc];
}
@end
#endif

bool vz_viewIsMainThread(void) { return [NSThread isMainThread]; }
void *vz_viewNew(void *machine)
{
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 120000
    if (@available(macOS 12, *)) {
        VZVirtualMachineView *view = [[VZVirtualMachineView alloc] initWithFrame:NSZeroRect];
        VZViewMachineLifetime *lifetime = [[VZViewMachineLifetime alloc] init];
        lifetime->machine = [(id)machine retain];
        objc_setAssociatedObject(view, &viewMachineKey, lifetime, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
        [lifetime release];
        view.virtualMachine = (VZVirtualMachine *)machine;
        return view;
    }
#endif
    return NULL;
}
void vz_viewRelease(void *pointer)
{
    void (^releaseView)(void) = ^{
        @autoreleasepool {
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 120000
            if (@available(macOS 12, *)) {
                VZVirtualMachineView *view = (VZVirtualMachineView *)pointer;
                [view removeFromSuperview];
                [view release];
            }
#endif
        }
    };
    if ([NSThread isMainThread])
        releaseView();
    else
        dispatch_async(dispatch_get_main_queue(), releaseView);
}
bool vz_viewCapturesKeys(void *view)
{
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 120000
    if (@available(macOS 12, *))
        return [(VZVirtualMachineView *)view capturesSystemKeys];
#endif
    return false;
}
void vz_viewSetCapturesKeys(void *view, bool value)
{
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 120000
    if (@available(macOS 12, *))
        [(VZVirtualMachineView *)view setCapturesSystemKeys:value];
#endif
}
bool vz_viewAutomaticallyReconfigures(void *view)
{
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 140000
    if (@available(macOS 14, *))
        return [(VZVirtualMachineView *)view automaticallyReconfiguresDisplay];
#endif
    return false;
}
void vz_viewSetAutomaticallyReconfigures(void *view, bool value)
{
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 140000
    if (@available(macOS 14, *))
        [(VZVirtualMachineView *)view setAutomaticallyReconfiguresDisplay:value];
#endif
}

void *vz_viewBorrowPoolNew(void) { return [[NSAutoreleasePool alloc] init]; }
void vz_viewBorrowPoolDrain(void *pool) { [(NSAutoreleasePool *)pool drain]; }

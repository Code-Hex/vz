#import "../native.m"
#include <assert.h>

@interface CallbackPayload : NSObject {
    BOOL *_destroyed;
}
- (instancetype)initWithDestroyed:(BOOL *)destroyed;
@end

@implementation CallbackPayload
- (instancetype)initWithDestroyed:(BOOL *)destroyed
{
    self = [super init];
    if (self)
        _destroyed = destroyed;
    return self;
}

- (void)dealloc
{
    *_destroyed = YES;
    [super dealloc];
}
@end

int main(void)
{
    @autoreleasepool {
        vz_setCallback(0);
        const struct {
            uint32_t kind;
            BOOL ownsFirst;
            BOOL ownsSecond;
        } cases[] = {
            { VZBridgeCallbackMachineStateChanged, NO, NO },
            { VZBridgeCallbackCompletion, YES, NO },
            { VZBridgeCallbackNetworkDisconnected, YES, NO },
            { VZBridgeCallbackContextReleased, NO, NO },
            { VZBridgeCallbackSocketConnected, YES, YES },
            { VZBridgeCallbackSocketAccepted, YES, NO },
            { VZBridgeCallbackNetworkBlockDeviceError, YES, NO },
            { VZBridgeCallbackNetworkBlockDeviceConnected, NO, NO },
            { VZBridgeCallbackRosettaInstallCompleted, YES, NO },
            { VZBridgeCallbackRestoreImageLoaded, YES, YES },
            { VZBridgeCallbackMacOSInstallCompleted, YES, NO },
            { VZBridgeCallbackMacOSInstallProgress, NO, NO },
        };
        for (size_t i = 0; i < sizeof(cases) / sizeof(cases[0]); i++) {
            BOOL firstDestroyed = NO;
            BOOL secondDestroyed = NO;
            CallbackPayload *first = [[CallbackPayload alloc] initWithDestroyed:&firstDestroyed];
            CallbackPayload *second = [[CallbackPayload alloc] initWithDestroyed:&secondDestroyed];
            assert(emit(cases[i].kind, 0, first, second, 0) == 0);
            assert(firstDestroyed == cases[i].ownsFirst);
            assert(secondDestroyed == cases[i].ownsSecond);
            if (!cases[i].ownsFirst)
                [first release];
            if (!cases[i].ownsSecond)
                [second release];
            assert(firstDestroyed && secondDestroyed);
            assert(emit(cases[i].kind, 0, nil, nil, 0) == 0);
        }
    }
    return 0;
}

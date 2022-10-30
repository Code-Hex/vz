#pragma once

#import <Availability.h>
#import <Foundation/Foundation.h>

#define RAISE_UNSUPPORTED_MACOS_EXCEPTION()                                                       \
    do {                                                                                          \
        [[NSException exceptionWithName:@"UnhandledException" reason:@"bug" userInfo:nil] raise]; \
        __builtin_unreachable();                                                                  \
    } while (0)

#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 130000
#define INCLUDE_TARGET_OSX_13 1
#else
#pragma message("macOS 13 API has been disabled")
#endif

typedef struct nbyteslice {
    void *ptr;
    int len;
} nbyteslice;

/* exported from cgo */
void virtualMachineCompletionHandler(void *cgoHandler, void *errPtr);

typedef void (^vm_completion_handler_t)(NSError *);

static inline vm_completion_handler_t makeVMCompletionHandler(void *completionHandler)
{
    return Block_copy(^(NSError *err) {
        virtualMachineCompletionHandler(completionHandler, err);
    });
}

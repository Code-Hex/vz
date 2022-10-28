#pragma once

#import <Availability.h>

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

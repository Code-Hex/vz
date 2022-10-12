#pragma once

#define RAISE_UNSUPPORTED_MACOS_EXCEPTION()                                                       \
    do {                                                                                          \
        [[NSException exceptionWithName:@"UnhandledException" reason:@"bug" userInfo:nil] raise]; \
        __builtin_unreachable();                                                                  \
    } while (0)
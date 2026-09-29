#pragma once
#import <Availability.h>
#import <Foundation/Foundation.h>
// for macOS 14 API
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 140000
#define INCLUDE_TARGET_OSX_14 1
#else
#pragma message("macOS 14 API has been disabled")
#endif

//
//  virtualization_15.h

#pragma once

// FIXME(codehex): this is dirty hack to avoid clang-format error like below
// "Configuration file(s) do(es) not support C++: /github.com/Code-Hex/vz/.clang-format"
#define NSURLComponents NSURLComponents

#import "virtualization_helper.h"
#import <Virtualization/Virtualization.h>

/* macOS 15 API */
bool isNestedVirtualizationSupported();
void setNestedVirtualizationEnabled(void *config, bool nestedVirtualizationEnabled);
void *newVZXHCIControllerConfiguration();
void setUSBControllersVZVirtualMachineConfiguration(void *config, void *usbControllers);
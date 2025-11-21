//
//  virtualization_26.h
//
//  Created by codehex.
//

#pragma once

// FIXME(codehex): this is dirty hack to avoid clang-format error like below
// "Configuration file(s) do(es) not support C++: /github.com/Code-Hex/vz/.clang-format"
#define NSURLComponents NSURLComponents

#import "internal/osversion/virtualization_helper.h"
#import <Virtualization/Virtualization.h>
#import <vmnet/vmnet.h>

/* macOS 26 API */
// VZVmnetNetworkDeviceAttachment
void *newVZVmnetNetworkDeviceAttachment(void *network);
void *VZVmnetNetworkDeviceAttachment_network(void *attachment);

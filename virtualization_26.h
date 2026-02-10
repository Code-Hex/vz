//
//  virtualization_26.h
//
//  Created by codehex.
//

#pragma once

#import "internal/osversion/virtualization_helper.h"
#import <Virtualization/Virtualization.h>
#import <vmnet/vmnet.h>

/* macOS 26 API */
// VZVmnetNetworkDeviceAttachment
void *newVZVmnetNetworkDeviceAttachment(void *network);
void *VZVmnetNetworkDeviceAttachment_network(void *attachment);

//
//  virtualization_26.h
//
//  Created by codehex.
//

#pragma once

#import "virtualization_helper.h"
#import <vmnet/vmnet.h>
#import <Virtualization/Virtualization.h>

// newVZVmnetNetworkDeviceAttachment wraps a vmnet_network_ref
// (created via the vmnet subpackage, passed in as void *) in a
// VZVmnetNetworkDeviceAttachment. The attachment does not retain
// the network; the caller's Network wrapper owns the +1.
void *newVZVmnetNetworkDeviceAttachment(void *network);

// VZVmnetNetworkDeviceAttachment_network reads the network back from
// an attachment. CFRetain'd before return so the Go-side Network
// wrapper can own a refcount independent of the attachment's
// storage.
void *VZVmnetNetworkDeviceAttachment_network(void *attachment);

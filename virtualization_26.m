//
//  virtualization_26.m
//
//  Created by codehex.
//

#import "virtualization_26.h"

#ifdef INCLUDE_TARGET_OSX_26
#import <CoreFoundation/CoreFoundation.h>
#import <vmnet/vmnet.h>
#endif // INCLUDE_TARGET_OSX_26

void *newVZVmnetNetworkDeviceAttachment(void *network)
{
#ifdef INCLUDE_TARGET_OSX_26
    if (@available(macOS 26, *)) {
        return [[VZVmnetNetworkDeviceAttachment alloc]
            initWithNetwork:(vmnet_network_ref)network];
    }
#endif
    (void)network;
    RAISE_UNSUPPORTED_MACOS_EXCEPTION();
    return NULL;
}

void *VZVmnetNetworkDeviceAttachment_network(void *attachment)
{
#ifdef INCLUDE_TARGET_OSX_26
    if (@available(macOS 26, *)) {
        vmnet_network_ref network = ((VZVmnetNetworkDeviceAttachment *)attachment).network;
        if (network) CFRetain(network);
        return network;
    }
#endif
    (void)attachment;
    return NULL;
}

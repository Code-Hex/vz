//
//  virtualization_26.m
//
//  Created by codehex.
//

#import "virtualization_26.h"

// VZVmnetNetworkDeviceAttachment
// see: https://developer.apple.com/documentation/virtualization/vzvmnetnetworkdeviceattachment/init(network:)?language=objc
void *newVZVmnetNetworkDeviceAttachment(void *network)
{
#ifdef INCLUDE_TARGET_OSX_26
    if (@available(macOS 26, *)) {
        return [[VZVmnetNetworkDeviceAttachment alloc] initWithNetwork:(vmnet_network_ref)network];
    }
#endif
    RAISE_UNSUPPORTED_MACOS_EXCEPTION();
}

// see: https://developer.apple.com/documentation/virtualization/vzvmnetnetworkdeviceattachment/network?language=objc
void *VZVmnetNetworkDeviceAttachment_network(void *attachment)
{
#ifdef INCLUDE_TARGET_OSX_26
    if (@available(macOS 26, *)) {
        return [(VZVmnetNetworkDeviceAttachment *)attachment network];
    }
#endif
    RAISE_UNSUPPORTED_MACOS_EXCEPTION();
}

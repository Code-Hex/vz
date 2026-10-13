#import "native.h"
#import <Foundation/Foundation.h>
#import <Virtualization/Virtualization.h>
#import <arpa/inet.h>
#import <string.h>
#import <vmnet/vmnet.h>

#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 260000
static void vmnetError(vmnet_return_t status, NSString *operation, void **errorOut)
{
    if (status != VMNET_SUCCESS && errorOut) {
        *errorOut = [[NSError errorWithDomain:@"com.apple.vmnet"
                                         code:status
                                     userInfo:@ { NSLocalizedDescriptionKey : [operation stringByAppendingString:@" failed"] }] retain];
    }
}
#endif

void *vz_newVmnetNetworkConfiguration(int32_t mode, void **errorOut)
{
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 260000
    if (@available(macOS 26, *)) {
        vmnet_return_t status;
        void *configuration = vmnet_network_configuration_create((vmnet_mode_t)mode, &status);
        vmnetError(status, @"vmnet_network_configuration_create", errorOut);
        return configuration;
    }
#endif
    return NULL;
}

void *vz_newVmnetNetwork(void *configuration, void **errorOut)
{
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 260000
    if (@available(macOS 26, *)) {
        vmnet_return_t status;
        void *network = vmnet_network_create(configuration, &status);
        vmnetError(status, @"vmnet_network_create", errorOut);
        return network;
    }
#endif
    return NULL;
}

void vz_setVmnetIPv4Subnet(void *configuration, uint32_t address, uint32_t mask, void **errorOut)
{
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 260000
    if (@available(macOS 26, *)) {
        struct in_addr subnet = { .s_addr = htonl(address) }, netmask = { .s_addr = htonl(mask) };
        vmnetError(vmnet_network_configuration_set_ipv4_subnet(configuration, &subnet, &netmask), @"vmnet_network_configuration_set_ipv4_subnet", errorOut);
    }
#endif
}

void vz_setVmnetIPv6Prefix(void *configuration, const void *address, uint8_t prefixLength, void **errorOut)
{
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 260000
    if (@available(macOS 26, *)) {
        struct in6_addr prefix;
        memcpy(&prefix, address, sizeof(prefix));
        vmnetError(vmnet_network_configuration_set_ipv6_prefix(configuration, &prefix, prefixLength), @"vmnet_network_configuration_set_ipv6_prefix", errorOut);
    }
#endif
}

void vz_addVmnetDHCPReservation(void *configuration, const void *mac, uint32_t address, void **errorOut)
{
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 260000
    if (@available(macOS 26, *)) {
        struct in_addr reservation = { .s_addr = htonl(address) };
        vmnetError(vmnet_network_configuration_add_dhcp_reservation(configuration, mac, &reservation), @"vmnet_network_configuration_add_dhcp_reservation", errorOut);
    }
#endif
}

uint64_t vz_vmnetIPv4Subnet(void *network)
{
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 260000
    if (@available(macOS 26, *)) {
        struct in_addr subnet, mask;
        vmnet_network_get_ipv4_subnet(network, &subnet, &mask);
        return ((uint64_t)ntohl(subnet.s_addr) << 32) | ntohl(mask.s_addr);
    }
#endif
    return 0;
}

uint8_t vz_vmnetIPv6Prefix(void *network, void *address)
{
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 260000
    if (@available(macOS 26, *)) {
        struct in6_addr prefix = { 0 };
        uint8_t prefixLength = UINT8_MAX;
        vmnet_network_get_ipv6_prefix(network, &prefix, &prefixLength);
        memcpy(address, &prefix, sizeof(prefix));
        return prefixLength;
    }
#endif
    return UINT8_MAX;
}

void *vz_newVmnetNetworkDeviceAttachment(void *network)
{
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 260000
    if (@available(macOS 26, *)) {
        return [[VZVmnetNetworkDeviceAttachment alloc] initWithNetwork:network];
    }
#endif
    return NULL;
}

void *vz_vmnetNetworkDeviceAttachmentNetwork(void *attachment)
{
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 260000
    if (@available(macOS 26, *)) {
        vmnet_network_ref network = [(VZVmnetNetworkDeviceAttachment *)attachment network];
        return network ? (void *)CFRetain(network) : NULL;
    }
#endif
    return NULL;
}

void vz_releaseCFObject(void *object)
{
    if (object)
        CFRelease(object);
}

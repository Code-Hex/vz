#import "vmnet_darwin.h"

// MARK: - CFRelease Wrapper

void vmnetRelease(void *obj)
{
    if (obj != NULL) {
        CFRelease((CFTypeRef)obj);
    }
}

// MARK: - vmnet_network_configuration_t (macOS 26+)

// see: https://developer.apple.com/documentation/vmnet/vmnet_network_configuration_add_dhcp_reservation(_:_:_:)?language=objc
uint32_t VmnetNetworkConfiguration_addDhcpReservation(void *config, ether_addr_t const *client, struct in_addr const *reservation)
{
#ifdef INCLUDE_TARGET_OSX_26
    if (@available(macOS 26, *)) {
        return vmnet_network_configuration_add_dhcp_reservation((vmnet_network_configuration_ref)config, client, reservation);
    }
#endif
    RAISE_UNSUPPORTED_MACOS_EXCEPTION();
}

// see: https://developer.apple.com/documentation/vmnet/vmnet_network_configuration_add_port_forwarding_rule(_:_:_:_:_:_:)?language=objc
uint32_t VmnetNetworkConfiguration_addPortForwardingRule(void *config, uint8_t protocol, sa_family_t address_family, uint16_t internal_port, uint16_t external_port, void const *internal_address)
{
#ifdef INCLUDE_TARGET_OSX_26
    if (@available(macOS 26, *)) {
        return vmnet_network_configuration_add_port_forwarding_rule((vmnet_network_configuration_ref)config, protocol, address_family, internal_port, external_port, internal_address);
    }
#endif
    RAISE_UNSUPPORTED_MACOS_EXCEPTION();
}

// see: https://developer.apple.com/documentation/vmnet/vmnet_network_configuration_create(_:_:)?language=objc
void *VmnetNetworkConfigurationCreate(uint32_t mode, uint32_t *status)
{
#ifdef INCLUDE_TARGET_OSX_26
    if (@available(macOS 26, *)) {
        return vmnet_network_configuration_create(mode, status);
    }
#endif
    RAISE_UNSUPPORTED_MACOS_EXCEPTION();
}

// see: https://developer.apple.com/documentation/vmnet/vmnet_network_configuration_disable_dhcp(_:)?language=objc
void VmnetNetworkConfiguration_disableDhcp(void *config)
{
#ifdef INCLUDE_TARGET_OSX_26
    if (@available(macOS 26, *)) {
        vmnet_network_configuration_disable_dhcp((vmnet_network_configuration_ref)config);
        return;
    }
#endif
    RAISE_UNSUPPORTED_MACOS_EXCEPTION();
}

// see: https://developer.apple.com/documentation/vmnet/vmnet_network_configuration_disable_dns_proxy(_:)?language=objc
void VmnetNetworkConfiguration_disableDnsProxy(void *config)
{
#ifdef INCLUDE_TARGET_OSX_26
    if (@available(macOS 26, *)) {
        vmnet_network_configuration_disable_dns_proxy((vmnet_network_configuration_ref)config);
        return;
    }
#endif
    RAISE_UNSUPPORTED_MACOS_EXCEPTION();
}

// see: https://developer.apple.com/documentation/vmnet/vmnet_network_configuration_disable_nat44(_:)?language=objc
void VmnetNetworkConfiguration_disableNat44(void *config)
{
#ifdef INCLUDE_TARGET_OSX_26
    if (@available(macOS 26, *)) {
        vmnet_network_configuration_disable_nat44((vmnet_network_configuration_ref)config);
        return;
    }
#endif
    RAISE_UNSUPPORTED_MACOS_EXCEPTION();
}

// see: https://developer.apple.com/documentation/vmnet/vmnet_network_configuration_disable_nat66(_:)?language=objc
void VmnetNetworkConfiguration_disableNat66(void *config)
{
#ifdef INCLUDE_TARGET_OSX_26
    if (@available(macOS 26, *)) {
        vmnet_network_configuration_disable_nat66((vmnet_network_configuration_ref)config);
        return;
    }
#endif
    RAISE_UNSUPPORTED_MACOS_EXCEPTION();
}

// see: https://developer.apple.com/documentation/vmnet/vmnet_network_configuration_disable_router_advertisement(_:)?language=objc
void VmnetNetworkConfiguration_disableRouterAdvertisement(void *config)
{
#ifdef INCLUDE_TARGET_OSX_26
    if (@available(macOS 26, *)) {
        vmnet_network_configuration_disable_router_advertisement((vmnet_network_configuration_ref)config);
        return;
    }
#endif
    RAISE_UNSUPPORTED_MACOS_EXCEPTION();
}

// see: https://developer.apple.com/documentation/vmnet/vmnet_network_configuration_set_external_interface(_:_:)?language=objc
uint32_t VmnetNetworkConfiguration_setExternalInterface(void *config, const char *ifname)
{
#ifdef INCLUDE_TARGET_OSX_26
    if (@available(macOS 26, *)) {
        return vmnet_network_configuration_set_external_interface((vmnet_network_configuration_ref)config, ifname);
    }
#endif
    RAISE_UNSUPPORTED_MACOS_EXCEPTION();
}

// see: https://developer.apple.com/documentation/vmnet/vmnet_network_configuration_set_ipv4_subnet(_:_:_:)?language=objc
uint32_t VmnetNetworkConfiguration_setIPv4Subnet(void *config, struct in_addr const *subnet_addr, struct in_addr const *subnet_mask)
{
#ifdef INCLUDE_TARGET_OSX_26
    if (@available(macOS 26, *)) {
        return vmnet_network_configuration_set_ipv4_subnet((vmnet_network_configuration_ref)config, subnet_addr, subnet_mask);
    }
#endif
    RAISE_UNSUPPORTED_MACOS_EXCEPTION();
}

// see: https://developer.apple.com/documentation/vmnet/vmnet_network_configuration_set_ipv6_prefix(_:_:_:)?language=objc
uint32_t VmnetNetworkConfiguration_setIPv6Prefix(void *config, struct in6_addr const *prefix, uint8_t prefix_len)
{
#ifdef INCLUDE_TARGET_OSX_26
    if (@available(macOS 26, *)) {
        return vmnet_network_configuration_set_ipv6_prefix((vmnet_network_configuration_ref)config, prefix, prefix_len);
    }
#endif
    RAISE_UNSUPPORTED_MACOS_EXCEPTION();
}

// see: https://developer.apple.com/documentation/vmnet/vmnet_network_configuration_set_mtu(_:_:)?language=objc
uint32_t VmnetNetworkConfiguration_setMtu(void *config, uint32_t mtu)
{
#ifdef INCLUDE_TARGET_OSX_26
    if (@available(macOS 26, *)) {
        return vmnet_network_configuration_set_mtu((vmnet_network_configuration_ref)config, mtu);
    }
#endif
    RAISE_UNSUPPORTED_MACOS_EXCEPTION();
}

// MARK: - vmnet_network_ref (macOS 26+)

// https://developer.apple.com/documentation/vmnet/vmnet_network_copy_serialization(_:_:)?language=objc
void *VmnetNetwork_copySerialization(void *network, uint32_t *status)
{
#ifdef INCLUDE_TARGET_OSX_26
    if (@available(macOS 26, *)) {
        return vmnet_network_copy_serialization((vmnet_network_ref)network, status);
    }
#endif
    RAISE_UNSUPPORTED_MACOS_EXCEPTION();
}

// vmnet_network
// see: https://developer.apple.com/documentation/vmnet/vmnet_network_create(_:_:)?language=objc
void *VmnetNetworkCreate(void *config, uint32_t *status)
{
#ifdef INCLUDE_TARGET_OSX_26
    if (@available(macOS 26, *)) {
        return vmnet_network_create((vmnet_network_configuration_ref)config, status);
    }
#endif
    RAISE_UNSUPPORTED_MACOS_EXCEPTION();
}

// see: https://developer.apple.com/documentation/vmnet/vmnet_network_create_with_serialization(_:_:)?language=objc
void *VmnetNetworkCreateWithSerialization(void *serialization, uint32_t *status)
{
#ifdef INCLUDE_TARGET_OSX_26
    if (@available(macOS 26, *)) {
        return vmnet_network_create_with_serialization((xpc_object_t)serialization, status);
    }
#endif
    RAISE_UNSUPPORTED_MACOS_EXCEPTION();
}

// see: https://developer.apple.com/documentation/vmnet/vmnet_network_get_ipv4_subnet(_:_:_:)?language=objc
void VmnetNetwork_getIPv4Subnet(void *network, struct in_addr *subnet, struct in_addr *mask)
{
#ifdef INCLUDE_TARGET_OSX_26
    if (@available(macOS 26, *)) {
        vmnet_network_get_ipv4_subnet((vmnet_network_ref)network, subnet, mask);
        return;
    }
#endif
    RAISE_UNSUPPORTED_MACOS_EXCEPTION();
}

// see: https://developer.apple.com/documentation/vmnet/vmnet_network_get_ipv6_prefix(_:_:_:)?language=objc
void VmnetNetwork_getIPv6Prefix(void *network, struct in6_addr *prefix, uint8_t *prefix_len)
{
#ifdef INCLUDE_TARGET_OSX_26
    if (@available(macOS 26, *)) {
        vmnet_network_get_ipv6_prefix((vmnet_network_ref)network, prefix, prefix_len);
        return;
    }
#endif
    RAISE_UNSUPPORTED_MACOS_EXCEPTION();
}

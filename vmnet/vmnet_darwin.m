#import "vmnet_darwin.h"

// MARK: - CFRetain/Release Wrapper
void vmnetRetain(void *obj)
{
    if (obj != NULL) {
        CFRetain((CFTypeRef)obj);
    }
}

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

// MARK: - interface_ref (macOS 26+)

extern void callPacketsAvailableEventCallback(uintptr_t cgoHandle, int estimatedCount);

uint32_t VmnetInterfaceSetPacketsAvailableEventCallback(void *iface, uintptr_t callback)
{
#ifdef INCLUDE_TARGET_OSX_26
    if (@available(macOS 26, *)) {
        dispatch_queue_t queue = dispatch_queue_create("vmnet.interface.eventcallback", DISPATCH_QUEUE_SERIAL);
        vmnet_return_t result = vmnet_interface_set_event_callback((interface_ref)iface, VMNET_INTERFACE_PACKETS_AVAILABLE, queue, ^(interface_event_t eventMask, xpc_object_t event) {
            if ((eventMask & VMNET_INTERFACE_PACKETS_AVAILABLE) != 0) {
                int estimated = (int)xpc_dictionary_get_uint64(event, vmnet_estimated_packets_available_key);
                callPacketsAvailableEventCallback(callback, estimated);
            }
        });
        dispatch_release(queue);
        return result;
    }
#endif
    RAISE_UNSUPPORTED_MACOS_EXCEPTION();
}

uint32_t VmnetStopInterface(void *interface)
{
#ifdef INCLUDE_TARGET_OSX_26
    if (@available(macOS 26, *)) {
        dispatch_semaphore_t sem = dispatch_semaphore_create(0);
        dispatch_queue_t queue = dispatch_get_global_queue(QOS_CLASS_DEFAULT, 0);
        __block vmnet_return_t status;
        vmnet_return_t scheduleStatus = vmnet_stop_interface((interface_ref)interface, queue, ^(vmnet_return_t stopStatus) {
            status = stopStatus;
            dispatch_semaphore_signal(sem);
        });
        dispatch_release(queue);
        if (scheduleStatus != VMNET_SUCCESS) {
            dispatch_release(sem);
            return scheduleStatus;
        }
        dispatch_semaphore_wait(sem, DISPATCH_TIME_FOREVER);
        dispatch_release(sem);
        return status;
    }
#endif
    RAISE_UNSUPPORTED_MACOS_EXCEPTION();
}

uint32_t VmnetRead(void *interface, struct vmpktdesc *packets, int *pktcnt)
{
#ifdef INCLUDE_TARGET_OSX_26
    if (@available(macOS 26, *)) {
        return vmnet_read((interface_ref)interface, packets, pktcnt);
    }
#endif
    RAISE_UNSUPPORTED_MACOS_EXCEPTION();
}

uint32_t VmnetWrite(void *interface, struct vmpktdesc *packets, int *pktcnt)
{
#ifdef INCLUDE_TARGET_OSX_26
    if (@available(macOS 26, *)) {
        return vmnet_write((interface_ref)interface, packets, pktcnt);
    }
#endif
    RAISE_UNSUPPORTED_MACOS_EXCEPTION();
}

struct vmnetInterfaceStartResult VmnetInterfaceStartWithNetwork(void *network, void *interfaceDesc)
{
#ifdef INCLUDE_TARGET_OSX_26
    if (@available(macOS 26, *)) {
        dispatch_semaphore_t sem = dispatch_semaphore_create(0);
        dispatch_queue_t queue = dispatch_get_global_queue(QOS_CLASS_DEFAULT, 0);
        __block struct vmnetInterfaceStartResult result;
        vmnet_start_interface_completion_handler_t handler = ^(vmnet_return_t vmnetReturn, xpc_object_t ifaceParam) {
            result.ifaceParam = xpc_retain(ifaceParam);
            result.maxPacketSize = xpc_dictionary_get_uint64(ifaceParam, vmnet_max_packet_size_key);
            result.maxReadPacketCount = xpc_dictionary_get_uint64(ifaceParam, vmnet_read_max_packets_key);
            result.maxWritePacketCount = xpc_dictionary_get_uint64(ifaceParam, vmnet_write_max_packets_key);
            result.vmnetReturn = vmnetReturn;
            dispatch_semaphore_signal(sem);
        };
        interface_ref iface = vmnet_interface_start_with_network((vmnet_network_ref)network, (xpc_object_t)interfaceDesc, queue, handler);
        result.iface = iface;
        dispatch_semaphore_wait(sem, DISPATCH_TIME_FOREVER);
        dispatch_release(queue);
        dispatch_release(sem);
        return result;
    }
#endif
    RAISE_UNSUPPORTED_MACOS_EXCEPTION();
}

// wrap vmnet_enable_virtio_header_key
const char *const wrap_vmnet_enable_virtio_header_key(void)
{
#ifdef INCLUDE_TARGET_OSX_15_4
    if (@available(macOS 15.4, *)) {
        return vmnet_enable_virtio_header_key;
    }
#endif
    return NULL;
}

// MARK: - vmpktdesc helper functions

struct vmpktdesc *allocateVMPktDescArray(int count, uint64_t maxPacketSize)
{
    // Calculate total size needed for pktdesc array and iovec array
    size_t totalSize = (sizeof(struct vmpktdesc) + sizeof(struct iovec)) * count;
    struct vmpktdesc *pktDescs = (struct vmpktdesc *)malloc(totalSize);
    return resetVMPktDescArray(pktDescs, count, maxPacketSize);
}

struct vmpktdesc *resetVMPktDescArray(struct vmpktdesc *pktDescs, int count, uint64_t maxPacketSize)
{
    struct iovec *iovecArray = (struct iovec *)(pktDescs + count);
    for (int i = 0; i < count; i++) {
        pktDescs[i].vm_pkt_size = maxPacketSize;
        pktDescs[i].vm_pkt_iov = &iovecArray[i];
        pktDescs[i].vm_pkt_iovcnt = 1;
        pktDescs[i].vm_flags = 0;
        iovecArray[i].iov_len = maxPacketSize;
    }
    return pktDescs;
}

void deallocateVMPktDescArray(struct vmpktdesc *pktDescs)
{
    if (pktDescs != NULL) {
        free(pktDescs);
    }
}

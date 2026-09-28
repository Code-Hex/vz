#pragma once

#import <net/ethernet.h>
#import <netinet/in.h>
#import <sys/uio.h>
// In older SDKs, vmnet.h does not include above headers, so we include them here.
#import "../internal/osversion/virtualization_helper.h"
#import <vmnet/vmnet.h>

// MARK: - CFRetain/Release Wrapper

void vmnetRetain(void *obj);
void vmnetRelease(void *obj);

// MARK: - vmnet_network_configuration_t (macOS 26+)

uint32_t VmnetNetworkConfiguration_addDhcpReservation(void *config, ether_addr_t const *client, struct in_addr const *reservation);
uint32_t VmnetNetworkConfiguration_addPortForwardingRule(void *config, uint8_t protocol, sa_family_t address_family, uint16_t internal_port, uint16_t external_port, void const *internal_address);
void *VmnetNetworkConfigurationCreate(uint32_t mode, uint32_t *status);
void VmnetNetworkConfiguration_disableDhcp(void *config);
void VmnetNetworkConfiguration_disableDnsProxy(void *config);
void VmnetNetworkConfiguration_disableNat44(void *config);
void VmnetNetworkConfiguration_disableNat66(void *config);
void VmnetNetworkConfiguration_disableRouterAdvertisement(void *config);
uint32_t VmnetNetworkConfiguration_setExternalInterface(void *config, const char *ifname);
uint32_t VmnetNetworkConfiguration_setIPv4Subnet(void *config, struct in_addr const *subnet_addr, struct in_addr const *subnet_mask);
uint32_t VmnetNetworkConfiguration_setIPv6Prefix(void *config, struct in6_addr const *prefix, uint8_t prefix_len);
uint32_t VmnetNetworkConfiguration_setMtu(void *config, uint32_t mtu);

// MARK: - vmnet_network_ref (macOS 26+)

void *VmnetNetwork_copySerialization(void *network, uint32_t *status);
void *VmnetNetworkCreate(void *config, uint32_t *status);
void *VmnetNetworkCreateWithSerialization(void *serialization, uint32_t *status);
void VmnetNetwork_getIPv4Subnet(void *network, struct in_addr *subnet, struct in_addr *mask);
void VmnetNetwork_getIPv6Prefix(void *network, struct in6_addr *prefix, uint8_t *prefix_len);

// MARK: - interface_ref (macOS 26+)

uint32_t VmnetInterfaceSetPacketsAvailableEventCallback(void *interface, uintptr_t callback);
uint32_t VmnetStopInterface(void *interface);
uint32_t VmnetRead(void *interface, struct vmpktdesc *packets, int *pktcnt);
uint32_t VmnetWrite(void *interface, struct vmpktdesc *packets, int *pktcnt);

struct vmnetInterfaceStartResult {
    void *iface; // interface_ref
    void *ifaceParam; // xpc_object_t
    uint64_t maxPacketSize;
    int maxReadPacketCount;
    int maxWritePacketCount;
    uint32_t vmnetReturn;
};

struct vmnetInterfaceStartResult VmnetInterfaceStartWithNetwork(void *network, void *interfaceDesc);

// wrap vmnet_enable_virtio_header_key
const char *const wrap_vmnet_enable_virtio_header_key(void);

// MARK: - vmpktdesc helper functions
struct vmpktdesc *allocateVMPktDescArray(int count, uint64_t maxPacketSize);
struct vmpktdesc *resetVMPktDescArray(struct vmpktdesc *pktDescs, int count, uint64_t maxPacketSize);
void deallocateVMPktDescArray(struct vmpktdesc *pktDescs);

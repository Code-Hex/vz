//
//  vmnet.m
//
//  Created by codehex.
//

#import "cgo.h"

#ifdef INCLUDE_TARGET_OSX_26
#import <CoreFoundation/CoreFoundation.h>
#import <arpa/inet.h>
#import <net/ethernet.h>
#import <string.h>
#import <vmnet/vmnet.h>
#endif // INCLUDE_TARGET_OSX_26

void *newNetworkConfiguration(int mode, int *status)
{
#ifdef INCLUDE_TARGET_OSX_26
    if (@available(macOS 26, *)) {
        vmnet_return_t s = VMNET_SUCCESS;
        vmnet_network_configuration_ref config =
            vmnet_network_configuration_create((vmnet_mode_t)mode, &s);
        if (status) *status = (int)s;
        return config;
    }
#endif
    if (status) *status = -1;
    return NULL;
}

void releaseNetworkConfiguration(void *config)
{
#ifdef INCLUDE_TARGET_OSX_26
    if (config) CFRelease((vmnet_network_configuration_ref)config);
#else
    (void)config;
#endif
}

// setIPv4Subnet / addDhcpReservation return 0 on success, the raw
// vmnet_return_t on failure (or -1 if the API isn't compiled in).
// vmnet's VMNET_SUCCESS is 1000, not 0 — callers can't compare the
// raw return against 0, so we normalize here.
int setIPv4Subnet(void *config, const char *gatewayIP, const char *mask)
{
#ifdef INCLUDE_TARGET_OSX_26
    if (@available(macOS 26, *)) {
        struct in_addr s, m;
        if (inet_pton(AF_INET, gatewayIP, &s) != 1) return (int)VMNET_INVALID_ARGUMENT;
        if (inet_pton(AF_INET, mask, &m) != 1) return (int)VMNET_INVALID_ARGUMENT;
        vmnet_return_t rc = vmnet_network_configuration_set_ipv4_subnet(
            (vmnet_network_configuration_ref)config, &s, &m);
        return rc == VMNET_SUCCESS ? 0 : (int)rc;
    }
#endif
    (void)config; (void)gatewayIP; (void)mask;
    return -1;
}

int addDhcpReservation(void *config, const uint8_t *mac, const char *reservationIP)
{
#ifdef INCLUDE_TARGET_OSX_26
    if (@available(macOS 26, *)) {
        ether_addr_t client;
        memcpy(&client, mac, 6);
        struct in_addr res;
        if (inet_pton(AF_INET, reservationIP, &res) != 1) return (int)VMNET_INVALID_ARGUMENT;
        vmnet_return_t rc = vmnet_network_configuration_add_dhcp_reservation(
            (vmnet_network_configuration_ref)config, &client, &res);
        return rc == VMNET_SUCCESS ? 0 : (int)rc;
    }
#endif
    (void)config; (void)mac; (void)reservationIP;
    return -1;
}

void *newNetwork(void *config, int *status)
{
#ifdef INCLUDE_TARGET_OSX_26
    if (@available(macOS 26, *)) {
        vmnet_return_t s = VMNET_SUCCESS;
        vmnet_network_ref network = vmnet_network_create(
            (vmnet_network_configuration_ref)config, &s);
        if (status) *status = (int)s;
        return network;
    }
#endif
    if (status) *status = -1;
    return NULL;
}

void releaseNetwork(void *network)
{
#ifdef INCLUDE_TARGET_OSX_26
    if (network) CFRelease((vmnet_network_ref)network);
#else
    (void)network;
#endif
}

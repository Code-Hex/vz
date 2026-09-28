//
//  vmnet.h
//
//  Created by codehex.
//

#pragma once

#import <Availability.h>
#include <stdint.h>
#include <stdlib.h>

// SDK guard, mirroring the parent package's virtualization_helper.h.
// Duplicated because cgo includes are per-package (subpackages don't
// share headers with the root unless explicitly wired). Keep in sync.
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 260000
#define INCLUDE_TARGET_OSX_26 1
#endif

// --- vmnet_network_configuration_* wrappers ------------------------------

// newNetworkConfiguration returns a +1-retained
// vmnet_network_configuration_ref for the given operating mode.
// On older macOS / older SDKs returns NULL; status (if non-NULL)
// gets the vmnet_return_t (or -1 when the API isn't compiled in).
void *newNetworkConfiguration(int mode, int *status);

// releaseNetworkConfiguration CFReleases a config ref. Safe on NULL.
void releaseNetworkConfiguration(void *config);

// setIPv4Subnet calls vmnet_network_configuration_set_ipv4_subnet.
// gatewayIP is the first usable address of the range (e.g.
// "192.168.200.1" for /24) — Apple's docstring calls the param
// "subnet_addr" but the actual semantic is the gateway IP. mask is
// dotted-quad ("255.255.255.0"). Returns vmnet_return_t (0=success).
int setIPv4Subnet(void *config, const char *gatewayIP, const char *mask);

// addDhcpReservation calls vmnet_network_configuration_add_dhcp_reservation.
// mac is exactly 6 bytes; reservationIP is a dotted-quad IPv4 string.
// Returns vmnet_return_t (0=success).
int addDhcpReservation(void *config, const uint8_t *mac, const char *reservationIP);

// --- vmnet_network_* wrappers --------------------------------------------

// newNetwork returns a +1-retained vmnet_network_ref from the given
// configuration. The configuration may be released after this call.
// On failure or older macOS / SDK returns NULL.
void *newNetwork(void *config, int *status);

// releaseNetwork CFReleases a network ref. Safe on NULL.
void releaseNetwork(void *network);

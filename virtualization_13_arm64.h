//
//  virtualization_13_arm64.h
//
//  Created by codehex.
//

#pragma once

#ifdef __arm64__

#import "internal/osversion/virtualization_helper.h"
#import <Virtualization/Virtualization.h>

/* exported from cgo */
void linuxInstallRosettaWithCompletionHandler(uintptr_t cgoHandle, void *errPtr);

void *newVZLinuxRosettaDirectoryShare(void **error);
void linuxInstallRosetta(uintptr_t cgoHandle);
int availabilityVZLinuxRosettaDirectoryShare();

void *newVZMacOSVirtualMachineStartOptions(bool startUpFromMacOSRecovery);

void *newVZMacTrackpadConfiguration();

#endif
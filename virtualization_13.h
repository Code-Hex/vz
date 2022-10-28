//
//  virtualization_13.h
//
//  Created by codehex.
//

#pragma once

#import "virtualization_helper.h"
#import <Foundation/Foundation.h>
#import <Virtualization/Virtualization.h>

/* exported from cgo */
void linuxInstallRosettaWithCompletionHandler(void *cgoHandler, void *errPtr);

/* macOS 13 API */
void setConsoleDevicesVZVirtualMachineConfiguration(void *config, void *consoleDevices);

void *newVZEFIBootLoader();
void setVariableStoreVZEFIBootLoader(void *bootLoaderPtr, void *variableStore);
void *newVZEFIVariableStorePath(const char *variableStorePath);
void *newCreatingVZEFIVariableStoreAtPath(const char *variableStorePath, void **error);

void *newVZGenericMachineIdentifierWithBytes(void *machineIdentifierBytes, int len);
nbyteslice getVZGenericMachineIdentifierDataRepresentation(void *machineIdentifierPtr);
void *newVZGenericMachineIdentifier();
void setMachineIdentifierVZGenericPlatformConfiguration(void *config, void *machineIdentifier);

void *newVZUSBMassStorageDeviceConfiguration(void *attachment);
void *newVZVirtioGraphicsDeviceConfiguration();
void setScanoutsVZVirtioGraphicsDeviceConfiguration(void *graphicsConfiguration, void *scanouts);
void *newVZVirtioGraphicsScanoutConfiguration(NSInteger widthInPixels, NSInteger heightInPixels);

void *newVZVirtioConsoleDeviceConfiguration();
void *portsVZVirtioConsoleDeviceConfiguration(void *consoleDevice);
uint32_t maximumPortCountVZVirtioConsolePortConfigurationArray(void *ports);
void *getObjectAtIndexedSubscriptVZVirtioConsolePortConfigurationArray(void *portsPtr, int portIndex);
void setObjectAtIndexedSubscriptVZVirtioConsolePortConfigurationArray(void *portsPtr, void *portConfig, int portIndex);

void *newVZVirtioConsolePortConfiguration();
void setNameVZVirtioConsolePortConfiguration(void *consolePortConfig, const char *name);
void setIsConsoleVZVirtioConsolePortConfiguration(void *consolePortConfig, bool isConsole);
void setAttachmentVZVirtioConsolePortConfiguration(void *consolePortConfig, void *serialPortAttachment);
void *newVZSpiceAgentPortAttachment();
void setSharesClipboardVZSpiceAgentPortAttachment(void *attachment, bool sharesClipboard);
const char *getSpiceAgentPortName();

void *newVZLinuxRosettaDirectoryShare(void **error);
void linuxInstallRosetta(void *cgoHandler);
int availabilityVZLinuxRosettaDirectoryShare();

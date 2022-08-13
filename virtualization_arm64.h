//
//  virtualization_arm64.h
//
//  Created by codehex.
//

#pragma once

#import <Foundation/Foundation.h>
#import <Foundation/NSNotification.h>
#import <Virtualization/Virtualization.h>

#ifdef __arm64__

typedef struct VZMacOSRestoreImageStruct {
	const char *url;
    const char *buildVersion;
	NSOperatingSystemVersion operatingSystemVersion;
    void *mostFeaturefulSupportedConfiguration; // (VZMacOSConfigurationRequirements *)
} VZMacOSRestoreImageStruct;

/* exported from cgo */
void macOSRestoreImageCompletionHandler(void *cgoHandler, void *restoreImage, void *errPtr);

/* Mac Configurations */
void *newVZMacPlatformConfiguration();
void *newVZMacAuxiliaryStorageWithCreating(const char *storagePath, void *hardwareModel, void **error);
void *newVZMacAuxiliaryStorage(const char *storagePath);
void *newVZMacPlatformConfiguration();
void setHardwareModelVZMacPlatformConfiguration(void *config, void *hardwareModel);
void storeHardwareModelDataVZMacPlatformConfiguration(void *config, const char *filePath);
void setMachineIdentifierVZMacPlatformConfiguration(void *config, void *machineIdentifier);
void storeMachineIdentifierDataVZMacPlatformConfiguration(void *config, const char *filePath);
void setAuxiliaryStorageVZMacPlatformConfiguration(void *config, void *auxiliaryStorage);
void *newVZMacOSBootLoader();
void *newVZMacGraphicsDeviceConfiguration();
void setDisplaysVZMacGraphicsDeviceConfiguration(void *graphicsConfiguration, void *displays);
void *newVZMacGraphicsDisplayConfiguration(NSInteger widthInPixels, NSInteger heightInPixels, NSInteger pixelsPerInch);
void *newVZMacHardwareModelWithPath(const char *hardwareModelPath);
void *newVZMacMachineIdentifierWithPath(const char *machineIdentifierPath);


VZMacOSRestoreImageStruct convertVZMacOSRestoreImage2Struct(VZMacOSRestoreImage *restoreImage);
void fetchLatestSupportedMacOSRestoreImageWithCompletionHandler(void *cgoHandler);
void loadMacOSRestoreImageFile(const char *ipswPath, void *cgoHandler);


#endif
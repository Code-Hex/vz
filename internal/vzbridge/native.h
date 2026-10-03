#pragma once
#include <Availability.h>
#include <stdbool.h>
#include <stdint.h>

// Object inputs are borrowed; owned results transfer one retain to Go.
typedef void *VZBridgeObject __attribute__((annotate("vzbridge:object")));
typedef void *VZBridgeOwnedObject __attribute__((annotate("vzbridge:owned")));
typedef void **VZBridgeErrorOut __attribute__((annotate("vzbridge:errorout")));

void vz_dispatchSync(uintptr_t callback, uintptr_t context) __attribute__((annotate("vzbridge:manual")));
void vz_attachDeviceVZUSBController(VZBridgeObject p0, VZBridgeObject p1, uint64_t p2);
void vz_cancelInstallVZMacOSInstaller(VZBridgeObject p0);
void vz_detachDeviceVZUSBController(VZBridgeObject p0, VZBridgeObject p1, uint64_t p2);
void *vz_dispatchQueuePointer(void) __attribute__((annotate("vzbridge:noqueue")));
void vz_drain(void) __attribute__((annotate("vzbridge:noqueue")));
void vz_fetchLatestSupportedMacOSRestoreImageWithCompletionHandler(uint64_t p0);
VZBridgeOwnedObject vz_getUUIDUSBDevice(VZBridgeObject p0);
void vz_installByVZMacOSInstaller(VZBridgeObject p0, uint64_t p1, uint64_t p2);
void vz_invalidateVZVirtioSocketListener(VZBridgeObject p0);
void vz_linuxInstallRosetta(uint64_t p0);
void vz_loadMacOSRestoreImageFile(const char *p0, uint64_t p1);
int64_t vz_macOSRestoreImageMajorVersion(VZBridgeObject p0);
int64_t vz_macOSRestoreImageMinorVersion(VZBridgeObject p0);
int64_t vz_macOSRestoreImagePatchVersion(VZBridgeObject p0);
void vz_setInvalidDiskModeError(VZBridgeErrorOut errorOut);
VZBridgeOwnedObject vz_newVZDiskBlockDeviceStorageDeviceAttachment(int32_t p0, bool p1, int32_t p2, VZBridgeErrorOut p3);
VZBridgeOwnedObject vz_newVZFileHandleNetworkDeviceAttachment(int32_t p0, VZBridgeErrorOut p1);
VZBridgeOwnedObject vz_newVZFileHandleSerialPortAttachment(int32_t p0, int32_t p1, VZBridgeErrorOut p2);
VZBridgeOwnedObject vz_newVZMacOSInstaller(VZBridgeObject p0, const char *p1);
VZBridgeOwnedObject vz_newVZNetworkBlockDeviceStorageDeviceAttachment(const char *p0, double p1, bool p2, int32_t p3, VZBridgeErrorOut p4, uint64_t p5);
VZBridgeOwnedObject vz_newVZVirtioSocketListener(uint64_t p0);
VZBridgeOwnedObject vz_newVZVirtualMachineWithDispatchQueue(VZBridgeObject p0, uint64_t p1, uint64_t p2);
void vz_pauseWithCompletionHandler(VZBridgeObject p0, uint64_t p1);
void vz_releaseObject(void *p0) __attribute__((annotate("vzbridge:noqueue")));
void vz_restoreMachineStateFromURLWithCompletionHandler(VZBridgeObject p0, uint64_t p1, const char *p2);
void vz_resumeWithCompletionHandler(VZBridgeObject p0, uint64_t p1);
void vz_saveMachineStateToURLWithCompletionHandler(VZBridgeObject p0, uint64_t p1, const char *p2);
void vz_setCallback(uint64_t p0);
int32_t vz_socketConnectionDuplicatedFileDescriptor(VZBridgeObject p0, VZBridgeErrorOut p1);
void vz_startVirtualMachineWindow(VZBridgeObject p0, void *p1, double p2, double p3, const char *p4, bool p5) __attribute__((annotate("vzbridge:noqueue")));
void vz_startWithCompletionHandler(VZBridgeObject p0, uint64_t p1);
void vz_startWithOptionsCompletionHandler(VZBridgeObject p0, VZBridgeObject p1, uint64_t p2);
void vz_stopWithCompletionHandler(VZBridgeObject p0, uint64_t p1);
void vz_VZVirtioSocketDevice_connectToPort(VZBridgeObject p0, uint32_t p1, uint64_t p2);

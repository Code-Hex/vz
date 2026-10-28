#pragma once
#include <Availability.h>
#include <stdbool.h>
#include <stdint.h>

enum {
    VZBridgeCallbackCompletion = 1,
    VZBridgeCallbackMachineStateChanged = 2,
    VZBridgeCallbackNetworkDisconnected = 3,
    VZBridgeCallbackContextReleased = 4,
    VZBridgeCallbackSocketConnected = 5,
    VZBridgeCallbackSocketAccepted = 6,
    VZBridgeCallbackNetworkBlockDeviceError = 7,
    VZBridgeCallbackNetworkBlockDeviceConnected = 8,
    VZBridgeCallbackRosettaInstallCompleted = 9,
    VZBridgeCallbackRestoreImageLoaded = 10,
    VZBridgeCallbackMacOSInstallCompleted = 11,
    VZBridgeCallbackMacOSInstallProgress = 12,
    VZBridgeCallbackMachineStoppedWithError = 13,
    VZBridgeCallbackGuestStopped = 14,
    VZBridgeCallbackConsolePortOpened = 15,
    VZBridgeCallbackConsolePortClosed = 16,
    VZBridgeCallbackDisplayReconfigurationBegan = 17,
    VZBridgeCallbackDisplayReconfigurationEnded = 18,
    VZBridgeCallbackAccessoryConnected = 30,
    VZBridgeCallbackAccessoryDisconnected = 31,
    VZBridgeCallbackAccessoryRegistered = 32,
    VZBridgeCallbackUSBPassthroughDisconnected = 33,

};

// Object inputs are borrowed; owned results transfer one retain to Go.
typedef void *VZBridgeObject __attribute__((annotate("vzbridge:object")));
typedef void *VZBridgeOwnedObject __attribute__((annotate("vzbridge:owned")));
typedef void **VZBridgeErrorOut __attribute__((annotate("vzbridge:errorout")));

void vz_dispatchSyncOnObject(void *object, uintptr_t callback, uintptr_t context) __attribute__((annotate("vzbridge:manual")));
void *vz_objectQueue(VZBridgeObject object) __attribute__((annotate("vzbridge:noqueue")));
void vz_setObjectQueue(void *object, void *queue) __attribute__((annotate("vzbridge:noqueue")));
void vz_inheritObjectQueue(void *object, void *source) __attribute__((annotate("vzbridge:noqueue")));
void vz_attachDeviceVZUSBController(VZBridgeObject p0, VZBridgeObject p1, uint64_t p2) __attribute__((annotate("vzbridge:objectqueue")));
void vz_cancelInstallVZMacOSInstaller(VZBridgeObject p0) __attribute__((annotate("vzbridge:objectqueue")));
void vz_detachDeviceVZUSBController(VZBridgeObject p0, VZBridgeObject p1, uint64_t p2) __attribute__((annotate("vzbridge:objectqueue")));
void *vz_dispatchQueuePointer(void) __attribute__((annotate("vzbridge:noqueue")));
void vz_drain(void) __attribute__((annotate("vzbridge:noqueue")));
void vz_fetchLatestSupportedMacOSRestoreImageWithCompletionHandler(uint64_t p0);
VZBridgeOwnedObject vz_getUUIDUSBDevice(VZBridgeObject p0) __attribute__((annotate("vzbridge:objectqueue")));
void vz_installByVZMacOSInstaller(VZBridgeObject p0, uint64_t p1, uint64_t p2) __attribute__((annotate("vzbridge:objectqueue")));
void vz_invalidateVZVirtioSocketListener(VZBridgeObject p0) __attribute__((annotate("vzbridge:objectqueue")));
void vz_linuxInstallRosetta(uint64_t p0);
void vz_loadMacOSRestoreImageFile(const char *p0, uint64_t p1);
int64_t vz_macOSRestoreImageMajorVersion(VZBridgeObject p0);
int64_t vz_macOSRestoreImageMinorVersion(VZBridgeObject p0);
int64_t vz_macOSRestoreImagePatchVersion(VZBridgeObject p0);
void vz_setInvalidDiskModeError(VZBridgeErrorOut errorOut);
VZBridgeOwnedObject vz_newVZDiskBlockDeviceStorageDeviceAttachment(int32_t p0, bool p1, int32_t p2, VZBridgeErrorOut p3);
VZBridgeOwnedObject vz_newVZFileHandleNetworkDeviceAttachment(int32_t p0, VZBridgeErrorOut p1);
VZBridgeOwnedObject vz_newVZFileHandleSerialPortAttachment(int32_t readFD, bool hasRead, int32_t writeFD, bool hasWrite, VZBridgeErrorOut errorOut);
VZBridgeOwnedObject vz_newVZMacOSInstaller(VZBridgeObject p0, const char *p1) __attribute__((annotate("vzbridge:objectqueue")));
VZBridgeOwnedObject vz_newVZNetworkBlockDeviceStorageDeviceAttachment(const char *p0, double p1, bool p2, int32_t p3, VZBridgeErrorOut p4, uint64_t p5);
VZBridgeOwnedObject vz_newVZVirtioSocketListener(VZBridgeObject device, uint64_t context) __attribute__((annotate("vzbridge:objectqueue")));
VZBridgeOwnedObject vz_newVZVirtualMachineWithDispatchQueue(VZBridgeObject p0, uint64_t p1, uint64_t p2) __attribute__((annotate("vzbridge:noqueue")));
void vz_pauseWithCompletionHandler(VZBridgeObject p0, uint64_t p1) __attribute__((annotate("vzbridge:objectqueue")));
void vz_releaseObject(void *p0) __attribute__((annotate("vzbridge:noqueue")));
void vz_restoreMachineStateFromURLWithCompletionHandler(VZBridgeObject p0, uint64_t p1, const char *p2) __attribute__((annotate("vzbridge:objectqueue")));
void vz_resumeWithCompletionHandler(VZBridgeObject p0, uint64_t p1) __attribute__((annotate("vzbridge:objectqueue")));
void vz_saveMachineStateToURLWithCompletionHandler(VZBridgeObject p0, uint64_t p1, const char *p2) __attribute__((annotate("vzbridge:objectqueue")));
void vz_setCallback(uint64_t p0);
int32_t vz_socketConnectionDuplicatedFileDescriptor(VZBridgeObject p0, VZBridgeErrorOut p1) __attribute__((annotate("vzbridge:objectqueue")));
void *vz_newGraphicApplication(void) __attribute__((annotate("vzbridge:noqueue")));
void vz_addGraphicWindow(void *application, VZBridgeObject machine, double width, double height, const char *title, bool enableController) __attribute__((annotate("vzbridge:noqueue")));
typedef enum : int32_t {
    VZGraphicApplicationSuccess = 0,
    VZGraphicApplicationNotMainThread = 1,
    VZGraphicApplicationDuplicateMachine = 2,
    VZGraphicApplicationStopping = 3,
} VZGraphicApplicationResult;
VZGraphicApplicationResult vz_runGraphicApplication(void *application) __attribute__((annotate("vzbridge:manual")));
void vz_freeGraphicApplication(void *application) __attribute__((annotate("vzbridge:noqueue")));
void vz_startWithCompletionHandler(VZBridgeObject p0, uint64_t p1) __attribute__((annotate("vzbridge:objectqueue")));
void vz_startWithOptionsCompletionHandler(VZBridgeObject p0, VZBridgeObject p1, uint64_t p2) __attribute__((annotate("vzbridge:objectqueue")));
void vz_stopWithCompletionHandler(VZBridgeObject p0, uint64_t p1) __attribute__((annotate("vzbridge:objectqueue")));
void vz_VZVirtioSocketDevice_connectToPort(VZBridgeObject p0, uint32_t p1, uint64_t p2) __attribute__((annotate("vzbridge:objectqueue")));
void *vz_newVmnetNetworkConfiguration(int32_t mode, VZBridgeErrorOut errorOut);
void *vz_newVmnetNetwork(VZBridgeObject configuration, VZBridgeErrorOut errorOut);
void vz_disableVmnetDHCP(VZBridgeObject configuration);
void vz_disableVmnetDNSProxy(VZBridgeObject configuration);
void vz_disableVmnetNAT44(VZBridgeObject configuration);
void vz_disableVmnetNAT66(VZBridgeObject configuration);
void vz_disableVmnetRouterAdvertisement(VZBridgeObject configuration);
void vz_setVmnetExternalInterface(VZBridgeObject configuration, const char *name, VZBridgeErrorOut errorOut);
void vz_setVmnetMTU(VZBridgeObject configuration, uint32_t mtu, VZBridgeErrorOut errorOut);
void vz_setVmnetIPv4Subnet(VZBridgeObject configuration, uint32_t address, uint32_t mask, VZBridgeErrorOut errorOut);
void vz_setVmnetIPv6Prefix(VZBridgeObject configuration, const void *address, uint8_t prefixLength, VZBridgeErrorOut errorOut);
void vz_addVmnetDHCPReservation(VZBridgeObject configuration, const void *mac, uint32_t address, VZBridgeErrorOut errorOut);
void vz_addVmnetPortForwarding(VZBridgeObject configuration, bool udp, bool ipv6, uint16_t externalPort, uint16_t internalPort, const void *address, VZBridgeErrorOut errorOut);
uint64_t vz_vmnetIPv4Subnet(VZBridgeObject network);
uint8_t vz_vmnetIPv6Prefix(VZBridgeObject network, void *address);
VZBridgeOwnedObject vz_newVmnetNetworkDeviceAttachment(VZBridgeObject network);
void *vz_vmnetNetworkDeviceAttachmentNetwork(VZBridgeObject attachment);
void vz_releaseCFObject(void *object);
void *vz_startVmnetInterface(VZBridgeObject network, const char *name, uint32_t options, uint64_t callback, uint64_t context, void *maxPacketSize, void *mac, VZBridgeErrorOut errorOut) __attribute__((annotate("vzbridge:noqueue")));
VZBridgeOwnedObject vz_copyVmnetBridgedInterfaces(VZBridgeErrorOut errorOut) __attribute__((annotate("vzbridge:noqueue")));
void vz_stopVmnetInterface(void *session, VZBridgeErrorOut errorOut) __attribute__((annotate("vzbridge:noqueue")));
int32_t vz_writeVmnetPacket(void *session, const void *buffer, uint64_t size, VZBridgeErrorOut errorOut) __attribute__((annotate("vzbridge:noqueue")));
int32_t vz_readVmnetPackets(void *session, void *buffers, void *sizes, int32_t count, VZBridgeErrorOut errorOut) __attribute__((annotate("vzbridge:noqueue")));
int32_t vz_writeVmnetPackets(void *session, void *buffers, void *sizes, int32_t count, VZBridgeErrorOut errorOut) __attribute__((annotate("vzbridge:noqueue")));
void *vz_copyVmnetNetworkSerialization(VZBridgeObject network, VZBridgeErrorOut errorOut);
void *vz_newVmnetNetworkWithSerialization(VZBridgeObject serialization, VZBridgeErrorOut errorOut);
void vz_releaseXPCObject(void *object) __attribute__((annotate("vzbridge:noqueue")));
void *vz_listenVmnetNetwork(const char *name, VZBridgeObject serialization, const char *requirement, VZBridgeErrorOut errorOut) __attribute__((annotate("vzbridge:noqueue")));
void vz_closeVmnetNetworkService(void *service) __attribute__((annotate("vzbridge:noqueue")));
void *vz_requestVmnetNetwork(const char *name, const char *requirement, VZBridgeErrorOut errorOut) __attribute__((annotate("vzbridge:noqueue")));
void vz_cancelVmnetNetworkRequest(void *request) __attribute__((annotate("vzbridge:noqueue")));
void *vz_waitVmnetNetworkRequest(void *request, VZBridgeErrorOut errorOut) __attribute__((annotate("vzbridge:noqueue")));
void vz_releaseVmnetNetworkRequest(void *request) __attribute__((annotate("vzbridge:noqueue")));

void vz_graphicsDisplaySize(VZBridgeObject display, void *size) __attribute__((annotate("vzbridge:objectqueue")));
void vz_reconfigureGraphicsDisplaySize(VZBridgeObject display, double width, double height, VZBridgeErrorOut errorOut) __attribute__((annotate("vzbridge:objectqueue")));

VZBridgeOwnedObject vz_newEFICertificate(VZBridgeObject data);
VZBridgeOwnedObject vz_efiCertificateData(VZBridgeObject signature);
bool vz_isEFICertificate(VZBridgeObject signature);
void vz_enableEFISecureBoot(VZBridgeObject store, VZBridgeObject signature, VZBridgeErrorOut errorOut);

VZBridgeOwnedObject vz_registerUSBAccessories(uint64_t context) __attribute__((annotate("vzbridge:noqueue")));
void vz_unregisterUSBAccessories(VZBridgeObject listener, uint64_t context) __attribute__((annotate("vzbridge:noqueue")));
uint64_t vz_USBAccessoryRegistryID(VZBridgeObject accessory);
VZBridgeOwnedObject vz_USBAccessoryDeviceDescriptor(VZBridgeObject accessory);
VZBridgeOwnedObject vz_USBAccessoryConfigurationDescriptor(VZBridgeObject accessory);
VZBridgeOwnedObject vz_observeUSBDisconnect(VZBridgeObject controller, uint64_t context, VZBridgeErrorOut errorOut) __attribute__((annotate("vzbridge:objectqueue")));
void vz_stopObservingUSBDisconnect(VZBridgeObject controller, VZBridgeObject observer) __attribute__((annotate("vzbridge:objectqueue")));
bool vz_efiSecureBootEnabled(VZBridgeObject store, VZBridgeErrorOut errorOut);

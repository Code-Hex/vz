//
//  virtualization.h
//
//  Created by codehex.
//

#pragma once

#import <Foundation/Foundation.h>
#import <Virtualization/Virtualization.h>

/* exported from cgo */
void startHandler(void *err, char *id);
void pauseHandler(void *err, char *id);
void resumeHandler(void *err, char *id);
void connectionHandler(void *connection, void *err, char *id);
void changeStateOnObserver(int state, char *id);
bool shouldAcceptNewConnectionHandler(void *listener, void *connection, void *socketDevice);

@interface Observer : NSObject
- (void)observeValueForKeyPath:(NSString *)keyPath ofObject:(id)object change:(NSDictionary *)change context:(void *)context;
@end

/* VZVirtioSocketListener */
@interface VZVirtioSocketListenerDelegateImpl : NSObject <VZVirtioSocketListenerDelegate>
- (BOOL)listener:(VZVirtioSocketListener *)listener shouldAcceptNewConnection:(VZVirtioSocketConnection *)connection fromSocketDevice:(VZVirtioSocketDevice *)socketDevice;
@end

/* BootLoader */
void *newVZLinuxBootLoader(const char *kernelPath);
void setCommandLineVZLinuxBootLoader(void *bootLoaderPtr, const char *commandLine);
void setInitialRamdiskURLVZLinuxBootLoader(void *bootLoaderPtr, const char *ramdiskPath);

/* VirtualMachineConfiguration */
bool validateVZVirtualMachineConfiguration(void *config, void **error);
void *newVZVirtualMachineConfiguration(void *bootLoader,
                                       unsigned int CPUCount,
                                       unsigned long long memorySize);
void setEntropyDevicesVZVirtualMachineConfiguration(void *config,
                                                    void *entropyDevices);
void setMemoryBalloonDevicesVZVirtualMachineConfiguration(void *config,
                                                          void *memoryBalloonDevices);
void setNetworkDevicesVZVirtualMachineConfiguration(void *config,
                                                    void *networkDevices);
void setSerialPortsVZVirtualMachineConfiguration(void *config,
                                                 void *serialPorts);
void setSocketDevicesVZVirtualMachineConfiguration(void *config,
                                                   void *socketDevices);
void setStorageDevicesVZVirtualMachineConfiguration(void *config,
                                                    void *storageDevices);
void setDirectorySharingDevicesVZVirtualMachineConfiguration(void *config, void *directorySharingDevices);

/* Configurations */
void *newVZFileHandleSerialPortAttachment(int readFileDescriptor, int writeFileDescriptor);
void *newVZFileSerialPortAttachment(const char *filePath, bool shouldAppend, void **error);
void *newVZVirtioConsoleDeviceSerialPortConfiguration(void *attachment);
void *newVZBridgedNetworkDeviceAttachment(void *networkInterface);
void *newVZNATNetworkDeviceAttachment(void);
void *newVZFileHandleNetworkDeviceAttachment(int fileDescriptor);
void *newVZVirtioNetworkDeviceConfiguration(void *attachment);
void setNetworkDevicesVZMACAddress(void *config, void *macAddress);
void *newVZVirtioEntropyDeviceConfiguration(void);
void *newVZVirtioBlockDeviceConfiguration(void *attachment);
void *newVZDiskImageStorageDeviceAttachment(const char *diskPath, bool readOnly, void **error);
void *newVZVirtioTraditionalMemoryBalloonDeviceConfiguration();
void *newVZVirtioSocketDeviceConfiguration();
void *newVZMACAddress(const char *macAddress);
void *newRandomLocallyAdministeredVZMACAddress();
const char *getVZMACAddressString(void *macAddress);
void *newVZVirtioSocketListener();
void *newVZSharedDirectory(const char *dirPath, bool readOnly);
void *newVZSingleDirectoryShare(void *sharedDirectory);
void *newVZMultipleDirectoryShare(void *sharedDirectories);
void *newVZVirtioFileSystemDeviceConfiguration(const char *tag);
void setVZVirtioFileSystemDeviceConfigurationShare(void *config, void *share);
void *VZVirtualMachine_socketDevices(void *machine);
void VZVirtioSocketDevice_setSocketListenerForPort(void *socketDevice, void *vmQueue, void *listener, uint32_t port);
void VZVirtioSocketDevice_removeSocketListenerForPort(void *socketDevice, void *vmQueue, uint32_t port);
void VZVirtioSocketDevice_connectToPort(void *socketDevice, void *vmQueue, uint32_t port, const char *socketDeviceID);

/* VirtualMachine */
void *newVZVirtualMachineWithDispatchQueue(void *config, void *queue, const char *vmid);
bool requestStopVirtualMachine(void *machine, void *queue, void **error);
void startWithCompletionHandler(void *machine, void *queue, const char *vmid);
void pauseWithCompletionHandler(void *machine, void *queue, const char *vmid);
void resumeWithCompletionHandler(void *machine, void *queue, const char *vmid);
bool vmCanStart(void *machine, void *queue);
bool vmCanPause(void *machine, void *queue);
bool vmCanResume(void *machine, void *queue);
bool vmCanRequestStop(void *machine, void *queue);

void *makeDispatchQueue(const char *label);

/* VZVirtioSocketConnection */
typedef struct VZVirtioSocketConnectionFlat
{
    uint32_t destinationPort;
    uint32_t sourcePort;
    int fileDescriptor;
} VZVirtioSocketConnectionFlat;

VZVirtioSocketConnectionFlat convertVZVirtioSocketConnection2Flat(void *connection);

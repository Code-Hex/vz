//
//  virtualization.m
//
//  Created by codehex.
//

#import "virtualization.h"
#import "virtualization_view.h"

char *copyCString(NSString *nss)
{
    const char *cc = [nss UTF8String];
    char *c = calloc([nss length] + 1, 1);
    strncpy(c, cc, [nss length]);
    return c;
}

@implementation Observer
- (void)observeValueForKeyPath:(NSString *)keyPath ofObject:(id)object change:(NSDictionary *)change context:(void *)context;
{

    @autoreleasepool {
        if ([keyPath isEqualToString:@"state"]) {
            int newState = (int)[change[NSKeyValueChangeNewKey] integerValue];
            changeStateOnObserver(newState, context);
        } else {
            // bool canVal = (bool)[change[NSKeyValueChangeNewKey] boolValue];
            // char *vmid = copyCString((NSString *)context);
            // char *key = copyCString(keyPath);
            // changeCanPropertyOnObserver(canVal, vmid, key);
            // free(vmid);
            // free(key);
        }
    }
}
@end

@implementation VZVirtioSocketListenerDelegateImpl
- (BOOL)listener:(VZVirtioSocketListener *)listener shouldAcceptNewConnection:(VZVirtioSocketConnection *)connection fromSocketDevice:(VZVirtioSocketDevice *)socketDevice;
{
    return (BOOL)shouldAcceptNewConnectionHandler(listener, connection, socketDevice);
}
@end

/*!
 @abstract Create a VZLinuxBootLoader with the Linux kernel passed as URL.
 @param kernelPath  Path of Linux kernel on the local file system.
*/
void *newVZLinuxBootLoader(const char *kernelPath)
{
    VZLinuxBootLoader *ret;
    @autoreleasepool {
        NSString *kernelPathNSString = [NSString stringWithUTF8String:kernelPath];
        NSURL *kernelURL = [NSURL fileURLWithPath:kernelPathNSString];
        ret = [[VZLinuxBootLoader alloc] initWithKernelURL:kernelURL];
    }
    return ret;
}

/*!
 @abstract Set the command-line parameters.
 @param bootLoader VZLinuxBootLoader
 @param commandLine The command-line parameters passed to the kernel on boot.
 @link https://www.kernel.org/doc/html/latest/admin-guide/kernel-parameters.html
 */
void setCommandLineVZLinuxBootLoader(void *bootLoaderPtr, const char *commandLine)
{
    VZLinuxBootLoader *bootLoader = (VZLinuxBootLoader *)bootLoaderPtr;
    @autoreleasepool {
        NSString *commandLineNSString = [NSString stringWithUTF8String:commandLine];
        [bootLoader setCommandLine:commandLineNSString];
    }
}

/*!
 @abstract Set the optional initial RAM disk.
 @param bootLoader VZLinuxBootLoader
 @param ramdiskPath The RAM disk is mapped into memory before booting the kernel.
 @link https://www.kernel.org/doc/html/latest/admin-guide/kernel-parameters.html
 */
void setInitialRamdiskURLVZLinuxBootLoader(void *bootLoaderPtr, const char *ramdiskPath)
{
    VZLinuxBootLoader *bootLoader = (VZLinuxBootLoader *)bootLoaderPtr;
    @autoreleasepool {
        NSString *ramdiskPathNSString = [NSString stringWithUTF8String:ramdiskPath];
        NSURL *ramdiskURL = [NSURL fileURLWithPath:ramdiskPathNSString];
        [bootLoader setInitialRamdiskURL:ramdiskURL];
    }
}

/*!
 @abstract Validate the configuration.
 @param config  Virtual machine configuration.
 @param error If not nil, assigned with the validation error if the validation failed.
 @return true if the configuration is valid.
 */
bool validateVZVirtualMachineConfiguration(void *config, void **error)
{
    return (bool)[(VZVirtualMachineConfiguration *)config
        validateWithError:(NSError *_Nullable *_Nullable)error];
}

/*!
 @abstract: Minimum amount of memory required by virtual machines.
 @see VZVirtualMachineConfiguration.memorySize
 */
unsigned long long minimumAllowedMemorySizeVZVirtualMachineConfiguration()
{
    return (unsigned long long)[VZVirtualMachineConfiguration minimumAllowedMemorySize];
}

/*!
 @abstract: Maximum amount of memory allowed for a virtual machine.
 @see VZVirtualMachineConfiguration.memorySize
 */
unsigned long long maximumAllowedMemorySizeVZVirtualMachineConfiguration()
{
    return (unsigned long long)[VZVirtualMachineConfiguration maximumAllowedMemorySize];
}

/*!
 @abstract: Minimum number of CPUs for a virtual machine.
 @see VZVirtualMachineConfiguration.CPUCount
 */
unsigned int minimumAllowedCPUCountVZVirtualMachineConfiguration()
{
    return (unsigned int)[VZVirtualMachineConfiguration minimumAllowedCPUCount];
}

/*!
 @abstract: Maximum number of CPUs for a virtual machine.
 @see VZVirtualMachineConfiguration.CPUCount
 */
unsigned int maximumAllowedCPUCountVZVirtualMachineConfiguration()
{
    return (unsigned int)[VZVirtualMachineConfiguration maximumAllowedCPUCount];
}

/*!
 @abstract Create a new Virtual machine configuration.
 @param bootLoader Boot loader used when the virtual machine starts.

 @param CPUCount Number of CPUs.
 @discussion
    The number of CPUs must be a value between VZVirtualMachineConfiguration.minimumAllowedCPUCount
    and VZVirtualMachineConfiguration.maximumAllowedCPUCount.

 @see VZVirtualMachineConfiguration.minimumAllowedCPUCount
 @see VZVirtualMachineConfiguration.maximumAllowedCPUCount

 @param memorySize Virtual machine memory size in bytes.
 @discussion
    The memory size must be a multiple of a 1 megabyte (1024 * 1024 bytes) between VZVirtualMachineConfiguration.minimumAllowedMemorySize
    and VZVirtualMachineConfiguration.maximumAllowedMemorySize.

    The memorySize represents the total physical memory seen by a guest OS running in the virtual machine.
    Not all memory is allocated on start, the virtual machine allocates memory on demand.
 @see VZVirtualMachineConfiguration.minimumAllowedMemorySize
 @see VZVirtualMachineConfiguration.maximumAllowedMemorySize
 */
void *newVZVirtualMachineConfiguration(void *bootLoaderPtr,
    unsigned int CPUCount,
    unsigned long long memorySize)
{
    VZVirtualMachineConfiguration *config = [[VZVirtualMachineConfiguration alloc] init];
    [config setBootLoader:(VZLinuxBootLoader *)bootLoaderPtr];
    [config setCPUCount:(NSUInteger)CPUCount];
    [config setMemorySize:memorySize];
    return config;
}

/*!
 @abstract List of entropy devices. Empty by default.
 @see VZVirtioEntropyDeviceConfiguration
*/
void setEntropyDevicesVZVirtualMachineConfiguration(void *config,
    void *entropyDevices)
{
    [(VZVirtualMachineConfiguration *)config setEntropyDevices:[(NSMutableArray *)entropyDevices copy]];
}

/*!
 @abstract List of memory balloon devices. Empty by default.
 @see VZVirtioTraditionalMemoryBalloonDeviceConfiguration
*/
void setMemoryBalloonDevicesVZVirtualMachineConfiguration(void *config,
    void *memoryBalloonDevices)
{
    [(VZVirtualMachineConfiguration *)config setMemoryBalloonDevices:[(NSMutableArray *)memoryBalloonDevices copy]];
}

/*!
 @abstract List of network adapters. Empty by default.
 @see VZVirtioNetworkDeviceConfiguration
 */
void setNetworkDevicesVZVirtualMachineConfiguration(void *config,
    void *networkDevices)
{
    [(VZVirtualMachineConfiguration *)config setNetworkDevices:[(NSMutableArray *)networkDevices copy]];
}

/*!
 @abstract List of serial ports. Empty by default.
 @see VZVirtioConsoleDeviceSerialPortConfiguration
 */
void setSerialPortsVZVirtualMachineConfiguration(void *config,
    void *serialPorts)
{
    [(VZVirtualMachineConfiguration *)config setSerialPorts:[(NSMutableArray *)serialPorts copy]];
}

/*!
 @abstract List of socket devices. Empty by default.
 @see VZVirtioSocketDeviceConfiguration
 */
void setSocketDevicesVZVirtualMachineConfiguration(void *config,
    void *socketDevices)
{
    [(VZVirtualMachineConfiguration *)config setSocketDevices:[(NSMutableArray *)socketDevices copy]];
}

/*!
 @abstract List of disk devices. Empty by default.
 @see VZVirtioBlockDeviceConfiguration
 */
void setStorageDevicesVZVirtualMachineConfiguration(void *config,
    void *storageDevices)
{
    [(VZVirtualMachineConfiguration *)config setStorageDevices:[(NSMutableArray *)storageDevices copy]];
}
/*!
 @abstract List of directory sharing devices. Empty by default.
 @see VZDirectorySharingDeviceConfiguration
 */
void setDirectorySharingDevicesVZVirtualMachineConfiguration(void *config, void *directorySharingDevices)
{
    [(VZVirtualMachineConfiguration *)config setDirectorySharingDevices:[(NSMutableArray *)directorySharingDevices copy]];
}

/*!
 @abstract The hardware platform to use.
 @discussion
    Can be an instance of a VZGenericPlatformConfiguration or VZMacPlatformConfiguration. Defaults to VZGenericPlatformConfiguration.
 */
void setPlatformVZVirtualMachineConfiguration(void *config, void *platform)
{
    [(VZVirtualMachineConfiguration *)config setPlatform:(VZPlatformConfiguration *)platform];
}

/*!
 @abstract List of graphics devices. Empty by default.
 @see VZMacGraphicsDeviceConfiguration
 */
void setGraphicsDevicesVZVirtualMachineConfiguration(void *config, void *graphicsDevices)
{
    [(VZVirtualMachineConfiguration *)config setGraphicsDevices:[(NSMutableArray *)graphicsDevices copy]];
}

/*!
 @abstract List of pointing devices. Empty by default.
 @see VZUSBScreenCoordinatePointingDeviceConfiguration
 */
void setPointingDevicesVZVirtualMachineConfiguration(void *config, void *pointingDevices)
{
    [(VZVirtualMachineConfiguration *)config setPointingDevices:[(NSMutableArray *)pointingDevices copy]];
}

/*!
 @abstract List of keyboards. Empty by default.
 @see VZUSBKeyboardConfiguration
 */
void setKeyboardsVZVirtualMachineConfiguration(void *config, void *keyboards)
{
    [(VZVirtualMachineConfiguration *)config setKeyboards:[(NSMutableArray *)keyboards copy]];
}

/*!
 @abstract List of audio devices. Empty by default.
 @see VZVirtioSoundDeviceConfiguration
 */
void setAudioDevicesVZVirtualMachineConfiguration(void *config, void *audioDevices)
{
    [(VZVirtualMachineConfiguration *)config setAudioDevices:[(NSMutableArray *)audioDevices copy]];
}

/*!
 @abstract Initialize a new Virtio Sound Device Configuration.
 @discussion The device exposes a source or destination of sound.
 */
void *newVZVirtioSoundDeviceConfiguration()
{
    return [[VZVirtioSoundDeviceConfiguration alloc] init];
}

/*!
 @abstract Set the list of audio streams exposed by this device. Empty by default.
*/
void setStreamsVZVirtioSoundDeviceConfiguration(void *audioDeviceConfiguration, void *streams)
{
    [(VZVirtioSoundDeviceConfiguration *)audioDeviceConfiguration setStreams:[(NSMutableArray *)streams copy]];
}

/*!
 @abstract Initialize a new Virtio Sound Device Input Stream Configuration.
 @discussion A PCM stream of input audio data, such as from a microphone.
 */
void *newVZVirtioSoundDeviceInputStreamConfiguration()
{
    return [[VZVirtioSoundDeviceInputStreamConfiguration alloc] init];
}

/*!
 @abstract Initialize a new Virtio Sound Device Host Audio Input Stream Configuration.
 */
void *newVZVirtioSoundDeviceHostInputStreamConfiguration()
{
    VZVirtioSoundDeviceInputStreamConfiguration *inputStream = (VZVirtioSoundDeviceInputStreamConfiguration *)newVZVirtioSoundDeviceInputStreamConfiguration();
    [inputStream setSource:[[VZHostAudioInputStreamSource alloc] init]];
    return inputStream;
}

/*!
 @abstract Initialize a new Virtio Sound Device Output Stream Configuration.
 @discussion A PCM stream of output audio data, such as to a speaker.
 */
void *newVZVirtioSoundDeviceOutputStreamConfiguration()
{
    return [[VZVirtioSoundDeviceOutputStreamConfiguration alloc] init];
}

/*!
 @abstract Initialize a new Virtio Sound Device Host Audio Output Stream Configuration.
 */
void *newVZVirtioSoundDeviceHostOutputStreamConfiguration()
{
    VZVirtioSoundDeviceOutputStreamConfiguration *outputStream = (VZVirtioSoundDeviceOutputStreamConfiguration *)newVZVirtioSoundDeviceOutputStreamConfiguration();
    [outputStream setSink:[[VZHostAudioOutputStreamSink alloc] init]];
    return outputStream;
}

/*!
 @abstract The platform configuration for a generic Intel or ARM virtual machine.
*/
void *newVZGenericPlatformConfiguration()
{
    return [[VZGenericPlatformConfiguration alloc] init];
}

/*!
 @abstract Intialize the VZFileHandleSerialPortAttachment from file descriptors.
 @param readFileDescriptor File descriptor for reading from the file.
 @param writeFileDescriptor File descriptor for writing to the file.
 @discussion
    Each file descriptor must a valid.
*/
void *newVZFileHandleSerialPortAttachment(int readFileDescriptor, int writeFileDescriptor)
{
    VZFileHandleSerialPortAttachment *ret;
    @autoreleasepool {
        NSFileHandle *fileHandleForReading = [[NSFileHandle alloc] initWithFileDescriptor:readFileDescriptor];
        NSFileHandle *fileHandleForWriting = [[NSFileHandle alloc] initWithFileDescriptor:writeFileDescriptor];
        ret = [[VZFileHandleSerialPortAttachment alloc]
            initWithFileHandleForReading:fileHandleForReading
                    fileHandleForWriting:fileHandleForWriting];
    }
    return ret;
}

/*!
 @abstract Initialize the VZFileSerialPortAttachment from a URL of a file.
 @param filePath The path of the file for the attachment on the local file system.
 @param shouldAppend True if the file should be opened in append mode, false otherwise.
        When a file is opened in append mode, writing to that file will append to the end of it.
 @param error If not nil, used to report errors if initialization fails.
 @return A VZFileSerialPortAttachment on success. Nil otherwise and the error parameter is populated if set.
 */
void *newVZFileSerialPortAttachment(const char *filePath, bool shouldAppend, void **error)
{
    VZFileSerialPortAttachment *ret;
    @autoreleasepool {
        NSString *filePathNSString = [NSString stringWithUTF8String:filePath];
        NSURL *fileURL = [NSURL fileURLWithPath:filePathNSString];
        ret = [[VZFileSerialPortAttachment alloc]
            initWithURL:fileURL
                 append:(BOOL)shouldAppend
                  error:(NSError *_Nullable *_Nullable)error];
    }
    return ret;
}

/*!
 @abstract Create a new Virtio Console Serial Port Device configuration
 @param attachment Base class for a serial port attachment.
 @discussion
    The device creates a console which enables communication between the host and the guest through the Virtio interface.

    The device sets up a single port on the Virtio console device.
 */
void *newVZVirtioConsoleDeviceSerialPortConfiguration(void *attachment)
{
    VZVirtioConsoleDeviceSerialPortConfiguration *config = [[VZVirtioConsoleDeviceSerialPortConfiguration alloc] init];
    [config setAttachment:(VZSerialPortAttachment *)attachment];
    return config;
}

/*!
 @abstract Create a new Network device attachment bridging a host physical interface with a virtual network device.
 @param networkInterface a network interface that bridges a physical interface.
 @discussion
    A bridged network allows the virtual machine to use the same physical interface as the host. Both host and virtual machine
    send and receive packets on the same physical interface but have distinct network layers.

    The bridge network device attachment is used with a VZNetworkDeviceConfiguration to define a virtual network device.

    Using a VZBridgedNetworkDeviceAttachment requires the app to have the "com.apple.vm.networking" entitlement.

 @see VZBridgedNetworkInterface
 @see VZNetworkDeviceConfiguration
 @see VZVirtioNetworkDeviceConfiguration
 */
void *newVZBridgedNetworkDeviceAttachment(void *networkInterface)
{
    return [[VZBridgedNetworkDeviceAttachment alloc] initWithInterface:(VZBridgedNetworkInterface *)networkInterface];
}

/*!
 @abstract Create a new Network device attachment using network address translation (NAT) with outside networks.
 @discussion
    Using the NAT attachment type, the host serves as router and performs network address translation for accesses to outside networks.

 @see VZNetworkDeviceConfiguration
 @see VZVirtioNetworkDeviceConfiguration
 */
void *newVZNATNetworkDeviceAttachment()
{
    return [[VZNATNetworkDeviceAttachment alloc] init];
}

/*!
 @abstract Create a new Network device attachment sending raw network packets over a file handle.
 @discussion
    The file handle attachment transmits the raw packets/frames between the virtual network interface and a file handle.
    The data transmitted through this attachment is at the level of the data link layer.

    The file handle must hold a connected datagram socket.

 @see VZNetworkDeviceConfiguration
 @see VZVirtioNetworkDeviceConfiguration
 */
void *newVZFileHandleNetworkDeviceAttachment(int fileDescriptor)
{
    VZFileHandleNetworkDeviceAttachment *ret;
    @autoreleasepool {
        NSFileHandle *fileHandle = [[NSFileHandle alloc] initWithFileDescriptor:fileDescriptor];
        ret = [[VZFileHandleNetworkDeviceAttachment alloc] initWithFileHandle:fileHandle];
    }
    return ret;
}

/*!
 @abstract Create  a new Configuration of a paravirtualized network device of type Virtio Network Device.
 @discussion
    The communication channel used on the host is defined through the attachment. It is set with the VZNetworkDeviceConfiguration.attachment property.

    The configuration is only valid with valid MACAddress and attachment.

 @see VZVirtualMachineConfiguration.networkDevices

 @param attachment  Base class for a network device attachment.
 @discussion
    A network device attachment defines how a virtual network device interfaces with the host system.

    VZNetworkDeviceAttachment should not be instantiated directly. One of its subclasses should be used instead.

    Common attachment types include:
    - VZNATNetworkDeviceAttachment
    - VZFileHandleNetworkDeviceAttachment

 @see VZBridgedNetworkDeviceAttachment
 @see VZFileHandleNetworkDeviceAttachment
 @see VZNATNetworkDeviceAttachment
 */
void *newVZVirtioNetworkDeviceConfiguration(void *attachment)
{
    VZVirtioNetworkDeviceConfiguration *config = [[VZVirtioNetworkDeviceConfiguration alloc] init];
    [config setAttachment:(VZNetworkDeviceAttachment *)attachment];
    return config;
}

/*!
 @abstract Create a new Virtio Entropy Device confiuration
 @discussion The device exposes a source of entropy for the guest's random number generator.
*/
void *newVZVirtioEntropyDeviceConfiguration()
{
    return [[VZVirtioEntropyDeviceConfiguration alloc] init];
}

/*!
 @abstract Initialize a VZVirtioBlockDeviceConfiguration with a device attachment.
 @param attachment The storage device attachment. This defines how the virtualized device operates on the host side.
 @see VZDiskImageStorageDeviceAttachment
 */
void *newVZVirtioBlockDeviceConfiguration(void *attachment)
{
    return [[VZVirtioBlockDeviceConfiguration alloc] initWithAttachment:(VZStorageDeviceAttachment *)attachment];
}

/*!
 @abstract Initialize the attachment from a local file url.
 @param diskPath Local file path to the disk image in RAW format.
 @param readOnly If YES, the device attachment is read-only, otherwise the device can write data to the disk image.
 @param error If not nil, assigned with the error if the initialization failed.
 @return A VZDiskImageStorageDeviceAttachment on success. Nil otherwise and the error parameter is populated if set.
 */
void *newVZDiskImageStorageDeviceAttachment(const char *diskPath, bool readOnly, void **error)
{
    NSString *diskPathNSString = [NSString stringWithUTF8String:diskPath];
    NSURL *diskURL = [NSURL fileURLWithPath:diskPathNSString];
    return [[VZDiskImageStorageDeviceAttachment alloc]
        initWithURL:diskURL
           readOnly:(BOOL)readOnly
              error:(NSError *_Nullable *_Nullable)error];
}

/*!
 @abstract Create a configuration of the Virtio traditional memory balloon device.
 @discussion
    This configuration creates a Virtio traditional memory balloon device which allows for managing guest memory.
    Only one Virtio traditional memory balloon device can be used per virtual machine.
 @see VZVirtioTraditionalMemoryBalloonDevice
 */
void *newVZVirtioTraditionalMemoryBalloonDeviceConfiguration()
{
    return [[VZVirtioTraditionalMemoryBalloonDeviceConfiguration alloc] init];
}

/*!
 @abstract Create a configuration of the Virtio socket device.
 @discussion
    This configuration creates a Virtio socket device for the guest which communicates with the host through the Virtio interface.

    Only one Virtio socket device can be used per virtual machine.
 @see VZVirtioSocketDevice
 */
void *newVZVirtioSocketDeviceConfiguration()
{
    return [[VZVirtioSocketDeviceConfiguration alloc] init];
}

/*!
 @abstract The VZVirtioSocketListener object represents a listener for the Virtio socket device.
 @discussion
    The listener encompasses a VZVirtioSocketListenerDelegate object.
    VZVirtioSocketListener is used with VZVirtioSocketDevice to listen to a particular port.
    The delegate is used when a guest connects to a port associated with the listener.
 @see VZVirtioSocketDevice
 @see VZVirtioSocketListenerDelegate
 */
void *newVZVirtioSocketListener()
{
    VZVirtioSocketListener *ret = [[VZVirtioSocketListener alloc] init];
    [ret setDelegate:[[VZVirtioSocketListenerDelegateImpl alloc] init]];
    return ret;
}

/*!
 @abstract Sets a listener at a specified port.
 @discussion
    There is only one listener per port, any existing listener will be removed, and the specified listener here will be set instead.
    The same listener can be registered on multiple ports.
    The listener's delegate will be called whenever the guest connects to that port.
 @param listener The VZVirtioSocketListener object to be set.
 @param port The port number to set the listener at.
 */
void VZVirtioSocketDevice_setSocketListenerForPort(void *socketDevice, void *vmQueue, void *listener, uint32_t port)
{
    dispatch_sync((dispatch_queue_t)vmQueue, ^{
        [(VZVirtioSocketDevice *)socketDevice setSocketListener:(VZVirtioSocketListener *)listener forPort:port];
    });
}

/*!
 @abstract Removes the listener at a specfied port.
 @discussion Does nothing if the port had no listener.
 @param port The port number at which the listener is to be removed.
 */
void VZVirtioSocketDevice_removeSocketListenerForPort(void *socketDevice, void *vmQueue, uint32_t port)
{
    dispatch_sync((dispatch_queue_t)vmQueue, ^{
        [(VZVirtioSocketDevice *)socketDevice removeSocketListenerForPort:port];
    });
}

/*!
 @abstract Connects to a specified port.
 @discussion Does nothing if the guest does not listen on that port.
 @param port The port number to connect to.
 @param completionHandler Block called after the connection has been successfully established or on error.
    The error parameter passed to the block is nil if the connection was successful.
 */
void VZVirtioSocketDevice_connectToPort(void *socketDevice, void *vmQueue, uint32_t port, void *cgoHandlerPtr)
{
    dispatch_sync((dispatch_queue_t)vmQueue, ^{
        [(VZVirtioSocketDevice *)socketDevice connectToPort:port
                                          completionHandler:^(VZVirtioSocketConnection *connection, NSError *err) {
                                              connectionHandler(connection, err, cgoHandlerPtr);
                                          }];
    });
}

VZVirtioSocketConnectionFlat convertVZVirtioSocketConnection2Flat(void *connection)
{
    VZVirtioSocketConnectionFlat ret;
    ret.sourcePort = [(VZVirtioSocketConnection *)connection sourcePort];
    ret.destinationPort = [(VZVirtioSocketConnection *)connection destinationPort];
    ret.fileDescriptor = [(VZVirtioSocketConnection *)connection fileDescriptor];
    return ret;
}

/*!
 @abstract Initialize the virtual machine.
 @param config The configuration of the virtual machine.
    The configuration must be valid. Validation can be performed at runtime with [VZVirtualMachineConfiguration validateWithError:].
    The configuration is copied by the initializer.
 @param queue The serial queue on which the virtual machine operates.
    Every operation on the virtual machine must be done on that queue. The callbacks and delegate methods are invoked on that queue.
    If the queue is not serial, the behavior is undefined.
 */
void *newVZVirtualMachineWithDispatchQueue(void *config, void *queue, void *statusHandler)
{
    VZVirtualMachine *vm = [[VZVirtualMachine alloc]
        initWithConfiguration:(VZVirtualMachineConfiguration *)config
                        queue:(dispatch_queue_t)queue];
    @autoreleasepool {
        Observer *o = [[Observer alloc] init];
        [vm addObserver:o
             forKeyPath:@"state"
                options:NSKeyValueObservingOptionNew
                context:statusHandler];
    }
    return vm;
}

/*!
 @abstract Return the list of socket devices configured on this virtual machine. Return an empty array if no socket device is configured.
 @see VZVirtioSocketDeviceConfiguration
 @see VZVirtualMachineConfiguration
 */
void *VZVirtualMachine_socketDevices(void *machine)
{
    return [(VZVirtualMachine *)machine socketDevices]; // NSArray<VZSocketDevice *>
}

/*!
 @abstract Initialize the VZMACAddress from a string representation of a MAC address.
 @param string
    The string should be formatted representing the 6 bytes in hexadecimal separated by a colon character.
        e.g. "01:23:45:ab:cd:ef"

    The alphabetical characters can appear lowercase or uppercase.
 @return A VZMACAddress or nil if the string is not formatted correctly.
 */
void *newVZMACAddress(const char *macAddress)
{
    VZMACAddress *ret;
    @autoreleasepool {
        NSString *str = [NSString stringWithUTF8String:macAddress];
        ret = [[VZMACAddress alloc] initWithString:str];
    }
    return ret;
}

/*!
 @abstract Create a valid, random, unicast, locally administered address.
 @discussion The generated address is not guaranteed to be unique.
 */
void *newRandomLocallyAdministeredVZMACAddress()
{
    return [VZMACAddress randomLocallyAdministeredAddress];
}

/*!
 @abstract Sets the media access control address of the device.
 */
void setNetworkDevicesVZMACAddress(void *config, void *macAddress)
{
    [(VZNetworkDeviceConfiguration *)config setMACAddress:[(VZMACAddress *)macAddress copy]];
}

/*!
 @abstract The address represented as a string.
 @discussion
    The 6 bytes are represented in hexadecimal form, separated by a colon character.
    Alphabetical characters are lowercase.

    The address is compatible with the parameter of -[VZMACAddress initWithString:].
 */
const char *getVZMACAddressString(void *macAddress)
{
    return [[(VZMACAddress *)macAddress string] UTF8String];
}

/*!
 @abstract Initialize the VZSharedDirectory from the directory path and read only option.
 @param dirPath
    The directory path that will be share.
 @param readOnly
    If the directory should be mounted read only.
 @return A VZSharedDirectory
 */
void *newVZSharedDirectory(const char *dirPath, bool readOnly)
{
    VZSharedDirectory *ret;
    @autoreleasepool {
        NSString *dirPathNSString = [NSString stringWithUTF8String:dirPath];
        NSURL *dirURL = [NSURL fileURLWithPath:dirPathNSString];
        ret = [[VZSharedDirectory alloc] initWithURL:dirURL readOnly:(BOOL)readOnly];
    }
    return ret;
}

/*!
 @abstract Initialize the VZSingleDirectoryShare from the shared directory.
 @param sharedDirectory
    The shared directory to use.
 @return A VZSingleDirectoryShare
 */
void *newVZSingleDirectoryShare(void *sharedDirectory)
{
    return [[VZSingleDirectoryShare alloc] initWithDirectory:(VZSharedDirectory *)sharedDirectory];
}

/*!
 @abstract Initialize the VZMultipleDirectoryShare from the shared directories.
 @param sharedDirectories
    NSDictionary mapping names to shared directories.
 @return A VZMultipleDirectoryShare
 */
void *newVZMultipleDirectoryShare(void *sharedDirectories)
{
    return [[VZMultipleDirectoryShare alloc] initWithDirectories:(NSDictionary<NSString *, VZSharedDirectory *> *)sharedDirectories];
}

/*!
 @abstract Initialize the VZVirtioFileSystemDeviceConfiguration from the fs tag.
 @param tag
    The tag to use for this device configuration.
 @return A VZVirtioFileSystemDeviceConfiguration
 */
void *newVZVirtioFileSystemDeviceConfiguration(const char *tag)
{
    VZVirtioFileSystemDeviceConfiguration *ret;
    @autoreleasepool {
        NSString *tagNSString = [NSString stringWithUTF8String:tag];
        ret = [[VZVirtioFileSystemDeviceConfiguration alloc] initWithTag:tagNSString];
    }
    return ret;
}

/*!
 @abstract Sets share associated with this configuration.
 */
void setVZVirtioFileSystemDeviceConfigurationShare(void *config, void *share)
{
    [(VZVirtioFileSystemDeviceConfiguration *)config setShare:(VZDirectoryShare *)share];
}

/*!
 @abstract Initialize a new configuration for a USB pointing device that reports absolute coordinates.
 @discussion This device can be used by VZVirtualMachineView to send pointer events to the virtual machine.
 */
void *newVZUSBScreenCoordinatePointingDeviceConfiguration()
{
    return [[VZUSBScreenCoordinatePointingDeviceConfiguration alloc] init];
}

/*!
 @abstract Initialize a new configuration for a USB keyboard.
 @discussion This device can be used by VZVirtualMachineView to send key events to the virtual machine.
 */
void *newVZUSBKeyboardConfiguration()
{
    return [[VZUSBKeyboardConfiguration alloc] init];
}

/*!
 @abstract Request that the guest turns itself off.
 @param error If not nil, assigned with the error if the request failed.
 @return YES if the request was made successfully.
 */
bool requestStopVirtualMachine(void *machine, void *queue, void **error)
{
    __block BOOL ret;
    dispatch_sync((dispatch_queue_t)queue, ^{
        ret = [(VZVirtualMachine *)machine requestStopWithError:(NSError *_Nullable *_Nullable)error];
    });
    return (bool)ret;
}

void *makeDispatchQueue(const char *label)
{
    // dispatch_queue_attr_t attr = dispatch_queue_attr_make_with_qos_class(DISPATCH_QUEUE_SERIAL, QOS_CLASS_DEFAULT, 0);
    dispatch_queue_t queue = dispatch_queue_create(label, DISPATCH_QUEUE_SERIAL);
    // dispatch_retain(queue);
    return queue;
}

typedef void (^handler_t)(NSError *);

handler_t generateHandler(void handler(void *, char *))
{
    handler_t ret;
    @autoreleasepool {
        ret = Block_copy(^(NSError *err){
            virtualMachineCompletionHandler(handler, err);
        });
    }
    return ret;
}

void startWithCompletionHandler(void *machine, void *queue, void *completionHandler)
{
    handler_t handler = generateHandler(completionHandler);
    dispatch_sync((dispatch_queue_t)queue, ^{
        [(VZVirtualMachine *)machine startWithCompletionHandler:handler];
    });
    Block_release(handler);
}

void pauseWithCompletionHandler(void *machine, void *queue, void *completionHandler)
{
    handler_t handler = generateHandler(completionHandler);
    dispatch_sync((dispatch_queue_t)queue, ^{
        [(VZVirtualMachine *)machine pauseWithCompletionHandler:handler];
    });
    Block_release(handler);
}

void resumeWithCompletionHandler(void *machine, void *queue, void *completionHandler)
{
    handler_t handler = generateHandler(completionHandler);
    dispatch_sync((dispatch_queue_t)queue, ^{
        [(VZVirtualMachine *)machine resumeWithCompletionHandler:handler];
    });
    Block_release(handler);
}

void stopWithCompletionHandler(void *machine, void *queue, void *completionHandler)
{
    handler_t handler = generateHandler(completionHandler);
    dispatch_sync((dispatch_queue_t)queue, ^{
        [(VZVirtualMachine *)machine stopWithCompletionHandler:handler];
    });
    Block_release(handler);
}

// TODO(codehex): use KVO
bool vmCanStart(void *machine, void *queue)
{
    __block BOOL result;
    dispatch_sync((dispatch_queue_t)queue, ^{
        result = ((VZVirtualMachine *)machine).canStart;
    });
    return (bool)result;
}

bool vmCanPause(void *machine, void *queue)
{
    __block BOOL result;
    dispatch_sync((dispatch_queue_t)queue, ^{
        result = ((VZVirtualMachine *)machine).canPause;
    });
    return (bool)result;
}

bool vmCanResume(void *machine, void *queue)
{
    __block BOOL result;
    dispatch_sync((dispatch_queue_t)queue, ^{
        result = ((VZVirtualMachine *)machine).canResume;
    });
    return (bool)result;
}

bool vmCanRequestStop(void *machine, void *queue)
{
    __block BOOL result;
    dispatch_sync((dispatch_queue_t)queue, ^{
        result = ((VZVirtualMachine *)machine).canRequestStop;
    });
    return (bool)result;
}

bool vmCanStop(void *machine, void *queue)
{
    __block BOOL result;
    dispatch_sync((dispatch_queue_t)queue, ^{
        result = ((VZVirtualMachine *)machine).canStop;
    });
    return (bool)result;
}
// --- TODO end

void sharedApplication()
{
    // Create a shared app instance.
    // This will initialize the global variable
    // 'NSApp' with the application instance.
    [VZApplication sharedApplication];
}

void startVirtualMachineWindow(void *machine, double width, double height)
{
    @autoreleasepool {
        AppDelegate *appDelegate = [[[AppDelegate alloc]
            initWithVirtualMachine:(VZVirtualMachine *)machine
                       windowWidth:(CGFloat)width
                      windowHeight:(CGFloat)height] autorelease];

        NSApp.delegate = appDelegate;
        [NSApp run];
    }
}

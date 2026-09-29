import Foundation
import Virtualization
import Dispatch
import ObjectiveC
import Darwin

@c(vz_newVZLinuxBootLoader)
public func newVZLinuxBootLoader(_ path: CString) -> OwnedObject {
    bridgeSync {
        if #available(macOS 11.0, *) {
            return own(VZLinuxBootLoader(kernelURL: URL(fileURLWithPath: text(path))))
        }
        preconditionFailure("API requires macOS 11.0")
    }
}

@c(vz_setCommandLineVZLinuxBootLoader)
public func setCommandLineVZLinuxBootLoader(_ object: BorrowedObject, _ value: CString) {
    bridgeSync {
        if #available(macOS 11.0, *) {
            borrow(object, as: VZLinuxBootLoader.self).commandLine = text(value)
            return
        }
        preconditionFailure("API requires macOS 11.0")
    }
}

@c(vz_setInitialRamdiskURLVZLinuxBootLoader)
public func setInitialRamdiskURLVZLinuxBootLoader(_ object: BorrowedObject, _ value: CString) {
    bridgeSync {
        if #available(macOS 11.0, *) {
            borrow(object, as: VZLinuxBootLoader.self).initialRamdiskURL = URL(fileURLWithPath: text(value))
            return
        }
        preconditionFailure("API requires macOS 11.0")
    }
}

@c(vz_setVariableStoreVZEFIBootLoader)
public func setVariableStoreVZEFIBootLoader(_ object: BorrowedObject, _ value: BorrowedObject) {
    bridgeSync {
        if #available(macOS 13.0, *) {
            borrow(object, as: VZEFIBootLoader.self).variableStore = value.map { borrow($0, as: VZEFIVariableStore.self) }
            return
        }
        preconditionFailure("API requires macOS 13.0")
    }
}

@c(vz_newVZEFIVariableStorePath)
public func newVZEFIVariableStorePath(_ path: CString) -> OwnedObject {
    bridgeSync {
        if #available(macOS 13.0, *) {
            return own(VZEFIVariableStore(url: URL(fileURLWithPath: text(path))))
        }
        preconditionFailure("API requires macOS 13.0")
    }
}

@c(vz_newCreatingVZEFIVariableStoreAtPath)
public func newCreatingVZEFIVariableStoreAtPath(_ path: CString, _ errorOut: ErrorOut) -> OwnedObject {
    bridgeSync {
        if #available(macOS 13.0, *) {
            do {
            return own(try VZEFIVariableStore(creatingVariableStoreAt: URL(fileURLWithPath: text(path)), options: [.allowOverwrite]))
            } catch {
                fail(error, errorOut)
                return nil
            }
        }
        preconditionFailure("API requires macOS 13.0")
    }
}

@c(vz_newVZVirtualMachineConfiguration)
public func newVZVirtualMachineConfiguration(_ bootLoader: BorrowedObject, _ cpu: UInt32, _ memory: UInt64) -> OwnedObject {
    bridgeSync {
        if #available(macOS 11.0, *) {
            let config = VZVirtualMachineConfiguration()
            config.bootLoader = borrow(bootLoader, as: VZBootLoader.self)
            config.cpuCount = Int(cpu)
            config.memorySize = memory
            return own(config)
        }
        preconditionFailure("API requires macOS 11.0")
    }
}

@c(vz_validateVZVirtualMachineConfiguration)
public func validateVZVirtualMachineConfiguration(_ object: BorrowedObject, _ errorOut: ErrorOut) -> Bool {
    bridgeSync {
        if #available(macOS 11.0, *) {
            do {
            try borrow(object, as: VZVirtualMachineConfiguration.self).validate()
            return true
            } catch { fail(error, errorOut); return false }
        }
        preconditionFailure("API requires macOS 11.0")
    }
}

#if arch(arm64)
@c(vz_validateSaveRestoreSupportWithError)
public func validateSaveRestoreSupportWithError(_ object: BorrowedObject, _ errorOut: ErrorOut) -> Bool {
    bridgeSync {
        if #available(macOS 14.0, *) {
            do {
            try borrow(object, as: VZVirtualMachineConfiguration.self).validateSaveRestoreSupport()
            return true
            } catch { fail(error, errorOut); return false }
        }
        preconditionFailure("API requires macOS 14.0")
    }
}
#endif

@c(vz_setEntropyDevicesVZVirtualMachineConfiguration)
public func setEntropyDevicesVZVirtualMachineConfiguration(_ object: BorrowedObject, _ value: BorrowedObject) {
    bridgeSync {
        if #available(macOS 11.0, *) {
            borrow(object, as: VZVirtualMachineConfiguration.self).entropyDevices = borrow(value, as: NSArray.self) as! [VZEntropyDeviceConfiguration]
            return
        }
        preconditionFailure("API requires macOS 11.0")
    }
}

@c(vz_setMemoryBalloonDevicesVZVirtualMachineConfiguration)
public func setMemoryBalloonDevicesVZVirtualMachineConfiguration(_ object: BorrowedObject, _ value: BorrowedObject) {
    bridgeSync {
        if #available(macOS 11.0, *) {
            borrow(object, as: VZVirtualMachineConfiguration.self).memoryBalloonDevices = borrow(value, as: NSArray.self) as! [VZMemoryBalloonDeviceConfiguration]
            return
        }
        preconditionFailure("API requires macOS 11.0")
    }
}

@c(vz_setNetworkDevicesVZVirtualMachineConfiguration)
public func setNetworkDevicesVZVirtualMachineConfiguration(_ object: BorrowedObject, _ value: BorrowedObject) {
    bridgeSync {
        if #available(macOS 11.0, *) {
            borrow(object, as: VZVirtualMachineConfiguration.self).networkDevices = borrow(value, as: NSArray.self) as! [VZNetworkDeviceConfiguration]
            return
        }
        preconditionFailure("API requires macOS 11.0")
    }
}

@c(vz_setSerialPortsVZVirtualMachineConfiguration)
public func setSerialPortsVZVirtualMachineConfiguration(_ object: BorrowedObject, _ value: BorrowedObject) {
    bridgeSync {
        if #available(macOS 11.0, *) {
            borrow(object, as: VZVirtualMachineConfiguration.self).serialPorts = borrow(value, as: NSArray.self) as! [VZSerialPortConfiguration]
            return
        }
        preconditionFailure("API requires macOS 11.0")
    }
}

@c(vz_setSocketDevicesVZVirtualMachineConfiguration)
public func setSocketDevicesVZVirtualMachineConfiguration(_ object: BorrowedObject, _ value: BorrowedObject) {
    bridgeSync {
        if #available(macOS 11.0, *) {
            borrow(object, as: VZVirtualMachineConfiguration.self).socketDevices = borrow(value, as: NSArray.self) as! [VZSocketDeviceConfiguration]
            return
        }
        preconditionFailure("API requires macOS 11.0")
    }
}

@c(vz_setStorageDevicesVZVirtualMachineConfiguration)
public func setStorageDevicesVZVirtualMachineConfiguration(_ object: BorrowedObject, _ value: BorrowedObject) {
    bridgeSync {
        if #available(macOS 11.0, *) {
            borrow(object, as: VZVirtualMachineConfiguration.self).storageDevices = borrow(value, as: NSArray.self) as! [VZStorageDeviceConfiguration]
            return
        }
        preconditionFailure("API requires macOS 11.0")
    }
}

@c(vz_setDirectorySharingDevicesVZVirtualMachineConfiguration)
public func setDirectorySharingDevicesVZVirtualMachineConfiguration(_ object: BorrowedObject, _ value: BorrowedObject) {
    bridgeSync {
        if #available(macOS 12.0, *) {
            borrow(object, as: VZVirtualMachineConfiguration.self).directorySharingDevices = borrow(value, as: NSArray.self) as! [VZDirectorySharingDeviceConfiguration]
            return
        }
        preconditionFailure("API requires macOS 12.0")
    }
}

@c(vz_setGraphicsDevicesVZVirtualMachineConfiguration)
public func setGraphicsDevicesVZVirtualMachineConfiguration(_ object: BorrowedObject, _ value: BorrowedObject) {
    bridgeSync {
        if #available(macOS 12.0, *) {
            borrow(object, as: VZVirtualMachineConfiguration.self).graphicsDevices = borrow(value, as: NSArray.self) as! [VZGraphicsDeviceConfiguration]
            return
        }
        preconditionFailure("API requires macOS 12.0")
    }
}

@c(vz_setPointingDevicesVZVirtualMachineConfiguration)
public func setPointingDevicesVZVirtualMachineConfiguration(_ object: BorrowedObject, _ value: BorrowedObject) {
    bridgeSync {
        if #available(macOS 12.0, *) {
            borrow(object, as: VZVirtualMachineConfiguration.self).pointingDevices = borrow(value, as: NSArray.self) as! [VZPointingDeviceConfiguration]
            return
        }
        preconditionFailure("API requires macOS 12.0")
    }
}

@c(vz_setKeyboardsVZVirtualMachineConfiguration)
public func setKeyboardsVZVirtualMachineConfiguration(_ object: BorrowedObject, _ value: BorrowedObject) {
    bridgeSync {
        if #available(macOS 12.0, *) {
            borrow(object, as: VZVirtualMachineConfiguration.self).keyboards = borrow(value, as: NSArray.self) as! [VZKeyboardConfiguration]
            return
        }
        preconditionFailure("API requires macOS 12.0")
    }
}

@c(vz_setAudioDevicesVZVirtualMachineConfiguration)
public func setAudioDevicesVZVirtualMachineConfiguration(_ object: BorrowedObject, _ value: BorrowedObject) {
    bridgeSync {
        if #available(macOS 12.0, *) {
            borrow(object, as: VZVirtualMachineConfiguration.self).audioDevices = borrow(value, as: NSArray.self) as! [VZAudioDeviceConfiguration]
            return
        }
        preconditionFailure("API requires macOS 12.0")
    }
}

@c(vz_setConsoleDevicesVZVirtualMachineConfiguration)
public func setConsoleDevicesVZVirtualMachineConfiguration(_ object: BorrowedObject, _ value: BorrowedObject) {
    bridgeSync {
        if #available(macOS 13.0, *) {
            borrow(object, as: VZVirtualMachineConfiguration.self).consoleDevices = borrow(value, as: NSArray.self) as! [VZConsoleDeviceConfiguration]
            return
        }
        preconditionFailure("API requires macOS 13.0")
    }
}

@c(vz_setUSBControllersVZVirtualMachineConfiguration)
public func setUSBControllersVZVirtualMachineConfiguration(_ object: BorrowedObject, _ value: BorrowedObject) {
    bridgeSync {
        if #available(macOS 15.0, *) {
            borrow(object, as: VZVirtualMachineConfiguration.self).usbControllers = borrow(value, as: NSArray.self) as! [VZUSBControllerConfiguration]
            return
        }
        preconditionFailure("API requires macOS 15.0")
    }
}

@c(vz_setPlatformVZVirtualMachineConfiguration)
public func setPlatformVZVirtualMachineConfiguration(_ object: BorrowedObject, _ value: BorrowedObject) {
    bridgeSync {
        if #available(macOS 12.0, *) {
            borrow(object, as: VZVirtualMachineConfiguration.self).platform = borrow(value, as: VZPlatformConfiguration.self)
            return
        }
        preconditionFailure("API requires macOS 12.0")
    }
}

@c(vz_socketDevicesVZVirtualMachineConfiguration)
public func socketDevicesVZVirtualMachineConfiguration(_ object: BorrowedObject) -> OwnedObject {
    bridgeSync {
        if #available(macOS 11.0, *) {
            return own(borrow(object, as: VZVirtualMachineConfiguration.self).socketDevices as NSArray)
        }
        preconditionFailure("API requires macOS 11.0")
    }
}

@c(vz_newVZBridgedNetworkDeviceAttachment)
public func newVZBridgedNetworkDeviceAttachment(_ networkInterface: BorrowedObject) -> OwnedObject {
    bridgeSync {
        if #available(macOS 11.0, *) {
            return own(VZBridgedNetworkDeviceAttachment(interface: borrow(networkInterface, as: VZBridgedNetworkInterface.self)))
        }
        preconditionFailure("API requires macOS 11.0")
    }
}

@c(vz_VZBridgedNetworkInterface_networkInterfaces)
public func VZBridgedNetworkInterface_networkInterfaces() -> OwnedObject {
    bridgeSync {
        if #available(macOS 11.0, *) {
            return own(VZBridgedNetworkInterface.networkInterfaces as NSArray)
        }
        preconditionFailure("API requires macOS 11.0")
    }
}

@c(vz_VZBridgedNetworkInterface_identifier)
public func VZBridgedNetworkInterface_identifier(_ object: BorrowedObject) -> OwnedObject {
    bridgeSync {
        if #available(macOS 11.0, *) {
            return own(borrow(object, as: VZBridgedNetworkInterface.self).identifier as NSString)
        }
        preconditionFailure("API requires macOS 11.0")
    }
}

@c(vz_VZBridgedNetworkInterface_localizedDisplayName)
public func VZBridgedNetworkInterface_localizedDisplayName(_ object: BorrowedObject) -> OwnedObject {
    bridgeSync {
        if #available(macOS 11.0, *) {
            return own((borrow(object, as: VZBridgedNetworkInterface.self).localizedDisplayName ?? borrow(object, as: VZBridgedNetworkInterface.self).identifier) as NSString)
        }
        preconditionFailure("API requires macOS 11.0")
    }
}

@c(vz_newVZMACAddress)
public func newVZMACAddress(_ value: CString) -> OwnedObject {
    bridgeSync {
        if #available(macOS 11.0, *) {
            return own(VZMACAddress(string: text(value)))
        }
        preconditionFailure("API requires macOS 11.0")
    }
}

@c(vz_newRandomLocallyAdministeredVZMACAddress)
public func newRandomLocallyAdministeredVZMACAddress() -> OwnedObject {
    bridgeSync {
        if #available(macOS 11.0, *) {
            return own(VZMACAddress.randomLocallyAdministered())
        }
        preconditionFailure("API requires macOS 11.0")
    }
}

@c(vz_getVZMACAddressString)
public func getVZMACAddressString(_ object: BorrowedObject) -> OwnedObject {
    bridgeSync {
        if #available(macOS 11.0, *) {
            return own(borrow(object, as: VZMACAddress.self).string as NSString)
        }
        preconditionFailure("API requires macOS 11.0")
    }
}

@c(vz_setNetworkDevicesVZMACAddress)
public func setNetworkDevicesVZMACAddress(_ object: BorrowedObject, _ value: BorrowedObject) {
    bridgeSync {
        if #available(macOS 11.0, *) {
            borrow(object, as: VZNetworkDeviceConfiguration.self).macAddress = borrow(value, as: VZMACAddress.self)
            return
        }
        preconditionFailure("API requires macOS 11.0")
    }
}

@c(vz_newVZVirtioNetworkDeviceConfiguration)
public func newVZVirtioNetworkDeviceConfiguration(_ attachment: BorrowedObject) -> OwnedObject {
    bridgeSync {
        if #available(macOS 11.0, *) {
            let config = VZVirtioNetworkDeviceConfiguration()
            config.attachment = attachment.map { borrow($0, as: VZNetworkDeviceAttachment.self) }
            return own(config)
        }
        preconditionFailure("API requires macOS 11.0")
    }
}

@c(vz_newVZVirtioConsoleDeviceSerialPortConfiguration)
public func newVZVirtioConsoleDeviceSerialPortConfiguration(_ attachment: BorrowedObject) -> OwnedObject {
    bridgeSync {
        if #available(macOS 11.0, *) {
            let config = VZVirtioConsoleDeviceSerialPortConfiguration()
            config.attachment = attachment.map { borrow($0, as: VZSerialPortAttachment.self) }
            return own(config)
        }
        preconditionFailure("API requires macOS 11.0")
    }
}

@c(vz_newVZVirtioBlockDeviceConfiguration)
public func newVZVirtioBlockDeviceConfiguration(_ attachment: BorrowedObject) -> OwnedObject {
    bridgeSync {
        if #available(macOS 11.0, *) {
            return own(VZVirtioBlockDeviceConfiguration(attachment: borrow(attachment, as: VZStorageDeviceAttachment.self)))
        }
        preconditionFailure("API requires macOS 11.0")
    }
}

@c(vz_newVZUSBMassStorageDeviceConfiguration)
public func newVZUSBMassStorageDeviceConfiguration(_ attachment: BorrowedObject) -> OwnedObject {
    bridgeSync {
        if #available(macOS 13.0, *) {
            return own(VZUSBMassStorageDeviceConfiguration(attachment: borrow(attachment, as: VZStorageDeviceAttachment.self)))
        }
        preconditionFailure("API requires macOS 13.0")
    }
}

@c(vz_newVZNVMExpressControllerDeviceConfiguration)
public func newVZNVMExpressControllerDeviceConfiguration(_ attachment: BorrowedObject) -> OwnedObject {
    bridgeSync {
        if #available(macOS 14.0, *) {
            return own(VZNVMExpressControllerDeviceConfiguration(attachment: borrow(attachment, as: VZStorageDeviceAttachment.self)))
        }
        preconditionFailure("API requires macOS 14.0")
    }
}

@c(vz_newVZFileHandleSerialPortAttachment)
public func newVZFileHandleSerialPortAttachment(_ readFD: Int32, _ writeFD: Int32, _ errorOut: ErrorOut) -> OwnedObject {
    bridgeSync {
        if #available(macOS 11.0, *) {
            do {
            let read = try duplicateDeviceFile(readFD)
            let write = try duplicateDeviceFile(writeFD)
            return own(VZFileHandleSerialPortAttachment(fileHandleForReading: read, fileHandleForWriting: write))
            } catch {
                fail(error, errorOut)
                return nil
            }
        }
        preconditionFailure("API requires macOS 11.0")
    }
}

@c(vz_newVZFileHandleNetworkDeviceAttachment)
public func newVZFileHandleNetworkDeviceAttachment(_ fd: Int32, _ errorOut: ErrorOut) -> OwnedObject {
    bridgeSync {
        if #available(macOS 11.0, *) {
            do {
            return own(VZFileHandleNetworkDeviceAttachment(fileHandle: try duplicateDeviceFile(fd)))
            } catch {
                fail(error, errorOut)
                return nil
            }
        }
        preconditionFailure("API requires macOS 11.0")
    }
}

@c(vz_newVZFileSerialPortAttachment)
public func newVZFileSerialPortAttachment(_ path: CString, _ append: Bool, _ errorOut: ErrorOut) -> OwnedObject {
    bridgeSync {
        if #available(macOS 11.0, *) {
            do {
            return own(try VZFileSerialPortAttachment(url: URL(fileURLWithPath: text(path)), append: append))
            } catch {
                fail(error, errorOut)
                return nil
            }
        }
        preconditionFailure("API requires macOS 11.0")
    }
}

@c(vz_setMaximumTransmissionUnitVZFileHandleNetworkDeviceAttachment)
public func setMaximumTransmissionUnitVZFileHandleNetworkDeviceAttachment(_ object: BorrowedObject, _ value: Int64) {
    bridgeSync {
        if #available(macOS 13.0, *) {
            borrow(object, as: VZFileHandleNetworkDeviceAttachment.self).maximumTransmissionUnit = Int(value)
            return
        }
        preconditionFailure("API requires macOS 13.0")
    }
}

@c(vz_newVZDiskImageStorageDeviceAttachment)
public func newVZDiskImageStorageDeviceAttachment(_ path: CString, _ readOnly: Bool, _ errorOut: ErrorOut) -> OwnedObject {
    bridgeSync {
        if #available(macOS 11.0, *) {
            do {
            return own(try VZDiskImageStorageDeviceAttachment(url: URL(fileURLWithPath: text(path)), readOnly: readOnly))
            } catch {
                fail(error, errorOut)
                return nil
            }
        }
        preconditionFailure("API requires macOS 11.0")
    }
}

@c(vz_newVZDiskImageStorageDeviceAttachmentWithCacheAndSyncMode)
public func newVZDiskImageStorageDeviceAttachmentWithCacheAndSyncMode(_ path: CString, _ readOnly: Bool, _ cacheMode: Int32, _ syncMode: Int32, _ errorOut: ErrorOut) -> OwnedObject {
    bridgeSync {
        if #available(macOS 12.0, *) {
            do {
            guard let caching = VZDiskImageCachingMode(rawValue: Int(cacheMode)),
                  let synchronization = VZDiskImageSynchronizationMode(rawValue: Int(syncMode)) else {
                throw NSError(domain: NSPOSIXErrorDomain, code: Int(EINVAL), userInfo: [NSLocalizedDescriptionKey: "Invalid disk caching or synchronization mode"])
            }
            return own(try VZDiskImageStorageDeviceAttachment(url: URL(fileURLWithPath: text(path)), readOnly: readOnly, cachingMode: caching, synchronizationMode: synchronization))
            } catch {
                fail(error, errorOut)
                return nil
            }
        }
        preconditionFailure("API requires macOS 12.0")
    }
}

@c(vz_newVZDiskBlockDeviceStorageDeviceAttachment)
public func newVZDiskBlockDeviceStorageDeviceAttachment(_ fd: Int32, _ readOnly: Bool, _ syncMode: Int32, _ errorOut: ErrorOut) -> OwnedObject {
    bridgeSync {
        if #available(macOS 14.0, *) {
            do {
            guard let synchronization = VZDiskSynchronizationMode(rawValue: Int(syncMode)) else {
                throw NSError(domain: NSPOSIXErrorDomain, code: Int(EINVAL), userInfo: [NSLocalizedDescriptionKey: "Invalid disk synchronization mode"])
            }
            return own(try VZDiskBlockDeviceStorageDeviceAttachment(fileHandle: duplicateDeviceFile(fd), readOnly: readOnly, synchronizationMode: synchronization))
            } catch {
                fail(error, errorOut)
                return nil
            }
        }
        preconditionFailure("API requires macOS 14.0")
    }
}

@c(vz_setBlockDeviceIdentifierVZVirtioBlockDeviceConfiguration)
public func setBlockDeviceIdentifierVZVirtioBlockDeviceConfiguration(_ object: BorrowedObject, _ value: CString, _ errorOut: ErrorOut) {
    bridgeSync {
        if #available(macOS 12.3, *) {
            do {
            try VZVirtioBlockDeviceConfiguration.validateBlockDeviceIdentifier(text(value))
            borrow(object, as: VZVirtioBlockDeviceConfiguration.self).blockDeviceIdentifier = text(value)
            } catch { fail(error, errorOut) }
            return
        }
        preconditionFailure("API requires macOS 12.3")
    }
}

@c(vz_newVZSharedDirectory)
public func newVZSharedDirectory(_ path: CString, _ readOnly: Bool) -> OwnedObject {
    bridgeSync {
        if #available(macOS 12.0, *) {
            return own(VZSharedDirectory(url: URL(fileURLWithPath: text(path)), readOnly: readOnly))
        }
        preconditionFailure("API requires macOS 12.0")
    }
}

@c(vz_newVZSingleDirectoryShare)
public func newVZSingleDirectoryShare(_ directory: BorrowedObject) -> OwnedObject {
    bridgeSync {
        if #available(macOS 12.0, *) {
            return own(VZSingleDirectoryShare(directory: borrow(directory, as: VZSharedDirectory.self)))
        }
        preconditionFailure("API requires macOS 12.0")
    }
}

@c(vz_newVZMultipleDirectoryShare)
public func newVZMultipleDirectoryShare(_ directories: BorrowedObject) -> OwnedObject {
    bridgeSync {
        if #available(macOS 12.0, *) {
            return own(VZMultipleDirectoryShare(directories: borrow(directories, as: NSDictionary.self) as! [String: VZSharedDirectory]))
        }
        preconditionFailure("API requires macOS 12.0")
    }
}

@c(vz_newVZVirtioFileSystemDeviceConfiguration)
public func newVZVirtioFileSystemDeviceConfiguration(_ tag: CString, _ errorOut: ErrorOut) -> OwnedObject {
    bridgeSync {
        if #available(macOS 12.0, *) {
            do {
            try VZVirtioFileSystemDeviceConfiguration.validateTag(text(tag))
            return own(VZVirtioFileSystemDeviceConfiguration(tag: text(tag)))
            } catch {
                fail(error, errorOut)
                return nil
            }
        }
        preconditionFailure("API requires macOS 12.0")
    }
}

@c(vz_setVZVirtioFileSystemDeviceConfigurationShare)
public func setVZVirtioFileSystemDeviceConfigurationShare(_ object: BorrowedObject, _ value: BorrowedObject) {
    bridgeSync {
        if #available(macOS 12.0, *) {
            borrow(object, as: VZVirtioFileSystemDeviceConfiguration.self).share = value.map { borrow($0, as: VZDirectoryShare.self) }
            return
        }
        preconditionFailure("API requires macOS 12.0")
    }
}

@c(vz_portsVZVirtioConsoleDeviceConfiguration)
public func portsVZVirtioConsoleDeviceConfiguration(_ object: BorrowedObject) -> OwnedObject {
    bridgeSync {
        if #available(macOS 13.0, *) {
            return own(borrow(object, as: VZVirtioConsoleDeviceConfiguration.self).ports)
        }
        preconditionFailure("API requires macOS 13.0")
    }
}

@c(vz_setObjectAtIndexedSubscriptVZVirtioConsolePortConfigurationArray)
public func setObjectAtIndexedSubscriptVZVirtioConsolePortConfigurationArray(_ object: BorrowedObject, _ value: BorrowedObject, _ index: Int32) {
    bridgeSync {
        if #available(macOS 13.0, *) {
            borrow(object, as: VZVirtioConsolePortConfigurationArray.self)[Int(index)] = value.map { borrow($0, as: VZVirtioConsolePortConfiguration.self) }
            return
        }
        preconditionFailure("API requires macOS 13.0")
    }
}

@c(vz_setNameVZVirtioConsolePortConfiguration)
public func setNameVZVirtioConsolePortConfiguration(_ object: BorrowedObject, _ value: CString) {
    bridgeSync {
        if #available(macOS 13.0, *) {
            borrow(object, as: VZVirtioConsolePortConfiguration.self).name = text(value)
            return
        }
        preconditionFailure("API requires macOS 13.0")
    }
}

@c(vz_setAttachmentVZVirtioConsolePortConfiguration)
public func setAttachmentVZVirtioConsolePortConfiguration(_ object: BorrowedObject, _ value: BorrowedObject) {
    bridgeSync {
        if #available(macOS 13.0, *) {
            borrow(object, as: VZVirtioConsolePortConfiguration.self).attachment = value.map { borrow($0, as: VZSerialPortAttachment.self) }
            return
        }
        preconditionFailure("API requires macOS 13.0")
    }
}

@c(vz_newVZGenericMachineIdentifierWithBytes)
public func newVZGenericMachineIdentifierWithBytes(_ bytes: RawPointer, _ count: Int32) -> OwnedObject {
    bridgeSync {
        if #available(macOS 13.0, *) {
            guard count >= 0, let bytes else { return nil }
            return own(VZGenericMachineIdentifier(dataRepresentation: Data(bytes: bytes, count: Int(count))))
        }
        preconditionFailure("API requires macOS 13.0")
    }
}

@c(vz_getVZGenericMachineIdentifierDataRepresentation)
public func getVZGenericMachineIdentifierDataRepresentation(_ object: BorrowedObject) -> OwnedObject {
    bridgeSync {
        if #available(macOS 13.0, *) {
            return own(borrow(object, as: VZGenericMachineIdentifier.self).dataRepresentation as NSData)
        }
        preconditionFailure("API requires macOS 13.0")
    }
}

@c(vz_setMachineIdentifierVZGenericPlatformConfiguration)
public func setMachineIdentifierVZGenericPlatformConfiguration(_ object: BorrowedObject, _ value: BorrowedObject) {
    bridgeSync {
        if #available(macOS 13.0, *) {
            borrow(object, as: VZGenericPlatformConfiguration.self).machineIdentifier = borrow(value, as: VZGenericMachineIdentifier.self)
            return
        }
        preconditionFailure("API requires macOS 13.0")
    }
}

#if arch(arm64)
@c(vz_setHardwareModelVZMacPlatformConfiguration)
public func setHardwareModelVZMacPlatformConfiguration(_ object: BorrowedObject, _ value: BorrowedObject) {
    bridgeSync {
        if #available(macOS 12.0, *) {
            borrow(object, as: VZMacPlatformConfiguration.self).hardwareModel = borrow(value, as: VZMacHardwareModel.self)
            return
        }
        preconditionFailure("API requires macOS 12.0")
    }
}
#endif

#if arch(arm64)
@c(vz_setMachineIdentifierVZMacPlatformConfiguration)
public func setMachineIdentifierVZMacPlatformConfiguration(_ object: BorrowedObject, _ value: BorrowedObject) {
    bridgeSync {
        if #available(macOS 12.0, *) {
            borrow(object, as: VZMacPlatformConfiguration.self).machineIdentifier = borrow(value, as: VZMacMachineIdentifier.self)
            return
        }
        preconditionFailure("API requires macOS 12.0")
    }
}
#endif

#if arch(arm64)
@c(vz_setAuxiliaryStorageVZMacPlatformConfiguration)
public func setAuxiliaryStorageVZMacPlatformConfiguration(_ object: BorrowedObject, _ value: BorrowedObject) {
    bridgeSync {
        if #available(macOS 12.0, *) {
            borrow(object, as: VZMacPlatformConfiguration.self).auxiliaryStorage = value.map { borrow($0, as: VZMacAuxiliaryStorage.self) }
            return
        }
        preconditionFailure("API requires macOS 12.0")
    }
}
#endif

#if arch(arm64)
@c(vz_newVZLinuxRosettaDirectoryShare)
public func newVZLinuxRosettaDirectoryShare(_ errorOut: ErrorOut) -> OwnedObject {
    bridgeSync {
        if #available(macOS 13.0, *) {
            do {
            return own(try VZLinuxRosettaDirectoryShare())
            } catch {
                fail(error, errorOut)
                return nil
            }
        }
        preconditionFailure("API requires macOS 13.0")
    }
}
#endif

#if arch(arm64)
@c(vz_setOptionsVZLinuxRosettaDirectoryShare)
public func setOptionsVZLinuxRosettaDirectoryShare(_ object: BorrowedObject, _ value: BorrowedObject) {
    bridgeSync {
        if #available(macOS 14.0, *) {
            borrow(object, as: VZLinuxRosettaDirectoryShare.self).__options = value.map { borrow($0, as: __VZLinuxRosettaCachingOptions.self) }
            return
        }
        preconditionFailure("API requires macOS 14.0")
    }
}
#endif

#if arch(arm64)
@c(vz_newVZLinuxRosettaUnixSocketCachingOptionsWithPath)
public func newVZLinuxRosettaUnixSocketCachingOptionsWithPath(_ path: CString, _ errorOut: ErrorOut) -> OwnedObject {
    bridgeSync {
        if #available(macOS 14.0, *) {
            do {
            return own(try __VZLinuxRosettaUnixSocketCachingOptions(path: text(path)))
            } catch {
                fail(error, errorOut)
                return nil
            }
        }
        preconditionFailure("API requires macOS 14.0")
    }
}
#endif

#if arch(arm64)
@c(vz_newVZLinuxRosettaAbstractSocketCachingOptionsWithName)
public func newVZLinuxRosettaAbstractSocketCachingOptionsWithName(_ name: CString, _ errorOut: ErrorOut) -> OwnedObject {
    bridgeSync {
        if #available(macOS 14.0, *) {
            do {
            return own(try __VZLinuxRosettaAbstractSocketCachingOptions(name: text(name)))
            } catch {
                fail(error, errorOut)
                return nil
            }
        }
        preconditionFailure("API requires macOS 14.0")
    }
}
#endif

#if arch(arm64)
@c(vz_linuxInstallRosetta)
public func linuxInstallRosetta(_ context: UInt64) {
    bridgeSync {
        if #available(macOS 13.0, *) {
            VZLinuxRosettaDirectoryShare.installRosetta { error in
                emit(9, context, own(error as NSError?))
            }
            return
        }
        preconditionFailure("API requires macOS 13.0")
    }
}
#endif

private func duplicateDeviceFile(_ descriptor: Int32) throws -> FileHandle {
    let copied = dup(descriptor)
    guard copied >= 0 else { throw NSError(domain: NSPOSIXErrorDomain, code: Int(errno)) }
    return FileHandle(fileDescriptor: copied, closeOnDealloc: true)
}

@available(macOS 14.0, *)
private final class NetworkAttachmentDelegate: NSObject, VZNetworkBlockDeviceStorageDeviceAttachmentDelegate {
    let context: UInt64
    init(context: UInt64) { self.context = context }
    func attachment(_ attachment: VZNetworkBlockDeviceStorageDeviceAttachment, didEncounterError error: Error) {
        emit(7, context, own(error as NSError))
    }
    func attachmentWasConnected(_ attachment: VZNetworkBlockDeviceStorageDeviceAttachment) {
        emit(8, context)
    }
    deinit { emit(4, context) }
}

@c(vz_newVZNetworkBlockDeviceStorageDeviceAttachment)
public func newVZNetworkBlockDeviceStorageDeviceAttachment(_ uri: CString, _ timeout: Double, _ readOnly: Bool, _ syncMode: Int32, _ errorOut: ErrorOut, _ context: UInt64) -> OwnedObject {
    bridgeSync {
        if #available(macOS 14.0, *) {
            do {
            guard let url = URL(string: text(uri)) else { throw NSError(domain: NSURLErrorDomain, code: NSURLErrorBadURL) }
            guard let synchronization = VZDiskSynchronizationMode(rawValue: Int(syncMode)) else {
                throw NSError(domain: NSPOSIXErrorDomain, code: Int(EINVAL), userInfo: [NSLocalizedDescriptionKey: "Invalid disk synchronization mode"])
            }
            let attachment = try VZNetworkBlockDeviceStorageDeviceAttachment(url: url, timeout: timeout, isForcedReadOnly: readOnly, synchronizationMode: synchronization)
            let delegate = NetworkAttachmentDelegate(context: context)
            attachment.delegate = delegate
            objc_setAssociatedObject(attachment, UnsafeRawPointer(bitPattern: 0x565a4e4244)!, delegate, .OBJC_ASSOCIATION_RETAIN_NONATOMIC)
            return own(attachment)
            } catch {
                fail(error, errorOut)
                return nil
            }
        }
        preconditionFailure("API requires macOS 14.0")
    }
}

@c(vz_newVZVirtioSocketDeviceConfiguration)
public func newVZVirtioSocketDeviceConfiguration() -> OwnedObject {
    bridgeSync { own(VZVirtioSocketDeviceConfiguration()) }
}

@c(vz_newVZXHCIControllerConfiguration)
public func newVZXHCIControllerConfiguration() -> OwnedObject {
    guard #available(macOS 15, *) else { return nil }
    return bridgeSync { own(VZXHCIControllerConfiguration()) }
}

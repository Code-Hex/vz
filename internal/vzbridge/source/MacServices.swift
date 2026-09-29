import Foundation
import Virtualization

#if arch(arm64)

@c(vz_newVZMacHardwareModelWithBytes)
public func newVZMacHardwareModelWithBytes(_ bytes: RawPointer, _ count: Int32) -> OwnedObject {
    guard #available(macOS 12, *) else { return nil }
    return bridgeSync {
        guard let data = count >= 0 && (count == 0 || bytes != nil) ? Data(bytes: bytes ?? UnsafeMutableRawPointer(bitPattern: 1)!, count: Int(count)) : nil else { return nil }
        return own(VZMacHardwareModel(dataRepresentation: data))
    }
}

@c(vz_getVZMacHardwareModelDataRepresentation)
public func getVZMacHardwareModelDataRepresentation(_ object: BorrowedObject) -> OwnedObject {
    guard #available(macOS 12, *) else { return nil }
    return bridgeSync { own(borrow(object, as: VZMacHardwareModel.self).dataRepresentation as NSData) }
}

@c(vz_newVZMacMachineIdentifierWithBytes)
public func newVZMacMachineIdentifierWithBytes(_ bytes: RawPointer, _ count: Int32) -> OwnedObject {
    guard #available(macOS 12, *) else { return nil }
    return bridgeSync {
        guard let data = count >= 0 && (count == 0 || bytes != nil) ? Data(bytes: bytes ?? UnsafeMutableRawPointer(bitPattern: 1)!, count: Int(count)) : nil else { return nil }
        return own(VZMacMachineIdentifier(dataRepresentation: data))
    }
}

@c(vz_getVZMacMachineIdentifierDataRepresentation)
public func getVZMacMachineIdentifierDataRepresentation(_ object: BorrowedObject) -> OwnedObject {
    guard #available(macOS 12, *) else { return nil }
    return bridgeSync { own(borrow(object, as: VZMacMachineIdentifier.self).dataRepresentation as NSData) }
}

@c(vz_newVZMacMachineIdentifier)
public func newVZMacMachineIdentifier() -> OwnedObject {
    guard #available(macOS 12, *) else { return nil }
    return bridgeSync { own(VZMacMachineIdentifier()) }
}

@c(vz_macHardwareModelSupported)
public func macHardwareModelSupported(_ object: BorrowedObject) -> Bool {
    guard #available(macOS 12, *) else { return false }
    return bridgeSync { borrow(object, as: VZMacHardwareModel.self).isSupported }
}

@c(vz_newVZMacAuxiliaryStorageWithCreating)
public func newVZMacAuxiliaryStorageWithCreating(_ path: CString, _ hardwareModel: BorrowedObject, _ errorOutput: ErrorOut) -> OwnedObject {
    guard #available(macOS 12, *) else { return nil }
    return bridgeSync {
        do { return own(try VZMacAuxiliaryStorage(creatingStorageAt: URL(fileURLWithPath: text(path)), hardwareModel: borrow(hardwareModel, as: VZMacHardwareModel.self), options: [.allowOverwrite])) }
        catch { fail(error, errorOutput); return nil }
    }
}

@c(vz_newVZMacAuxiliaryStorage)
public func newVZMacAuxiliaryStorage(_ path: CString) -> OwnedObject {
    guard #available(macOS 12, *) else { return nil }
    return bridgeSync { own(VZMacAuxiliaryStorage(contentsOf: URL(fileURLWithPath: text(path)))) }
}

@c(vz_fetchLatestSupportedMacOSRestoreImageWithCompletionHandler)
public func fetchLatestSupportedMacOSRestoreImageWithCompletionHandler(_ context: UInt64) {
    guard #available(macOS 12, *) else { return }
    bridgeSync {
        VZMacOSRestoreImage.fetchLatestSupported { result in
            bridgeSync {
                switch result {
                case .success(let image): _ = emit(10, context, own(image))
                case .failure(let error): _ = emit(10, context, nil, own(error as NSError))
                }
            }
        }
    }
}

@c(vz_loadMacOSRestoreImageFile)
public func loadMacOSRestoreImageFile(_ path: CString, _ context: UInt64) {
    guard #available(macOS 12, *) else { return }
    bridgeSync {
        VZMacOSRestoreImage.load(from: URL(fileURLWithPath: text(path))) { result in
            bridgeSync {
                switch result {
                case .success(let image): _ = emit(10, context, own(image))
                case .failure(let error): _ = emit(10, context, nil, own(error as NSError))
                }
            }
        }
    }
}

@c(vz_macOSRestoreImageURL)
public func macOSRestoreImageURL(_ object: BorrowedObject) -> OwnedObject {
    guard #available(macOS 12, *) else { return nil }
    return bridgeSync { own(borrow(object, as: VZMacOSRestoreImage.self).url.absoluteString as NSString) }
}

@c(vz_macOSRestoreImageBuildVersion)
public func macOSRestoreImageBuildVersion(_ object: BorrowedObject) -> OwnedObject {
    guard #available(macOS 12, *) else { return nil }
    return bridgeSync { own(borrow(object, as: VZMacOSRestoreImage.self).buildVersion as NSString) }
}

@c(vz_macOSRestoreImageConfiguration)
public func macOSRestoreImageConfiguration(_ object: BorrowedObject) -> OwnedObject {
    guard #available(macOS 12, *) else { return nil }
    return bridgeSync { own(borrow(object, as: VZMacOSRestoreImage.self).mostFeaturefulSupportedConfiguration) }
}

@c(vz_macOSConfigurationHardwareModel)
public func macOSConfigurationHardwareModel(_ object: BorrowedObject) -> OwnedObject {
    guard #available(macOS 12, *) else { return nil }
    return bridgeSync { own(borrow(object, as: VZMacOSConfigurationRequirements.self).hardwareModel) }
}

@c(vz_macOSConfigurationMinimumCPUCount)
public func macOSConfigurationMinimumCPUCount(_ object: BorrowedObject) -> UInt64 {
    guard #available(macOS 12, *) else { return 0 }
    return bridgeSync { UInt64(borrow(object, as: VZMacOSConfigurationRequirements.self).minimumSupportedCPUCount) }
}

@c(vz_macOSConfigurationMinimumMemorySize)
public func macOSConfigurationMinimumMemorySize(_ object: BorrowedObject) -> UInt64 {
    guard #available(macOS 12, *) else { return 0 }
    return bridgeSync { UInt64(borrow(object, as: VZMacOSConfigurationRequirements.self).minimumSupportedMemorySize) }
}

@c(vz_macOSRestoreImageMajorVersion)
public func macOSRestoreImageMajorVersion(_ object: BorrowedObject) -> Int64 {
    guard #available(macOS 12, *) else { return 0 }
    return bridgeSync { Int64(borrow(object, as: VZMacOSRestoreImage.self).operatingSystemVersion.majorVersion) }
}

@c(vz_macOSRestoreImageMinorVersion)
public func macOSRestoreImageMinorVersion(_ object: BorrowedObject) -> Int64 {
    guard #available(macOS 12, *) else { return 0 }
    return bridgeSync { Int64(borrow(object, as: VZMacOSRestoreImage.self).operatingSystemVersion.minorVersion) }
}

@c(vz_macOSRestoreImagePatchVersion)
public func macOSRestoreImagePatchVersion(_ object: BorrowedObject) -> Int64 {
    guard #available(macOS 12, *) else { return 0 }
    return bridgeSync { Int64(borrow(object, as: VZMacOSRestoreImage.self).operatingSystemVersion.patchVersion) }
}

@available(macOS 12, *)
// Every payload access and final release is confined to bridgeQueue.
final class ManagedMacOSInstaller: NSObject, @unchecked Sendable {
    private var installer: VZMacOSInstaller?
    private var observation: NSKeyValueObservation?
    private var progressContext: UInt64?

    init(machine: VZVirtualMachine, path: String) {
        installer = VZMacOSInstaller(virtualMachine: machine, restoringFromImageAt: URL(fileURLWithPath: path))
        super.init()
    }

    func install(completion: UInt64, progress: UInt64) {
        dispatchPrecondition(condition: .onQueue(bridgeQueue))
        guard let installer else { preconditionFailure("installer has been released") }
        precondition(progressContext == nil, "installation is already active")
        progressContext = progress
        observation = installer.progress.observe(\.fractionCompleted, options: [.initial, .new]) { [weak self] _, change in
            guard let value = change.newValue else { return }
            bridgeSync {
                guard self?.progressContext == progress else { return }
                _ = emit(12, progress, nil, nil, value.bitPattern)
            }
        }
        installer.install { [self] error in
            bridgeSync {
                finishProgress()
                switch error {
                case .success: _ = emit(11, completion)
                case .failure(let error): _ = emit(11, completion, own(error as NSError))
                }
            }
        }
    }

    private func finishProgress() {
        observation?.invalidate()
        observation = nil
        guard let context = progressContext else { return }
        progressContext = nil
        _ = emit(4, context)
    }

    func cancel() {
        dispatchPrecondition(condition: .onQueue(bridgeQueue))
        guard let installer else { return }
        if installer.progress.isCancellable { installer.progress.cancel() }
    }

    deinit {
        bridgeSync {
            finishProgress()
            installer = nil
        }
    }
}

@c(vz_newVZMacOSInstaller)
public func newVZMacOSInstaller(_ machine: BorrowedObject, _ path: CString) -> OwnedObject {
    guard #available(macOS 12, *) else { return nil }
    return bridgeSync { own(ManagedMacOSInstaller(machine: borrow(machine, as: VZVirtualMachine.self), path: text(path))) }
}

@c(vz_installByVZMacOSInstaller)
public func installByVZMacOSInstaller(_ installer: BorrowedObject, _ completion: UInt64, _ progress: UInt64) {
    guard #available(macOS 12, *) else { return }
    bridgeSync { borrow(installer, as: ManagedMacOSInstaller.self).install(completion: completion, progress: progress) }
}

@c(vz_cancelInstallVZMacOSInstaller)
public func cancelInstallVZMacOSInstaller(_ installer: BorrowedObject) {
    guard #available(macOS 12, *) else { return }
    bridgeSync {
        borrow(installer, as: ManagedMacOSInstaller.self).cancel()
    }
}

@c(vz_saveMachineStateToURLWithCompletionHandler)
public func saveMachineStateToURLWithCompletionHandler(_ machine: BorrowedObject, _ context: UInt64, _ path: CString) {
    guard #available(macOS 14, *) else { return }
    bridgeSync {
        let vm = borrow(machine, as: VZVirtualMachine.self)
        vm.saveMachineStateTo(url: URL(fileURLWithPath: text(path))) { [vm] error in
            withExtendedLifetime(vm) { _ = emit(1, context, own(error as NSError?)) }
        }
    }
}

@c(vz_restoreMachineStateFromURLWithCompletionHandler)
public func restoreMachineStateFromURLWithCompletionHandler(_ machine: BorrowedObject, _ context: UInt64, _ path: CString) {
    guard #available(macOS 14, *) else { return }
    bridgeSync {
        let vm = borrow(machine, as: VZVirtualMachine.self)
        vm.restoreMachineStateFrom(url: URL(fileURLWithPath: text(path))) { [vm] error in
            withExtendedLifetime(vm) { _ = emit(1, context, own(error as NSError?)) }
        }
    }
}

@c(vz_newVZMacOSVirtualMachineStartOptions)
public func newVZMacOSVirtualMachineStartOptions(_ startUpFromMacOSRecovery: Bool) -> OwnedObject {
    guard #available(macOS 13, *) else { return nil }
    return bridgeSync {
        let options = VZMacOSVirtualMachineStartOptions()
        options.startUpFromMacOSRecovery = startUpFromMacOSRecovery
        return own(options)
    }
}

#endif

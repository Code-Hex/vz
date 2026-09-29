import Foundation
import Virtualization
import Darwin

private final class MachineDelegate: NSObject, VZVirtualMachineDelegate {
    let context: UInt64
    init(_ context: UInt64) { self.context = context }

    func virtualMachine(_ virtualMachine: VZVirtualMachine, didStopWithError error: Error) {}
    func guestDidStop(_ virtualMachine: VZVirtualMachine) {}

    @available(macOS 12, *)
    func virtualMachine(_ virtualMachine: VZVirtualMachine, networkDevice: VZNetworkDevice, attachmentWasDisconnectedWithError error: Error) {
        let index = virtualMachine.networkDevices.firstIndex { $0 === networkDevice }
        _ = emit(3, context, own(error as NSError), nil, UInt64(bitPattern: Int64(index ?? -1)))
    }
}

final class ManagedVirtualMachine: VZVirtualMachine {
    private var observation: NSKeyValueObservation?
    private let events: MachineDelegate
    private let stateContext: UInt64

    init(configuration: VZVirtualMachineConfiguration, stateContext: UInt64, disconnectedContext: UInt64) {
        self.stateContext = stateContext
        events = MachineDelegate(disconnectedContext)
        super.init(configuration: configuration, queue: bridgeQueue)
        delegate = events
        observation = observe(\.state) { machine, _ in
            _ = emit(2, stateContext, nil, nil, UInt64(machine.state.rawValue))
        }
    }

    deinit {
        observation?.invalidate()
        observation = nil
        delegate = nil
        _ = emit(4, stateContext)
        _ = emit(4, events.context)
    }
}

private final class SocketDelegate: NSObject, VZVirtioSocketListenerDelegate {
    private var context: UInt64?
    init(_ context: UInt64) { self.context = context }

    func listener(_ listener: VZVirtioSocketListener, shouldAcceptNewConnection connection: VZVirtioSocketConnection, from socketDevice: VZVirtioSocketDevice) -> Bool {
        guard let context else { return false }
        return emit(6, context, own(connection), Unmanaged.passUnretained(socketDevice).toOpaque()) != 0
    }

    func invalidate() {
        guard let context else { return }
        self.context = nil
        _ = emit(4, context)
    }
}

final class ManagedSocketListener: VZVirtioSocketListener {
    private let events: SocketDelegate
    init(_ context: UInt64) {
        events = SocketDelegate(context)
        super.init()
        delegate = events
    }
    func invalidate() {
        delegate = nil
        events.invalidate()
    }
    deinit { events.invalidate() }
}

@c(vz_newVZVirtualMachineWithDispatchQueue)
public func newVZVirtualMachineWithDispatchQueue(_ config: BorrowedObject, _ statusUpdateCgoHandle: UInt64, _ disconnectedCgoHandle: UInt64) -> OwnedObject {
    bridgeSync { own(ManagedVirtualMachine(configuration: borrow(config, as: VZVirtualMachineConfiguration.self), stateContext: statusUpdateCgoHandle, disconnectedContext: disconnectedCgoHandle)) }
}

@c(vz_requestStopVirtualMachine)
public func requestStopVirtualMachine(_ machine: BorrowedObject, _ errorOutput: ErrorOut) -> Bool {
    bridgeSync {
        do { try borrow(machine, as: VZVirtualMachine.self).requestStop(); return true }
        catch { fail(error, errorOutput); return false }
    }
}

@c(vz_vmCanStart)
public func vmCanStart(_ machine: BorrowedObject) -> Bool {
    return bridgeSync { borrow(machine, as: VZVirtualMachine.self).canStart }
}

@c(vz_vmCanPause)
public func vmCanPause(_ machine: BorrowedObject) -> Bool {
    return bridgeSync { borrow(machine, as: VZVirtualMachine.self).canPause }
}

@c(vz_vmCanResume)
public func vmCanResume(_ machine: BorrowedObject) -> Bool {
    return bridgeSync { borrow(machine, as: VZVirtualMachine.self).canResume }
}

@c(vz_vmCanRequestStop)
public func vmCanRequestStop(_ machine: BorrowedObject) -> Bool {
    return bridgeSync { borrow(machine, as: VZVirtualMachine.self).canRequestStop }
}

@c(vz_vmCanStop)
public func vmCanStop(_ machine: BorrowedObject) -> Bool {
    guard #available(macOS 12, *) else { return false }; return bridgeSync { borrow(machine, as: VZVirtualMachine.self).canStop }
}

@c(vz_startWithCompletionHandler)
public func startWithCompletionHandler(_ machine: BorrowedObject, _ cgoHandle: UInt64) {
    bridgeSync {
        let vm = borrow(machine, as: VZVirtualMachine.self)
        vm.start { [vm] result in
            withExtendedLifetime(vm) {
                switch result {
                case .success: _ = emit(1, cgoHandle)
                case .failure(let error): _ = emit(1, cgoHandle, own(error as NSError))
                }
            }
        }
    }
}

@c(vz_pauseWithCompletionHandler)
public func pauseWithCompletionHandler(_ machine: BorrowedObject, _ cgoHandle: UInt64) {
    bridgeSync {
        let vm = borrow(machine, as: VZVirtualMachine.self)
        vm.pause { [vm] result in
            withExtendedLifetime(vm) {
                switch result {
                case .success: _ = emit(1, cgoHandle)
                case .failure(let error): _ = emit(1, cgoHandle, own(error as NSError))
                }
            }
        }
    }
}

@c(vz_resumeWithCompletionHandler)
public func resumeWithCompletionHandler(_ machine: BorrowedObject, _ cgoHandle: UInt64) {
    bridgeSync {
        let vm = borrow(machine, as: VZVirtualMachine.self)
        vm.resume { [vm] result in
            withExtendedLifetime(vm) {
                switch result {
                case .success: _ = emit(1, cgoHandle)
                case .failure(let error): _ = emit(1, cgoHandle, own(error as NSError))
                }
            }
        }
    }
}

@c(vz_stopWithCompletionHandler)
public func stopWithCompletionHandler(_ machine: BorrowedObject, _ cgoHandle: UInt64) {
    guard #available(macOS 12, *) else { return }; bridgeSync {
        let vm = borrow(machine, as: VZVirtualMachine.self)
        vm.stop { [vm] result in
            withExtendedLifetime(vm) {
                _ = emit(1, cgoHandle, own(result as NSError?))
            }
        }
    }
}

@c(vz_VZVirtualMachine_socketDevices)
public func VZVirtualMachine_socketDevices(_ machine: BorrowedObject) -> OwnedObject {
    return bridgeSync { own(borrow(machine, as: VZVirtualMachine.self).socketDevices as NSArray) }
}

@c(vz_VZVirtualMachine_memoryBalloonDevices)
public func VZVirtualMachine_memoryBalloonDevices(_ machine: BorrowedObject) -> OwnedObject {
    return bridgeSync { own(borrow(machine, as: VZVirtualMachine.self).memoryBalloonDevices as NSArray) }
}

@c(vz_VZVirtualMachine_usbControllers)
public func VZVirtualMachine_usbControllers(_ machine: BorrowedObject) -> OwnedObject {
    guard #available(macOS 15, *) else { return nil }; return bridgeSync { own(borrow(machine, as: VZVirtualMachine.self).usbControllers as NSArray) }
}

@c(vz_startWithOptionsCompletionHandler)
public func startWithOptionsCompletionHandler(_ machine: BorrowedObject, _ options: BorrowedObject, _ cgoHandle: UInt64) {
    #if arch(arm64)
    guard #available(macOS 13, *) else { return }
    bridgeSync {
        let vm = borrow(machine, as: VZVirtualMachine.self)
        vm.start(options: borrow(options, as: VZMacOSVirtualMachineStartOptions.self)) { [vm] result in
            withExtendedLifetime(vm) {
                _ = emit(1, cgoHandle, own(result as NSError?))
            }
        }
    }
    #else
    startWithCompletionHandler(machine, cgoHandle)
    #endif
}

@c(vz_newVZVirtioSocketListener)
public func newVZVirtioSocketListener(_ cgoHandle: UInt64) -> OwnedObject {
    bridgeSync { own(ManagedSocketListener(cgoHandle)) }
}

@c(vz_invalidateVZVirtioSocketListener)
public func invalidateVZVirtioSocketListener(_ listener: BorrowedObject) {
    bridgeSync { borrow(listener, as: ManagedSocketListener.self).invalidate() }
}

@c(vz_VZVirtioSocketDevice_setSocketListenerForPort)
public func VZVirtioSocketDevice_setSocketListenerForPort(_ socketDevice: BorrowedObject, _ listener: BorrowedObject, _ port: UInt32) {
    bridgeSync { borrow(socketDevice, as: VZVirtioSocketDevice.self).setSocketListener(borrow(listener, as: VZVirtioSocketListener.self), forPort: port) }
}

@c(vz_VZVirtioSocketDevice_removeSocketListenerForPort)
public func VZVirtioSocketDevice_removeSocketListenerForPort(_ socketDevice: BorrowedObject, _ port: UInt32) {
    bridgeSync { borrow(socketDevice, as: VZVirtioSocketDevice.self).removeSocketListener(forPort: port) }
}

@c(vz_VZVirtioSocketDevice_connectToPort)
public func VZVirtioSocketDevice_connectToPort(_ socketDevice: BorrowedObject, _ port: UInt32, _ cgoHandle: UInt64) {
    bridgeSync {
        let device = borrow(socketDevice, as: VZVirtioSocketDevice.self)
        device.connect(toPort: port) { [device] result in
            withExtendedLifetime(device) {
                switch result {
                case .success(let connection): _ = emit(5, cgoHandle, own(connection))
                case .failure(let error): _ = emit(5, cgoHandle, nil, own(error as NSError))
                }
            }
        }
    }
}

@c(vz_socketConnectionDestinationPort)
public func socketConnectionDestinationPort(_ connection: BorrowedObject) -> UInt32 {
    bridgeSync { borrow(connection, as: VZVirtioSocketConnection.self).destinationPort }
}

@c(vz_socketConnectionSourcePort)
public func socketConnectionSourcePort(_ connection: BorrowedObject) -> UInt32 {
    bridgeSync { borrow(connection, as: VZVirtioSocketConnection.self).sourcePort }
}

@c(vz_socketConnectionDuplicatedFileDescriptor)
public func socketConnectionDuplicatedFileDescriptor(_ connection: BorrowedObject, _ errorOutput: ErrorOut) -> Int32 {
    bridgeSync {
        let fd = dup(borrow(connection, as: VZVirtioSocketConnection.self).fileDescriptor)
        if fd < 0 { fail(NSError(domain: NSPOSIXErrorDomain, code: Int(errno)), errorOutput) }
        return fd
    }
}

@c(vz_VZVirtioTraditionalMemoryBalloonDevice_setTargetVirtualMachineMemorySize)
public func VZVirtioTraditionalMemoryBalloonDevice_setTargetVirtualMachineMemorySize(_ balloonDevice: BorrowedObject, _ targetMemorySize: UInt64) {
    bridgeSync { borrow(balloonDevice, as: VZVirtioTraditionalMemoryBalloonDevice.self).targetVirtualMachineMemorySize = targetMemorySize }
}

@c(vz_VZVirtioTraditionalMemoryBalloonDevice_getTargetVirtualMachineMemorySize)
public func VZVirtioTraditionalMemoryBalloonDevice_getTargetVirtualMachineMemorySize(_ balloonDevice: BorrowedObject) -> UInt64 {
    bridgeSync { borrow(balloonDevice, as: VZVirtioTraditionalMemoryBalloonDevice.self).targetVirtualMachineMemorySize }
}

@c(vz_usbDevicesVZUSBController)
public func usbDevicesVZUSBController(_ usbController: BorrowedObject) -> OwnedObject {
    guard #available(macOS 15, *) else { return nil }
    return bridgeSync { own(borrow(usbController, as: VZUSBController.self).usbDevices as NSArray) }
}

@c(vz_getUUIDUSBDevice)
public func getUUIDUSBDevice(_ usbDevice: BorrowedObject) -> OwnedObject {
    guard #available(macOS 15, *) else { return nil }
    return bridgeSync { own(checkedUSBDevice(usbDevice).uuid.uuidString as NSString) }
}

@c(vz_newVZUSBMassStorageDeviceWithConfiguration)
public func newVZUSBMassStorageDeviceWithConfiguration(_ config: BorrowedObject) -> OwnedObject {
    guard #available(macOS 15, *) else { return nil }
    return bridgeSync { own(VZUSBMassStorageDevice(configuration: borrow(config, as: VZUSBMassStorageDeviceConfiguration.self))) }
}

@c(vz_attachDeviceVZUSBController)
public func attachDeviceVZUSBController(_ usbController: BorrowedObject, _ usbDevice: BorrowedObject, _ cgoHandle: UInt64) {
    guard #available(macOS 15, *) else { return }
    bridgeSync {
        let controller = borrow(usbController, as: VZUSBController.self)
        let device = checkedUSBDevice(usbDevice)
        controller.attach(device: device) { [controller, device] error in
            withExtendedLifetime((controller, device)) { _ = emit(1, cgoHandle, own(error as NSError?)) }
        }
    }
}

@c(vz_detachDeviceVZUSBController)
public func detachDeviceVZUSBController(_ usbController: BorrowedObject, _ usbDevice: BorrowedObject, _ cgoHandle: UInt64) {
    guard #available(macOS 15, *) else { return }
    bridgeSync {
        let controller = borrow(usbController, as: VZUSBController.self)
        let device = checkedUSBDevice(usbDevice)
        controller.detach(device: device) { [controller, device] error in
            withExtendedLifetime((controller, device)) { _ = emit(1, cgoHandle, own(error as NSError?)) }
        }
    }
}

@available(macOS 15, *)
func checkedUSBDevice(_ pointer: UnsafeMutableRawPointer?) -> any VZUSBDevice {
    guard let device = borrow(pointer, as: NSObject.self) as? any VZUSBDevice else {
        preconditionFailure("expected a VZUSBDevice")
    }
    return device
}

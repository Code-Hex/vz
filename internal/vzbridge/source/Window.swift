import Foundation

@c(vz_startVirtualMachineWindow)
public func startVirtualMachineWindow(_ machine: BorrowedObject, _ queue: RawPointer, _ width: Double, _ height: Double, _ title: CString, _ enableController: Bool) {
    precondition(Thread.isMainThread, "Window UI must run on the main thread")
    VZBridgeStartWindow(machine, queue, width, height, title, enableController)
}

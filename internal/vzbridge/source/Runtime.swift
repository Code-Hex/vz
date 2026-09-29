import Foundation
import Dispatch

public typealias BorrowedObject = UnsafeMutableRawPointer?
public typealias OwnedObject = UnsafeMutableRawPointer?
public typealias RawPointer = UnsafeMutableRawPointer?
public typealias RawBytes = UnsafeRawPointer?
public typealias CString = UnsafePointer<CChar>?
public typealias ErrorOut = UnsafeMutablePointer<UnsafeMutableRawPointer?>?

let bridgeQueue = DispatchQueue(label: "com.codehex.vz.native")
private let bridgeQueueKey = DispatchSpecificKey<UInt8>()
private let configureBridgeQueue: Void = bridgeQueue.setSpecific(key: bridgeQueueKey, value: 1)

func bridgeSync<T>(_ body: () -> T) -> T {
    _ = configureBridgeQueue
    if DispatchQueue.getSpecific(key: bridgeQueueKey) != nil {
        return autoreleasepool(invoking: body)
    }
    return bridgeQueue.sync { autoreleasepool(invoking: body) }
}

func borrow<T: NSObject>(_ pointer: UnsafeMutableRawPointer?, as type: T.Type) -> T {
    guard let pointer,
          let object = Unmanaged<NSObject>.fromOpaque(pointer).takeUnretainedValue() as? T else {
        preconditionFailure("invalid native object type: \(type)")
    }
    return object
}

func own(_ object: NSObject?) -> UnsafeMutableRawPointer? {
    object.map { Unmanaged.passRetained($0).toOpaque() }
}

func text(_ pointer: UnsafePointer<CChar>?) -> String {
    pointer.map(String.init(cString:)) ?? ""
}

func fail(_ error: Error, _ output: UnsafeMutablePointer<UnsafeMutableRawPointer?>?) {
    output?.pointee = own(error as NSError)
}

private typealias BridgeCallback = @convention(c) (UInt32, UInt64, UnsafeMutableRawPointer?, UnsafeMutableRawPointer?, UInt64) -> UInt64
private final class CallbackStorage: @unchecked Sendable {
    var callback: BridgeCallback?
}
private let callbacks = CallbackStorage()

@discardableResult
func emit(_ kind: UInt32, _ context: UInt64, _ first: UnsafeMutableRawPointer? = nil, _ second: UnsafeMutableRawPointer? = nil, _ value: UInt64 = 0) -> UInt64 {
    bridgeSync {
        guard let callback = callbacks.callback else {
            preconditionFailure("native callback dispatcher is not initialized")
        }
        return callback(kind, context, first, second, value)
    }
}

@c(vz_setCallback)
public func setCallback(_ address: UInt64) {
    bridgeSync {
        guard let pointer = UnsafeMutableRawPointer(bitPattern: UInt(address)) else { preconditionFailure("null callback dispatcher") }
        precondition(callbacks.callback == nil, "callback dispatcher is already initialized")
        callbacks.callback = unsafeBitCast(pointer, to: BridgeCallback.self)
    }
}

@c(vz_releaseObject)
public func releaseObject(_ pointer: RawPointer) {
    guard let pointer else { return }
    let address = UInt(bitPattern: pointer)
    bridgeQueue.async {
        autoreleasepool {
            Unmanaged<NSObject>.fromOpaque(UnsafeMutableRawPointer(bitPattern: address)!).release()
        }
    }
}

@c(vz_retainObject)
public func retainObject(_ object: BorrowedObject) -> OwnedObject {
    bridgeSync { own(borrow(object, as: NSObject.self)) }
}

@c(vz_dispatchQueuePointer)
public func dispatchQueuePointer() -> RawPointer {
    VZBridgeQueuePointer(bridgeQueue)
}

@c(vz_objectStringData)
public func objectStringData(_ object: BorrowedObject) -> OwnedObject {
    bridgeSync { own(Data((borrow(object, as: NSString.self) as String).utf8) as NSData) }
}

@c(vz_dataBytes)
public func dataBytes(_ object: BorrowedObject) -> RawBytes {
    bridgeSync {
        let data = borrow(object, as: NSData.self)
        return data.length == 0 ? nil : data.bytes
    }
}

@c(vz_dataLength)
public func dataLength(_ object: BorrowedObject) -> UInt64 {
    bridgeSync { UInt64(borrow(object, as: NSData.self).length) }
}

@c(vz_arrayCount)
public func arrayCount(_ object: BorrowedObject) -> UInt64 {
    bridgeSync { UInt64(borrow(object, as: NSArray.self).count) }
}

@c(vz_arrayObject)
public func arrayObject(_ object: BorrowedObject, _ index: UInt64) -> OwnedObject {
    bridgeSync { own(borrow(object, as: NSArray.self)[Int(index)] as? NSObject) }
}

@c(vz_errorDomain)
public func errorDomain(_ object: BorrowedObject) -> OwnedObject {
    bridgeSync { own(borrow(object, as: NSError.self).domain as NSString) }
}

@c(vz_errorCode)
public func errorCode(_ object: BorrowedObject) -> Int64 {
    bridgeSync { Int64(borrow(object, as: NSError.self).code) }
}

@c(vz_errorDescription)
public func errorDescription(_ object: BorrowedObject) -> OwnedObject {
    bridgeSync { own(borrow(object, as: NSError.self).localizedDescription as NSString) }
}

@c(vz_errorUserInfo)
public func errorUserInfo(_ object: BorrowedObject) -> OwnedObject {
    bridgeSync { own((borrow(object, as: NSError.self).userInfo as NSDictionary).description as NSString) }
}

@c(vz_drain)
public func drain() {
    bridgeSync {}
}

@c(vz_newArray)
public func newArray() -> OwnedObject {
    bridgeSync { own(NSMutableArray()) }
}

@c(vz_arrayAppend)
public func arrayAppend(_ array: BorrowedObject, _ object: BorrowedObject) {
    bridgeSync { borrow(array, as: NSMutableArray.self).add(borrow(object, as: NSObject.self)) }
}

@c(vz_newDictionary)
public func newDictionary() -> OwnedObject {
    bridgeSync { own(NSMutableDictionary()) }
}

@c(vz_dictionarySet)
public func dictionarySet(_ dictionary: BorrowedObject, _ key: CString, _ object: BorrowedObject) {
    bridgeSync { borrow(dictionary, as: NSMutableDictionary.self)[text(key)] = borrow(object, as: NSObject.self) }
}

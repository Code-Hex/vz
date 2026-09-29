import Foundation
import Virtualization

@c(vz_NewVirtioSoundDeviceHostInputStreamConfiguration)
public func NewVirtioSoundDeviceHostInputStreamConfiguration() -> OwnedObject {
    bridgeSync {
        guard #available(macOS 12, *) else { preconditionFailure("API requires macOS 12") }
        let config = VZVirtioSoundDeviceInputStreamConfiguration()
        config.source = VZHostAudioInputStreamSource()
        return own(config)
    }
}

@c(vz_NewVirtioSoundDeviceHostOutputStreamConfiguration)
public func NewVirtioSoundDeviceHostOutputStreamConfiguration() -> OwnedObject {
    bridgeSync {
        guard #available(macOS 12, *) else { preconditionFailure("API requires macOS 12") }
        let config = VZVirtioSoundDeviceOutputStreamConfiguration()
        config.sink = VZHostAudioOutputStreamSink()
        return own(config)
    }
}

@c(vz_NewVirtioGraphicsScanoutConfiguration)
public func NewVirtioGraphicsScanoutConfiguration(_ width: Int64, _ height: Int64) -> OwnedObject {
    bridgeSync {
        guard #available(macOS 13, *) else { preconditionFailure("API requires macOS 13") }
        return own(VZVirtioGraphicsScanoutConfiguration(widthInPixels: Int(width), heightInPixels: Int(height)))
    }
}

#if arch(arm64)
@c(vz_NewMacGraphicsDisplayConfiguration)
public func NewMacGraphicsDisplayConfiguration(_ width: Int64, _ height: Int64, _ density: Int64) -> OwnedObject {
    bridgeSync {
        guard #available(macOS 12, *) else { preconditionFailure("API requires macOS 12") }
        return own(VZMacGraphicsDisplayConfiguration(widthInPixels: Int(width), heightInPixels: Int(height), pixelsPerInch: Int(density)))
    }
}
#endif

@c(vz_SetSoundStreams)
public func SetSoundStreams(_ object: BorrowedObject, _ values: BorrowedObject) {
    bridgeSync {
        guard #available(macOS 12, *) else { preconditionFailure("API requires macOS 12") }
        borrow(object, as: VZVirtioSoundDeviceConfiguration.self).streams = borrow(values, as: NSArray.self) as! [VZVirtioSoundDeviceStreamConfiguration]
    }
}

@c(vz_SetGraphicsScanouts)
public func SetGraphicsScanouts(_ object: BorrowedObject, _ values: BorrowedObject) {
    bridgeSync {
        guard #available(macOS 13, *) else { preconditionFailure("API requires macOS 13") }
        borrow(object, as: VZVirtioGraphicsDeviceConfiguration.self).scanouts = borrow(values, as: NSArray.self) as! [VZVirtioGraphicsScanoutConfiguration]
    }
}

#if arch(arm64)
@c(vz_SetMacGraphicsDisplays)
public func SetMacGraphicsDisplays(_ object: BorrowedObject, _ values: BorrowedObject) {
    bridgeSync {
        guard #available(macOS 12, *) else { preconditionFailure("API requires macOS 12") }
        borrow(object, as: VZMacGraphicsDeviceConfiguration.self).displays = borrow(values, as: NSArray.self) as! [VZMacGraphicsDisplayConfiguration]
    }
}
#endif

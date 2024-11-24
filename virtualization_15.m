//
//  virtualization_15.m
//
#import "virtualization_15.h"

/*!
 @abstract Check if nested virtualization is supported.
 @return true if supported.
 */
bool isNestedVirtualizationSupported()
{
#ifdef INCLUDE_TARGET_OSX_15
    if (@available(macOS 15, *)) {
        return (bool)VZGenericPlatformConfiguration.isNestedVirtualizationSupported;
    }
#endif
    RAISE_UNSUPPORTED_MACOS_EXCEPTION();
}

/*!
 @abstract Set nestedVirtualizationEnabled. The default is false.
 */
void setNestedVirtualizationEnabled(void *config, bool nestedVirtualizationEnabled)
{
#ifdef INCLUDE_TARGET_OSX_15
    if (@available(macOS 15, *)) {
        VZGenericPlatformConfiguration *platformConfig = (VZGenericPlatformConfiguration *)config;
        platformConfig.nestedVirtualizationEnabled = (BOOL)nestedVirtualizationEnabled;
        return;
    }
#endif
    RAISE_UNSUPPORTED_MACOS_EXCEPTION();
}

/*!
 @abstract Configuration for the USB XHCI controller.
 @discussion This configuration creates a USB XHCI controller device for the guest.
 */
void *newVZXHCIControllerConfiguration()
{
#ifdef INCLUDE_TARGET_OSX_15
    if (@available(macOS 15, *)) {
        return [[VZXHCIControllerConfiguration alloc] init];
    }
#endif
    RAISE_UNSUPPORTED_MACOS_EXCEPTION();
}

void setUSBControllersVZVirtualMachineConfiguration(void *config, void *usbControllers)
{
#ifdef INCLUDE_TARGET_OSX_15
    if (@available(macOS 15, *)) {
        [(VZVirtualMachineConfiguration *)config
            setUsbControllers:[(NSMutableArray *)usbControllers copy]];
        return;
    }
#endif
    RAISE_UNSUPPORTED_MACOS_EXCEPTION();
}

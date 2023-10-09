//
//  virtualization_14.m
//
//  Created by codehex.
//

#import "virtualization_14.h"

/*!
 @abstract Initialize a VZNVMExpressControllerDeviceConfiguration with a device attachment.
 @param attachment The storage device attachment. This defines how the virtualized device operates on the host side.
 @see VZDiskImageStorageDeviceAttachment
 @see https://nvmexpress.org/wp-content/uploads/NVM-Express-1_1b-1.pdf
 */
void *newVZNVMExpressControllerDeviceConfiguration(void *attachment)
{
#ifdef INCLUDE_TARGET_OSX_14
    if (@available(macOS 14, *)) {
        return [[VZNVMExpressControllerDeviceConfiguration alloc] initWithAttachment:(VZStorageDeviceAttachment *)attachment];
    }
#endif
    RAISE_UNSUPPORTED_MACOS_EXCEPTION();
}
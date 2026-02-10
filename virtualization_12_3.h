//
//  virtualization_12_3.h
//
//  Created by codehex.
//

#pragma once

#import "internal/osversion/virtualization_helper.h"
#import <Virtualization/Virtualization.h>

void setBlockDeviceIdentifierVZVirtioBlockDeviceConfiguration(void *blockDeviceConfig, const char *identifier, void **error);
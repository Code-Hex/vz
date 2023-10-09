//
//  virtualization_14_arm64.h
//
//  Created by codehex.
//

#pragma once

#import "virtualization_helper.h"
#import <Virtualization/Virtualization.h>

#ifdef __arm64__
void saveMachineStateToURLWithCompletionHandler(void *machine, void *queue, uintptr_t cgoHandle, const char *saveFilePath);
void restoreMachineStateFromURLWithCompletionHandler(void *machine, void *queue, uintptr_t cgoHandle, const char *saveFilePath);
#endif
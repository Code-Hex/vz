//
//  virtualization_14_arm64.h
//
//  Created by codehex.
//
#pragma once

#ifdef __arm64__

#import "internal/osversion/virtualization_helper.h"
#import <Virtualization/Virtualization.h>

void saveMachineStateToURLWithCompletionHandler(void *machine, void *queue, uintptr_t cgoHandle, const char *saveFilePath);
void restoreMachineStateFromURLWithCompletionHandler(void *machine, void *queue, uintptr_t cgoHandle, const char *saveFilePath);
void *newVZLinuxRosettaAbstractSocketCachingOptionsWithName(const char *name, void **error);
void *newVZLinuxRosettaUnixSocketCachingOptionsWithPath(const char *path, void **error);
uint32_t maximumPathLengthVZLinuxRosettaUnixSocketCachingOptions();
uint32_t maximumNameLengthVZLinuxRosettaAbstractSocketCachingOptions();
void setOptionsVZLinuxRosettaDirectoryShare(void *rosetta, void *cachingOptions);
void *newVZMacKeyboardConfiguration();
bool validateSaveRestoreSupportWithError(void *config, void **error);
#endif
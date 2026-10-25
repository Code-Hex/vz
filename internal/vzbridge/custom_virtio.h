#pragma once
#include "native.h"
void *vz_newCustomVirtioProvider(uintptr_t callback, uint64_t context);
void vz_closeCustomVirtioProvider(void *provider);
void vz_customVirtioUpdate(void *device, void *configuration, uintptr_t callback, uint64_t context);
void vz_customVirtioMap(void *region, void *memory, uint64_t offset, uint64_t size, uintptr_t callback, uint64_t context);
void vz_customVirtioUnmap(void *region, uint64_t offset, uint64_t size, uintptr_t callback, uint64_t context);

enum {
    VirtioCreated,
    VirtioNotified,
    VirtioDriverReady,
    VirtioWillStop,
    VirtioWillPause,
    VirtioWillResume,
    VirtioWillReset,
    VirtioSave,
    VirtioRestore,
    VirtioReleased,
    VirtioCompleted,
};

uint64_t vz_customVirtioConfigurationLength(void *device);
uint64_t vz_customVirtioDeviceID(void *device);

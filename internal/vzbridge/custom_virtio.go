//go:build darwin

package vzbridge

/*
#include "custom_virtio.h"
*/
import "C"

import (
	"runtime"
	"unsafe"

	"github.com/Code-Hex/vz/v4/internal/objc"
	"github.com/ebitengine/purego"
)

var customVirtioNew lazyBinding[func(uintptr, uint64) unsafe.Pointer]

func NewCustomVirtioProvider(callback uintptr, context uint64) *objc.Pointer {
	call := customVirtioNew.get(func() func(uintptr, uint64) unsafe.Pointer {
		var call func(uintptr, uint64) unsafe.Pointer
		purego.RegisterFunc(&call, uintptr(C.vz_newCustomVirtioProvider))
		return call
	})
	var p unsafe.Pointer
	onQueue(func() { p = call(callback, context) })
	return objc.NewManagedPointer(p, ReleaseObject)
}

var customVirtioClaim lazyBinding[func(unsafe.Pointer) bool]

func ClaimCustomVirtioProviders(configuration objc.NSObject) bool {
	call := customVirtioClaim.get(func() func(unsafe.Pointer) bool {
		var call func(unsafe.Pointer) bool
		purego.RegisterFunc(&call, uintptr(C.vz_claimCustomVirtioProviders))
		return call
	})
	var claimed bool
	onQueue(func() { claimed = call(objc.Ptr(configuration)) })
	runtime.KeepAlive(configuration)
	return claimed
}

var customVirtioClose lazyBinding[func(unsafe.Pointer)]

func CloseCustomVirtioProvider(provider objc.NSObject) {
	call := customVirtioClose.get(func() func(unsafe.Pointer) {
		var call func(unsafe.Pointer)
		purego.RegisterFunc(&call, uintptr(C.vz_closeCustomVirtioProvider))
		return call
	})
	onObjectQueue(objc.Ptr(provider), func() { call(objc.Ptr(provider)) })
	runtime.KeepAlive(provider)
}

var customVirtioUpdate lazyBinding[func(unsafe.Pointer, unsafe.Pointer, uintptr, uint64)]

func UpdateCustomVirtioConfiguration(device, configuration objc.NSObject, callback uintptr, context uint64) {
	call := customVirtioUpdate.get(func() func(unsafe.Pointer, unsafe.Pointer, uintptr, uint64) {
		var call func(unsafe.Pointer, unsafe.Pointer, uintptr, uint64)
		purego.RegisterFunc(&call, uintptr(C.vz_customVirtioUpdate))
		return call
	})
	onObjectQueue(objc.Ptr(device), func() { call(objc.Ptr(device), objc.Ptr(configuration), callback, context) })
	runtime.KeepAlive(device)
	runtime.KeepAlive(configuration)
}

var customVirtioMap lazyBinding[func(unsafe.Pointer, unsafe.Pointer, uint64, uint64, uintptr, uint64)]

func MapCustomVirtioMemory(region objc.NSObject, memory unsafe.Pointer, offset, size uint64, callback uintptr, context uint64) {
	call := customVirtioMap.get(func() func(unsafe.Pointer, unsafe.Pointer, uint64, uint64, uintptr, uint64) {
		var call func(unsafe.Pointer, unsafe.Pointer, uint64, uint64, uintptr, uint64)
		purego.RegisterFunc(&call, uintptr(C.vz_customVirtioMap))
		return call
	})
	onObjectQueue(objc.Ptr(region), func() { call(objc.Ptr(region), memory, offset, size, callback, context) })
	runtime.KeepAlive(region)
}

var customVirtioUnmap lazyBinding[func(unsafe.Pointer, uint64, uint64, uintptr, uint64)]

func UnmapCustomVirtioMemory(region objc.NSObject, offset, size uint64, callback uintptr, context uint64) {
	call := customVirtioUnmap.get(func() func(unsafe.Pointer, uint64, uint64, uintptr, uint64) {
		var call func(unsafe.Pointer, uint64, uint64, uintptr, uint64)
		purego.RegisterFunc(&call, uintptr(C.vz_customVirtioUnmap))
		return call
	})
	onObjectQueue(objc.Ptr(region), func() { call(objc.Ptr(region), offset, size, callback, context) })
	runtime.KeepAlive(region)
}

func TransferCustomVirtioData(data objc.NSObject) uintptr {
	result := uintptr(directRetain(objc.Ptr(data)))
	objc.Release(data)
	return result
}

func OnCustomVirtioQueue(object objc.NSObject, body func()) {
	onObjectQueue(objc.Ptr(object), body)
	runtime.KeepAlive(object)
}

const (
	CustomVirtioCreated     = C.VirtioCreated
	CustomVirtioNotified    = C.VirtioNotified
	CustomVirtioDriverReady = C.VirtioDriverReady
	CustomVirtioWillStop    = C.VirtioWillStop
	CustomVirtioWillPause   = C.VirtioWillPause
	CustomVirtioWillResume  = C.VirtioWillResume
	CustomVirtioWillReset   = C.VirtioWillReset
	CustomVirtioSave        = C.VirtioSave
	CustomVirtioRestore     = C.VirtioRestore
	CustomVirtioReleased    = C.VirtioReleased
	CustomVirtioCompleted   = C.VirtioCompleted
)

var customVirtioLength lazyBinding[func(unsafe.Pointer) uint64]

func CustomVirtioConfigurationLength(device objc.NSObject) uint64 {
	call := customVirtioLength.get(func() func(unsafe.Pointer) uint64 {
		var call func(unsafe.Pointer) uint64
		purego.RegisterFunc(&call, uintptr(C.vz_customVirtioConfigurationLength))
		return call
	})
	var length uint64
	onObjectQueue(objc.Ptr(device), func() { length = call(objc.Ptr(device)) })
	runtime.KeepAlive(device)
	return length
}

var customVirtioDeviceID lazyBinding[func(unsafe.Pointer) uint64]

func CustomVirtioDeviceID(device objc.NSObject) uint64 {
	call := customVirtioDeviceID.get(func() func(unsafe.Pointer) uint64 {
		var call func(unsafe.Pointer) uint64
		purego.RegisterFunc(&call, uintptr(C.vz_customVirtioDeviceID))
		return call
	})
	id := call(objc.Ptr(device))
	runtime.KeepAlive(device)
	return id
}

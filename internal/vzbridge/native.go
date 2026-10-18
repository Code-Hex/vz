//go:build darwin

package vzbridge

/*
#cgo CFLAGS: -flto -mmacosx-version-min=11 -x objective-c -fno-objc-arc
#cgo LDFLAGS: -framework Foundation -framework Virtualization -framework Cocoa -framework vmnet
#include "native.h"
*/
import "C"

import (
	"runtime"
	"runtime/cgo"
	"unsafe"

	"github.com/ebitengine/purego"
)

//go:generate go run ./nativegen

const SDKVersion = C.__MAC_OS_X_VERSION_MAX_ALLOWED

const (
	CallbackCompletion                  = C.VZBridgeCallbackCompletion
	CallbackMachineStateChanged         = C.VZBridgeCallbackMachineStateChanged
	CallbackNetworkDisconnected         = C.VZBridgeCallbackNetworkDisconnected
	CallbackContextReleased             = C.VZBridgeCallbackContextReleased
	CallbackSocketConnected             = C.VZBridgeCallbackSocketConnected
	CallbackSocketAccepted              = C.VZBridgeCallbackSocketAccepted
	CallbackNetworkBlockDeviceError     = C.VZBridgeCallbackNetworkBlockDeviceError
	CallbackNetworkBlockDeviceConnected = C.VZBridgeCallbackNetworkBlockDeviceConnected
	CallbackRosettaInstallCompleted     = C.VZBridgeCallbackRosettaInstallCompleted
	CallbackRestoreImageLoaded          = C.VZBridgeCallbackRestoreImageLoaded
	CallbackMacOSInstallCompleted       = C.VZBridgeCallbackMacOSInstallCompleted
	CallbackMacOSInstallProgress        = C.VZBridgeCallbackMacOSInstallProgress
)

type GraphicApplicationResult int32

const (
	GraphicApplicationSuccess        GraphicApplicationResult = C.VZGraphicApplicationSuccess
	GraphicApplicationNotMainThread  GraphicApplicationResult = C.VZGraphicApplicationNotMainThread
	GraphicApplicationAlreadyRunning GraphicApplicationResult = C.VZGraphicApplicationAlreadyRunning
)

var callRunGraphicApplication lazyBinding[func(unsafe.Pointer) GraphicApplicationResult]

func RunGraphicApplication(application unsafe.Pointer) GraphicApplicationResult {
	call := callRunGraphicApplication.get(func() func(unsafe.Pointer) GraphicApplicationResult {
		var call func(unsafe.Pointer) GraphicApplicationResult
		purego.RegisterFunc(&call, uintptr(C.vz_runGraphicApplication))
		return call
	})
	return call(application)
}

var dispatchSyncOnObject func(unsafe.Pointer, uintptr, uintptr)
var queueCallback uintptr

type queueCall struct {
	body       func()
	panicValue any
}

func init() {
	purego.RegisterFunc(&dispatchSyncOnObject, uintptr(C.vz_dispatchSyncOnObject))
	queueCallback = purego.NewCallback(func(context uintptr) {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		call := cgo.Handle(context).Value().(*queueCall)
		defer func() { call.panicValue = recover() }()
		call.body()
	})
}

func onQueue(body func()) {
	onObjectQueue(nil, body)
}

func onObjectQueue(object unsafe.Pointer, body func()) {
	call := &queueCall{body: body}
	handle := cgo.NewHandle(call)
	defer handle.Delete()
	dispatchSyncOnObject(object, queueCallback, uintptr(handle))
	if call.panicValue != nil {
		panic(call.panicValue)
	}
}

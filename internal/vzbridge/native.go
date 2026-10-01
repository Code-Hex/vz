//go:build darwin

package vzbridge

/*
#cgo CFLAGS: -flto -mmacosx-version-min=11 -x objective-c -fno-objc-arc
#cgo LDFLAGS: -framework Foundation -framework Virtualization -framework Cocoa
#include "native.h"
*/
import "C"

import (
	"runtime"
	"runtime/cgo"

	"github.com/ebitengine/purego"
)

//go:generate go run ./nativegen

const SDKVersion = C.__MAC_OS_X_VERSION_MAX_ALLOWED

var dispatchSync func(uintptr, uintptr)
var queueCallback uintptr

type queueCall struct {
	body       func()
	panicValue any
}

func init() {
	purego.RegisterFunc(&dispatchSync, uintptr(C.vz_dispatchSync))
	queueCallback = purego.NewCallback(func(context uintptr) {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		call := cgo.Handle(context).Value().(*queueCall)
		defer func() { call.panicValue = recover() }()
		call.body()
	})
}

func onQueue(body func()) {
	call := &queueCall{body: body}
	handle := cgo.NewHandle(call)
	defer handle.Delete()
	dispatchSync(queueCallback, uintptr(handle))
	if call.panicValue != nil {
		panic(call.panicValue)
	}
}

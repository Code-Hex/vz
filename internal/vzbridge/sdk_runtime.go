//go:build darwin

package vzbridge

import (
	"fmt"
	"sync"
	"syscall"
	"unsafe"

	runtimeobjc "github.com/ebitengine/purego/objc"
	"golang.org/x/mod/semver"
)

var sdkOSVersion = func() string {
	version, err := syscall.Sysctl("kern.osproductversion")
	if err != nil {
		panic(err)
	}
	return version
}()
var sdkAvailable sync.Map

func sdkVersionAvailable(version, minimum, obsolete string) bool {
	return (minimum == "" || semver.Compare("v"+version, "v"+minimum) >= 0) && (obsolete == "" || semver.Compare("v"+version, "v"+obsolete) < 0)
}

func sdkRequireVersion(name, minimum, obsolete string) {
	if !sdkVersionAvailable(sdkOSVersion, minimum, obsolete) {
		panic(fmt.Sprintf("%s requires macOS %s (running %s)", name, minimum, sdkOSVersion))
	}
}

func sdkRequire(class, selector, minimum, obsolete string, instance bool, receiver ...unsafe.Pointer) {
	sdkRequireVersion(class+":"+selector, minimum, obsolete)
	if instance && len(receiver) > 0 && receiver[0] == nil {
		return
	}
	key := class + "+" + selector
	if instance {
		key = class + "-" + selector
	}
	if len(receiver) > 0 {
		key += fmt.Sprintf("/%p", directObjectClass(receiver[0]))
	}
	if _, ok := sdkAvailable.Load(key); ok {
		return
	}
	owner := directClass(class)
	if instance && len(receiver) > 0 {
		actual := directObjectClass(receiver[0])
		ancestor := actual
		for ancestor != nil && ancestor != owner {
			ancestor = directSuperclass(ancestor)
		}
		if ancestor == nil {
			panic(fmt.Sprintf("receiver is not an instance of %s", class))
		}
		owner = actual
	}
	if !instance {
		owner = directObjectClass(owner)
	}
	if directInstanceMethod(owner, runtimeobjc.RegisterName(selector)) == nil {
		panic(fmt.Sprintf("Objective-C method %s is unavailable", key))
	}
	sdkAvailable.Store(key, struct{}{})
}

func sdkRequireInitializer(class, selector, minimum, obsolete string, allocated unsafe.Pointer) {
	defer func() {
		if failure := recover(); failure != nil {
			directRelease(allocated)
			panic(failure)
		}
	}()
	sdkRequire(class, selector, minimum, obsolete, true, allocated)
}

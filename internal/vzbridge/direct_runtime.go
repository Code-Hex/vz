//go:build darwin

package vzbridge

import (
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"github.com/Code-Hex/vz/v4/internal/objc"
	"github.com/ebitengine/purego"
	runtimeobjc "github.com/ebitengine/purego/objc"
)

//go:generate go run ../../cmd/vzbridgegen -input ../../cmd/vzbridgegen/metadata/sdk.json -output .

type directBinding[T any] struct {
	call     T
	selector runtimeobjc.SEL
}

type lazyBinding[T any] struct {
	once    sync.Once
	value   T
	failure any
	ready   bool
}

func (b *lazyBinding[T]) get(load func() T) T {
	b.once.Do(func() {
		defer func() { b.failure = recover() }()
		b.value = load()
		b.ready = true
	})
	if !b.ready {
		panic(b.failure)
	}
	return b.value
}

var directMessageAddress lazyBinding[uintptr]

func bindDirect[T any](selector string) directBinding[T] {
	address := directMessageAddress.get(func() uintptr {
		address, err := purego.Dlsym(purego.RTLD_DEFAULT, "objc_msgSend")
		if err != nil {
			panic(err)
		}
		return address
	})
	var call T
	purego.RegisterFunc(&call, address)
	return directBinding[T]{call: call, selector: runtimeobjc.RegisterName(selector)}
}

func directSymbol[T any](name string) T {
	var call T
	purego.RegisterLibFunc(&call, purego.RTLD_DEFAULT, name)
	return call
}

var directClassCall lazyBinding[func(string) unsafe.Pointer]

func directGetClass(name string) unsafe.Pointer {
	call := directClassCall.get(func() func(string) unsafe.Pointer {
		return directSymbol[func(string) unsafe.Pointer]("objc_getClass")
	})
	return call(name)
}

var directObjectClassCall lazyBinding[func(unsafe.Pointer) unsafe.Pointer]

func directObjectClass(object unsafe.Pointer) unsafe.Pointer {
	call := directObjectClassCall.get(func() func(unsafe.Pointer) unsafe.Pointer {
		return directSymbol[func(unsafe.Pointer) unsafe.Pointer]("object_getClass")
	})
	return call(object)
}

var directSuperclassCall lazyBinding[func(unsafe.Pointer) unsafe.Pointer]

func directSuperclass(class unsafe.Pointer) unsafe.Pointer {
	call := directSuperclassCall.get(func() func(unsafe.Pointer) unsafe.Pointer {
		return directSymbol[func(unsafe.Pointer) unsafe.Pointer]("class_getSuperclass")
	})
	return call(class)
}

var directInstanceMethodCall lazyBinding[func(unsafe.Pointer, runtimeobjc.SEL) unsafe.Pointer]

func directInstanceMethod(class unsafe.Pointer, selector runtimeobjc.SEL) unsafe.Pointer {
	call := directInstanceMethodCall.get(func() func(unsafe.Pointer, runtimeobjc.SEL) unsafe.Pointer {
		return directSymbol[func(unsafe.Pointer, runtimeobjc.SEL) unsafe.Pointer]("class_getInstanceMethod")
	})
	return call(class, selector)
}

var directAllocMessage lazyBinding[directBinding[func(unsafe.Pointer, runtimeobjc.SEL) unsafe.Pointer]]
var directRetainMessage lazyBinding[directBinding[func(unsafe.Pointer, runtimeobjc.SEL) unsafe.Pointer]]
var directReleaseMessage lazyBinding[directBinding[func(unsafe.Pointer, runtimeobjc.SEL)]]

func directClass(name string) unsafe.Pointer {
	class := directGetClass(name)
	if class == nil {
		panic(fmt.Sprintf("Objective-C class %s is unavailable", name))
	}
	return class
}
func directAllocate(class unsafe.Pointer) unsafe.Pointer {
	binding := directAllocMessage.get(func() directBinding[func(unsafe.Pointer, runtimeobjc.SEL) unsafe.Pointer] {
		return bindDirect[func(unsafe.Pointer, runtimeobjc.SEL) unsafe.Pointer]("alloc")
	})
	return binding.call(class, binding.selector)
}
func directRetain(object unsafe.Pointer) unsafe.Pointer {
	binding := directRetainMessage.get(func() directBinding[func(unsafe.Pointer, runtimeobjc.SEL) unsafe.Pointer] {
		return bindDirect[func(unsafe.Pointer, runtimeobjc.SEL) unsafe.Pointer]("retain")
	})
	return binding.call(object, binding.selector)
}
func directRelease(object unsafe.Pointer) {
	binding := directReleaseMessage.get(func() directBinding[func(unsafe.Pointer, runtimeobjc.SEL)] {
		return bindDirect[func(unsafe.Pointer, runtimeobjc.SEL)]("release")
	})
	binding.call(object, binding.selector)
}
func directOwned(body func() unsafe.Pointer) *objc.Pointer {
	var result unsafe.Pointer
	onQueue(func() { result = body() })
	return objc.NewManagedPointer(result, ReleaseObject)
}

// RetainObject returns a pointer with independent ownership of the object.
func RetainObject(p0 objc.NSObject) *objc.Pointer {
	value := directOwned(func() unsafe.Pointer { return directRetain(objc.Ptr(p0)) })
	runtime.KeepAlive(p0)
	return value
}

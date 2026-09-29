//go:build darwin

package objc

import (
	"fmt"
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego"
	pureobjc "github.com/ebitengine/purego/objc"
)

var (
	selAlloc         = pureobjc.RegisterName("alloc")
	selInit          = pureobjc.RegisterName("init")
	selRelease       = pureobjc.RegisterName("release")
	selRetain        = pureobjc.RegisterName("retain")
	selCount         = pureobjc.RegisterName("count")
	selObjectAtIndex = pureobjc.RegisterName("objectAtIndex:")
)

func init() {
	if _, err := purego.Dlopen("/System/Library/Frameworks/Foundation.framework/Foundation", purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
		panic(fmt.Errorf("vz: load Foundation: %w", err))
	}
}

// NewObject allocates and initializes an object. The caller owns its release.
// The class must be available on the running OS.
func NewObject(className string) *Pointer {
	allocated := pureobjc.ID(pureobjc.GetClass(className)).Send(selAlloc)
	return NewPointer(pureobjc.Send[unsafe.Pointer](allocated, selInit))
}

// Pointer indicates any pointers which are allocated in objective-c world.
type Pointer struct {
	_ptr    unsafe.Pointer
	managed *managedReference
}

// NewPointer creates a new Pointer for objc
func NewPointer(p unsafe.Pointer) *Pointer {
	return &Pointer{_ptr: p}
}

// release releases allocated resources in objective-c world.
// decrements reference count.
func (p *Pointer) release() {
	if p.managed != nil {
		p.managed.close()
		return
	}
	pureobjc.ID(uintptr(p._ptr)).Send(selRelease)
	runtime.KeepAlive(p)
}

// retain increments reference count in objective-c world.
func (p *Pointer) retain() {
	pureobjc.ID(uintptr(p._ptr)).Send(selRetain)
	runtime.KeepAlive(p)
}

// Ptr returns raw pointer.
func (o *Pointer) ptr() unsafe.Pointer {
	if o == nil {
		return nil
	}
	return o._ptr
}

// NSObject indicates NSObject
type NSObject interface {
	ptr() unsafe.Pointer
	release()
	retain()
}

// Release releases allocated resources in objective-c world.
func Release(o NSObject) {
	o.release()
}

// Retain increments reference count in objective-c world.
func Retain(o NSObject) {
	o.retain()
}

// Ptr returns unsafe.Pointer of the NSObject
func Ptr(o NSObject) unsafe.Pointer {
	if o == nil {
		return nil
	}
	return o.ptr()
}

// NSArray indicates NSArray
type NSArray struct {
	*Pointer
}

// NewNSArray creates a new NSArray from pointer.
func NewNSArray(p unsafe.Pointer) *NSArray {
	return &NSArray{NewPointer(p)}
}

// ToPointerSlice method returns slice of the obj-c object as unsafe.Pointer.
func (n *NSArray) ToPointerSlice() []unsafe.Pointer {
	count := int(pureobjc.ID(uintptr(n.ptr())).Send(selCount))
	ret := make([]unsafe.Pointer, count)
	for i := 0; i < count; i++ {
		ret[i] = pureobjc.Send[unsafe.Pointer](pureobjc.ID(uintptr(n.ptr())), selObjectAtIndex, uintptr(i))
	}
	runtime.KeepAlive(n)
	return ret
}

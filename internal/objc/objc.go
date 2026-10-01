//go:build darwin

package objc

import "unsafe"

// Pointer indicates any pointers which are allocated in objective-c world.
type Pointer struct {
	managed *managedOwner
}

func (p *Pointer) release() {
	if p.managed != nil {
		p.managed.reference.close()
	}
}

// Ptr returns raw pointer.
func (o *Pointer) ptr() unsafe.Pointer {
	if o == nil || o.managed == nil {
		return nil
	}
	return o.managed.reference.pointer
}

// NSObject indicates NSObject
type NSObject interface {
	ptr() unsafe.Pointer
	release()
}

// Release releases allocated resources in objective-c world.
func Release(o NSObject) {
	o.release()
}

// Ptr returns unsafe.Pointer of the NSObject
func Ptr(o NSObject) unsafe.Pointer {
	if o == nil {
		return nil
	}
	return o.ptr()
}

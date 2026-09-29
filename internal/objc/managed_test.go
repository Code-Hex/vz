//go:build darwin

package objc

import (
	"runtime"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"
)

func TestManagedPointerSharesLifetime(t *testing.T) {
	object := NewObject("NSObject")
	address := Ptr(object)
	released := make(chan struct{}, 1)
	owner := NewManagedPointer(address, func(p unsafe.Pointer) {
		Release(NewPointer(p))
		released <- struct{}{}
	})
	alias := owner
	owner = nil
	for range 3 {
		runtime.GC()
	}
	select {
	case <-released:
		t.Fatal("released while alias remained live")
	default:
	}
	runtime.KeepAlive(alias)
	alias = nil
	deadline := time.After(5 * time.Second)
	for {
		runtime.GC()
		select {
		case <-released:
			return
		case <-deadline:
			t.Fatal("managed reference was not released")
		case <-time.After(time.Millisecond * 10):
		}
	}
}

func TestManagedPointerReleaseIsIdempotent(t *testing.T) {
	object := NewObject("NSObject")
	var releases atomic.Int32
	p := NewManagedPointer(Ptr(object), func(address unsafe.Pointer) {
		Release(NewPointer(address))
		releases.Add(1)
	})
	Release(p)
	Release(p)
	if releases.Load() != 1 {
		t.Fatal("native ownership released more than once")
	}
	runtime.KeepAlive(p)
}

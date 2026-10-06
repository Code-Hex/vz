//go:build darwin

package objc

import (
	"runtime"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	pureobjc "github.com/ebitengine/purego/objc"
)

func TestManagedPointerSharesLifetime(t *testing.T) {
	address := pureobjc.Send[unsafe.Pointer](pureobjc.ID(pureobjc.GetClass("NSObject")), pureobjc.RegisterName("new"))
	released := make(chan struct{}, 1)
	owner := NewManagedPointer(address, func(p unsafe.Pointer) {
		pureobjc.ID(uintptr(p)).Send(pureobjc.RegisterName("release"))
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
	address := pureobjc.Send[unsafe.Pointer](pureobjc.ID(pureobjc.GetClass("NSObject")), pureobjc.RegisterName("new"))
	var releases atomic.Int32
	p := NewManagedPointer(address, func(address unsafe.Pointer) {
		pureobjc.ID(uintptr(address)).Send(pureobjc.RegisterName("release"))
		releases.Add(1)
	})
	Release(p)
	Release(p)
	if releases.Load() != 1 {
		t.Fatal("native ownership released more than once")
	}
	runtime.KeepAlive(p)
}

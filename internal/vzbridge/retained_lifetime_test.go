//go:build darwin

package vzbridge

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"github.com/Code-Hex/vz/v4/internal/objc"
	runtimeobjc "github.com/ebitengine/purego/objc"
)

var retainedTestClass = sync.OnceValue(func() runtimeobjc.Class {
	class, err := runtimeobjc.RegisterClass("VZRetainedLifetimeTestObject", runtimeobjc.GetClass("NSObject"), nil, nil, []runtimeobjc.MethodDef{{
		Cmd: runtimeobjc.RegisterName("dealloc"),
		Fn: func(self runtimeobjc.ID, cmd runtimeobjc.SEL) {
			counter, ok := retainedTestDeallocations.LoadAndDelete(uintptr(self))
			self.SendSuper(cmd)
			if ok {
				counter.(*atomic.Int32).Add(1)
			}
		},
	}, {
		Cmd: runtimeobjc.RegisterName("marker"),
		Fn:  func(runtimeobjc.ID, runtimeobjc.SEL) int64 { return 42 },
	}})
	if err != nil {
		panic(err)
	}
	return class
})

var retainedTestDeallocations sync.Map

func newRetainedTestObject() (*objc.Pointer, *atomic.Int32) {
	deallocations := new(atomic.Int32)
	owner := directOwned(func() unsafe.Pointer {
		address := runtimeobjc.Send[unsafe.Pointer](runtimeobjc.ID(retainedTestClass()), runtimeobjc.RegisterName("new"))
		retainedTestDeallocations.Store(uintptr(address), deallocations)
		return address
	})
	return owner, deallocations
}

func collectRetainedTestGarbage() {
	for range 5 {
		runtime.GC()
		onQueue(func() {})
		time.Sleep(10 * time.Millisecond)
	}
}

func assertRetainedTestObjectUsable(t *testing.T, owner objc.NSObject) {
	t.Helper()
	var marker int64
	onQueue(func() {
		marker = runtimeobjc.Send[int64](runtimeobjc.ID(uintptr(objc.Ptr(owner))), runtimeobjc.RegisterName("marker"))
	})
	runtime.KeepAlive(owner)
	if marker != 42 {
		t.Fatalf("native marker = %d, want 42", marker)
	}
}

func waitForRetainedTestDeallocation(t *testing.T, deallocations *atomic.Int32) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for deallocations.Load() == 0 && time.Now().Before(deadline) {
		collectRetainedTestGarbage()
	}
	if got := deallocations.Load(); got != 1 {
		t.Fatalf("native deallocations = %d, want 1", got)
	}
	collectRetainedTestGarbage()
	if got := deallocations.Load(); got != 1 {
		t.Fatalf("native deallocations after further GC = %d, want 1", got)
	}
}

func TestRetainObjectIndependentOwnership(t *testing.T) {
	for _, firstOwner := range []string{"original", "retained"} {
		for _, firstCleanup := range []string{"explicit", "gc"} {
			for _, lastCleanup := range []string{"explicit", "gc"} {
				t.Run(firstOwner+"_first/"+firstCleanup+"/"+lastCleanup, func(t *testing.T) {
					original, deallocations := newRetainedTestObject()
					retained := RetainObject(original)
					first, last := original, retained
					if firstOwner == "retained" {
						first, last = retained, original
					}
					original, retained = nil, nil
					if firstCleanup == "explicit" {
						objc.Release(first)
						objc.Release(first)
					}
					first = nil
					collectRetainedTestGarbage()
					got := deallocations.Load()
					if got != 0 {
						t.Fatalf("native deallocations while the other owner remains live = %d, want 0", got)
					}
					assertRetainedTestObjectUsable(t, last)
					runtime.KeepAlive(last)
					if lastCleanup == "explicit" {
						objc.Release(last)
						objc.Release(last)
					}
					last = nil
					waitForRetainedTestDeallocation(t, deallocations)
				})
			}
		}
	}
}

//go:noinline
func copyRetainedTestObject() (objc.Pointer, *atomic.Int32) {
	original, deallocations := newRetainedTestObject()
	retained := RetainObject(original)
	objc.Release(original)
	return *retained, deallocations
}

func TestRetainObjectValueCopyKeepsObjectAlive(t *testing.T) {
	copied, deallocations := copyRetainedTestObject()
	collectRetainedTestGarbage()
	got := deallocations.Load()
	if got != 0 {
		t.Fatalf("native deallocations with a live retained Pointer value copy = %d, want 0", got)
	}
	assertRetainedTestObjectUsable(t, &copied)
	runtime.KeepAlive(&copied)
	copied = objc.Pointer{}
	waitForRetainedTestDeallocation(t, deallocations)
}

func TestRetainObjectNil(t *testing.T) {
	var typedNil *objc.Pointer
	for _, object := range []objc.NSObject{nil, typedNil} {
		if retained := RetainObject(object); retained != nil {
			t.Fatalf("RetainObject(%v) = %v, want nil", object, retained)
		}
	}
}

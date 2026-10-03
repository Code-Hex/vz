//go:build darwin

package objc

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	pureobjc "github.com/ebitengine/purego/objc"
)

var managedTestClass = sync.OnceValue(func() pureobjc.Class {
	class, err := pureobjc.RegisterClass("VZManagedLifetimeTestObject", pureobjc.GetClass("NSObject"), nil, nil, []pureobjc.MethodDef{{
		Cmd: pureobjc.RegisterName("dealloc"),
		Fn: func(self pureobjc.ID, cmd pureobjc.SEL) {
			counter, ok := managedTestDeallocations.LoadAndDelete(uintptr(self))
			self.SendSuper(cmd)
			if ok {
				counter.(*atomic.Int32).Add(1)
			}
		},
	}})
	if err != nil {
		panic(err)
	}
	return class
})

var managedTestDeallocations sync.Map

func newManagedTestObject() (*Pointer, *atomic.Int32) {
	address := pureobjc.Send[unsafe.Pointer](pureobjc.ID(managedTestClass()), pureobjc.RegisterName("new"))
	deallocations := new(atomic.Int32)
	managedTestDeallocations.Store(uintptr(address), deallocations)
	return NewManagedPointer(address, func(p unsafe.Pointer) { pureobjc.ID(uintptr(p)).Send(pureobjc.RegisterName("release")) }), deallocations
}

func collectManagedTestGarbage() {
	for range 5 {
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}
}

func waitForManagedTestDeallocation(t *testing.T, deallocations *atomic.Int32) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for deallocations.Load() == 0 && time.Now().Before(deadline) {
		collectManagedTestGarbage()
	}
	if got := deallocations.Load(); got != 1 {
		t.Fatalf("native deallocations = %d, want 1", got)
	}
}

//go:noinline
func copyManagedTestObject() (Pointer, *atomic.Int32) {
	p, deallocations := newManagedTestObject()
	return *p, deallocations
}

func TestManagedPointerValueCopyKeepsObjectAlive(t *testing.T) {
	copied, deallocations := copyManagedTestObject()
	collectManagedTestGarbage()
	got := deallocations.Load()
	runtime.KeepAlive(&copied)
	if got != 0 {
		t.Fatalf("native deallocations with a live Pointer value copy = %d, want 0", got)
	}
	copied = Pointer{}
	waitForManagedTestDeallocation(t, deallocations)
}

func TestManagedPointerConcurrentValueCopiesReleaseOnce(t *testing.T) {
	copied, deallocations := copyManagedTestObject()
	release := make(chan struct{})
	var workers sync.WaitGroup
	for range 64 {
		workers.Add(1)
		go func(alias Pointer) {
			defer workers.Done()
			<-release
			Release(&alias)
		}(copied)
	}
	copied = Pointer{}
	collectManagedTestGarbage()
	got := deallocations.Load()
	close(release)
	workers.Wait()
	if got != 0 {
		t.Fatalf("native deallocations with live concurrent Pointer copies = %d, want 0", got)
	}
	if got := deallocations.Load(); got != 1 {
		t.Fatalf("native deallocations after concurrent Release = %d, want 1", got)
	}
	collectManagedTestGarbage()
	if got := deallocations.Load(); got != 1 {
		t.Fatalf("native deallocations after Release and GC = %d, want 1", got)
	}
}

//go:build darwin

package objc

import (
	"runtime"
	"testing"
)

type initializationTestPointer = Pointer

type initializationTestObject struct{ initializationTestPointer }

func TestInitializeManagedPointerSharesLifetime(t *testing.T) {
	source, deallocations := newManagedTestObject()
	address := Ptr(source)
	object := new(initializationTestObject)
	InitializeManagedPointer(object, source)
	if got := Ptr(object); got != address {
		t.Fatalf("initialized pointer = %p, want %p", got, address)
	}
	copy := *object
	source = nil
	object = nil
	collectManagedTestGarbage()
	if got := deallocations.Load(); got != 0 {
		t.Fatalf("native deallocations with a live initialized copy = %d, want 0", got)
	}
	Release(&copy)
	Release(&copy)
	if got := deallocations.Load(); got != 1 {
		t.Fatalf("native deallocations after Release = %d, want 1", got)
	}
	runtime.KeepAlive(&copy)
	copy = initializationTestObject{}
	collectManagedTestGarbage()
	if got := deallocations.Load(); got != 1 {
		t.Fatalf("native deallocations after Release and GC = %d, want 1", got)
	}
}

func TestInitializeManagedPointerRejectsEmptySource(t *testing.T) {
	for _, tt := range []struct {
		name   string
		source *Pointer
	}{{"nil", nil}, {"zero", new(Pointer)}} {
		t.Run(tt.name, func(t *testing.T) {
			object := new(initializationTestObject)
			defer func() {
				if got := recover(); got != "objc: managed pointer source is nil or uninitialized" {
					t.Fatalf("panic = %v", got)
				}
				if Ptr(object) != nil {
					t.Fatal("empty source changed the destination")
				}
			}()
			InitializeManagedPointer(object, tt.source)
		})
	}
}

func TestInitializeManagedPointerRejectsOverwrite(t *testing.T) {
	original, originalDeallocations := newManagedTestObject()
	replacement, replacementDeallocations := newManagedTestObject()
	defer Release(original)
	defer Release(replacement)
	object := new(initializationTestObject)
	InitializeManagedPointer(object, original)
	defer func() {
		if got := recover(); got != "objc: managed pointer destination is already initialized" {
			t.Fatalf("panic = %v", got)
		}
		if Ptr(object) != Ptr(original) {
			t.Fatal("overwrite changed the destination")
		}
		if originalDeallocations.Load() != 0 || replacementDeallocations.Load() != 0 {
			t.Fatal("rejected overwrite released a live native reference")
		}
	}()
	InitializeManagedPointer(object, replacement)
}

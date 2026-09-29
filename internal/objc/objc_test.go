//go:build darwin

package objc

import (
	"testing"
	"unsafe"

	pureobjc "github.com/ebitengine/purego/objc"
)

func testString(t *testing.T, value string) *Pointer {
	t.Helper()
	allocated := pureobjc.ID(pureobjc.GetClass("NSMutableString")).Send(pureobjc.RegisterName("alloc"))
	p := pureobjc.Send[unsafe.Pointer](allocated, pureobjc.RegisterName("initWithUTF8String:"), value)
	return NewPointer(p)
}

func nativeString(p unsafe.Pointer) string {
	return pureobjc.Send[string](pureobjc.ID(uintptr(p)), pureobjc.RegisterName("UTF8String"))
}

func TestRetainKeepsObjectAlive(t *testing.T) {
	value := testString(t, "retained")
	Retain(value)
	Release(value)
	defer Release(value)
	if got := nativeString(Ptr(value)); got != "retained" {
		t.Fatalf("retained string = %q", got)
	}
}

func TestNilPointer(t *testing.T) {
	var p *Pointer
	if Ptr(nil) != nil || Ptr(p) != nil || Ptr(NewPointer(nil)) != nil {
		t.Fatal("nil object must return a nil pointer")
	}
	Retain(NewPointer(nil))
	Release(NewPointer(nil))
	if got := NewNSArray(nil).ToPointerSlice(); len(got) != 0 {
		t.Fatalf("nil NSArray = %v", got)
	}
}

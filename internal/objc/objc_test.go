//go:build darwin

package objc

import "testing"

func TestNilPointer(t *testing.T) {
	var p *Pointer
	if Ptr(nil) != nil || Ptr(p) != nil || Ptr(&Pointer{}) != nil {
		t.Fatal("nil object must return a nil pointer")
	}
	Release(&Pointer{})
}

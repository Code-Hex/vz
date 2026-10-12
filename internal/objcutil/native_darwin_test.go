package objcutil_test

import (
	"bytes"
	"testing"

	"github.com/Code-Hex/vz/v4/internal/objc"
	"github.com/Code-Hex/vz/v4/internal/objcutil"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
)

func TestNativeCopies(t *testing.T) {
	const want = "Shared network \u65e5\u672c\u8a9e"
	object := vzbridge.NSString_StringWithUTF8String(want)
	if got := objcutil.String(object); got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
	object = vzbridge.NSString_StringWithUTF8String(want)
	data := vzbridge.NSString_DataUsingEncoding(object, vzbridge.NSUTF8StringEncoding)
	objc.Release(object)
	got := objcutil.Bytes(data)
	vzbridge.Drain()
	if !bytes.Equal(got, []byte(want)) {
		t.Fatalf("Bytes() = %q, want %q", got, want)
	}
	if got := objcutil.NewNSError(nil); got != nil {
		t.Fatalf("NewNSError(nil) = %v, want nil", got)
	}
	if got := objcutil.String(nil); got != "" {
		t.Fatalf("String(nil) = %q, want empty", got)
	}
	if got := objcutil.Bytes(nil); got != nil {
		t.Fatalf("Bytes(nil) = %v, want nil", got)
	}
}

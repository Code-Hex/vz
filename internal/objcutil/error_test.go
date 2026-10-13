package objcutil_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Code-Hex/vz/v4/internal/objcutil"
)

func TestNSError(t *testing.T) {
	native := &objcutil.NSError{
		Domain: "com.apple.vmnet", Code: 1001,
		LocalizedDescription: "vmnet_network_create failed", UserInfo: "{key = value;}",
	}
	want := `Error Domain=com.apple.vmnet Code=1001 Description="vmnet_network_create failed" UserInfo={key = value;}`
	if got := native.Error(); got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
	var recovered *objcutil.NSError
	if !errors.As(fmt.Errorf("create network: %w", native), &recovered) || recovered != native {
		t.Fatal("wrapped NSError cannot be recovered")
	}
	var empty *objcutil.NSError
	if got := empty.Error(); got != "<nil>" {
		t.Fatalf("nil Error() = %q, want <nil>", got)
	}
}

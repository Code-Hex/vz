//go:build darwin

package vzbridge

import (
	"github.com/Code-Hex/vz/v4/internal/objc"
	runtimeobjc "github.com/ebitengine/purego/objc"
	"strings"
	"testing"
	"unsafe"
)

func sdkABITypeMatches(actual, want string) bool {
	actual = strings.TrimLeft(actual, "rnNoORV")
	if want == "*int8" && actual == "*" {
		return true
	}
	if want == "*uint8" && actual == "[16C]" {
		return true
	}
	if strings.HasPrefix(want, "*") {
		if want == "*unsafe.Pointer" {
			return actual == "^@"
		}
		return strings.HasPrefix(actual, "^") && sdkABITypeMatches(actual[1:], want[1:])
	}
	switch want {
	case "":
		return actual == "v"
	case "bool":
		return actual == "B" || actual == "c"
	case "int8":
		return actual == "c"
	case "uint8":
		return actual == "C"
	case "int16":
		return actual == "s"
	case "uint16":
		return actual == "S"
	case "int32":
		return actual == "i" || actual == "l"
	case "uint32":
		return actual == "I" || actual == "L"
	case "int64":
		return actual == "q" || actual == "l"
	case "uint64":
		return actual == "Q" || actual == "L"
	case "float32":
		return actual == "f"
	case "float64":
		return actual == "d"
	case "string":
		return actual == "*" || actual == "^c"
	case "unsafe.Pointer":
		return actual == "@" || actual == "#" || actual == "^v" || strings.HasPrefix(actual, "@\"")
	}
	return false
}

type sdkMethodContract struct {
	Owner, Selector, GoName, Introduced, Obsoleted, Category string
	Instance, Factory                                        bool
	ResultABI                                                string
	Parameters                                               []string
}

func TestSDKCatalogMatchesRuntimeABI(t *testing.T) {
	categoryCount := 0
	defer func() {
		t.Logf("%d category methods need a concrete implementation for runtime ABI verification", categoryCount)
	}()
	for _, m := range sdkContracts {
		t.Run(m.GoName, func(t *testing.T) {
			if !sdkVersionAvailable(sdkOSVersion, m.Introduced, m.Obsoleted) {
				t.Skipf("SDK availability %s through %s", m.Introduced, m.Obsoleted)
			}
			onQueue(func() {
				class := directGetClass(m.Owner)
				if class == nil {
					t.Errorf("class %s missing", m.Owner)
					return
				}
				if !m.Instance {
					class = directObjectClass(class)
				}
				method := directInstanceMethod(class, runtimeobjc.RegisterName(m.Selector))
				if method == nil && m.Factory {
					allocated := directAllocate(directClass(m.Owner))
					defer directRelease(allocated)
					method = directInstanceMethod(directObjectClass(allocated), runtimeobjc.RegisterName(m.Selector))
				}
				if method == nil && m.Category != "" {
					categoryCount++
					t.Logf("unverified category %s: requires concrete receiver for %s", m.Category, m.Selector)
					return
				}
				if method == nil {
					t.Errorf("%s missing selector %s", m.Owner, m.Selector)
					return
				}
				api := loadPrivateRuntime()
				if count := api.methodArgumentCount(method); count != uint32(len(m.Parameters)+2) {
					t.Errorf("argument count = %d, want %d", count, len(m.Parameters)+2)
					return
				}
				if actual := sdkRuntimeEncoding(api.methodReturn(method)); !sdkABITypeMatches(actual, m.ResultABI) {
					t.Errorf("return ABI %s != %s", actual, m.ResultABI)
				}
				for i, p := range m.Parameters {
					if actual := sdkRuntimeEncoding(api.methodArgument(method, uint32(i+2))); !sdkABITypeMatches(actual, p) {
						t.Errorf("argument %d ABI %s != %s", i, actual, p)
					}
				}
			})
		})
	}
}
func TestSDKAvailability(t *testing.T) {
	for _, test := range []struct {
		version, min, max string
		want              bool
	}{{"12.2", "12.3", "", false}, {"12.3", "12.3", "", true}, {"14.0", "12.3", "14.0", false}, {"13.0", "12.3", "14.0", true}, {"11.0", "", "", true}} {
		if got := sdkVersionAvailable(test.version, test.min, test.max); got != test.want {
			t.Errorf("%+v: got %v", test, got)
		}
	}
}

func TestSDKRequireRejectsUnavailableBeforeDispatch(t *testing.T) {
	for _, test := range []struct {
		name, class, selector, minimum string
		instance                       bool
		panicWant                      bool
	}{
		{"available", "NSObject", "description", "", true, false},
		{"missing selector", "NSObject", "vz_missingSDKSelector", "", true, true},
		{"future version", "NSObject", "hash", "999.0", true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			panicked := false
			func() {
				defer func() { panicked = recover() != nil }()
				sdkRequire(test.class, test.selector, test.minimum, "", test.instance)
			}()
			if panicked != test.panicWant {
				t.Errorf("panic = %v, want %v", panicked, test.panicWant)
			}
		})
	}
}

func TestSDKReceiverClassAndNilMessaging(t *testing.T) {
	if got := NSString_Length(nil); got != 0 {
		t.Fatalf("nil receiver length = %d", got)
	}
	data := NSData_New()
	defer objc.Release(data)
	panicked := false
	func() { defer func() { panicked = recover() != nil }(); NSString_Length(data) }()
	if !panicked {
		t.Fatal("NSString method accepted a NSData receiver with the same selector")
	}
}

func sdkRuntimeEncoding(pointer unsafe.Pointer) string {
	if pointer == nil {
		return ""
	}
	defer loadPrivateRuntime().free(pointer)
	var value []byte
	for i := 0; ; i++ {
		b := *(*byte)(unsafe.Add(pointer, i))
		if b == 0 {
			return string(value)
		}
		value = append(value, b)
	}
}

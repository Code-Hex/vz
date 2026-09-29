package main

import (
	"debug/macho"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestNativeCompilerABI(t *testing.T) {
	if runtime.GOOS != "darwin" || os.Getenv("VZ_BRIDGEGEN_INTEGRATION") != "1" {
		t.Skip("requires macOS and VZ_BRIDGEGEN_INTEGRATION=1")
	}
	directory := t.TempDir()
	source := filepath.Join(directory, "source")
	output := filepath.Join(directory, "output")
	if err := os.Mkdir(source, 0755); err != nil {
		t.Fatal(err)
	}
	swift := `import Foundation
import Dispatch
import Virtualization
public typealias CString = UnsafePointer<CChar>?
public typealias RawPointer = UnsafeMutableRawPointer?
public typealias BorrowedObject = UnsafeMutableRawPointer
public typealias OwnedObject = UnsafeMutableRawPointer
final class ValueBox: NSObject {
    let value: Int64 = 42
    let onRelease: (@convention(c) () -> Void)?
    init(_ onRelease: (@convention(c) () -> Void)? = nil) { self.onRelease = onRelease }
    deinit { onRelease?() }
}
@c(vz_unusedProbe)
public func unusedProbe() -> Int64 { 99 }
@c(vz_privateProbe)
public func privateProbe(_ value: Int64) -> Int64 { value + 1 }
@c(vz_sum)
public func sum(_ left: Int32, _ right: Int32) -> Int32 { left + right }
@c(vz_addFloat)
public func addFloat(_ value: Float) -> Double { Double(value) + 0.5 }
@c(vz_length)
public func length(_ string: CString) -> UInt64 { UInt64(String(cString: string!).utf8.count) }
@c(vz_releaseObject)
public func releaseObject(_ pointer: RawPointer) { if let pointer { Unmanaged<NSObject>.fromOpaque(pointer).release() } }
@c(vz_newObject)
public func newObject() -> OwnedObject { Unmanaged.passRetained(ValueBox()).toOpaque() }
@c(vz_newTrackedObject)
public func newTrackedObject(_ address: UInt64) -> OwnedObject {
    let callback = unsafeBitCast(UInt(address), to: (@convention(c) () -> Void).self)
    return Unmanaged.passRetained(ValueBox(callback)).toOpaque()
}
@c(vz_value)
public func value(_ object: BorrowedObject) -> Int64 { (Unmanaged<NSObject>.fromOpaque(object).takeUnretainedValue() as! ValueBox).value }

@c(vz_newBootLoader)
public func newBootLoader() -> OwnedObject {
    if #available(macOS 13, *) { return Unmanaged.passRetained(VZEFIBootLoader()).toOpaque() }
    fatalError("requires macOS 13")
}
@c(vz_newMACAddress)
public func newMACAddress() -> OwnedObject { Unmanaged.passRetained(VZMACAddress.randomLocallyAdministered()).toOpaque() }

@c(vz_callback)
public func callback(_ address: UInt64, _ value: UInt64) -> UInt64 {
    let function = unsafeBitCast(UInt(address), to: (@convention(c) (UInt64) -> UInt64).self)
    return function(value)
}
@c(vz_callbackAsync)
public func callbackAsync(_ address: UInt64, _ value: UInt64) {
    let function = unsafeBitCast(UInt(address), to: (@convention(c) (UInt64) -> UInt64).self)
    DispatchQueue.global().async { _ = function(value) }
}
`
	if err := os.WriteFile(filepath.Join(source, "Fixture.swift"), []byte(swift), 0644); err != nil {
		t.Fatal(err)
	}
	if err := build(source, output, false); err != nil {
		t.Fatal(err)
	}
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	client := filepath.Join(directory, "client")
	if err := os.MkdirAll(filepath.Join(client, "api"), 0755); err != nil {
		t.Fatal(err)
	}
	binding, err := os.ReadFile(filepath.Join(output, "binding_"+runtime.GOARCH+".go"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(client, "api", "bindings.go"), binding, 0644); err != nil {
		t.Fatal(err)
	}
	bundleDir := filepath.Join(client, "api", "abi_"+runtime.GOARCH)
	if err := os.MkdirAll(bundleDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"libBridge.a", "Virtualization.tbd"} {
		data, err := os.ReadFile(filepath.Join(output, "abi_"+runtime.GOARCH, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(bundleDir, name), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	module := "module github.com/Code-Hex/vz/v3/internal/vzbridgegentest\n\ngo 1.25.0\nrequire github.com/Code-Hex/vz/v3 v3.0.0\nreplace github.com/Code-Hex/vz/v3 => " + repo + "\n"
	if err := os.WriteFile(filepath.Join(client, "go.mod"), []byte(module), 0644); err != nil {
		t.Fatal(err)
	}
	program := `package main
import (
    "runtime"
    "time"
    "github.com/Code-Hex/vz/v3/internal/objc"
    "github.com/Code-Hex/vz/v3/internal/vzbridgegentest/api"
    "github.com/ebitengine/purego"
)
//go:noinline
func abandonObject(callback uint64) {
    object := vzbridge.NewTrackedObject(callback)
    if vzbridge.Value(object) != 42 { panic("cleanup object") }
    runtime.KeepAlive(object)
}
func main() {
    released := make(chan struct{}, 2)
    releaseCallback := purego.NewCallback(func() { released <- struct{}{} })
    abandonObject(uint64(releaseCallback))
    deadline := time.After(5 * time.Second)
cleanup:
    for {
        runtime.GC()
        select {
        case <-released:
            break cleanup
        case <-deadline:
            panic("owned object cleanup timed out")
        case <-time.After(10 * time.Millisecond):
        }
    }
    tracked := vzbridge.NewTrackedObject(uint64(releaseCallback))
    objc.Release(tracked)
    objc.Release(tracked)
    select {
    case <-released:
    default:
        panic("owned object was not released")
    }
    select {
    case <-released:
        panic("owned object released twice")
    default:
    }
    if vzbridge.Sum(20,22) != 42 { panic("sum") }
    if vzbridge.AddFloat(1.25) != 1.75 { panic("float conversion") }
    if vzbridge.Length("\u65e5\u672c\u8a9e") != 9 { panic("UTF-8 length") }
    vzbridge.ReleaseObject(nil)
    object := vzbridge.NewObject()
    runtime.GC()
    if vzbridge.Value(object) != 42 { panic("owned object") }
    objc.Release(object)
    bootLoader := vzbridge.NewBootLoader()
    if objc.Ptr(bootLoader) == nil { panic("weak framework import") }
    objc.Release(bootLoader)
    macAddress := vzbridge.NewMACAddress()
    if objc.Ptr(macAddress) == nil { panic("SDK framework import") }
    objc.Release(macAddress)
    callback := purego.NewCallback(func(value uint64) uint64 { return value + 1 })
    if vzbridge.Callback(uint64(callback), 41) != 42 { panic("callback") }
    result := make(chan uint64, 1)
    asyncCallback := purego.NewCallback(func(value uint64) uint64 { result <- value; return 0 })
    vzbridge.CallbackAsync(uint64(asyncCallback), 42)
    select {
    case value := <-result:
        if value != 42 { panic("async callback") }
    case <-time.After(5 * time.Second):
        panic("async callback timed out")
    }
}
`

	frameworks := filepath.Join(directory, "frameworks")
	framework := filepath.Join(frameworks, "Virtualization.framework")
	if err := os.MkdirAll(framework, 0755); err != nil {
		t.Fatal(err)
	}
	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "x86_64"
	}
	// Omit the newer EFI class from the SDK stub while using the real framework at runtime.
	stub := fmt.Sprintf(`--- !tapi-tbd
tbd-version: 4
targets: [ %s-macos ]
install-name: '/System/Library/Frameworks/Virtualization.framework/Versions/A/Virtualization'
exports:
  - targets: [ %s-macos ]
    symbols: [ '_OBJC_CLASS_$_VZMACAddress' ]
...
`, arch, arch)
	if err := os.WriteFile(filepath.Join(framework, "Virtualization.tbd"), []byte(stub), 0644); err != nil {
		t.Fatal(err)
	}
	libraries := filepath.Join(directory, "libraries")
	if err := os.Mkdir(libraries, 0755); err != nil {
		t.Fatal(err)
	}
	// Older SDKs lack this overlay and its force-load symbol.
	floatStub := fmt.Sprintf(`--- !tapi-tbd
tbd-version: 4
targets: [ %s-macos ]
install-name: '/usr/lib/swift/libswift_Builtin_float.dylib'
exports: []
...
`, arch)
	if err := os.WriteFile(filepath.Join(libraries, "libswift_Builtin_float.tbd"), []byte(floatStub), 0644); err != nil {
		t.Fatal(err)
	}
	concurrentProgram := func(extra string) string {
		return `package main
import (
    "sync"
    "github.com/Code-Hex/vz/v3/internal/vzbridgegentest/api"
)
func main() {
    start := make(chan struct{})
    var group sync.WaitGroup
    for range 64 {
        group.Add(1)
        go func() {
            defer group.Done()
            <-start
            if vzbridge.Sum(20, 22) != 42 { panic("concurrent sum") }
            ` + extra + `
        }()
    }
    close(start)
    group.Wait()
}
`
	}
	tests := []struct {
		name    string
		program string
		linked  []string
	}{
		{
			name:    "one",
			program: concurrentProgram(""),
			linked:  []string{"sum"},
		},
		{
			name:    "two",
			program: concurrentProgram(`if vzbridge.PrivateProbe(41) != 42 { panic("private probe") }`),
			linked:  []string{"sum", "privateProbe"},
		},
		{
			name:    "lifecycle_and_callbacks",
			program: program,
			linked: []string{
				"sum", "addFloat", "length", "releaseObject", "newObject", "newTrackedObject",
				"value", "newBootLoader", "newMACAddress", "callback", "callbackAsync",
			},
		},
	}
	nativeSymbols := []string{
		"bridge_abi", "sum", "addFloat", "privateProbe", "unusedProbe", "length", "releaseObject",
		"newObject", "newTrackedObject", "value", "newBootLoader", "newMACAddress", "callback", "callbackAsync",
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(client, "main.go"), []byte(test.program), 0644); err != nil {
				t.Fatal(err)
			}
			binary := filepath.Join(directory, test.name)
			cmd := exec.Command(
				filepath.Join(runtime.GOROOT(), "bin", "go"),
				"build", "-race", "-modcacherw", "-mod=mod", "-o", binary, ".",
			)
			cmd.Dir = client
			cmd.Env = append(os.Environ(),
				"CGO_ENABLED=1", "CGO_LDFLAGS=-L"+libraries+" -F"+frameworks,
				"GOCACHE="+filepath.Join(directory, "gocache"),
				"GOMODCACHE="+filepath.Join(directory, "modcache"),
			)
			if data, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("build native client: %v\n%s", err, data)
			}
			image, err := macho.Open(binary)
			if err != nil {
				t.Fatal(err)
			}
			defer image.Close()
			if image.Symtab == nil {
				t.Fatal("native client has no symbol table")
			}
			linked := make(map[string]bool)
			for _, symbol := range image.Symtab.Syms {
				if symbol.Sect != 0 {
					linked[symbol.Name] = true
				}
			}
			want := map[string]bool{"bridge_abi": true}
			for _, name := range test.linked {
				want[name] = true
			}
			for _, name := range nativeSymbols {
				if got := linked["_vz_"+name]; got != want[name] {
					t.Errorf("native function vz_%s linked = %t, want %t", name, got, want[name])
				}
			}
			if data, err := exec.Command(binary).CombinedOutput(); err != nil {
				t.Fatalf("run native client: %v\n%s", err, data)
			}
		})
	}
}

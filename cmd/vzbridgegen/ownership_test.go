package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestInferOperationsFromSwiftCompiler(t *testing.T) {
	if runtime.GOOS != "darwin" || os.Getenv("VZ_BRIDGEGEN_INTEGRATION") != "1" {
		t.Skip("requires macOS and VZ_BRIDGEGEN_INTEGRATION=1")
	}
	source := filepath.Join(t.TempDir(), "Ownership.swift")
	code := `public typealias BorrowedObject = UnsafeMutableRawPointer?
public typealias OwnedObject = UnsafeMutableRawPointer?
public typealias RawPointer = UnsafeMutableRawPointer?
public typealias CString = UnsafePointer<CChar>?
public typealias ErrorOut = UnsafeMutablePointer<UnsafeMutableRawPointer?>?
@c(vz_copy)
public func copy(_ value: BorrowedObject, _ name: CString, _ error: ErrorOut) -> OwnedObject { value }
@c(vz_releaseObject)
public func releaseObject(_ pointer: RawPointer) {}
@c(vz_count)
public func count(_ value: UInt64) -> UInt64 {
    func nested(_ pointer: UnsafeMutableRawPointer?) {}
    return value
}
#if arch(arm64)
@c(vz_armOnly)
public func armOnly() -> UInt64 { 64 }
#endif
`
	if err := os.WriteFile(source, []byte(code), 0600); err != nil {
		t.Fatal(err)
	}
	sdk, err := exec.Command("xcrun", "--sdk", "macosx", "--show-sdk-path").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, arch := range []string{"arm64", "x86_64"} {
		t.Run(arch, func(t *testing.T) {
			ast, err := exec.Command("xcrun", "swiftc", "-swift-version", "6", "-sdk", strings.TrimSpace(string(sdk)), "-target", arch+"-apple-macosx11.0", "-module-cache-path", t.TempDir(), "-dump-ast", source).CombinedOutput()
			if err != nil {
				t.Fatalf("Swift compiler: %v\n%s", err, ast)
			}
			got, err := inferOperations(strings.NewReader(string(ast)))
			if err != nil {
				t.Fatal(err)
			}
			want := []operation{
				{Symbol: "vz_copy", Name: "Copy", Parameters: map[string]string{"value": "object", "name": "cstring", "error": "error-out"}, Result: "owned"},
				{Symbol: "vz_releaseObject", Name: "ReleaseObject", Parameters: map[string]string{"pointer": "raw"}},
				{Symbol: "vz_count", Name: "Count"},
			}
			if arch == "arm64" {
				want = append(want, operation{Symbol: "vz_armOnly", Name: "ArmOnly"})
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("operations = %#v, want %#v", got, want)
			}
		})
	}
}

func TestInferOperationsRejectsUnclassifiedPointer(t *testing.T) {
	ast := `(source_file "Fixture.swift"
  (func_decl "copy(_:)" interface_type="(UnsafeMutableRawPointer?) -> ()"
    (cdecl_attr name="vz_copy") result="()"
    (parameter_list
      (parameter "value" interface_type="UnsafeMutableRawPointer?"))
    (brace_stmt)))`
	_, err := inferOperations(strings.NewReader(ast))
	if err == nil || !strings.Contains(err.Error(), "vz_copy parameter value") {
		t.Fatalf("error = %v, want an unclassified pointer error", err)
	}
}

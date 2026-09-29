package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestGenerateAllPrivateMethods(t *testing.T) {
	metadata := privateMetadata{Methods: []privateMethod{
		{Class: "Thing", Selector: "value", Result: "@", Arguments: []string{"@", ":"}},
		{Class: "Thing", Selector: "setValue:", Result: "v", Arguments: []string{"@", ":", "@"}},
		{Class: "Thing", Selector: "count", ClassMethod: true, Result: "q", Arguments: []string{"@", ":"}},
		{Class: "Thing", Selector: "completion", Result: "@?", Arguments: []string{"@", ":"}},
		{Class: "Thing", Selector: "point", Result: "{Point=dd}", Arguments: []string{"@", ":"}},
	}}
	_, contracts, unsupported, err := generatePrivate(metadata)
	if err != nil {
		t.Fatal(err)
	}
	if len(contracts) != 4 || len(unsupported) != 2 {
		t.Fatalf("got %d operations and %v", len(contracts), unsupported)
	}
	for _, contract := range contracts {
		if contract.Name == "UnsafePrivateAllocate" {
			continue
		}
		for name, role := range contract.Parameters {
			if role != "raw" {
				t.Fatalf("%s %s inferred ownership %s", contract.Name, name, role)
			}
		}
		if contract.Name == privateBindingName(metadata.Methods[0]) && contract.Result != "raw" {
			t.Fatalf("object result inferred ownership %s", contract.Result)
		}
	}
}

func TestPrivateBindingNamesPreserveSelectorIdentity(t *testing.T) {
	colon := privateMethod{Class: "Thing", Selector: "setValue:"}
	underscore := privateMethod{Class: "Thing", Selector: "setValue_"}
	classMethod := colon
	classMethod.ClassMethod = true
	if privateBindingName(colon) == privateBindingName(underscore) || privateBindingName(colon) == privateBindingName(classMethod) {
		t.Fatal("distinct runtime methods have the same binding name")
	}
}

func TestPrivateFrameworkBridge(t *testing.T) {
	if runtime.GOOS != "darwin" || os.Getenv("VZ_BRIDGEGEN_INTEGRATION") != "1" {
		t.Skip("requires macOS and VZ_BRIDGEGEN_INTEGRATION=1")
	}
	directory := t.TempDir()
	metadata, err := discoverPrivate(directory, nil)
	if err != nil {
		t.Fatal(err)
	}
	metadata.Methods = append(metadata.Methods,
		privateMethod{Class: "PrivateBridgeFixture", Selector: "freshObject", ClassMethod: true, Result: "@", Arguments: []string{"@", ":"}},
		privateMethod{Class: "PrivateBridgeFixture", Selector: "missingObject", ClassMethod: true, Result: "@", Arguments: []string{"@", ":"}},
	)
	generated, _, unsupported, err := generatePrivate(metadata)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("discovered %d methods, unsupported %d", len(metadata.Methods), len(unsupported))
	name := func(class, selector string, classMethod bool) string {
		for _, method := range metadata.Methods {
			if method.Class == class && method.Selector == selector && method.ClassMethod == classMethod {
				return privateBindingName(method)
			}
		}
		t.Fatalf("missing %s %s", class, selector)
		return ""
	}
	var widths strings.Builder
	for _, encoding := range []string{"B", "c", "C", "s", "S", "i", "I", "l", "L", "q", "Q", "f", "d"} {
		native, err := privateSwiftType(encoding)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&widths, "do { var size = 0; NSGetSizeAndAlignment(%q, &size, nil); precondition(MemoryLayout<%s>.size == size, %q) }\n", encoding, native, "ABI width for "+encoding)
	}
	program := string(generated) + widths.String() + fmt.Sprintf(`
func bridgeSync<T>(_ body: () -> T) -> T { autoreleasepool(invoking: body) }
@objc(PrivateBridgeLifetime)
final class PrivateBridgeLifetime: NSObject {
    nonisolated(unsafe) static var destroyed = 0
    deinit { Self.destroyed += 1 }
}
@objc(PrivateBridgeFixture)
final class PrivateBridgeFixture: NSObject {
    @objc class func freshObject() -> NSObject { PrivateBridgeLifetime() }
    @objc class func missingObject() -> NSObject? { nil }
}
var invoked = false
let owned = %s(true, &invoked)!
precondition(invoked)
precondition(PrivateBridgeLifetime.destroyed == 0)
Unmanaged<AnyObject>.fromOpaque(owned).release()
precondition(PrivateBridgeLifetime.destroyed == 1)
invoked = false
precondition(%s(true, &invoked) == nil)
precondition(invoked)
let wrongAllocation = "NSObject".withCString { UnsafePrivateAllocate($0) }!
invoked = true
precondition(%s(wrongAllocation, 12345, false, &invoked) == nil)
precondition(!invoked)
Unmanaged<AnyObject>.fromOpaque(wrongAllocation).release()
precondition(%s() == VZVirtualMachineConfiguration.minimumAllowedCPUCount)
let allocation = "_VZGDBDebugStubConfiguration".withCString { UnsafePrivateAllocate($0) }!
let stub = %s(allocation, 12345, false, nil)!
precondition(%s(stub) == 12345)
let config = VZVirtualMachineConfiguration()
%s(Unmanaged.passUnretained(config).toOpaque(), stub)
let result = %s(Unmanaged.passUnretained(config).toOpaque(), true, nil)!
precondition(result == stub)
Unmanaged<AnyObject>.fromOpaque(result).release()
Unmanaged<AnyObject>.fromOpaque(stub).release()
let cls: AnyClass = NSClassFromString("_VZGDBDebugStubConfiguration")!
precondition(privateBridgeMethod(cls, NSSelectorFromString("initWithPort:"), false, "@", ["@", ":", "q"]) == nil)
precondition(privateBridgeMethod(cls, NSSelectorFromString("absentSelector"), false, "v", ["@", ":"]) == nil)
print("private bridge passed")
`, name("PrivateBridgeFixture", "freshObject", true),
		name("PrivateBridgeFixture", "missingObject", true),
		name("_VZGDBDebugStubConfiguration", "initWithPort:", false),
		name("VZVirtualMachineConfiguration", "minimumAllowedCPUCount", true),
		name("_VZGDBDebugStubConfiguration", "initWithPort:", false),
		name("_VZGDBDebugStubConfiguration", "port", false),
		name("VZVirtualMachineConfiguration", "_setDebugStub:", false),
		name("VZVirtualMachineConfiguration", "_debugStub", false))
	source := filepath.Join(directory, "main.swift")
	if err := os.WriteFile(source, []byte(program), 0600); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(directory, "private-bridge")
	output, err := exec.Command("xcrun", "swiftc", "-swift-version", "6", "-strict-concurrency=complete", "-warnings-as-errors", "-module-cache-path", filepath.Join(directory, "cache"), source, "-o", binary).CombinedOutput()
	if err != nil {
		t.Fatalf("compile: %v\n%s", err, output)
	}
	output, err = exec.Command(binary).CombinedOutput()
	if err != nil || strings.TrimSpace(string(output)) != "private bridge passed" {
		t.Fatalf("run: %v\n%s", err, output)
	}
}

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestFrameworkGeneratedBridge(t *testing.T) {
	if runtime.GOOS != "darwin" || os.Getenv("VZ_BRIDGEGEN_INTEGRATION") != "1" {
		t.Skip("requires macOS and VZ_BRIDGEGEN_INTEGRATION=1")
	}
	for _, arch := range []string{"arm64", "amd64"} {
		t.Run(arch, func(t *testing.T) { testFrameworkGeneratedBridge(t, arch) })
	}
}

func testFrameworkGeneratedBridge(t *testing.T, goarch string) {
	dir := t.TempDir()
	sdkData, err := command("xcrun", "--show-sdk-path")
	if err != nil {
		t.Fatal(err)
	}
	arch := goarch
	if arch == "amd64" {
		arch = "x86_64"
	}
	target := arch + "-apple-macos11.0"
	sdk := strings.TrimSpace(string(sdkData))
	symbols, err := discoverFramework(sdk, target, dir)
	if err != nil {
		t.Fatal(err)
	}
	checks := []struct {
		Path, Name string
		Setter     bool
	}{
		{Path: "VZVirtioGraphicsScanoutConfiguration.init(widthInPixels:heightInPixels:)", Name: "NewScanout"},
		{Path: "VZVirtioGraphicsScanoutConfiguration.widthInPixels", Name: "Width"},
		{Path: "VZVirtioGraphicsScanoutConfiguration.widthInPixels", Name: "SetWidth", Setter: true},
		{Path: "VZMACAddress.init(string:)", Name: "NewMAC"},
		{Path: "VZMACAddress.string", Name: "MACString"},
		{Path: "VZMACAddress.randomLocallyAdministered()", Name: "RandomMAC"},
		{Path: "VZMACAddress.isLocallyAdministeredAddress", Name: "IsLocalMAC"},
		{Path: "VZVirtualMachineConfiguration.minimumAllowedMemorySize", Name: "MinimumMemory"},
		{Path: "VZVirtualMachineConfiguration.maximumAllowedCPUCount", Name: "MaximumCPU"},
		{Path: "VZVirtualMachineConfiguration.validate()", Name: "Validate"},
		{Path: "VZVirtualMachineConfiguration.bootLoader", Name: "BootLoader"},
		{Path: "VZVirtualMachineConfiguration.bootLoader", Name: "SetBootLoader", Setter: true},
		{Path: "VZEFIBootLoader.init()", Name: "NewEFI"},
	}

	source, ops, skipped, err := generateAllFramework(symbols)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("generated %d bindings; skipped %d declarations: %s", len(ops), len(skipped), strings.Join(skipped, "\n"))
	for _, path := range []string{
		"VZMACAddress.init(ethernetAddress:)",
		"VZVirtualMachine.start()",
		"VZVirtioConsolePortConfiguration.name setter",
	} {
		found := false
		for _, diagnostic := range skipped {
			found = found || strings.HasPrefix(diagnostic, path+":")
		}
		if !found {
			t.Fatalf("missing unsupported declaration diagnostic for %s", path)
		}
	}

	if err := os.WriteFile(filepath.Join(dir, "Generated.swift"), []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	support, err := os.ReadFile("../../internal/vzbridge/source/Runtime.swift")
	if err != nil {
		t.Fatal(err)
	}
	support = support[:strings.Index(string(support), "private typealias BridgeCallback")]
	if err := os.WriteFile(filepath.Join(dir, "Support.swift"), support, 0644); err != nil {
		t.Fatal(err)
	}
	main := `import Foundation
import Virtualization
func release(_ pointer: UnsafeMutableRawPointer?) { if let pointer { Unmanaged<NSObject>.fromOpaque(pointer).release() } }
let scanout = vz_NewScanout(1920, 1080)
precondition(vz_Width(scanout) == 1920)
vz_SetWidth(scanout, 1280)
precondition(vz_Width(scanout) == 1280)
release(scanout)
let mac = "02:00:00:00:00:42".withCString { vz_NewMAC($0) }
precondition(mac != nil)
let macString = vz_MACString(mac)
precondition(borrow(macString, as: NSString.self) == "02:00:00:00:00:42")
release(macString)
release(mac)
precondition("invalid".withCString { vz_NewMAC($0) } == nil)
let randomMAC = vz_RandomMAC()
precondition(vz_IsLocalMAC(randomMAC))
release(randomMAC)
precondition(vz_MinimumMemory() == VZVirtualMachineConfiguration.minimumAllowedMemorySize)
precondition(vz_MaximumCPU() == Int64(VZVirtualMachineConfiguration.maximumAllowedCPUCount))
let configuration = own(VZVirtualMachineConfiguration())
var error: UnsafeMutableRawPointer?
vz_Validate(configuration, &error)
precondition(error != nil)
release(error)
precondition(vz_BootLoader(configuration) == nil)
let loader = vz_NewEFI()
vz_SetBootLoader(configuration, loader)
release(loader)
let gotLoader = vz_BootLoader(configuration)
precondition(gotLoader != nil)
release(gotLoader)
vz_SetBootLoader(configuration, nil)
precondition(vz_BootLoader(configuration) == nil)
release(configuration)
print("framework bridge passed")
`
	for _, check := range checks {
		for _, symbol := range symbols {
			if strings.Join(symbol.Path, ".") == check.Path {
				main = strings.ReplaceAll(main, "vz_"+check.Name+"(", "vz_"+frameworkBindingName(symbol, check.Setter)+"(")
			}
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "main.swift"), []byte(main), 0644); err != nil {
		t.Fatal(err)
	}
	header := filepath.Join(dir, "Generated.h")
	args := []string{"swiftc", "-swift-version", "6", "-strict-concurrency=complete", "-warnings-as-errors", "-module-cache-path", filepath.Join(dir, "cache"), "-sdk", sdk, "-target", target, "-emit-objc-header", "-emit-objc-header-path", header, filepath.Join(dir, "Generated.swift"), filepath.Join(dir, "Support.swift"), filepath.Join(dir, "main.swift"), "-o", filepath.Join(dir, "check")}
	if _, err := command("xcrun", args...); err != nil {
		t.Fatal(err)
	}
	ast, err := command("xcrun", "clang", "-target", target, "-isysroot", sdk, "-fsyntax-only", "-x", "objective-c", "-Xclang", "-ast-dump=json", "-Xclang", "-ast-dump-filter=vz_", header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := checkHeader(strings.NewReader(string(ast)), ops, goarch); err != nil {
		t.Fatal(err)
	}
	if goarch != runtime.GOARCH {
		return
	}
	output, err := command(filepath.Join(dir, "check"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(output)) != "framework bridge passed" {
		t.Fatalf("unexpected output: %s", output)
	}
}

func TestFrameworkReportsObsoleteDeclarations(t *testing.T) {
	owner := frameworkSymbol{Path: []string{"Widget"}}
	owner.Kind.Identifier = "swift.class"
	owner.Identifier.Precise = "widget"
	property := frameworkSymbol{Path: []string{"Widget", "count"}, Fragments: []frameworkFragment{
		{Kind: "keyword", Spelling: "class var"},
		{Kind: "text", Spelling: " "},
		{Kind: "identifier", Spelling: "count"},
		{Kind: "text", Spelling: ": "},
		{Kind: "typeIdentifier", Spelling: "Int64"},
		{Kind: "text", Spelling: " { get }"},
	}}
	property.Kind.Identifier = "swift.type.property"
	property.Identifier.Precise = "widget-count"
	source, ops, skipped, err := generateAllFramework([]frameworkSymbol{owner, property})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(source, "return Widget.count") || len(ops) != 1 || len(skipped) != 0 {
		t.Fatalf("getter was not generated: %s, %#v, %v", source, ops, skipped)
	}

	for _, domain := range []string{"macOS", "Swift"} {
		t.Run(domain, func(t *testing.T) {
			obsolete := property
			obsolete.Availability = []frameworkAvailability{{Domain: domain, Obsoleted: &frameworkVersion{Major: 15}}}
			if _, ops, skipped, err := generateAllFramework([]frameworkSymbol{owner, obsolete}); err != nil || len(ops) != 0 || len(skipped) != 1 || !strings.Contains(skipped[0], "unavailable") {
				t.Fatalf("obsolete declaration was not reported: %#v, %v, %v", ops, skipped, err)
			}
		})
	}
}

func TestFrameworkDiscoversNewMembersAndOverloads(t *testing.T) {
	owner := frameworkSymbol{Path: []string{"Widget"}}
	owner.Kind.Identifier = "swift.class"
	owner.Identifier.Precise = "widget"
	property := frameworkSymbol{Path: []string{"Widget", "count"}, Fragments: []frameworkFragment{
		{Kind: "keyword", Spelling: "var"}, {Kind: "text", Spelling: " "},
		{Kind: "identifier", Spelling: "count"}, {Kind: "text", Spelling: ": "},
		{Kind: "typeIdentifier", Spelling: "Int64"}, {Kind: "text", Spelling: " { get set }"},
	}}
	property.Kind.Identifier = "swift.property"
	property.Identifier.Precise = "widget-count"
	source, ops, skipped, err := generateAllFramework([]frameworkSymbol{owner, property})
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 2 || len(skipped) != 0 || !strings.Contains(source, ".count = value") || !strings.Contains(source, "return borrow(receiver, as: Widget.self).count") {
		t.Fatalf("unexpected generated property: %s, %#v, %v", source, ops, skipped)
	}
	firstName := ops[0].Name
	added := property
	added.Path = []string{"Widget", "enabled"}
	added.Identifier.Precise = "widget-enabled"
	added.Fragments = append([]frameworkFragment(nil), property.Fragments...)
	added.Fragments[2].Spelling = "enabled"
	source, ops, skipped, err = generateAllFramework([]frameworkSymbol{added, owner, property})
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 4 || len(skipped) != 0 || ops[0].Name != firstName || !strings.Contains(source, ".enabled = value") {
		t.Fatalf("new SDK property was not discovered: %s, %#v, %v", source, ops, skipped)
	}
	method := frameworkSymbol{Path: []string{"Widget", "run()"}}
	method.Kind.Identifier = "swift.method"
	method.Identifier.Precise = "widget-run-one"
	overload := method
	overload.Identifier.Precise = "widget-run-two"
	_, ops, skipped, err = generateAllFramework([]frameworkSymbol{owner, method, overload})
	if err != nil || len(ops) != 2 || len(skipped) != 0 || ops[0].Name == ops[1].Name {
		t.Fatalf("overloads did not get distinct bindings: %#v, %v, %v", ops, skipped, err)
	}
	unsupported := property
	unsupported.Path = []string{"Widget", "numbers"}
	unsupported.Identifier.Precise = "widget-numbers"
	unsupported.Fragments = []frameworkFragment{{Kind: "text", Spelling: "var numbers: "}, {Kind: "typeIdentifier", Spelling: "[Int]"}, {Kind: "text", Spelling: " { get }"}}
	_, ops, skipped, err = generateAllFramework([]frameworkSymbol{owner, unsupported})
	if err != nil || len(ops) != 0 || len(skipped) != 1 || !strings.Contains(skipped[0], "unsupported framework type") {
		t.Fatalf("unsupported declaration was not reported: %#v, %v, %v", ops, skipped, err)
	}
}

func TestFrameworkReportsCallsBeyondABIArgumentLimit(t *testing.T) {
	owner := frameworkSymbol{Path: []string{"Widget"}}
	owner.Kind.Identifier = "swift.class"
	owner.Identifier.Precise = "widget"
	for _, tc := range []struct {
		name, kind      string
		arguments       int
		throws, skipped bool
	}{
		{name: "instance limit", kind: "swift.method", arguments: 14},
		{name: "instance overflow", kind: "swift.method", arguments: 15, skipped: true},
		{name: "throwing limit", kind: "swift.method", arguments: 13, throws: true},
		{name: "throwing overflow", kind: "swift.method", arguments: 14, throws: true, skipped: true},
		{name: "static limit", kind: "swift.type.method", arguments: 15},
		{name: "static overflow", kind: "swift.type.method", arguments: 16, skipped: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			method := frameworkSymbol{Path: []string{"Widget", "run(" + strings.Repeat("_:", tc.arguments) + ")"}}
			method.Kind.Identifier = tc.kind
			method.Identifier.Precise = "widget-run"
			if tc.throws {
				method.Fragments = []frameworkFragment{{Kind: "keyword", Spelling: "throws"}}
			}
			for range tc.arguments {
				method.Signature.Parameters = append(method.Signature.Parameters, struct {
					Name      string              `json:"name"`
					Fragments []frameworkFragment `json:"declarationFragments"`
				}{Name: "arg", Fragments: []frameworkFragment{{Kind: "text", Spelling: "arg: "}, {Kind: "typeIdentifier", Spelling: "Int64"}}})
			}
			_, ops, skipped, err := generateAllFramework([]frameworkSymbol{owner, method})
			if err != nil {
				t.Fatal(err)
			}
			if tc.skipped {
				if len(ops) != 0 || len(skipped) != 1 || !strings.Contains(skipped[0], "15 arguments") {
					t.Fatalf("ABI overflow was not reported: %#v, %v", ops, skipped)
				}
			} else if len(ops) != 1 || len(skipped) != 0 {
				t.Fatalf("call within ABI limit was not generated: %#v, %v", ops, skipped)
			}
		})
	}
}

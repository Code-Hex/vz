package main

import (
	"go/parser"
	"go/token"
	"runtime"
	"strings"
	"testing"
)

func TestPrivateGenerationPreservesRuntimeContract(t *testing.T) {
	metadata := privateMetadata{Architecture: "arm64", OSVersion: "fixture", Methods: []privateMethod{
		{Class: "Example", Selector: "initWithPort:", Result: "@", Arguments: []string{"@", ":", "S"}},
		{Class: "Example", ClassMethod: true, Selector: "version", Result: "Q", Arguments: []string{"@", ":"}},
		{Class: "Example", Selector: "setPointer:", Result: "v", Arguments: []string{"@", ":", "^{Record=ii}"}},
		{Class: "Example", Selector: "setCompletion:", Result: "v", Arguments: []string{"@", ":", "@?"}},
		{Class: "Example", Selector: "record", Result: "{Record=ii}", Arguments: []string{"@", ":"}},
		{Class: "Example", Selector: "broken:", Result: "v", Arguments: []string{"@", ":"}},
	}}
	for _, arch := range []string{"arm64", "amd64"} {
		code, report, err := generatePrivate(metadata, arch)
		if err != nil {
			t.Fatal(err)
		}
		if report.Generated != 3 || report.Discovered != 6 || len(report.Unsupported) != 3 {
			t.Fatalf("report = %+v", report)
		}
		if _, err := parser.ParseFile(token.NewFileSet(), "private.go", code, parser.AllErrors); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(code), "^{Record=ii}") {
			t.Error("generated contract missing pointer ABI encoding")
		}
		if !strings.Contains(strings.Join(report.Unsupported, "\n"), "setCompletion:") {
			t.Fatalf("unsupported report = %v", report.Unsupported)
		}
	}
}

func TestPrivateRuntimeDiscovery(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Objective-C runtime requires macOS")
	}
	metadata, err := discoverPrivate(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Architecture != runtime.GOARCH {
		t.Fatalf("host architecture = %q", metadata.Architecture)
	}
	for _, method := range metadata.Methods {
		if method.Class == "_VZGDBDebugStubConfiguration" && method.Selector == "initWithPort:" {
			if method.Result != "@" || strings.Join(method.Arguments, ",") != "@,:,S" {
				t.Fatalf("GDB initializer = %+v", method)
			}
			return
		}
	}
	t.Fatal("runtime discovery omitted GDB initializer")
}

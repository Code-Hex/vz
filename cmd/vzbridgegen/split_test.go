package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSplitSwiftSources(t *testing.T) {
	if runtime.GOOS != "darwin" || os.Getenv("VZ_BRIDGEGEN_INTEGRATION") != "1" {
		t.Skip("requires macOS and VZ_BRIDGEGEN_INTEGRATION=1")
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "Fixture.swift")
	if err := os.WriteFile(source, []byte(`
#if os(macOS)
import Foundation
#endif
func helper(_ n: Int32) -> Int32 { n + 1 }
#if arch(arm64)
@c(vz_first)
public func first(_ n: Int32) -> Int32 {
    let text = #"{not a brace}"#
    if text.isEmpty { return 0 }
    return helper(n)
}
#elseif arch(x86_64)
@c(vz_first)
public func first(_ n: Int32) -> Int32 { helper(n) }
#endif
#if os(macOS)
#if arch(arm64) || arch(x86_64)
@c(vz_second)
public func second(_ n: Int32) -> Int32 { first(n) + 1 }
#endif
#endif
#if os(Linux)
@c(vz_inactive)
public func inactive() {}
#endif
`), 0600); err != nil {
		t.Fatal(err)
	}
	paths, err := splitSwiftSources([]string{source}, []string{"vz_first", "vz_second"}, filepath.Join(dir, "split"))
	if err != nil {
		t.Fatal(err)
	}
	main := filepath.Join(dir, "main.swift")
	if err := os.WriteFile(main, []byte("precondition(first(40) == 41)\nprecondition(second(40) == 42)\n"), 0600); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(dir, "fixture")
	args := append([]string{"swiftc", "-swift-version", "6", "-warnings-as-errors"}, paths...)
	args = append(args, main, "-o", binary)
	if _, err := command("xcrun", args...); err != nil {
		t.Fatal(err)
	}
	if _, err := command(binary); err != nil {
		t.Fatal(err)
	}
}

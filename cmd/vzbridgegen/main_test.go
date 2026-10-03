package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const fixture = `
typedef signed char BOOL;
typedef unsigned long NSUInteger;
typedef long NSInteger;
typedef enum VZMode : NSInteger { VZModeOne = 1, VZModeNegative = -7 } VZMode;
@class NSError;
__attribute__((objc_root_class))
@interface NSObject
+ (instancetype)alloc;
+ (instancetype)new __attribute__((availability(swift,unavailable)));
- (instancetype)init;
@end
__attribute__((availability(macos,introduced=12.3)))
@interface VZFixture : NSObject
- (instancetype)initWithCount:(NSUInteger)count;
- (id)copyValue;
- (id)value;
- (NSInteger)score;
- (VZMode)mode;
- (BOOL)validateWithError:(NSError **)error;
- (id)consume __attribute__((ns_consumes_self,ns_returns_retained));
- (id)initAsNormal __attribute__((objc_method_family(none),ns_returns_retained));
- (void)take:(id)__attribute__((ns_consumed))value;
- (void)future __attribute__((availability(macos,introduced=15.0)));
- (void)swiftOnlyUnavailable __attribute__((availability(swift,unavailable)));
@property BOOL isEnabled;
@end
@interface VZFixture (Extra)
- (unsigned int)extra;
@end
@interface VZNoDefault : NSObject
- (instancetype)init __attribute__((unavailable));
@end
`

func compileFixture(t *testing.T, source string) sdkTarget {
	t.Helper()
	if _, err := exec.LookPath("xcrun"); err != nil {
		t.Skip("Clang SDK fixture requires Xcode")
	}
	directory := t.TempDir()
	input := filepath.Join(directory, "fixture.m")
	if err := os.WriteFile(input, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	target, err := extractTarget(input, "arm64", "arm64-apple-macos11", directory)
	if err != nil {
		t.Fatal(err)
	}
	return target
}

func findMethod(t *testing.T, target sdkTarget, name string) sdkMethod {
	t.Helper()
	for _, m := range target.Methods {
		if m.GoName == name {
			return m
		}
	}
	t.Fatalf("method %s missing", name)
	return sdkMethod{}
}

func TestSDKCompilerFacts(t *testing.T) {
	target := compileFixture(t, fixture)
	ctor := findMethod(t, target, "VZFixture_InitWithCount")
	if ctor.Family != "init" || ctor.Ownership != "owned" || ctor.Introduced != "12.3" || ctor.Parameters[0].Type.ABI != "uint64" {
		t.Fatalf("initializer facts = %+v", ctor)
	}
	if findMethod(t, target, "VZFixture_Value").Ownership != "borrowed" {
		t.Fatal("getter must be borrowed")
	}
	if findMethod(t, target, "VZFixture_CopyValue").Ownership != "owned" {
		t.Fatal("copy method must return retained object")
	}
	if findMethod(t, target, "VZFixture_Extra").Parameters != nil {
		t.Fatal("category method unexpectedly has arguments")
	}
	if len(findMethod(t, target, "VZFixture_SwiftOnlyUnavailable").Unsupported) != 0 {
		t.Fatal("Swift availability rejected an Objective-C method")
	}
	if findMethod(t, target, "VZFixture_InitAsNormal").Family != "none" {
		t.Fatal("explicit family(none) was lost")
	}
	if len(findMethod(t, target, "VZFixture_Consume").Unsupported) == 0 {
		t.Fatal("consuming ordinary method needs an explicit transfer policy")
	}
	if len(findMethod(t, target, "VZFixture_Take").Unsupported) == 0 {
		t.Fatal("consumed argument was accepted")
	}
	if len(findMethod(t, target, "VZNoDefault_New").Unsupported) == 0 {
		t.Fatal("new bypassed unavailable init")
	}
	if len(target.Enums) != 1 || target.Enums[0].Values[1].Value != "-7" {
		t.Fatalf("enum = %+v", target.Enums)
	}
	code, err := generateSDK(target)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"func VZFixture_InitWithCount(p0 uint64) *objc.Pointer", "func VZFixture_SetIsEnabled(receiver objc.NSObject, p0 bool)"} {
		if !bytes.Contains(code, []byte(want)) {
			t.Errorf("generated API missing %q", want)
		}
	}
}

func TestSDKMutationChangesGeneratedABI(t *testing.T) {
	initial := compileFixture(t, fixture)
	changed := compileFixture(t, strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(fixture, "(NSInteger)score", "(unsigned int)score"), "- (id)value;", "- (id)value __attribute__((ns_returns_retained));"), "introduced=12.3", "introduced=14.0"))
	original, err := generateSDK(initial)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := generateSDK(changed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(original, []byte("func VZFixture_Score(receiver objc.NSObject) int64")) || !bytes.Contains(updated, []byte("func VZFixture_Score(receiver objc.NSObject) uint32")) {
		t.Fatal("compiler ABI mutation did not change generated signature")
	}
	if findMethod(t, changed, "VZFixture_Value").Ownership != "owned" || findMethod(t, changed, "VZFixture_Value").Introduced != "14.0" {
		t.Fatal("ownership or availability mutation was ignored")
	}
}

func TestSDKUnknownABIAndProtocolAdoption(t *testing.T) {
	source := strings.Replace(fixture, "__attribute__((objc_root_class))\n@interface NSObject", "@protocol Basics\n- (id)title;\n@end\n__attribute__((objc_root_class))\n@interface NSObject <Basics>", 1) + `
 struct VZPair { long x; long y; };
 __attribute__((availability(macos,introduced=15.0)))
 @interface VZFixture (Later)
 - (struct VZPair)pair;
 - (void)withBlock:(void (^)(void))block;
 @end
 `
	target := compileFixture(t, source)
	if m := findMethod(t, target, "NSObject_Title"); m.Result.Kind != "object" || m.InheritedFrom != "protocol:Basics" {
		t.Fatalf("adopted protocol: %+v", m)
	}
	for _, name := range []string{"VZFixture_Pair", "VZFixture_WithBlock"} {
		m := findMethod(t, target, name)
		if len(m.Unsupported) == 0 || m.Introduced != "15.0" {
			t.Fatalf("unknown ABI/availability: %+v", m)
		}
	}
}

func TestSDKTargetValidation(t *testing.T) {
	if _, err := generateSDK(sdkTarget{Architecture: "mips"}); err == nil {
		t.Fatal("unsupported target accepted")
	}
	target := sdkTarget{Architecture: "arm64", Methods: []sdkMethod{{ID: "broken", GoName: "Broken", Owner: "VZFixture", Selector: "broken", Result: sdkType{Kind: "mystery"}}}}
	if _, err := generateSDK(target); err == nil {
		t.Fatal("unknown result kind accepted")
	}
}

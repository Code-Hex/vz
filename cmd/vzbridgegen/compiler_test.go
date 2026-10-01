package main

import (
	"strings"
	"testing"
)

func TestSDKProtocolAvailability(t *testing.T) {
	source := strings.Replace(fixture, "__attribute__((availability(macos,introduced=12.3)))\n@interface VZFixture : NSObject", "__attribute__((availability(macos,introduced=15.0,obsoleted=16.0)))\n@protocol FutureProtocol\n- (id)protocolValue;\n@end\n@interface VZFixture : NSObject <FutureProtocol>", 1)
	target := compileFixture(t, source)
	m := findMethod(t, target, "VZFixture_ProtocolValue")
	t.Logf("adopted protocol: %+v", m)
	if m.Introduced != "15.0" || m.Obsoleted != "16.0" {
		t.Fatal("protocol availability lost on concrete method")
	}
}

func TestSDKProtocolInCategory(t *testing.T) {
	target := compileFixture(t, fixture+"\n@protocol CategoryProtocol\n- (id)categoryValue;\n@end\n@interface VZFixture (ProtocolCategory) <CategoryProtocol>\n@end\n")
	findMethod(t, target, "VZFixture_CategoryValue")
}

func TestSDKOrdinaryEnumWidth(t *testing.T) {
	target := compileFixture(t, fixture+"\nenum VZWide { VZWideHuge = 1ULL << 40 };\n@interface VZFixture (Wide)\n- (enum VZWide)wide;\n@end\n")
	m := findMethod(t, target, "VZFixture_Wide")
	t.Logf("wide enum result: %+v", m.Result)
	if m.Result.ABI != "uint64" && len(m.Unsupported) == 0 {
		t.Fatalf("64-bit enum lowered as %q", m.Result.ABI)
	}
}

func TestSDKInheritedNewUsesInitAvailability(t *testing.T) {
	target := compileFixture(t, fixture+"\n@interface VZLateInit : NSObject\n- (instancetype)init __attribute__((availability(macos,introduced=15.0)));\n@end\n")
	m := findMethod(t, target, "VZLateInit_New")
	t.Logf("inherited new: %+v", m)
	if m.Introduced != "15.0" {
		t.Fatal("new ignores selected init availability")
	}
}

func TestSDKConsumedUnknownParameter(t *testing.T) {
	target := compileFixture(t, fixture+"\n@interface VZFixture (Consumed)\n- (void)takeCF:(void *)__attribute__((cf_consumed))value;\n@end\n")
	m := findMethod(t, target, "VZFixture_TakeCF")
	t.Logf("CF consumed parameter: %+v", m)
	if len(m.Unsupported) == 0 {
		t.Fatal("CFConsumedAttr accepted without transfer policy")
	}
}

func TestSDKAvailabilityWhitespace(t *testing.T) {
	for _, attr := range []string{`availability(macos, unavailable)`, `availability( macos, introduced=15.0)`} {
		t.Run(attr, func(t *testing.T) {
			source := strings.Replace(fixture, "- (id)value;", "- (id)value __attribute__(("+attr+"));", 1)
			m := findMethod(t, compileFixture(t, source), "VZFixture_Value")
			t.Logf("whitespace availability: %+v", m)
			if strings.Contains(attr, "unavailable") && len(m.Unsupported) == 0 {
				t.Fatal("spaced unavailable attribute accepted")
			}
			if strings.Contains(attr, "introduced") && m.Introduced != "15.0" {
				t.Fatal("spaced platform silently ignored")
			}
		})
	}
}

func TestSDKDuplicateObsoleteAvailability(t *testing.T) {
	target := compileFixture(t, fixture+"\n@interface VZFixture (Obsolete)\n- (id)value __attribute__((availability(macos,obsoleted=16.0)));\n@end\n")
	m := findMethod(t, target, "VZFixture_Value")
	t.Logf("duplicate declaration: %+v", m)
	if m.Obsoleted != "16.0" {
		t.Fatal("duplicate obsolete availability lost")
	}
}

func TestSDKRuntimeMemoryMethodsAreNotCallable(t *testing.T) {
	for _, family := range []string{"", " __attribute__((objc_method_family(none)))"} {
		t.Run("dealloc"+family, func(t *testing.T) {
			target := compileFixture(t, fixture+`
@interface VZFixture (Lifetime)
- (void)dealloc`+family+`;
- (void)finalize;
- (instancetype)retain;
- (void)release;
- (instancetype)autorelease;
- (NSUInteger)retainCount;
- (void)deallocateCache;
- (NSUInteger)deallocatingInfo;
- (void)finalizeResources;
- (id)retainValue;
- (void)releaseResources;
@end
`)
			code, err := generateSDK(target)
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"Dealloc", "Finalize", "Retain", "Release", "Autorelease", "RetainCount"} {
				name = "VZFixture_" + name
				if len(findMethod(t, target, name).Unsupported) == 0 {
					t.Errorf("runtime memory method %s accepted as an ordinary managed receiver method", name)
				}
				if strings.Contains(string(code), "func "+name+"(") {
					t.Errorf("generated API exposes runtime memory method %s", name)
				}
			}
			for _, name := range []string{"DeallocateCache", "DeallocatingInfo", "FinalizeResources", "RetainValue", "ReleaseResources", "New", "InitWithCount"} {
				name = "VZFixture_" + name
				if len(findMethod(t, target, name).Unsupported) != 0 {
					t.Errorf("ordinary method %s rejected", name)
				}
				if !strings.Contains(string(code), "func "+name+"(") {
					t.Errorf("generated API missing ordinary method %s", name)
				}
			}
		})
	}
}
